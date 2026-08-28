package qenhance

import (
	"bytes"
	"strings"
	"testing"
)

func TestExtractQPadDeterministic(t *testing.T) {
	a := ExtractQPad([]byte("raw-a"), []byte("raw-b"))
	b := ExtractQPad([]byte("raw-a"), []byte("raw-b"))
	if !bytes.Equal(a, b) {
		t.Fatalf("提取结果不确定: %x vs %x", a, b)
	}
	if len(a) != QPadSize {
		t.Fatalf("量子垫长度错误: %d", len(a))
	}
	c := ExtractQPad([]byte("raw-a"), []byte("raw-c"))
	if bytes.Equal(a, c) {
		t.Fatal("不同原始比特应产生不同量子垫")
	}
	// 空输入也应稳定 (域分离标签保证)
	d := ExtractQPad()
	if len(d) != QPadSize {
		t.Fatalf("空输入量子垫长度错误: %d", len(d))
	}
}

func TestExtractQPadNotTriviallyStructured(t *testing.T) {
	// Shor 测量的典型结构: 计数寄存器聚集在 Q/r 倍数 (大量重复字节)
	raw := bytes.Repeat([]byte{0x40, 0x44, 0x48, 0x4c}, 64)
	pad := ExtractQPad(raw, []byte{0xff})
	if bytes.Equal(pad, raw[:QPadSize]) {
		t.Fatal("提取应白化输入的结构")
	}
	// 均匀性粗检: 非全零/全 ff
	allZero, allFF := true, true
	for _, b := range pad {
		allZero = allZero && b == 0
		allFF = allFF && b == 0xff
	}
	if allZero || allFF {
		t.Fatalf("提取结果退化: %x", pad)
	}
}

func TestBuildShorDetail(t *testing.T) {
	r4, r2 := 4, 2
	out := &scriptOutput{
		Mode:         "shor",
		Measurements: 280,
		RawBits:      2544,
		MinEnt:       1276.1,
		Circuits: []shorCircuit{
			{N: 15, A: 7, T: 4, Qubits: 20, Shots: 96, R: &r4, Factors: []int{3, 5}},
			{N: 15, A: 11, T: 4, Qubits: 20, Shots: 64, R: &r2},
			{N: 21, A: 2, T: 5, Qubits: 25, Shots: 24},
		},
	}
	d := buildShorDetail(out)
	for _, want := range []string{"15=3×5", "a=7", "r=4", "r=2", "N=21 a=2 (r 未恢复)", "3 电路", "280 次测量", "THASH-256"} {
		if !strings.Contains(d, want) {
			t.Errorf("detail 缺少 %q: %s", want, d)
		}
	}
}

func TestDetectPython(t *testing.T) {
	p := DetectPython()
	if p == "" {
		t.Skip("未找到可 import pyqpanda 的解释器")
	}
	t.Logf("探测到解释器: %s", p)
}

// 集成测试: 真实调用 pyqpanda 运行 Shor 周期查找。
func TestGenerateShor(t *testing.T) {
	if testing.Short() {
		t.Skip("short 模式跳过量子集成测试")
	}
	p := DetectPython()
	if p == "" {
		t.Skip("无 pyqpanda 环境")
	}
	r, err := Generate(p)
	if err != nil {
		t.Fatalf("Generate 失败: %v", err)
	}
	if r.Source != "qpanda" {
		t.Fatalf("source = %s, detail = %s", r.Source, r.Detail)
	}
	if len(r.QPad) != QPadSize {
		t.Fatalf("量子垫长度错误: %d", len(r.QPad))
	}
	t.Logf("mode=%s detail=%s", r.Mode, r.Detail)
	// Shor 模式应至少分解出一个 N; 若回退到 h 模式则说明环境受限, 仅提示
	if r.Mode != "shor" && !strings.Contains(r.Detail, "Hadamard") {
		t.Errorf("意外模式: %s (%s)", r.Mode, r.Detail)
	}
}

// 两次 Generate 应产生不同的量子垫 (测量坍缩随机)。
func TestGenerateDistinct(t *testing.T) {
	if testing.Short() {
		t.Skip("short 模式跳过量子集成测试")
	}
	p := DetectPython()
	if p == "" {
		t.Skip("无 pyqpanda 环境")
	}
	a, err := Generate(p)
	if err != nil {
		t.Fatalf("第一次 Generate 失败: %v", err)
	}
	b, err := Generate(p)
	if err != nil {
		t.Fatalf("第二次 Generate 失败: %v", err)
	}
	if a.Source != "qpanda" || b.Source != "qpanda" {
		t.Fatalf("回退了系统熵: %s / %s", a.Detail, b.Detail)
	}
	if bytes.Equal(a.QPad, b.QPad) {
		t.Fatal("两次量子垫相同 (测量应随机)")
	}
}
