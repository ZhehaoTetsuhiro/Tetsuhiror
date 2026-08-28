package tcontainer

import (
	"bytes"
	"strings"
	"testing"
)

func testStream() []byte {
	// 模拟一个归档流 (实际由 tarchive + tcz 产生)
	return []byte("fake archive stream data for testing purposes 0123456789")
}

func TestPackUnpackRoundtrip(t *testing.T) {
	stream := testStream()
	meta := Meta{
		Name:      "testdir",
		Files:     3,
		Dirs:      1,
		OrigSize:  1000,
		CompSize:  int64(len(stream)),
		TCZ:       true,
	}

	qpad := make([]byte, QPadSize)
	for i := range qpad {
		qpad[i] = byte(i * 7)
	}

	var out bytes.Buffer
	res, err := Pack(PackInput{
		Stream:  stream,
		Meta:    meta,
		QPad:    qpad,
		Quantum: false,
		AutoKey: true,
	}, &out)
	if err != nil {
		t.Fatalf("Pack 失败: %v", err)
	}
	if len(res.PrivateKey) == 0 {
		t.Fatal("自动密钥模式应返回私钥")
	}

	// 私钥回读
	ur, err := Unpack(bytes.NewReader(out.Bytes()), strings.NewReader(string(res.PrivateKey)))
	if err != nil {
		t.Fatalf("Unpack 失败: %v", err)
	}
	if !bytes.Equal(ur.Stream, stream) {
		t.Fatal("流往返不一致")
	}
	if ur.Meta.Name != "testdir" || ur.Meta.Files != 3 {
		t.Fatalf("meta 不一致: %+v", ur.Meta)
	}
}

func TestUnpackWrongKey(t *testing.T) {
	stream := testStream()
	meta := Meta{Name: "x"}

	qpad := make([]byte, QPadSize)

	var out bytes.Buffer
	res, err := Pack(PackInput{Stream: stream, Meta: meta, QPad: qpad, AutoKey: true}, &out)
	if err != nil {
		t.Fatal(err)
	}

	// 篡改私钥
	badKey := strings.Replace(string(res.PrivateKey), "A", "B", 1)
	if _, err := Unpack(bytes.NewReader(out.Bytes()), strings.NewReader(badKey)); err == nil {
		t.Fatal("错误密钥未被拒绝")
	}
}

func TestUnpackTamperedPayload(t *testing.T) {
	stream := testStream()
	meta := Meta{Name: "x"}
	qpad := make([]byte, QPadSize)

	var out bytes.Buffer
	res, err := Pack(PackInput{Stream: stream, Meta: meta, QPad: qpad, AutoKey: true}, &out)
	if err != nil {
		t.Fatal(err)
	}

	raw := out.Bytes()
	// 篡改 payload 中部
	raw[len(raw)-40] ^= 0xff

	if _, err := Unpack(bytes.NewReader(raw), strings.NewReader(string(res.PrivateKey))); err == nil {
		t.Fatal("篡改未被检测")
	}
}

func TestNotTthr(t *testing.T) {
	if _, err := Unpack(bytes.NewReader([]byte("not a tthr file")), strings.NewReader("key")); err == nil {
		t.Fatal("非 tthr 文件未被拒绝")
	}
}

func TestKeyFormatRoundtrip(t *testing.T) {
	// 间接通过 Pack 检查密钥文本格式
	stream := testStream()
	var out bytes.Buffer
	res, err := Pack(PackInput{Stream: stream, Meta: Meta{Name: "k"}, QPad: make([]byte, QPadSize), AutoKey: true}, &out)
	if err != nil {
		t.Fatal(err)
	}
	k := string(res.PrivateKey)
	if !strings.Contains(k, "-----BEGIN TTHR PRIVATE KEY-----") {
		t.Fatal("缺少头标记")
	}
	if !strings.Contains(k, "-----END TTHR PRIVATE KEY-----") {
		t.Fatal("缺少尾标记")
	}
}
