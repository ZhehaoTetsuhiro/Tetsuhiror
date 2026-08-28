# 更新日志 (Changelog)

本项目的所有重要变更都记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/),
版本号遵循 [语义化版本 2.0.0](https://semver.org/lang/zh-CN/)。

## [Unreleased]

## [0.1.0] - 2026-08-28

首个发布版本。

### 新增

- **TCZ 压缩算法** (`internal/tcz`): 自研两级流水线压缩
  - TLZ (LZ77 变体): 32 KiB 滑动窗口, 哈希链候选 + 贪心/惰性双策略匹配
  - 算术编码 (`internal/tcz/arith.go`)
- **Tarchive 归档流** (`internal/tarchive`): 类 tar 目录归档格式
- **TAC / T-IES 非对称加密** (`internal/tac`): 自研加密, 含 THASH、KDF 与流加密
- **Tcontainer 容器格式** (`internal/tcontainer`): `.tthr` 容器与密钥文件读写
- **QPanda 量子随机增强** (`internal/qenhance`): 通过 pyqpanda 调用 Shor 周期查找
  (QPE) 电路的测量结果参与 KDF, 增强加密熵源; 可用 `--no-quantum` 回退系统熵
- 命令行工具 `tthr`:
  - `tthr pack` — 目录压缩 + 加密为 `.tthr` (自动生成密钥或使用指定公钥)
  - `tthr unpack` — 解密 + 解压 `.tthr`
  - `tthr keygen` — 生成 T-IES 密钥对
  - `tthr info` — 查看容器头部信息
- 基准测试工具 `tthrbench` (`cmd/tthrbench`)
- 各核心模块的 Go 单元测试
- `tools/` 辅助脚本 (头部检查、素数生成/修补、QVM 探测等)

### 构建

- 提供 Windows x86-64、Linux x86-64 静态二进制
- 提供 Linux AppImage 打包

### 许可证

- 以 AGPL-3.0 许可证发布
