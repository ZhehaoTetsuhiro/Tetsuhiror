<div align="center">

<img src="assets/logo.png" width="128" alt="Tetsuhiror logo"/>

# Tetsuhiror

**自研压缩 · 自研非对称加密 · 量子增强熵源**

命令行归档工具 — Go 实现核心, pyqpanda 提供量子随机熵源

[![Release](https://img.shields.io/github/v/release/ZhehaoTetsuhiro/Tetsuhiror?logo=github)](https://github.com/ZhehaoTetsuhiro/Tetsuhiror/releases)
[![License](https://img.shields.io/github/license/ZhehaoTetsuhiro/Tetsuhiror?color=blue)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux-4D4D4D?logo=icloud&logoColor=white)](#-安装)

</div>

---

## ✨ 特性

| | |
|---|---|
| 🔬 **TCZ 压缩** | 自研两级流水线 (LZ77 变体 + 自适应算术编码), 文本场景压缩率显著优于 `gzip -9` |
| 🔐 **T-IES 加密** | 自研 ElGamal 型混合加密, RFC 3526 组 14 (2048 位 MODP) 安全素数群 |
| ⚛️ **量子增强** | QPanda Shor 周期查找电路 (QPE) 测量比特经 THASH-256 提取白化, 参与密钥派生 |
| 🗜 **零膨胀保证** | 不可压缩数据自动回退存储模式, 最坏情况零膨胀 |
| 🛡 **完整性** | TMAC 认证 + 条目 CRC 校验 + 路径穿越防护; 元数据全部密文内, 头部零泄露 |
| 💻 **单文件** | 静态链接, 免安装依赖, Windows / Linux 开箱即用 |

## 📦 安装

从 [Releases](https://github.com/ZhehaoTetsuhiro/Tetsuhiror/releases/latest) 下载:

| 平台 | 文件 |
|------|------|
| Windows x86-64 | `tetsuhiror-vX.Y.Z-windows-amd64.exe` |
| Linux x86-64 | `tetsuhiror-vX.Y.Z-linux-amd64` (静态链接) |

或从源码构建 (需要 Go ≥ 1.25):

```bash
git clone https://github.com/ZhehaoTetsuhiro/Tetsuhiror.git
cd Tetsuhiror
go build -o tthr ./cmd/tthr
```

## 🚀 快速开始

```bash
# 把目录压缩+加密为 .tthr (自动生成密钥)
./tthr pack mydir                 # 产出 mydir.tthr + mydir.tthr.key

# 解密+解压 (未指定目录时在当前目录创建同名文件夹)
./tthr unpack mydir.tthr          # 提取到 ./mydir/

# 使用指定私钥解压
./tthr unpack mydir.tthr -k mydir.tthr.key -d /target/dir

# 查看容器信息 (不泄露内容)
./tthr info mydir.tthr
```

## 🧭 命令一览

| 命令 | 说明 |
|------|------|
| `tthr pack [选项] <目录>` | 压缩 + 加密为 `.tthr` 容器 |
| `tthr unpack [选项] <文件.tthr>` | 解密 + 解压 |
| `tthr keygen [选项]` | 生成 T-IES 密钥对 |
| `tthr info <文件.tthr>` | 查看容器头部信息 |

<details>
<summary><b>pack 选项</b></summary>

```text
-o, --output <file>   输出文件 (默认 <目录名>.tthr)
-k, --key <file>      接收方公钥 (.pub); 省略则自动生成并保存私钥到 <输出>.key
-q, --quantum         启用 QPanda 量子随机增强 (默认开, Shor 周期查找模式)
    --no-quantum      禁用量子增强, 回退系统熵
    --python <path>   pyqpanda 解释器路径 (或环境变量 TTHR_PYTHON)
    --quiet           静默模式
```

</details>

<details>
<summary><b>unpack 选项</b></summary>

```text
-d, --dest <dir>      目标目录 (默认 ./<容器名>)
-k, --key <file>      私钥文件 (默认查找 <文件>.key)
    --quiet           静默模式
```

</details>

## 🏗 架构

```text
目录 ──▶ tarchive (类 tar 归档流) ──▶ tcz (压缩) ──▶ tcontainer (加密) ──▶ .tthr
                                          │
                                          └──▶ qpad (Shor QPE 量子测量 + THASH 提取, 参与 KDF)
```

### 压缩基准

200 KB 级样本 (`cmd/tthrbench`), 数值为相对压缩率, 越小越好:

| 数据类型 | TCZ | gzip -9 |
|----------|------:|--------:|
| 英文文本 | **0.09%** | 0.39% |
| 中英混合 | **0.18%** | 0.48% |
| 结构化日志 | **2.71%** | 5.36% |
| 随机二进制 | 100.0% | 100.1% |
| 重复模式 | **0.11%** | 0.25% |

<details>
<summary><b>🔬 TCZ 压缩算法 (<code>internal/tcz</code>)</b></summary>

两级流水线:

- **TLZ (LZ77 变体)** — 32 KiB 滑动窗口, 哈希链候选 + 贪心/惰性双策略匹配, 输出 literal 与 (dist, len) token 流;
- **自适应二进制算术编码** — 12 位概率, 上下文模型覆盖 token 类型 (order-2)、literal (order-1 二叉树)、匹配长度与距离 (按长度分桶的二叉树)。

对不可压缩数据自动回退存储模式, 保证最坏情况零膨胀。

> [!NOTE]
> 2025-08 修复了算术编码器重归一化的一个罕见 bug (区间塌缩到 x1==x2 时循环被错误跳过,
> 约 150 MiB 以上输入解压损坏)。修复前打包的大容器无法解包, 需用新版本重新打包;
> 详见 `internal/tcz/arith.go` 与回归测试 `TestIntervalCollapseRegression`。

</details>

<details>
<summary><b>🔐 TAC 加密套件 (<code>internal/tac</code>)</b></summary>

| 组件 | 说明 |
|------|------|
| **THASH-256** | 自研海绵哈希。256 位状态 (4×64 位字), 12 轮 ARX+S 盒混合 (theta 常数扩散 / chi 非线性层 / GF(2⁸) S 盒 / pi 移位), 16 字节吸收率, pad10*1 填充 |
| **TStream** | hash-counter 模式密钥流 (第 i 块 = THASH(key‖nonce‖ctr)), 用于对称加密 |
| **THASH-KDF** | 迭代哈希密钥派生, 支持盐与 info 域分离 |
| **TMAC** | 前缀 MAC + 恒定时间比较, 容器完整性认证 |
| **T-IES** | ElGamal 型混合加密。2048 位 MODP 安全素数群 (RFC 3526 组 14, 参数经 `tools/getprime.py` 从 RFC 原文提取并验证), 一次性会话密钥封装; 私钥 256 位, 公钥 2048 位 |

</details>

<details>
<summary><b>⚛️ QPanda 量子增强 (<code>internal/qenhance</code>)</b></summary>

pack 时默认以 **Shor 周期查找增强模式** 通过 pyqpanda CPUQVM 产生量子随机垫 (qpad):

1. 计数寄存器 t 比特全部 Hadamard 进入均匀叠加, 工作寄存器置 |1⟩;
2. 经 pyqpanda 内置算术电路 `constModExp` 施加受控模幂 B ← B·a^A mod N;
3. 计数寄存器施加逆 QFT (`QFT().dagger()`) 完成量子相位估计;
4. 测量计数寄存器与工作寄存器: 测量坍缩给出量子随机比特, 计数寄存器读数 m ≈ s·Q/r 同时携带阶 (周期) 信息;
5. 连分数后处理恢复阶 r, 验证 a^r ≡ 1 (mod N) 并给出分解 N = p×q — 每次打包都会真实运行一遍 Shor 算法分解 N ∈ {15, 21} (CPUQVM 25 量子比特上限内的完整演示);
6. Shor 测量比特呈结构性分布 (计数寄存器聚集在 Q/r 的倍数附近), 不能直接作密钥材料, 与一批 Hadamard 测量比特一起经 **THASH-256 提取白化**为 32 字节量子垫 (min-entropy 千余位 → 256 位);
7. 量子垫加密存于容器, 明文参与文件密钥与 MAC 密钥派生。

`constModExp`/`QFT` 不可用时逐级回退: Hadamard 叠加态测量模式 (旧行为) → 系统 CSPRNG。

Python 解释器发现顺序: `--python` / `TTHR_PYTHON` > 可执行文件与工作目录附近 venv (`.venv-py310`/`.venv`/`venv`) > PATH 上的 python3/python (需可 import pyqpanda)。全部失败时回退系统 CSPRNG 并在输出中注明。

</details>

<details>
<summary><b>📦 .tthr 容器格式 (<code>internal/tcontainer</code>)</b></summary>

```text
magic "TTHR" | ver u8 | flags u8 | rsvd u16 | keyBlobLen u32 | payloadLen u64 | macLen u32
keyBlob: ephPub(256) | salt(16) | qpadEnc(32) | nonce(16)
payload: TStream(key, nonce) XOR (metaLen u32 | metaJSON | tczStream)
尾部:    TMAC(32)  覆盖 header + keyBlob + payload
```

密钥派生 (域分离):

```text
shared  = DH(ephPub, priv)
padKey  = KDF(shared, "tthr/qpad/v1", salt)        # 解密量子垫
fileKey = KDF(shared, "tthr/file/v1", salt‖qpad)   # qpad 明文参与!
macKey  = KDF(shared, "tthr/mac/v1",  salt‖qpad)
```

元数据 (文件列表、目录名等) 全部在密文内, 容器头部不泄露任何信息。

</details>

## ⚛️ QPanda 环境搭建

pyqpanda 需要 Python 3.8–3.10 (不支持 3.13):

```bash
conda create -y -p .venv-py310 python=3.10
.venv-py310/bin/pip install pyqpanda

# 项目根目录 (或 tthr 可执行文件所在目录) 的 .venv-py310 会被自动发现;
# 也可显式指定:
TTHR_PYTHON=$PWD/.venv-py310/bin/python ./tthr pack mydir
```

> 不想折腾量子环境? 加 `--no-quantum` 即可, 回退系统熵, 功能不受影响。

## 🔧 开发

```bash
go test ./internal/...      # 全部单元测试
go run ./cmd/tthrbench      # 压缩基准 (TCZ vs gzip)
```

## ⚠️ 安全性说明

本项目为自研算法的实验性实现: THASH-256、TStream、TMAC、T-IES 均为本项目原创构造, **未经过第三方密码分析与审计**, 不应用于生产环境的高价值数据保护。

T-IES 使用的群参数为公开标准 (RFC 3526 组 14), 构造本身自研。压缩解压带有路径穿越防护与条目 CRC 校验。

## 📄 许可证

本项目以 [AGPL-3.0](LICENSE) 许可证发布, 变更记录见 [CHANGELOG.md](CHANGELOG.md)。

<div align="center">

**Tetsuhiror** © ZhehaoTetsuhiro — 由 [Go](https://go.dev) 与 [QPanda](https://github.com/OriginQ/QPanda-2) 构建

</div>
