package tac

// T-IES — 自研非对称封装机制 (Tetsuhiro Integrated Encryption Scheme)。
//
// 构造 (ElGamal 型混合加密, 一次性会话密钥):
//   密钥对: 私钥 x (256 位随机), 公钥 X = g^x mod p
//   加密:   临时密钥 e, 临时公钥 E = g^e mod p;
//           共享秘密 S = X^e mod p;
//           会话密钥 = THASH-KDF(S || E || salt || qpad);
//           密文 = 明文 ⊕ TStream(会话密钥, nonce);
//           认证 = TMAC(会话密钥, E || salt || qpad || ct)。
//   解密:   S = E^x mod p, 同样派生, 验证 MAC 后解密。
//
// 群参数采用 RFC 3526 第 14 组 (2048 位安全素数), 与采用标准
// 椭圆曲线参数同理: 参数公开标准化, 方案构造为本项目自研。
//
// 安全性说明: 这是研究性自研方案, 未经过第三方审计。

import (
	"crypto/rand"
	"errors"
	"math/big"
)

var (
	// RFC 3526 组 14: 2048 位安全素数, g=2 的阶为 q=(p-1)/2。
	tIESP, _ = new(big.Int).SetString(
		"FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD129024E088A67CC74" +
		"020BBEA63B139B22514A08798E3404DDEF9519B3CD3A431B302B0A6DF25F1437" +
		"4FE1356D6D51C245E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3DC2007CB8A163BF05" +
		"98DA48361C55D39A69163FA8FD24CF5F83655D23DCA3AD961C62F356208552BB" +
		"9ED529077096966D670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9DE2BCBF695581718" +
		"3995497CEA956AE515D2261898FA051015728E5A8AACAA68FFFFFFFFFFFFFFFF", 16)
	tIESG  = big.NewInt(2)
	tIESQ  = new(big.Int).Rsh(tIESP, 1) // (p-1)/2 的素数阶子群
)

const (
	PrivKeyBytes = 32  // 私钥固定 256 位
	PubKeyBytes  = 256 // 公钥 2048 位
)

// PrivateKey 是 T-IES 私钥。
type PrivateKey struct {
	X *big.Int
}

// PublicKey 是 T-IES 公钥。
type PublicKey struct {
	Y *big.Int
}

// GenerateKey 生成密钥对。
func GenerateKey() (*PrivateKey, *PublicKey, error) {
	// 私钥取 [1, q) 内 256 位随机值
	max := new(big.Int).Lsh(big.NewInt(1), 256)
	if max.Cmp(tIESQ) > 0 {
		max = tIESQ
	}
	max.Sub(max, big.NewInt(1))
	for {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return nil, nil, err
		}
		x := new(big.Int).SetBytes(buf)
		if x.Sign() > 0 && x.Cmp(max) < 0 {
			y := new(big.Int).Exp(tIESG, x, tIESP)
			return &PrivateKey{X: x}, &PublicKey{Y: y}, nil
		}
	}
}

// MarshalPrivateKey 序列化私钥为固定长度字节。
func (k *PrivateKey) MarshalPrivateKey() []byte {
	out := make([]byte, PrivKeyBytes)
	b := k.X.Bytes()
	copy(out[PrivKeyBytes-len(b):], b)
	return out
}

// ParsePrivateKey 从字节恢复私钥。
func ParsePrivateKey(b []byte) (*PrivateKey, error) {
	if len(b) != PrivKeyBytes {
		return nil, errors.New("tac: 私钥长度错误")
	}
	x := new(big.Int).SetBytes(b)
	if x.Sign() <= 0 {
		return nil, errors.New("tac: 私钥非法")
	}
	return &PrivateKey{X: x}, nil
}

// MarshalPublicKey 序列化公钥。
func (k *PublicKey) MarshalPublicKey() []byte {
	out := make([]byte, PubKeyBytes)
	b := k.Y.Bytes()
	copy(out[PubKeyBytes-len(b):], b)
	return out
}

// ParsePublicKey 从字节恢复公钥。
func ParsePublicKey(b []byte) (*PublicKey, error) {
	if len(b) != PubKeyBytes {
		return nil, errors.New("tac: 公钥长度错误")
	}
	y := new(big.Int).SetBytes(b)
	if y.Cmp(tIESP) >= 0 || y.Cmp(big.NewInt(1)) < 0 {
		return nil, errors.New("tac: 公钥超出群范围")
	}
	return &PublicKey{Y: y}, nil
}

// Public 由私钥推导公钥。
func (k *PrivateKey) Public() *PublicKey {
	return &PublicKey{Y: new(big.Int).Exp(tIESG, k.X, tIESP)}
}

// Envelope 是一次 T-IES 封装的结果。
type Envelope struct {
	EphPub []byte // 临时公钥 (256B)
}

// SealRandom 生成临时密钥的随机 e (不导出)。
func randomExponent() (*big.Int, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	e := new(big.Int).SetBytes(buf)
	e.Mod(e, new(big.Int).Sub(tIESQ, big.NewInt(1)))
	e.Add(e, big.NewInt(1))
	return e, nil
}

// EncapsulateWithRandom 为给定公钥生成共享秘密与临时公钥。
// info 混入 KDF (量子垫等)。
func EncapsulateWithRandom(pub *PublicKey, info []byte) (shared [32]byte, env Envelope, err error) {
	e, err := randomExponent()
	if err != nil {
		return shared, env, err
	}
	E := new(big.Int).Exp(tIESG, e, tIESP)
	S := new(big.Int).Exp(pub.Y, e, tIESP)
	env.EphPub = make([]byte, PubKeyBytes)
	b := E.Bytes()
	copy(env.EphPub[PubKeyBytes-len(b):], b)
	sb := S.Bytes()
	shared = DeriveKey(sb, env.EphPub, info, 1)
	return shared, env, nil
}

// DecapsulateWithInfo 用私钥恢复共享秘密。
func DecapsulateWithInfo(priv *PrivateKey, ephPub, info []byte) (shared [32]byte, err error) {
	E, err := ParsePublicKey(ephPub)
	if err != nil {
		return shared, err
	}
	S := new(big.Int).Exp(E.Y, priv.X, tIESP)
	sb := S.Bytes()
	shared = DeriveKey(sb, ephPub, info, 1)
	return shared, nil
}
