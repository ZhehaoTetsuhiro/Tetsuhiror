package tcontainer

// 密钥文件的文本格式 (TTHR-KEY v1):
//
//	-----BEGIN TTHR PRIVATE KEY-----
//	base64 (32 字节私钥标量)
//	-----END TTHR PRIVATE KEY-----
//
// 公钥从私钥推导, 无需单独存储。

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"tetsuhiro/tthr/internal/tac"
)

const (
	privHeader = "-----BEGIN TTHR PRIVATE KEY-----"
	privFooter = "-----END TTHR PRIVATE KEY-----"
)

// FormatPrivateKey 把私钥格式化为文本。
func FormatPrivateKey(priv *tac.PrivateKey) ([]byte, error) {
	b64 := base64.StdEncoding.EncodeToString(priv.MarshalPrivateKey())
	var buf bytes.Buffer
	buf.WriteString(privHeader + "\n")
	for len(b64) > 0 {
		n := 64
		if len(b64) < n {
			n = len(b64)
		}
		buf.WriteString(b64[:n] + "\n")
		b64 = b64[n:]
	}
	buf.WriteString(privFooter + "\n")
	return buf.Bytes(), nil
}

// ParsePrivateKey 从 io.Reader 解析私钥文本。
func ParsePrivateKey(r io.Reader) (*tac.PrivateKey, error) {
	if r == nil {
		return nil, errors.New("未提供密钥")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, errors.New("密钥文件过大")
	}
	text := string(data)
	if !strings.Contains(text, privHeader) || !strings.Contains(text, privFooter) {
		return nil, errors.New("密钥格式错误 (缺少 TTHR PRIVATE KEY 标记)")
	}
	begin := strings.Index(text, privHeader) + len(privHeader)
	end := strings.Index(text, privFooter)
	b64 := strings.TrimSpace(strings.ReplaceAll(text[begin:end], "\n", ""))
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("base64 解码失败: %w", err)
	}
	return tac.ParsePrivateKey(raw)
}

const (
	pubHeader = "-----BEGIN TTHR PUBLIC KEY-----"
	pubFooter = "-----END TTHR PUBLIC KEY-----"
)

// FormatPublicKey 把公钥格式化为文本。
func FormatPublicKey(pub *tac.PublicKey) []byte {
	return []byte(pubHeader + "\n" + encodePubB64(pub.MarshalPublicKey()) + "\n" + pubFooter + "\n")
}

// ParsePublicKeyText 从 PEM 样式文本解析公钥; 也接受裸 base64。
func ParsePublicKeyText(data []byte) (*tac.PublicKey, error) {
	text := strings.TrimSpace(string(data))
	if strings.Contains(text, pubHeader) {
		begin := strings.Index(text, pubHeader) + len(pubHeader)
		end := strings.Index(text, pubFooter)
		if end < 0 || end <= begin {
			return nil, errors.New("公钥格式错误")
		}
		text = strings.TrimSpace(strings.ReplaceAll(text[begin:end], "\n", ""))
	}
	raw, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil, fmt.Errorf("公钥 base64 解码失败: %w", err)
	}
	return tac.ParsePublicKey(raw)
}

func encodePubB64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
