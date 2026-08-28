// Package tarchive 定义 .tthr 的归档流格式。
//
// 归档流是一个简单的顺序记录格式 (类 tar):
//
//	u16  nameLen      路径长度 (UTF-8, '/' 分隔)
//	name bytes        相对路径
//	u8   type         0=文件 1=目录 2=符号链接
//	u32  mode         Unix 权限位
//	u64  size         文件内容长度 (目录为 0; 链接为目标路径长度)
//	u64  modTime      Unix 秒
//	content bytes     文件内容或链接目标
//	u32  contentHash  内容的 CRC32 (防截断)
//
// 结束标记: nameLen=0 的记录。
// 全部小端。归档流整体再交给 tcz 压缩。
package tarchive

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EntryType 常量。
const (
	TypeFile    = 0
	TypeDir     = 1
	TypeSymlink = 2
)

// Entry 是一条归档记录。
type Entry struct {
	Name    string
	Type    uint8
	Mode    uint32
	Size    uint64
	ModTime int64
	Content []byte
}

// Writer 写归档流。
type Writer struct {
	bw   *bufio.Writer
	buf  [16]byte
	err  error
	seen map[string]bool
}

// NewWriter 包装 w。
func NewWriter(w io.Writer) *Writer {
	return &Writer{bw: bufio.NewWriter(w), seen: make(map[string]bool)}
}

func (w *Writer) writeEntry(e *Entry) error {
	if err := checkName(e.Name); err != nil {
		return err
	}
	if w.seen[e.Name] {
		return fmt.Errorf("重复路径: %s", e.Name)
	}
	w.seen[e.Name] = true

	var hdr bytes.Buffer
	binary.Write(&hdr, binary.LittleEndian, uint16(len(e.Name)))
	hdr.WriteString(e.Name)
	hdr.WriteByte(e.Type)
	binary.Write(&hdr, binary.LittleEndian, e.Mode)
	binary.Write(&hdr, binary.LittleEndian, e.Size)
	binary.Write(&hdr, binary.LittleEndian, uint64(e.ModTime))
	hdr.Write(e.Content)
	binary.Write(&hdr, binary.LittleEndian, crc32.ChecksumIEEE(e.Content))

	_, err := w.bw.Write(hdr.Bytes())
	return err
}

// WriteFile 写入一个文件 (内容已在内存中)。
func (w *Writer) WriteFile(name string, mode uint32, modTime int64, content []byte) error {
	return w.writeEntry(&Entry{Name: name, Type: TypeFile, Mode: mode, ModTime: modTime, Content: content, Size: uint64(len(content))})
}

// WriteDir 写入目录记录。
func (w *Writer) WriteDir(name string, mode uint32, modTime int64) error {
	return w.writeEntry(&Entry{Name: name, Type: TypeDir, Mode: mode, ModTime: modTime})
}

// WriteSymlink 写入符号链接记录。
func (w *Writer) WriteSymlink(name string, target string, modTime int64) error {
	return w.writeEntry(&Entry{Name: name, Type: TypeSymlink, Mode: 0o777, ModTime: modTime, Content: []byte(target), Size: uint64(len(target))})
}

// Close 写结束标记并刷新。
func (w *Writer) Close() error {
	if _, err := w.bw.Write([]byte{0, 0}); err != nil {
		return err
	}
	return w.bw.Flush()
}

// Reader 读归档流。
type Reader struct {
	br  *bufio.Reader
	buf []byte
}

// NewReader 包装 r。
func NewReader(r io.Reader) *Reader {
	return &Reader{br: bufio.NewReader(r)}
}

// Next 返回下一条记录; io.EOF 表示流结束。
func (r *Reader) Next() (*Entry, error) {
	nameLenBuf := make([]byte, 2)
	if _, err := io.ReadFull(r.br, nameLenBuf); err != nil {
		return nil, err
	}
	nameLen := binary.LittleEndian.Uint16(nameLenBuf)
	if nameLen == 0 {
		return nil, io.EOF
	}
	// 布局: nameLen | name | type | mode | size | mtime | content | crc
	name := make([]byte, nameLen)
	if _, err := io.ReadFull(r.br, name); err != nil {
		return nil, fmt.Errorf("路径截断: %w", err)
	}
	hdr := make([]byte, 1+4+8+8)
	if _, err := io.ReadFull(r.br, hdr); err != nil {
		return nil, fmt.Errorf("归档头截断: %w", err)
	}
	e := &Entry{
		Name: string(name),
		Type: hdr[0],
		Mode: binary.LittleEndian.Uint32(hdr[1:5]),
		Size: binary.LittleEndian.Uint64(hdr[5:13]),
		ModTime: int64(binary.LittleEndian.Uint64(hdr[13:21])),
	}
	if err := checkName(e.Name); err != nil {
		return nil, err
	}
	if e.Size > maxEntrySize {
		return nil, fmt.Errorf("条目 %s 大小 %d 超出上限", e.Name, e.Size)
	}
	if e.Type == TypeDir && e.Size != 0 {
		return nil, fmt.Errorf("目录 %s 带内容", e.Name)
	}
	e.Content = make([]byte, e.Size)
	if _, err := io.ReadFull(r.br, e.Content); err != nil {
		return nil, fmt.Errorf("条目 %s 内容截断: %w", e.Name, err)
	}
	crcBuf := make([]byte, 4)
	if _, err := io.ReadFull(r.br, crcBuf); err != nil {
		return nil, fmt.Errorf("条目 %s 校验截断: %w", e.Name, err)
	}
	if got := crc32.ChecksumIEEE(e.Content); got != binary.LittleEndian.Uint32(crcBuf) {
		return nil, fmt.Errorf("条目 %s 校验失败", e.Name)
	}
	return e, nil
}

const maxEntrySize = 8 << 30 // 单条目 8 GiB 上限

// checkName 拒绝绝对路径与 .. 穿越。
func checkName(name string) error {
	if name == "" {
		return errors.New("空路径")
	}
	if strings.Contains(name, "\\") {
		return fmt.Errorf("路径 %s 含反斜杠", name)
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("绝对路径: %s", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return fmt.Errorf("路径穿越: %s", name)
		}
	}
	return nil
}

// WalkDir 遍历 root, 把整个目录树写入归档流。
// skip 指定输出文件自身 (避免把自己打进去)。
func WalkDir(w *Writer, root string, skipAbs string) (files, dirs, links int, totalSize int64, err error) {
	root = filepath.Clean(root)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if skipAbs != "" && path == skipAbs {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		switch {
		case d.Type()&os.ModeSymlink != 0:
			target, lerr := os.Readlink(path)
			if lerr != nil {
				return lerr
			}
			if err := w.WriteSymlink(filepath.ToSlash(rel), target, info.ModTime().Unix()); err != nil {
				return err
			}
			links++
		case d.IsDir():
			if err := w.WriteDir(filepath.ToSlash(rel), uint32(info.Mode().Perm()), info.ModTime().Unix()); err != nil {
				return err
			}
			dirs++
		case d.Type().IsRegular():
			content, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			if err := w.WriteFile(filepath.ToSlash(rel), uint32(info.Mode().Perm()), info.ModTime().Unix(), content); err != nil {
				return err
			}
			files++
			totalSize += int64(len(content))
		default:
			// 套接字/FIFO/设备文件: 跳过
		}
		return nil
	})
	return
}

// Extract 把归档流解出到 destDir。
func Extract(r *Reader, destDir string) (files, dirs, links int, totalSize int64, err error) {
	for {
		e, nerr := r.Next()
		if nerr == io.EOF {
			return
		}
		if nerr != nil {
			return files, dirs, links, totalSize, nerr
		}
		target := filepath.Join(destDir, filepath.FromSlash(e.Name))
		// 双重防护: 解析后必须在 destDir 内
		absDest, aerr := filepath.Abs(destDir)
		if aerr != nil {
			return files, dirs, links, totalSize, aerr
		}
		absTarget, aerr := filepath.Abs(target)
		if aerr != nil {
			return files, dirs, links, totalSize, aerr
		}
		if absTarget != absDest && !strings.HasPrefix(absTarget, absDest+string(os.PathSeparator)) {
			return files, dirs, links, totalSize, fmt.Errorf("拒绝越界路径: %s", e.Name)
		}

		switch e.Type {
		case TypeDir:
			if err := os.MkdirAll(target, os.FileMode(e.Mode)); err != nil {
				return files, dirs, links, totalSize, err
			}
			os.Chmod(target, os.FileMode(e.Mode))
			dirs++
		case TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return files, dirs, links, totalSize, err
			}
			_ = os.Remove(target)
			if err := os.Symlink(string(e.Content), target); err != nil {
				return files, dirs, links, totalSize, err
			}
			links++
		case TypeFile:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return files, dirs, links, totalSize, err
			}
			if err := os.WriteFile(target, e.Content, os.FileMode(e.Mode)); err != nil {
				return files, dirs, links, totalSize, err
			}
			os.Chmod(target, os.FileMode(e.Mode))
			os.Chtimes(target, time.Unix(e.ModTime, 0), time.Unix(e.ModTime, 0))
			files++
			totalSize += int64(len(e.Content))
		default:
			return files, dirs, links, totalSize, fmt.Errorf("未知条目类型 %d: %s", e.Type, e.Name)
		}
	}
}
