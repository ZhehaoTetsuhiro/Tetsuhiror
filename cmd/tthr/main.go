// tthr — Tetsuhiro 高压缩比 + 量子增强加密归档工具。
//
// 用法:
//
//	tthr pack   [选项] <目录>              压缩+加密为 .tthr
//	tthr unpack [选项] <文件.tthr>          解密+解压
//	tthr keygen [选项]                      生成密钥对
//	tthr info   <文件.tthr>                 查看容器信息
package main

import (
	"fmt"
	"os"
)

const version = "1.0.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "tthr: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage(os.Stderr)
		return fmt.Errorf("缺少子命令")
	}
	switch args[0] {
	case "pack":
		return cmdPack(args[1:])
	case "unpack":
		return cmdUnpack(args[1:])
	case "keygen":
		return cmdKeygen(args[1:])
	case "info":
		return cmdInfo(args[1:])
	case "help", "-h", "--help":
		usage(os.Stdout)
		return nil
	case "version", "-v", "--version":
		printVersion()
		return nil
	default:
		usage(os.Stderr)
		return fmt.Errorf("未知子命令: %s", args[0])
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, usageText)
}

func printVersion() {
	fmt.Printf("tthr %s (TCZ-1 压缩 / TAC 加密 / QPanda 量子增强)\n", version)
}
const usageText = `tthr — Tetsuhiro 归档工具 (自研压缩 TCZ + 自研加密 TAC + QPanda 量子增强)

用法:
  tthr pack   [选项] <目录>
      把目录压缩 (TCZ) 并加密 (T-IES + 量子垫) 为 .tthr 文件。
      未指定输出文件时自动生成密钥并保存到 <输出名>.key。

  tthr unpack [选项] <文件.tthr>
      解密并解压 .tthr 到指定目录; 未指定时在当前目录创建同名文件夹。

  tthr keygen [选项]
      生成 T-IES 密钥对, 输出私钥文件与公钥文件。

  tthr info   <文件.tthr>
      显示容器头部信息。

pack 选项:
  -o, --output <file>    输出文件 (默认 <目录名>.tthr)
  -k, --key <file>       接收方公钥; 省略则自动生成并保存私钥
  -q, --quantum          启用 QPanda 量子随机增强 (默认开, Shor 周期查找模式)
      --no-quantum       禁用量子增强, 回退系统熵
      --python <path>    pyqpanda 解释器路径 (默认探测; TTHR_PYTHON 亦可)
      --quiet            静默模式, 只输出错误

unpack 选项:
  -d, --dest <dir>       目标目录 (默认 ./<容器名>)
  -k, --key <file>       私钥文件 (TTHR PRIVATE KEY 文本)
      --quiet            静默模式

keygen 选项:
  -o, --output <prefix>  输出前缀 (默认 tthr-key); 生成 <prefix>.key 与 <prefix>.pub
      --stdout           私钥打印到 stdout (公钥到 stderr)

环境变量:
  TTHR_PYTHON            pyqpanda 解释器路径
`
