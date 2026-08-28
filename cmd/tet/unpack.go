package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tetsuhiro/tthr/internal/tarchive"
	"tetsuhiro/tthr/internal/tcontainer"
	"tetsuhiro/tthr/internal/tcz"
)

// cmdUnpack: tet unpack [选项] <文件.tet>
func cmdUnpack(args []string) error {
	var opt struct {
		dest  string
		key   string
		quiet bool
	}

	var target string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-d" || a == "--dest":
			if i+1 >= len(args) {
				return fmt.Errorf("%s 需要参数", a)
			}
			i++
			opt.dest = args[i]
		case a == "-k" || a == "--key":
			if i+1 >= len(args) {
				return fmt.Errorf("%s 需要参数", a)
			}
			i++
			opt.key = args[i]
		case a == "--quiet":
			opt.quiet = true
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("未知选项: %s", a)
		default:
			if target != "" {
				return fmt.Errorf("多余的位置参数: %s", a)
			}
			target = a
		}
	}
	if target == "" {
		return fmt.Errorf("unpack 需要一个 .tet 文件参数")
	}
	if !strings.HasSuffix(target, ".tet") {
		// 宽容: 允许任意文件名, 但提示
	}

	f, err := os.Open(target)
	if err != nil {
		return err
	}
	defer f.Close()

	// 密钥来源: -k 指定, 或 <target>.key
	keyPath := opt.key
	if keyPath == "" {
		candidate := target + ".key"
		if _, err := os.Stat(candidate); err == nil {
			keyPath = candidate
		}
	}
	if keyPath == "" {
		return fmt.Errorf("未指定私钥: 用 -k <file> 提供, 或把 <文件>.key 放在同目录")
	}
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("读取私钥失败: %w", err)
	}

	logf := func(format string, a ...any) {
		if !opt.quiet {
			fmt.Printf(format+"\n", a...)
		}
	}

	logf("解密 %s ...", target)
	ur, err := tcontainer.Unpack(f, strings.NewReader(string(keyData)))
	if err != nil {
		return err
	}
	quantumStr := "否"
	if ur.Quantum {
		quantumStr = "是"
	}
	logf("  解密成功 (量子增强: %s)", quantumStr)

	// TCZ 解压
	logf("TCZ 解压中 ...")
	archiveStream, err := tcz.Decompress(ur.Stream)
	if err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}

	// 目标目录
	dest := opt.dest
	if dest == "" {
		base := filepath.Base(target)
		base = strings.TrimSuffix(base, ".tet")
		dest = base
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}

	logf("提取到 %s ...", dest)
	ar := tarchive.NewReader(bytes.NewReader(archiveStream))
	files, dirs, links, totalSize, err := tarchive.Extract(ar, dest)
	if err != nil {
		return fmt.Errorf("提取失败: %w", err)
	}
	logf("完成: %d 文件, %d 目录, %d 符号链接, %s", files, dirs, links, humanSize(totalSize))
	return nil
}
