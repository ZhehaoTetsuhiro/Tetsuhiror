// Package tcontainer 定义 .tet 容器格式。
//
// 文件布局 (小端):
//
//	offset  size  字段
//	0       4     magic "TTHR"
//	4       1     version (=1)
//	5       1     flags  (bit0: 量子增强)
//	6       2     保留 (0)
//	8       4     keyBlobLen  u32
//	12      8     payloadLen  u64
//	20      4     macLen (=32) u32
//	24      ...   keyBlob
//	...     ...   payload (密文)
//	...     32    TMAC
//
// keyBlob:
//	ephPub  256B  T-IES 临时公钥
//	salt    16B   KDF 盐
//	qpadEnc 32B   量子垫 (以 padKey 加密存储)
//	nonce   16B   TStream nonce
//
// 密钥派生 (qpad 参与域分离):
//	shared  = T-IES.Decapsulate(ephPub, info = salt || qpad)
//	fileKey = THASH-KDF(shared, "tthr/file/v1")
//	padKey  = THASH-KDF(shared, "tthr/qpad/v1")
//	macKey  = THASH-KDF(shared, "tthr/mac/v1")
//
// payload = TStream(fileKey, nonce) XOR (metaJSON || tczStream)
// metaJSON 在密文内, 不泄露路径信息。
package tcontainer

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"tetsuhiro/tthr/internal/tac"
)

var magic = [4]byte{'T', 'T', 'H', 'R'}

const (
	// Version 容器格式版本。
	Version = 1

	flagQuantum = 1 << 0

	hdrSize     = 24
	ephPubSize  = 256
	saltSize    = 16
	QPadSize    = 32
	nonceSize   = 16
	keyBlobSize = ephPubSize + saltSize + QPadSize + nonceSize
	macSize     = 32
	maxPayload  = 8 << 30
)

// ErrNotTthr 不是合法 .tet 文件。
var ErrNotTthr = errors.New("tcontainer: 不是合法的 .tet 文件")

// Meta 是容器内嵌元数据 (加密存放)。
type Meta struct {
	Name      string    `json:"name"`
	Files     int       `json:"files"`
	Dirs      int       `json:"dirs"`
	Symlinks  int       `json:"symlinks"`
	OrigSize  int64     `json:"orig_size"`
	CompSize  int64     `json:"comp_size"`
	CreatedAt time.Time `json:"created_at"`
	TCZ       bool      `json:"tcz"`
}

// PackInput 是打包所需的全部输入。
type PackInput struct {
	Stream    []byte // 已压缩的归档流 (tcz 输出)
	Meta      Meta   // 元数据 (将被加密)
	QPad      []byte // 量子垫 (32B), 由上层生成
	Quantum   bool   // qpad 是否来自 QPanda
	Pub       []byte // 接收方公钥 (nil 则自动生成并返回私钥)
	AutoKey   bool   // 自动生成密钥对
}

// PackResult 打包结果。
type PackResult struct {
	TotalSize  int64
	PrivateKey []byte // AutoKey 时非空 (PKCS 样式文本)
}

// Pack 加密封装并写入 out。
func Pack(in PackInput, out io.Writer) (*PackResult, error) {
	if len(in.QPad) != QPadSize {
		return nil, fmt.Errorf("量子垫长度错误: %d", len(in.QPad))
	}

	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	var priv *tac.PrivateKey
	var pub *tac.PublicKey
	var err error
	if in.AutoKey || in.Pub == nil {
		priv, pub, err = tac.GenerateKey()
		if err != nil {
			return nil, err
		}
	} else {
		pubKey, err := ParsePublicKeyText(in.Pub)
		if err != nil {
			return nil, fmt.Errorf("公钥无效: %w", err)
		}
		pub = pubKey
	}

	// 单次 T-IES 封装 (唯一 ephPub), 量子垫参与 KDF info:
	//   shared = DH(ephPub, pub)
	//   padKey/fileKey/macKey = KDF(shared, 域标签, salt||qpad)
	// qpad 以 padKey 加密存储; 解密端先解出 qpad, 再用
	//   salt||qpad 派生 fileKey/macKey (与加密端一致)。
	// 但解密端必须先解出 qpad —— 而 qpad 解密只需 padKey,
	// padKey 由 salt||qpadEnc 派生 (qpad 的密文形态, 文件中可见)。
	// 因此:
	//   padKey  = KDF(shared, "tthr/qpad/v1",  salt||qpadEnc)
	//   fileKey = KDF(shared, "tthr/file/v1",  salt||qpad)   ← qpad 明文参与
	//   macKey  = KDF(shared, "tthr/mac/v1",   salt||qpad)   ← qpad 明文参与
	// 解密流程: padKey 解出 qpad → fileKey/macKey → 验 MAC → 解密。
	shared, env, err := tac.EncapsulateWithRandom(pub, nil)
	if err != nil {
		return nil, err
	}
	padKey := tac.DeriveKey(shared[:], []byte("tthr/qpad/v1"), concat(salt, nil), 1)

	var nonceArr [16]byte
	copy(nonceArr[:], nonce)

	// 量子垫加密存储 (padKey 不依赖 qpad 明文)
	padStream := tac.NewTStream(padKey, nonceArr)
	qpadEnc := make([]byte, QPadSize)
	padStream.XORBytes(qpadEnc, in.QPad)

	// 最终密钥: qpad 明文参与域分离
	fileKey := tac.DeriveKey(shared[:], []byte("tthr/file/v1"), concat(salt, in.QPad), 1)
	macKey := tac.DeriveKey(shared[:], []byte("tthr/mac/v1"), concat(salt, in.QPad), 1)

	// meta + 压缩流拼装后整体加密
	metaJSON, err := json.Marshal(in.Meta)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, 0, 4+len(metaJSON)+len(in.Stream))
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(metaJSON)))
	plain = append(plain, lenBuf[:]...)
	plain = append(plain, metaJSON...)
	plain = append(plain, in.Stream...)

	stream := tac.NewTStream(fileKey, nonceArr)
	ct := make([]byte, len(plain))
	stream.XORBytes(ct, plain)

	// 头部
	var buf bytes.Buffer
	buf.Write(magic[:])
	buf.WriteByte(Version)
	var flags byte
	if in.Quantum {
		flags |= flagQuantum
	}
	buf.WriteByte(flags)
	buf.Write([]byte{0, 0})
	binary.Write(&buf, binary.LittleEndian, uint32(keyBlobSize))
	binary.Write(&buf, binary.LittleEndian, uint64(len(ct)))
	binary.Write(&buf, binary.LittleEndian, uint32(macSize))

	// keyBlob
	buf.Write(env.EphPub)
	buf.Write(salt)
	buf.Write(qpadEnc)
	buf.Write(nonce)

	// payload
	buf.Write(ct)

	// MAC 覆盖头部字段 + keyBlob + payload
	mac := tac.TMAC(macKey, buf.Bytes()[:hdrSize], env.EphPub, salt, qpadEnc, nonce, ct)
	buf.Write(mac[:])

	if _, err := out.Write(buf.Bytes()); err != nil {
		return nil, err
	}

	res := &PackResult{TotalSize: int64(buf.Len())}
	if priv != nil {
		keyText, err := FormatPrivateKey(priv)
		if err != nil {
			return nil, err
		}
		res.PrivateKey = keyText
	}
	return res, nil
}

// UnpackResult 解包结果。
type UnpackResult struct {
	Meta    Meta
	Stream  []byte // 解密后的 tcz 归档流
	Quantum bool
}

// Unpack 从 r 读入 .tet, 用 keyReader 的私钥解密。
func Unpack(r io.Reader, keyReader io.Reader) (*UnpackResult, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(raw) < hdrSize+keyBlobSize+macSize {
		return nil, ErrNotTthr
	}
	if !bytes.Equal(raw[:4], magic[:]) {
		return nil, ErrNotTthr
	}
	if raw[4] != Version {
		return nil, fmt.Errorf("tcontainer: 不支持的版本 %d", raw[4])
	}
	flags := raw[5]
	keyBlobLen := binary.LittleEndian.Uint32(raw[8:12])
	payloadLen := binary.LittleEndian.Uint64(raw[12:20])
	if keyBlobLen != keyBlobSize {
		return nil, ErrNotTthr
	}
	if payloadLen > maxPayload {
		return nil, errors.New("tcontainer: payload 过大")
	}
	if len(raw) != hdrSize+keyBlobSize+int(payloadLen)+macSize {
		return nil, ErrNotTthr
	}

	hdr := raw[:hdrSize]
	keyBlob := raw[hdrSize : hdrSize+keyBlobSize]
	payload := raw[hdrSize+keyBlobSize : hdrSize+keyBlobSize+int(payloadLen)]
	macBytes := raw[len(raw)-macSize:]

	ephPub := keyBlob[:ephPubSize]
	salt := keyBlob[ephPubSize : ephPubSize+saltSize]
	qpadEnc := keyBlob[ephPubSize+saltSize : ephPubSize+saltSize+QPadSize]
	nonce := keyBlob[ephPubSize+saltSize+QPadSize:]

	priv, err := ParsePrivateKey(keyReader)
	if err != nil {
		return nil, fmt.Errorf("私钥无效: %w", err)
	}

	// 单次解封装恢复 shared, 再对称派生各密钥。
	shared, err := tac.DecapsulateWithInfo(priv, ephPub, nil)
	if err != nil {
		return nil, err
	}
	padKey := tac.DeriveKey(shared[:], []byte("tthr/qpad/v1"), concat(salt, nil), 1)

	// 解出量子垫明文
	var nonceArr [16]byte
	copy(nonceArr[:], nonce)
	padStream := tac.NewTStream(padKey, nonceArr)
	qpad := make([]byte, QPadSize)
	padStream.XORBytes(qpad, qpadEnc)

	// qpad 明文参与最终密钥派生 (量子增强的落点)
	macKey := tac.DeriveKey(shared[:], []byte("tthr/mac/v1"), concat(salt, qpad), 1)
	fileKey := tac.DeriveKey(shared[:], []byte("tthr/file/v1"), concat(salt, qpad), 1)

	// 验证完整性 (密钥错误或 qpad 损坏在此处即被拒绝)
	var want [32]byte
	copy(want[:], macBytes)
	if !tac.TMACVerify(macKey, want, hdr, ephPub, salt, qpadEnc, nonce, payload) {
		return nil, errors.New("tcontainer: 完整性校验失败 (密钥错误或文件损坏)")
	}

	stream := tac.NewTStream(fileKey, nonceArr)
	pt := make([]byte, len(payload))
	stream.XORBytes(pt, payload)

	if len(pt) < 4 {
		return nil, errors.New("tcontainer: payload 过短")
	}
	metaLen := binary.LittleEndian.Uint32(pt[:4])
	if uint64(metaLen)+4 > uint64(len(pt)) {
		return nil, errors.New("tcontainer: meta 长度非法")
	}
	var meta Meta
	if err := json.Unmarshal(pt[4:4+metaLen], &meta); err != nil {
		return nil, fmt.Errorf("tcontainer: meta 解析失败: %w", err)
	}
	return &UnpackResult{
		Meta:    meta,
		Stream:  pt[4+metaLen:],
		Quantum: flags&flagQuantum != 0,
	}, nil
}

// ReadHeaderInfo 只读头部, 返回版本与 flags (info 子命令用)。
func ReadHeaderInfo(r io.Reader) (version byte, quantum bool, size int64, err error) {
	hdr := make([]byte, hdrSize)
	if _, err = io.ReadFull(r, hdr); err != nil {
		return 0, false, 0, ErrNotTthr
	}
	if !bytes.Equal(hdr[:4], magic[:]) {
		return 0, false, 0, ErrNotTthr
	}
	payloadLen := binary.LittleEndian.Uint64(hdr[12:20])
	keyBlobLen := binary.LittleEndian.Uint32(hdr[8:12])
	return hdr[4], hdr[5]&flagQuantum != 0, int64(hdrSize) + int64(keyBlobLen) + int64(payloadLen) + macSize, nil
}

func concat(a, b []byte) []byte {
	out := make([]byte, 0, len(a)+len(b))
	out = append(out, a...)
	return append(out, b...)
}
