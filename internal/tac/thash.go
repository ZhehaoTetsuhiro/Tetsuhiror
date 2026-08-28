package tac

// THASH-256 — 自研海绵结构哈希。
//
// 设计: 256 位状态 (4×64 位字), 轮函数为 ARX + S 盒混合:
//   - sigma: 每字节过 S 盒 (GF(2^8) 幂映射 x^251 与仿射变换的复合);
//   - theta: 字间乘常数的扩散;
//   - pi:    字内循环移位;
//   - chi:   非线性字间混合 (类 Keccak chi)。
// 吸收率 16 字节, 挤出 32 字节摘要。
//
// 注意: 这是为 tet 项目自研的实验性哈希, 未经过第三方密码分析,
// 不应替代 SHA-256 用于对抗性安全场景。

import "math/bits"

const thashRounds = 12

// thashSbox: GF(2^8) 上 x^251 幂映射与仿射变换的复合。
var thashSbox [256]byte

func init() {
	// 幂映射用平方-乘直接计算 (GF(2^8)/x^8+x^4+x^3+x^2+1), 不依赖生成元。
	for v := 0; v < 256; v++ {
		s := byte(0)
		if v != 0 {
			r := byte(1)
			base := byte(v)
			// 251 = 0b11111011
			for i := 7; i >= 0; i-- {
				r = gfMul(r, r)
				if (251>>uint(i))&1 == 1 {
					r = gfMul(r, base)
				}
			}
			s = r
		}
		// 仿射: 循环左移 3 位 + 异或常量
		s = bits.RotateLeft8(s, 3) ^ 0x5a
		thashSbox[v] = s
	}
}

func gfMul(a, b byte) byte {
	var p byte
	for i := 0; i < 8; i++ {
		if b&1 != 0 {
			p ^= a
		}
		hi := a & 0x80
		a <<= 1
		if hi != 0 {
			a ^= 0x1d
		}
		b >>= 1
	}
	return p
}

var thashIV = [4]uint64{
	0x7468725f68617368, // "thr_hash"
	0x6e6f6e63655f7061,
	0x6421_9e37_79b9_7f4a,
	0x5dee_6683_10bc_09a1,
}

type thashState struct {
	s [4]uint64
	buf [16]byte // 吸收缓冲 (rate)
	n  int
}

// NewTHASH 创建一个 THASH-256 状态。
func NewTHASH() *thashState {
	h := &thashState{}
	h.s = thashIV
	return h
}

func permute(s *[4]uint64) {
	const c0, c1, c2, c3 = 0x87c37b911185738d, 0x4cf5ad432745937f, 0x8ff51a3339cb27b4, 0x9e3779b97f4a7c15
	for r := 0; r < thashRounds; r++ {
		// theta: 常数扩散
		s[0] = bits.RotateLeft64(s[0]*c0, 7)
		s[1] = bits.RotateLeft64(s[1]*c1, 11)
		s[2] = bits.RotateLeft64(s[2]*c2, 13)
		s[3] = bits.RotateLeft64(s[3]*c3, 17)

		// chi: 非线性混合
		t0 := s[0] ^ ((^s[1]) & s[2])
		t1 := s[1] ^ ((^s[2]) & s[3])
		t2 := s[2] ^ ((^s[3]) & s[0])
		t3 := s[3] ^ ((^s[0]) & s[1])
		s[0], s[1], s[2], s[3] = t0, t1, t2, t3

		// sigma: S 盒层 (作用于 s[0] 与 s[2] 的全部字节 + 轮常数)
		s[0] = sboxWord(s[0])
		s[2] = sboxWord(s[2])
		s[1] ^= uint64(r+1) * 0x9e3779b97f4a7c15

		// pi: 字内循环移位
		s[1] = bits.RotateLeft64(s[1], 16)
		s[3] = bits.RotateLeft64(s[3], 23)
	}
}

func sboxWord(w uint64) uint64 {
	var out uint64
	for i := 0; i < 8; i++ {
		out |= uint64(thashSbox[byte(w>>(8*i))]) << (8 * i)
	}
	return out
}

// Write 吸收数据。
func (h *thashState) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		c := copy(h.buf[h.n:], p)
		h.n += c
		p = p[c:]
		if h.n == len(h.buf) {
			h.absorbBlock()
		}
	}
	return n, nil
}

func (h *thashState) absorbBlock() {
	// 缓冲 XOR 进状态前两字 (rate)
	var w0, w1 uint64
	for i := 0; i < 8; i++ {
		w0 |= uint64(h.buf[i]) << (8 * i)
		w1 |= uint64(h.buf[8+i]) << (8 * i)
	}
	h.s[0] ^= w0
	h.s[1] ^= w1
	permute(&h.s)
	h.n = 0
}

// Sum 挤出 32 字节摘要并重置状态。
func (h *thashState) Sum() [32]byte {
	// pad10*1 填充
	h.buf[h.n] = 0x01
	for i := h.n + 1; i < len(h.buf); i++ {
		h.buf[i] = 0
	}
	h.absorbBlockFinal()

	var out [32]byte
	for i := 0; i < 4; i++ {
		for j := 0; j < 8; j++ {
			out[i*8+j] = byte(h.s[i] >> (8 * j))
		}
		if i == 1 {
			permute(&h.s)
		}
	}
	h.s = thashIV
	h.n = 0
	return out
}

func (h *thashState) absorbBlockFinal() {
	var w0, w1 uint64
	for i := 0; i < 8; i++ {
		w0 |= uint64(h.buf[i]) << (8 * i)
		w1 |= uint64(h.buf[8+i]) << (8 * i)
	}
	h.s[0] ^= w0
	h.s[1] ^= w1
	h.s[3] ^= 0x80 // 域分离: 最终块
	permute(&h.s)
	h.n = 0
}

// THASH256 一次性计算摘要。
func THASH256(data ...[]byte) [32]byte {
	h := NewTHASH()
	for _, d := range data {
		h.Write(d)
	}
	return h.Sum()
}

// SumBytes 返回摘要的字节切片形式。
func SumBytes(h *thashState) []byte {
	s := h.Sum()
	return s[:]
}
