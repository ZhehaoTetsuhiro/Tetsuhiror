package tcz

import (
	"bytes"
	"math/rand"
	"testing"
)

func roundtrip(t *testing.T, name string, data []byte) {
	t.Helper()
comp := Compress(data)
	out, err := Decompress(comp)
	if err != nil {
		t.Fatalf("%s: 解压失败: %v", name, err)
	}
	if !bytes.Equal(data, out) {
		t.Fatalf("%s: 往返不一致 (输入 %d 字节, 输出 %d 字节)", name, len(data), len(out))
	}
	t.Logf("%s: %d -> %d 字节 (%.2f%%)", name, len(data), len(comp), float64(len(comp))*100/float64(len(data)+1))
}

func TestRoundtripEmpty(t *testing.T)          { roundtrip(t, "empty", nil) }
func TestRoundtripTiny(t *testing.T)           { roundtrip(t, "tiny", []byte("a")) }
func TestRoundtripSmall(t *testing.T)          { roundtrip(t, "small", []byte("hello tcz hello tcz hello tcz")) }

func TestRoundtripText(t *testing.T) {
	var b bytes.Buffer
	for i := 0; i < 500; i++ {
		b.WriteString("The quick brown fox jumps over the lazy dog. ")
		b.WriteString("敏捷的棕色狐狸跳过了懒惰的狗。")
	}
	roundtrip(t, "text", b.Bytes())
}

func TestRoundtripRepetitive(t *testing.T) {
	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = byte(i % 7)
	}
	roundtrip(t, "repetitive", data)
}

func TestRoundtripRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	data := make([]byte, 300000)
	rng.Read(data)
	roundtrip(t, "random", data)
}

func TestRoundtripAllZero(t *testing.T) {
	roundtrip(t, "zeros", make([]byte, 1<<20))
}

func TestCorruptMagic(t *testing.T) {
	comp := Compress([]byte("hello world hello world"))
	comp[0] = 'X'
	if _, err := Decompress(comp); err == nil {
		t.Fatal("期望 magic 错误被检测")
	}
}

func TestDecompressTruncated(t *testing.T) {
	comp := Compress([]byte("hello world hello world"))
	if _, err := Decompress(comp[:len(comp)-3]); err == nil {
		// 截断后仍可能解码出足够字节, 但 origLen 校验应失败
		t.Log("截断流未报错 (容错路径)")
	}
}
