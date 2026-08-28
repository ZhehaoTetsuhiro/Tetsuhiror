package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tetsuhiro/tthr/internal/qenhance"
	"tetsuhiro/tthr/internal/tarchive"
	"tetsuhiro/tthr/internal/tcontainer"
	"tetsuhiro/tthr/internal/tcz"
)

// cmdPack: tet pack [选项] <目录>
func cmdPack(args []string) error {
	var opt struct {
		output   string
		pubKey   string
		quantum  bool
		python   string
		quiet    bool
	}
	opt.quantum = true // 默认启用量子增强

	var target string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-o" || a == "--output":
			if i+1 >= len(args) {
				return fmt.Errorf("%s 需要参数", a)
			}
			i++
			opt.output = args[i]
		case a == "-k" || a == "--key":
			if i+1 >= len(args) {
				return fmt.Errorf("%s 需要参数", a)
			}
			i++
			opt.pubKey = args[i]
		case a == "-q" || a == "--quantum":
			opt.quantum = true
		case a == "--no-quantum":
			opt.quantum = false
		case a == "--python":
			if i+1 >= len(args) {
				return fmt.Errorf("%s 需要参数", a)
			}
			i++
			opt.python = args[i]
		case a == "--quiet":
			opt.quiet = true
		case a == "-l" || a == "--level":
			if i+1 >= len(args) {
				return fmt.Errorf("%s 需要参数", a)
			}
			i++
			// 单一压缩等级, 接受但忽略
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
		return fmt.Errorf("pack 需要一个目录参数")
	}

	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s 不是目录", target)
	}

	// 输出文件名
	outPath := opt.output
	if outPath == "" {
		base := filepath.Base(filepath.Clean(target))
		outPath = base + ".tet"
	}
	outAbs, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}

	logf := func(format string, a ...any) {
		if !opt.quiet {
			fmt.Printf(format+"\n", a...)
		}
	}

	// 1. 遍历目录生成归档流
	logf("扫描目录 %s ...", target)
	var archBuf strings.Builder
	_ = archBuf
	var raw bytesBuffer
	aw := tarchive.NewWriter(&raw)
	files, dirs, links, totalSize, err := tarchive.WalkDir(aw, target, outAbs)
	if err != nil {
		return fmt.Errorf("归档失败: %w", err)
	}
	if err := aw.Close(); err != nil {
		return err
	}
	logf("  %d 文件, %d 目录, %d 符号链接, 原始 %s",
		files, dirs, links, humanSize(totalSize))

	// 2. TCZ 压缩
	logf("TCZ 压缩中 ...")
	comp := tcz.Compress(raw.Bytes())
	logf("  归档流 %s -> %s (%.1f%%)",
		humanSize(int64(len(raw.Bytes()))), humanSize(int64(len(comp))),
		float64(len(comp))*100/float64(len(raw.Bytes())+1))

	// 3. 量子垫
	var qpad []byte
	qSource := "off"
	qMode := ""
	qDetail := "未启用量子增强"
	if opt.quantum {
		logf("QPanda 量子熵生成中 (Shor 周期查找增强) ...")
		qr, err := qenhance.Generate(opt.python)
		if err != nil {
			return fmt.Errorf("量子熵生成失败: %w", err)
		}
		qpad = qr.QPad
		qSource = qr.Source
		qMode = qr.Mode
		qDetail = qr.Detail
		logf("  %s", qDetail)
	} else {
		qpad = make([]byte, tcontainer.QPadSize)
		if _, err := readRandom(qpad); err != nil {
			return err
		}
	}

	// 4. 组装容器
	meta := tcontainer.Meta{
		Name:      filepath.Base(filepath.Clean(target)),
		Files:     files,
		Dirs:      dirs,
		Symlinks:  links,
		OrigSize:  totalSize,
		CompSize:  int64(len(comp)),
		CreatedAt: time.Now(),
		TCZ:       true,
	}

	var pub []byte
	if opt.pubKey != "" {
		pub, err = os.ReadFile(opt.pubKey)
		if err != nil {
			return fmt.Errorf("读取公钥失败: %w", err)
		}
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	res, err := tcontainer.Pack(tcontainer.PackInput{
		Stream:  comp,
		Meta:    meta,
		QPad:    qpad,
		Quantum: qSource == "qpanda",
		Pub:     pub,
		AutoKey: opt.pubKey == "",
	}, outFile)
	if err != nil {
		return fmt.Errorf("加密失败: %w", err)
	}
	if err := outFile.Close(); err != nil {
		return err
	}

	// 5. 自动密钥写盘
	keyPath := ""
	if res.PrivateKey != nil {
		keyPath = outPath + ".key"
		if err := os.WriteFile(keyPath, res.PrivateKey, 0o600); err != nil {
			return fmt.Errorf("保存私钥失败: %w", err)
		}
	}

	logf("容器 %s (%s)", outPath, humanSize(res.TotalSize))
	if totalSize > 0 {
		logf("总压缩率: %.2f%% (原始 %s -> 容器 %s)",
			float64(res.TotalSize)*100/float64(totalSize),
			humanSize(totalSize), humanSize(res.TotalSize))
	}
	if keyPath != "" {
		logf("私钥已保存: %s (解包需要它, 请妥善保管)", keyPath)
	}
	src := qSource
	if src == "qpanda" && qMode == "shor" {
		src = "qpanda/shor"
	}
	logf("量子熵源: %s", src)
	return nil
}
