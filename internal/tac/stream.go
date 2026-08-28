package tac

// TStream — 自研密钥流生成器 (hash-counter 模式)。
//
// 第 i 个 32 字节块 = THASH(key || nonce || u64le(i))。
// 密钥流质量直接依赖 THASH 的扩散性; 用于 tthr 的对称加密。

import "encoding/binary"

const streamBlockSize = 32

type TStream struct {
	key     [32]byte
	nonce   [16]byte
	counter uint64
	block   [32]byte
	used    int
}

// NewTStream 以 32 字节密钥与 16 字节 nonce 初始化。
func NewTStream(key [32]byte, nonce [16]byte) *TStream {
	s := &TStream{}
	s.key = key
	s.nonce = nonce
	s.used = streamBlockSize // 强制首块生成
	return s
}

func (s *TStream) refill() {
	var ctr [8]byte
	binary.LittleEndian.PutUint64(ctr[:], s.counter)
	d := THASH256(s.key[:], s.nonce[:], ctr[:])
	s.block = d
	s.counter++
	s.used = 0
}

// XORBytes 就地把 src 与密钥流异或, 返回写入 dst 的字节数。
func (s *TStream) XORBytes(dst, src []byte) int {
	n := 0
	for len(src) > 0 {
		if s.used == streamBlockSize {
			s.refill()
		}
		c := streamBlockSize - s.used
		if c > len(src) {
			c = len(src)
		}
		for i := 0; i < c; i++ {
			dst[n+i] = src[i] ^ s.block[s.used+i]
		}
		s.used += c
		src = src[c:]
		n += c
	}
	return n
}
