package tarchive

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriterReaderRoundtrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)


	if err := w.WriteDir("docs", 0o755, 1000); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile("docs/a.txt", 0o644, 2000, []byte("hello world")); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile("docs/exec.sh", 0o755, 3000, bytes.Repeat([]byte("echo hi\n"), 100)); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteSymlink("docs/link", "a.txt", 4000); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	r := NewReader(bytes.NewReader(buf.Bytes()))
	count := 0
	for {
		e, err := r.Next()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("读取失败: %v", err)
		}
		count++
		_ = e
	}
	if count != 4 {
		t.Fatalf("条目数错误: %d", count)
	}
}

func TestRejectTraversal(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.WriteFile("../evil.txt", 0o644, 0, nil); err == nil {
		t.Fatal("路径穿越未被拒绝")
	}
	if err := w.WriteFile("/abs.txt", 0o644, 0, nil); err == nil {
		t.Fatal("绝对路径未被拒绝")
	}
	if err := w.WriteFile("a\\b.txt", 0o644, 0, nil); err == nil {
		t.Fatal("反斜杠未被拒绝")
	}
}

func TestExtractRoundtrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "x.txt"), []byte("data"), 0o644)

	var buf bytes.Buffer
	w := NewWriter(&buf)
	files, _, _, _, err := WalkDir(w, src, "")
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	if files != 1 {
		t.Fatalf("文件数: %d", files)
	}

	out := filepath.Join(dir, "out")
	r := NewReader(bytes.NewReader(buf.Bytes()))
	of, _, _, _, err := Extract(r, out)
	if err != nil {
		t.Fatal(err)
	}
	if of != 1 {
		t.Fatalf("提取文件数: %d", of)
	}
	data, err := os.ReadFile(filepath.Join(out, "x.txt"))
	if err != nil || string(data) != "data" {
		t.Fatalf("提取内容错误: %q %v", data, err)
	}
}
