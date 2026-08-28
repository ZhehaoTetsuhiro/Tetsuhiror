package tcz

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"
)

// genMixed 生成确定性的混合内容 (文本 + 重复块 + 二进制)。
// 与 2025-08 发现算术编码器区间塌缩 bug 时使用的生成器同源,
// 用于回归测试大输入下罕见状态路径。
func genMixed(n int, seed int64) []byte {
	r := rand.New(rand.NewSource(seed))
	words := []string{"quantum", "shor", "period", "lattice", "entropy", "hash",
		"compression", "arithmetic", "coding", "token", "window", "match",
		"literal", "distance", "length", "context", "model", "probability"}
	buf := make([]byte, 0, n)
	for len(buf) < n {
		switch r.Intn(10) {
		case 0, 1, 2:
			s := bytes.Repeat([]byte("WOS-sym: the quick brown fox 0123456789\n"), 1+r.Intn(50))
			buf = append(buf, s...)
		case 3:
			b := make([]byte, 64+r.Intn(512))
			r.Read(b)
			buf = append(buf, b...)
		default:
			nw := 3 + r.Intn(12)
			for i := 0; i < nw; i++ {
				buf = append(buf, words[r.Intn(len(words))]...)
				buf = append(buf, ' ')
			}
			buf = append(buf, fmt.Appendf(nil, "ID=%d.%d\n", r.Int63(), r.Intn(9999))...)
		}
	}
	return buf[:n]
}

// TestIntervalCollapseRegression 回归: 算术编码器重归一化循环曾被加上
// x1 != x2 守卫, 区间收缩到 x1==x2 (约每 3e5 次重归一化出现一次) 时
// 循环被跳过, 区间冻死成一个点, 从该处开始静默损坏码流 —— 4 MiB 以上
// 输入解压失败或输出错误。此样本 (seed=5) 在旧代码上必然触发。
// 修复: 与 lpaq1 原版一致, 塌缩时照常移出字节并把区间再展开为
// [v<<8, v<<8|255] (见 arith.go Encode 注释)。
func TestIntervalCollapseRegression(t *testing.T) {
	data := genMixed(4<<20, 5)
	comp := Compress(data)
	out, err := Decompress(comp)
	if err != nil {
		t.Fatalf("4MiB seed5 回归样本解压失败: %v", err)
	}
	if !bytes.Equal(data, out) {
		t.Fatalf("4MiB seed5 回归样本内容不一致")
	}
	// 历史上最小化的失败前缀 (2597183 字节) 一并回归
	prefix := genMixed(4<<20, 5)[:2597183]
	comp = Compress(prefix)
	out, err = Decompress(comp)
	if err != nil || !bytes.Equal(prefix, out) {
		t.Fatalf("最小化前缀回归失败: err=%v equal=%v", err, bytes.Equal(prefix, out))
	}
}

// TestRoundtripMixedMultiSeed 多种子混合内容往返。
func TestRoundtripMixedMultiSeed(t *testing.T) {
	for seed := int64(1); seed <= 8; seed++ {
		data := genMixed(1<<20, seed)
		comp := Compress(data)
		out, err := Decompress(comp)
		if err != nil {
			t.Fatalf("seed=%d 解压失败: %v", seed, err)
		}
		if !bytes.Equal(data, out) {
			t.Fatalf("seed=%d 内容不一致", seed)
		}
	}
}
