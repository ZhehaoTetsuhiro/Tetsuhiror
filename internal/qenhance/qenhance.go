// Package qenhance 提供 QPanda 量子随机数桥接 (Shor 周期查找增强)。
//
// tthr 的量子增强层: pack 时通过 pyqpanda 量子虚拟机运行 Shor
// 周期查找电路 (QPE + 受控模幂 + 逆 QFT), 计数寄存器与工作寄存器的
// 测量坍缩给出量子随机比特, 经 THASH-256 提取白化为量子垫 (qpad)
// 混入密钥派生, 使会话密钥获得量子熵源。
//
// 测量比特带有结构性偏置 (计数寄存器聚集在 Q/r 的倍数附近), 因此
// 不直接作为密钥材料, 而是作为 THASH 海绵的吸收输入挤压出均匀的
// 32 字节量子垫 —— 标准的 QRNG 后处理 (提取/白化) 做法。
//
// constModExp / QFT 不可用时逐级回退: Hadamard 模式 (旧行为) →
// 系统 CSPRNG (source 标记为 system)。
//
// Python 解释器发现顺序:
//  1. 显式参数或 TTHR_PYTHON 环境变量;
//  2. 可执行文件与工作目录附近 venv (bin/python) 中的 pyqpanda;
//  3. PATH 上的 python3 与 python (需能 import pyqpanda)。
package qenhance

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"tetsuhiro/tthr/internal/tac"
)

//go:embed qenhance.py
var scriptSource string

// QPadSize 量子垫长度 (字节)。
const QPadSize = 32

// Result 是一次量子熵生成的结果。
type Result struct {
	QPad   []byte
	Source string // qpanda | system
	Mode   string // shor | h | "" (system 回退)
	Detail string
}

// shorCircuit 对应 Python 侧一个 Shor 周期查找电路的执行结果。
type shorCircuit struct {
	N       int   `json:"N"`
	A       int   `json:"a"`
	T       int   `json:"t"`
	Qubits  int   `json:"qubits"`
	Shots   int   `json:"shots"`
	R       *int  `json:"r"`
	Factors []int `json:"factors"`
}

// scriptOutput 是 qenhance.py 的 JSON 输出契约。
type scriptOutput struct {
	OK           bool          `json:"ok"`
	Mode         string        `json:"mode"`
	Hex          string        `json:"hex"`
	HHex         string        `json:"hhex"`
	Qubits       int           `json:"qubits"`
	Runs         int           `json:"runs"`
	Circuits     []shorCircuit `json:"circuits"`
	Measurements int           `json:"measurements"`
	RawBits      int           `json:"rawbits"`
	MinEnt       float64       `json:"minent"`
	Error        string        `json:"error"`
}

// Generate 生成 QPadSize 字节的量子随机垫。pythonPath 为空时自动探测。
func Generate(pythonPath string) (*Result, error) {
	if pythonPath == "" {
		pythonPath = DetectPython()
	}
	if pythonPath == "" {
		return systemFallback("未找到可用的 Python/pyqpanda")
	}
	pad, mode, detail, err := runQPanda(pythonPath, "shor")
	if err != nil {
		return systemFallback(fmt.Sprintf("qpanda 不可用: %v", err))
	}
	return &Result{QPad: pad, Source: "qpanda", Mode: mode, Detail: detail}, nil
}

// runQPanda 执行 qenhance.py 并解析输出, 返回 (量子垫, 模式, 说明)。
func runQPanda(pythonPath, mode string) ([]byte, string, string, error) {
	tmp, err := os.CreateTemp("", "tthr-qenhance-*.py")
	if err != nil {
		return nil, "", "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(scriptSource); err != nil {
		tmp.Close()
		return nil, "", "", err
	}
	tmp.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, pythonPath, tmp.Name(), fmt.Sprint(QPadSize), mode)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, "", "", fmt.Errorf("%v: %s", err, stderr.String())
	}

	var out scriptOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, "", "", fmt.Errorf("解析输出失败: %w", err)
	}
	if !out.OK {
		return nil, "", "", errors.New(out.Error)
	}

	switch out.Mode {
	case "shor":
		raw, err := hex.DecodeString(out.Hex)
		if err != nil {
			return nil, "", "", err
		}
		hraw, err := hex.DecodeString(out.HHex)
		if err != nil {
			return nil, "", "", err
		}
		// Shor 测量原始比特至少 64 字节才够提取 256 位垫
		if len(raw)+len(hraw) < 64 {
			return nil, "", "", fmt.Errorf("原始测量比特不足: %d 字节", len(raw)+len(hraw))
		}
		detail := buildShorDetail(&out)
		return ExtractQPad(raw, hraw), "shor", detail, nil
	default: // "h": Hadamard 回退模式, hex 即量子垫
		pad, err := hex.DecodeString(out.Hex)
		if err != nil {
			return nil, "", "", err
		}
		if len(pad) != QPadSize {
			return nil, "", "", fmt.Errorf("长度错误: %d", len(pad))
		}
		detail := fmt.Sprintf("qpanda Hadamard (%d 量子比特 x %d 次测量)", out.Qubits, out.Runs)
		return pad, "h", detail, nil
	}
}

// ExtractQPad 以 THASH-256 从 Shor/Hadamard 原始测量比特提取白化量子垫。
// 域分离标签防止与其他 THASH 用途混用。
func ExtractQPad(raws ...[]byte) []byte {
	in := make([][]byte, 0, len(raws)+1)
	in = append(in, []byte("tthr/shor/v1"))
	in = append(in, raws...)
	sum := tac.THASH256(in...)
	return sum[:]
}

// buildShorDetail 汇总 Shor 电路执行情况为人类可读说明。
func buildShorDetail(out *scriptOutput) string {
	var parts []string
	for _, c := range out.Circuits {
		switch {
		case c.R != nil && len(c.Factors) == 2:
			parts = append(parts, fmt.Sprintf("%d=%d×%d (a=%d, r=%d)",
				c.N, c.Factors[0], c.Factors[1], c.A, *c.R))
		case c.R != nil:
			parts = append(parts, fmt.Sprintf("N=%d a=%d r=%d (未分解)",
				c.N, c.A, *c.R))
		default:
			parts = append(parts, fmt.Sprintf("N=%d a=%d (r 未恢复)", c.N, c.A))
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("Shor 模式异常: 无电路结果 (%d 次测量) → THASH-256 提取", out.Measurements)
	}
	summary := fmt.Sprintf("Shor 周期查找: %s | %d 电路 / %d 次测量 / %d 原始位 (min-entropy≥%.0f) → THASH-256 提取",
		strings.Join(parts, "; "), len(out.Circuits), out.Measurements, out.RawBits, out.MinEnt)
	return summary
}

// DetectPython 探测可 import pyqpanda 的解释器, 失败返回空串。
func DetectPython() string {
	var candidates []string
	if p := os.Getenv("TTHR_PYTHON"); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates, localVenvCandidates()...)
	if runtime.GOOS == "windows" {
		candidates = append(candidates, "python", "py")
	} else {
		candidates = append(candidates, "python3", "python")
	}
	for _, c := range candidates {
		if probePython(c) {
			return c
		}
	}
	return ""
}

// localVenvCandidates 生成可执行文件与工作目录附近 venv 的 python 路径。
// 覆盖 ./tthr 在项目根运行、以及从子目录运行两种场景。
func localVenvCandidates() []string {
	var roots []string
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		roots = append(roots, d, filepath.Join(d, ".."), filepath.Join(d, "..", ".."))
	}
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, wd, filepath.Join(wd, ".."), filepath.Join(wd, "..", ".."))
	}

	names := []string{".venv-py310", ".venv", "venv"}
	var out []string
	seen := map[string]bool{}
	for _, root := range roots {
		for _, name := range names {
			var p string
			if runtime.GOOS == "windows" {
				p = filepath.Join(root, name, "Scripts", "python.exe")
			} else {
				p = filepath.Join(root, name, "bin", "python")
			}
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

func probePython(python string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-c", "import pyqpanda")
	return cmd.Run() == nil
}

func systemFallback(reason string) (*Result, error) {
	pad := make([]byte, QPadSize)
	if _, err := rand.Read(pad); err != nil {
		return nil, err
	}
	return &Result{QPad: pad, Source: "system", Mode: "", Detail: "系统熵回退: " + reason}, nil
}
