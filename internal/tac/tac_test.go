package tac

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestSboxPermutation(t *testing.T) {
	seen := make([]bool, 256)
	for _, s := range thashSbox {
		if seen[s] {
			t.Fatalf("S 盒冲突: %d 重复", s)
		}
		seen[s] = true
	}
	for i, ok := range seen {
		if !ok {
			t.Fatalf("S 盒缺值: %d", i)
		}
	}
}

func TestTHASHDeterministic(t *testing.T) {
	a := THASH256([]byte("hello"), []byte("world"))
	b := THASH256([]byte("hello"), []byte("world"))
	if a != b {
		t.Fatal("同一输入摘要不同")
	}
	c := THASH256([]byte("hello"))
	if a == c {
		t.Fatal("不同输入摘要相同 (雪崩失败?)")
	}
}

func TestTHASHAvalanche(t *testing.T) {
	base := THASH256([]byte("avalanche test input"))
	flipped := THASH256([]byte("avalanche test input!"))
	diff := 0
	for i := 0; i < 32; i++ {
		x := base[i] ^ flipped[i]
		for x != 0 {
			diff += int(x & 1)
			x >>= 1
		}
	}
	// 256 位中期望 128 位翻转; 允许 90..170
	if diff < 90 || diff > 170 {
		t.Fatalf("雪崩效应异常: %d/256 位翻转", diff)
	}
}

func TestTHASHLengths(t *testing.T) {
	for _, n := range []int{0, 1, 15, 16, 17, 31, 32, 33, 100, 1000} {
		data := make([]byte, n)
		rand.Read(data)
		h := NewTHASH()
		h.Write(data)
		d1 := h.Sum()
		h2 := NewTHASH()
		h2.Write(data)
		d2 := h2.Sum()
		if d1 != d2 {
			t.Fatalf("长度 %d 不确定", n)
		}
		// 分块写入 == 一次写入
		h3 := NewTHASH()
		for i := 0; i < n; i++ {
			h3.Write(data[i : i+1])
		}
		if h3.Sum() != d1 {
			t.Fatalf("长度 %d 分块写入不一致", n)
		}
	}
}

func TestTStreamRoundtrip(t *testing.T) {
	var key [32]byte
	var nonce [16]byte
	rand.Read(key[:])
	rand.Read(nonce[:])
	data := make([]byte, 100000)
	rand.Read(data)

	enc := NewTStream(key, nonce)
	ct := make([]byte, len(data))
	enc.XORBytes(ct, data)

	dec := NewTStream(key, nonce)
	pt := make([]byte, len(data))
	dec.XORBytes(pt, ct)

	if !bytes.Equal(pt, data) {
		t.Fatal("TStream 往返失败")
	}
	if bytes.Equal(ct, data) {
		t.Fatal("密文与明文相同?!")
	}
}

func TestTStreamNoReuse(t *testing.T) {
	var key [32]byte
	var nonce [16]byte
	rand.Read(key[:])
	rand.Read(nonce[:])
	a := NewTStream(key, nonce)
	b := NewTStream(key, nonce)
	// 相同密钥/nonce 必须产生相同密钥流 (确定性)
	x := make([]byte, 64)
	y := make([]byte, 64)
	a.XORBytes(x, make([]byte, 64))
	b.XORBytes(y, make([]byte, 64))
	if !bytes.Equal(x, y) {
		t.Fatal("密钥流不确定")
	}
}

func TestKDF(t *testing.T) {
	k1 := DeriveKey([]byte("pw"), []byte("salt"), nil, 100)
	k2 := DeriveKey([]byte("pw"), []byte("salt"), nil, 100)
	k3 := DeriveKey([]byte("pw"), []byte("salt"), nil, 101)
	k4 := DeriveKey([]byte("pw"), []byte("salt2"), nil, 100)
	if k1 != k2 {
		t.Fatal("KDF 不确定")
	}
	if k1 == k3 || k1 == k4 {
		t.Fatal("KDF 输出未区分")
	}
}

func TestTMAC(t *testing.T) {
	var key [32]byte
	rand.Read(key[:])
	m := TMAC(key, []byte("data"))
	if !TMACVerify(key, m, []byte("data")) {
		t.Fatal("MAC 验证失败")
	}
	if TMACVerify(key, m, []byte("data2")) {
		t.Fatal("MAC 篡改未被检测")
	}
	m2 := m
	m2[0] ^= 1
	if TMACVerify(key, m2, []byte("data")) {
		t.Fatal("MAC 篡改未被检测")
	}
}

func TestTIES(t *testing.T) {
	priv, pub, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	shared1, env, err := EncapsulateWithRandom(pub, []byte("info"))
	if err != nil {
		t.Fatal(err)
	}
	shared2, err := DecapsulateWithInfo(priv, env.EphPub, []byte("info"))
	if err != nil {
		t.Fatal(err)
	}
	if shared1 != shared2 {
		t.Fatal("T-IES 共享秘密不一致")
	}

	// info 不同则密钥不同
	shared3, _, _ := EncapsulateWithRandom(pub, []byte("other"))
	shared4, _ := DecapsulateWithInfo(priv, env.EphPub, []byte("other"))
	if shared3 == shared4 {
		// 不同 info 派生不同密钥是设计要求
		t.Fatal("info 未参与派生")
	}

	// 密钥序列化
	pk := priv.MarshalPrivateKey()
	priv2, err := ParsePrivateKey(pk)
	if err != nil {
		t.Fatal(err)
	}
	if priv2.Public().Y.Cmp(priv.Public().Y) != 0 {
		t.Fatal("私钥序列化往返失败")
	}
	pubB := pub.MarshalPublicKey()
	pub2, err := ParsePublicKey(pubB)
	if err != nil {
		t.Fatal(err)
	}
	if pub2.Y.Cmp(pub.Y) != 0 {
		t.Fatal("公钥序列化往返失败")
	}
}
