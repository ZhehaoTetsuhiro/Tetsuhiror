package main

import (
	"fmt"
	"os"

	"tetsuhiro/tthr/internal/tcontainer"
)

// cmdInfo: tthr info <文件.tthr>
func cmdInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("info 需要一个 .tthr 文件参数")
	}
	f, err := os.Open(args[0])
	if err != nil {
		return err
	}
	defer f.Close()

	ver, quantum, size, err := tcontainer.ReadHeaderInfo(f)
	if err != nil {
		return err
	}
	qstr := "否 (系统熵)"
	if quantum {
		qstr = "是 (QPanda)"
	}
	fmt.Printf("文件:     %s\n", args[0])
	fmt.Printf("格式:     TTHR v%d\n", ver)
	fmt.Printf("量子增强: %s\n", qstr)
	fmt.Printf("容器大小: %s\n", humanSize(size))
	fmt.Println("(元数据已加密; 详细信息需要私钥解包)")
	return nil
}
