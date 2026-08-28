// Package tcz 实现自研压缩算法 TCZ (Tetsuhiro Compressor)。
//
// 两级流水线:
//  1. TLZ — LZ77 变体, 把输入转为 literal / (dist,len) token 流;
//  2. TAC-AC — 自适应二进制算术编码, 上下文模型包括:
//     token 类型 (order-2), literal 字节 (order-1 二叉上下文树),
//     匹配长度与距离 (各自的二叉上下文树, 距离按长度分桶)。
//
// 文件布局 (小端):
//
//	magic "TCZ1" (4B) | flags u8 (0) | origLen u64 | bitstream...
package tcz

import (
	"errors"
	"fmt"
)

const (
	magic = "TCZ1"

	flagsStored = 1 // 原样存储 (不可压缩数据回退)
	flagsMask   = 0x07

	lenBits = 11 // len-minMatch ∈ [0,2047]
	distCtx = 4  // 距离模型按匹配长度分 4 桶
)

var ErrCorrupt = errors.New("tcz: 数据损坏或不是合法的 TCZ 流")

// 模型集合。全部使用 12 位自适应概率。
type models struct {
	flag [4]Prob // 上下文: 最近两个 token 类型 (1=literal)

	// literal: order-1 二叉树, 上文字节 × 树节点。
	// 索引: prevByte*256 + node, node ∈ [1,255], 根为 prevByte*256+1。
	lit [256 * 256]Prob

	// 匹配长度: v = len-minMatch, 11 位二叉树, node ∈ [1,2047]。
	leng [1 << lenBits]Prob

	// 距离: d-1, 15 位二叉树, 按长度桶分 4 份。
	dist [distCtx * (1 << 15)]Prob
}

func newModels() *models {
	m := &models{}
	for i := range m.flag {
		m.flag[i] = 2048
	}
	for i := range m.lit {
		m.lit[i] = 2048
	}
	for i := range m.leng {
		m.leng[i] = 2048
	}
	for i := range m.dist {
		m.dist[i] = 2048
	}
	return m
}

// codeTree 用共享二叉树编码 v 的低 n 位 (MSB first)。
// t 为树的概率数组, base 为该上下文的根偏移。
func encTree(e *Encoder, t []Prob, base int, v int, n uint) {
	node := 1
	for i := int(n) - 1; i >= 0; i-- {
		bit := (v >> i) & 1
		p := &t[base+node]
		e.Encode(bit, *p)
		p.update(bit, 5)
		node = node*2 + bit
	}
}

func decTree(d *Decoder, t []Prob, base int, n uint) int {
	node := 1
	v := 0
	for i := int(n) - 1; i >= 0; i-- {
		p := &t[base+node]
		bit := d.Decode(*p)
		p.update(bit, 5)
		node = node*2 + bit
		v = v<<1 | bit
	}
	return v
}

// Compress 压缩 data, 返回 TCZ 流。
func Compress(data []byte) []byte {
	literals, matches, tokens := compressLZ(data)

	bw := &BitWriter{}
	enc := NewEncoder(bw)
	m := newModels()

	flagCtx := 0 // (lastIsLit<<1 | secondIsLit), 初始 0
	prevByte := 0

	li, mi := 0, 0
	pos := 0 // 输入游标, 用于维护与解码端一致的 prevByte
	for _, t := range tokens {
		// 编码 token 类型
		fp := &m.flag[flagCtx]
		enc.Encode(int(t), *fp)
		fp.update(int(t), 5)

		if t == 1 {
			b := int(literals[li])
			li++
			// order-1 二叉树编码字节
			node := 1
			base := prevByte << 8
			for i := 7; i >= 0; i-- {
				bit := (b >> i) & 1
				p := &m.lit[base+node]
				enc.Encode(bit, *p)
				p.update(bit, 5)
				node = node*2 + bit
			}
			prevByte = b
			pos++
		} else {
			mm := matches[mi]
			mi++
			// 长度
			encTree(enc, m.leng[:], 0, mm.Len-minMatch, lenBits)
			// 距离 (按长度分桶)
			bucket := (mm.Len - minMatch) >> (lenBits - 2)
			if bucket >= distCtx {
				bucket = distCtx - 1
			}
			encTree(enc, m.dist[:], bucket*(1<<15), mm.Dist-1, 15)
			pos += mm.Len
			prevByte = int(data[pos-1])
		}

		flagCtx = (flagCtx<<1 | int(t)) & 3
	}
	enc.Flush()
	body := bw.Align()

	// 不可压缩数据回退: 直接存储。
	if len(body)+13 >= len(data) && len(data) > 0 {
		out := make([]byte, 0, len(data)+13)
		out = append(out, magic...)
		out = append(out, flagsStored)
		out = appendU64(out, uint64(len(data)))
		out = append(out, data...)
		return out
	}

	out := make([]byte, 0, len(body)+16)
	out = append(out, magic...)
	out = append(out, 0)
	out = appendU64(out, uint64(len(data)))
	out = append(out, body...)
	return out
}

// Decompress 解压 TCZ 流。
func Decompress(src []byte) ([]byte, error) {
	if len(src) < 13 {
		return nil, ErrCorrupt
	}
	if string(src[:4]) != magic {
		return nil, ErrCorrupt
	}
	origLen := readU64(src[5:13])
	if origLen > maxDecodedSize {
		return nil, fmt.Errorf("tcz: 声明的原始大小 %d 超出上限", origLen)
	}
	if src[4]&flagsMask == flagsStored {
		if uint64(len(src)-13) != origLen {
			return nil, ErrCorrupt
		}
		out := make([]byte, origLen)
		copy(out, src[13:])
		return out, nil
	}
	if src[4]&flagsMask != 0 {
		return nil, fmt.Errorf("tcz: 不支持的 flags 值 %d", src[4]&flagsMask)
	}

	br := NewBitReader(src[13:])
	dec := NewDecoder(br)
	m := newModels()

	out := make([]byte, 0, origLen)
	flagCtx := 0
	prevByte := 0

	for len(out) < int(origLen) {
		fp := &m.flag[flagCtx]
		t := dec.Decode(*fp)
		fp.update(t, 5)

		if t == 1 {
			node := 1
			base := prevByte << 8
			b := 0
			for i := 7; i >= 0; i-- {
				p := &m.lit[base+node]
				bit := dec.Decode(*p)
				p.update(bit, 5)
				node = node*2 + bit
				b = b<<1 | bit
			}
			out = append(out, byte(b))
			prevByte = b
		} else {
			l := decTree(dec, m.leng[:], 0, lenBits) + minMatch
			if l > maxMatch {
				return nil, ErrCorrupt
			}
			bucket := (l - minMatch) >> (lenBits - 2)
			if bucket >= distCtx {
				bucket = distCtx - 1
			}
			d := decTree(dec, m.dist[:], bucket*(1<<15), 15) + 1
			if d > windowSize || d > len(out) {
				return nil, ErrCorrupt
			}
			start := len(out) - d
			for k := 0; k < l; k++ {
				out = append(out, out[start+k])
			}
			// 上文取最后一个输出字节
			if len(out) > 0 {
				prevByte = int(out[len(out)-1])
			}
		}
		flagCtx = (flagCtx<<1 | t) & 3
	}

	if len(out) != int(origLen) {
		return nil, ErrCorrupt
	}
	return out, nil
}

// maxDecodedSize 限制解压输出, 防御炸弹 (4 GiB)。
const maxDecodedSize = 4 << 30

func appendU64(b []byte, v uint64) []byte {
	for i := 0; i < 8; i++ {
		b = append(b, byte(v>>(8*i)))
	}
	return b
}

func readU64(b []byte) uint64 {
	var v uint64
	for i := 7; i >= 0; i-- {
		v = v<<8 | uint64(b[i])
	}
	return v
}
