package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"math/rand"
	"os"

	"tetsuhiro/tthr/internal/tcz"
)

func main() {
	rng := rand.New(rand.NewSource(1))
	cases := []struct {
		name string
		data []byte
	}{
		{"英文文本", englishText(200000)},
		{"混合中英", mixedText(100000)},
		{"结构化日志", logText(150000)},
		{"随机二进制", randBytes(rng, 100000)},
		{"重复模式", patternBytes(100000)},
	}

	fmt.Printf("%-12s %10s %10s %8s %10s %8s\n", "数据", "原始", "TCZ", "TCZ%", "gzip-9", "gzip%")
	for _, c := range cases {
		tczOut := tcz.Compress(c.data)
		var gz bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&gz, 9)
		zw.Write(c.data)
		zw.Close()
		fmt.Printf("%-12s %10d %10d %7.1f%% %10d %7.1f%%\n",
			c.name, len(c.data), len(tczOut),
			float64(len(tczOut))*100/float64(len(c.data)),
			gz.Len(), float64(gz.Len())*100/float64(len(c.data)))
	}
	os.Exit(0)
}

func englishText(n int) []byte {
	var b bytes.Buffer
	for b.Len() < n {
		fmt.Fprintf(&b, "The quick brown fox jumps over the lazy dog near the river bank. ")
	}
	return b.Bytes()[:n]
}

func mixedText(n int) []byte {
	var b bytes.Buffer
	for b.Len() < n {
		fmt.Fprintf(&b, "Tetsuhiro 自研压缩算法与加密算法, quantum enhanced. 量子增强加密。")
	}
	return b.Bytes()[:n]
}

func logText(n int) []byte {
	var b bytes.Buffer
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "[2025-08-28 10:%02d:%02d] INFO worker=%d latency=%dms status=ok\n", i%60, (i*7)%60, i%8, 10+i%23)
	}
	return b.Bytes()[:n]
}

func randBytes(rng *rand.Rand, n int) []byte {
	b := make([]byte, n)
	rng.Read(b)
	return b
}

func patternBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte("ABCDEF0123456789"[i%16])
	}
	return b
}
