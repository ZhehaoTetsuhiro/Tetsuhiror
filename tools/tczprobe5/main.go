// tczprobe5: 最小化 4MiB/seed5 样本并插桩定位首个 bit 分歧。
package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"

	"tetsuhiro/tthr/internal/tcz"
)

func genData(n int, seed int64) []byte {
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
			fmt.Fprintf(&bytesBuffer{&buf}, "ID=%d.%d\n", r.Int63(), r.Intn(9999))
		}
	}
	return buf[:n]
}

type bytesBuffer struct{ b *[]byte }

func (w *bytesBuffer) Write(p []byte) (int, error) {
	*w.b = append(*w.b, p...)
	return len(p), nil
}

func fails(data []byte) bool {
	comp := tcz.Compress(data)
	out, err := tcz.Decompress(comp)
	return err != nil || !bytes.Equal(data, out)
}

func main() {
	data := genData(4<<20, 5)
	if !fails(data) {
		fmt.Println("未复现")
		return
	}
	lo, hi := 1, len(data)
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if fails(data[:mid]) {
			hi = mid
		} else {
			lo = mid
		}
	}
	fmt.Printf("最小失败前缀: %d 字节\n", hi)
	os.WriteFile("/tmp/tcz-fail-min.bin", data[:hi], 0644)
	fmt.Println("已保存 /tmp/tcz-fail-min.bin")
}
