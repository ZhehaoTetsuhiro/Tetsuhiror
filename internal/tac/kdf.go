package tac

// THASH-KDF — 自研密钥派生函数。
//
// state = THASH(ikm || salt || info)
// 迭代 iters 次 state = THASH(state) 后挤出 32 字节。
// 量子随机垫 (qpad) 可混入 info, 使最终密钥获得量子熵。

// DeriveKey 从输入密钥材料派生 32 字节密钥。
// iters 为 0 时按 1 处理。
func DeriveKey(ikm, salt, info []byte, iters int) [32]byte {
	if iters < 1 {
		iters = 1
	}
	h := NewTHASH()
	h.Write(ikm)
	h.Write(salt)
	h.Write(info)
	// 长度绑定, 防止拼接歧义
	h.Write([]byte{byte(len(ikm)), byte(len(salt)), byte(len(info) >> 8), byte(len(info))})
	state := h.Sum()

	for i := 0; i < iters-1; i++ {
		state = THASH256(state[:])
	}
	return state
}

// TMAC — THASH 前缀 MAC: THASH(key || 0x00 || data)。
// 与 KDF 域分离 (KDF 输入以 0x01 填充结尾)。
func TMAC(key [32]byte, data ...[]byte) [32]byte {
	h := NewTHASH()
	h.Write(key[:])
	h.Write([]byte{0x00})
	for _, d := range data {
		h.Write(d)
	}
	return h.Sum()
}

// TMACVerify 恒定时间比较。
func TMACVerify(key [32]byte, want [32]byte, data ...[]byte) bool {
	got := TMAC(key, data...)
	var diff byte
	for i := 0; i < 32; i++ {
		diff |= got[i] ^ want[i]
	}
	return diff == 0
}
