#!/usr/bin/env python3
"""tet 量子增强脚本 (QPanda) — Shor 周期查找增强模式。

用法: qenhance.py <bytes-needed> [mode] [batch-qubits]

mode:
  shor (默认) — Shor 周期查找 (QPE) 量子熵:
      1. 计数寄存器 (t 比特) 全部 Hadamard 进入均匀叠加;
      2. 工作寄存器置 |1>, 经 constModExp 施加受控模幂
         B <- B * a^A mod N (pyqpanda 内置算术电路);
      3. 计数寄存器施加逆 QFT (QFT().dagger());
      4. 测量计数寄存器与工作寄存器: 测量坍缩给出量子随机比特,
         计数寄存器读数 m ~= s*Q/r 同时携带阶 (周期) 信息;
      5. 连分数后处理恢复 r, 验证 a^r == 1 (mod N) 并分解 N = p*q
         (N in {15, 21}, 25 量子比特上限内可完整演示);
      6. 另收集一批 Hadamard 测量比特。
      全部原始测量比特以 hex 返回, 由 Go 侧 THASH-256 提取白化为量子垫
      (Shor 测量在计数寄存器上呈 Q/r 倍数聚集的结构性分布,
       不能直接作为密钥材料)。

  h — 仅 Hadamard 叠加态测量 (旧行为, shor 不可用时的回退)。

输出 JSON 到 stdout:
  shor: {"ok":true,"mode":"shor","hex":"...","hhex":"...","circuits":[...],
         "measurements":n,"rawbits":n,"minent":x.x}
  h:    {"ok":true,"mode":"h","hex":"...","qubits":batch,"runs":m}

任何异常输出 {"ok": false, "error": "..."}, Go 侧回退系统熵。
"""

import json
import sys
import time
from fractions import Fraction
from math import gcd, log2
from random import SystemRandom

# Shor 电路池 (受 CPUQVM 25 量子比特限制):
#   N=15: n=4, t=4  -> 20 比特, r=4 组 (a in {2,7,13}) 与 r=2 组 (a in {4,11});
#   N=21: n=5, t=5  -> 25 比特, a=2 (r=6)。25 比特电路每发约 1s
#   (536MB 状态向量, 内存带宽受限), 只测少量发数用于演示分解。
SHOR_POOL_R4 = (2, 7, 13)   # N=15, 阶 r=4
SHOR_POOL_R2 = (4, 11)      # N=15, 阶 r=2 (a=14 因 a^(r/2)=-1 无法分解, 已排除)
TIME_BUDGET_S = 60.0        # 全部 Shor 电路的软时限
SLOW_SKIP_S = 15.0          # N=15 电路超过此时长视为慢机, 跳过 25 比特的 N=21 电路


def pack_bits(bits):
    out = bytearray()
    for i in range(0, len(bits), 8):
        out.append(int(bits[i:i + 8].ljust(8, "0"), 2))
    return bytes(out).hex()


def hadamard_bits(qvm, nbits, batch):
    """Hadamard 叠加态测量随机比特 (与旧版行为一致)。"""
    from pyqpanda import QProg, H, measure_all

    bits = ""
    runs = 0
    while len(bits) < nbits:
        q = qvm.qAlloc_many(batch)
        c = qvm.cAlloc_many(batch)
        prog = QProg()
        for qb in q:
            prog << H(qb)
        prog << measure_all(q, c)
        counts = qvm.run_with_configuration(prog, c, 1)
        if not counts:
            raise RuntimeError("空测量结果")
        bits += next(iter(counts.keys()))
        runs += 1
        qvm.qFree_all(q)
        qvm.cFree_all(c)
    return bits[:nbits], runs


def decode_outcome(key, t, n):
    """测量键 -> (计数寄存器值 m, 工作寄存器值)。

    run_with_configuration 的键高位在左、列表末位 cbit 在最左,
    列表为 cA+cB, 故反转后前 t 位是 cA (LSB 在前)。
    """
    s = key[::-1]
    m = int(s[:t][::-1], 2)
    w = int(s[t:t + n][::-1], 2)
    return m, w


def find_period(m, Q, a, N):
    """连分数从 m/Q 恢复阶 r (a^r == 1 mod N), 失败返回 None。"""
    if m == 0:
        return None
    f = Fraction(m, Q)
    multiple = None
    for k in range(2, 4 * N):
        d = f.limit_denominator(k).denominator
        if d > 1 and pow(a, d, N) == 1:
            multiple = d
            break
    if multiple is None:
        return None
    # multiple 是 r 的倍数, 收缩到最小周期
    for p in range(2, multiple + 1):
        if multiple % p == 0 and pow(a, p, N) == 1:
            return p
    return None


def factors_from_r(a, r, N):
    """由阶 r 经 gcd(a^(r/2)±1, N) 分解 N, 失败返回 None。"""
    if not r or r % 2 == 1:
        return None
    x = pow(a, r // 2, N)
    if x in (1, N - 1):
        return None
    for cand in (x - 1, x + 1):
        g = gcd(cand, N)
        if 1 < g < N:
            return sorted((g, N // g))
    return None


def shor_circuit(qvm, N, a, t, shots):
    """运行一次 Shor 周期查找电路, 返回 (原始测量比特串, 电路元信息)。"""
    from pyqpanda import QProg, X, H, measure_all, constModExp, QFT

    n = N.bit_length()
    total = t + 4 * n
    if total > 25:
        raise RuntimeError("需要 %d 量子比特, 超过 CPUQVM 上限 25" % total)

    q = None
    c = None
    try:
        q = qvm.qAlloc_many(total)
        c = qvm.cAlloc_many(total)
        A = list(q[0:t])
        B = list(q[t:t + n])
        anc = [list(q[t + n + i * n: t + n + (i + 1) * n]) for i in range(3)]
        cA = list(c[0:t])
        cB = list(c[t:t + n])

        prog = QProg()
        for qb in A:
            prog << H(qb)                      # 计数寄存器均匀叠加
        prog << X(B[0])                        # 工作寄存器置 |1>
        prog << constModExp(A, B, a, N, anc[0], anc[1], anc[2])
        prog << QFT(A).dagger()                # 逆 QFT 完成相位估计
        clist = cA + cB
        prog << measure_all(A + B, clist)
        counts = qvm.run_with_configuration(prog, clist, shots)
    finally:
        if q is not None:
            qvm.qFree_all(q)
        if c is not None:
            qvm.cFree_all(c)
    if not counts:
        raise RuntimeError("空测量结果")

    Q = 1 << t
    raw = ""
    votes = {}
    for key, cnt in counts.items():
        raw += key * cnt
        m, _ = decode_outcome(key, t, n)
        r = find_period(m, Q, a, N)
        if r is not None:
            votes[r] = votes.get(r, 0) + cnt
    r = max(votes, key=votes.get) if votes else None
    return raw, {
        "N": N, "a": a, "t": t, "qubits": total, "shots": shots,
        "r": r, "factors": factors_from_r(a, r, N),
    }


def run_shor(nbytes, batch, rng):
    """Shor 增强模式主流程: 返回 (shor原始位, hadamard原始位, 电路列表, 统计)。"""
    from pyqpanda import CPUQVM

    specs = [{"N": 15, "a": a, "t": 4, "shots": 128} for a in rng.sample(SHOR_POOL_R4, 2)]
    specs.append({"N": 15, "a": rng.choice(SHOR_POOL_R2), "t": 4, "shots": 96})
    specs.append({"N": 21, "a": 2, "t": 5, "shots": 24})

    qvm = CPUQVM()
    qvm.init_qvm()
    circuits = []
    raw = ""
    measurements = 0
    minent = 0.0
    try:
        t0 = time.monotonic()
        for spec in specs:
            elapsed = time.monotonic() - t0
            if elapsed > TIME_BUDGET_S:
                break  # 软时限: 放弃剩余电路 (已收集的熵仍然有效)
            # 25 比特电路固定开销大 (~30s, 与发数无关), 慢机上直接跳过
            if spec["N"] == 21 and elapsed > SLOW_SKIP_S:
                break
            try:
                bits, meta = shor_circuit(qvm, spec["N"], spec["a"], spec["t"], spec["shots"])
            except Exception:
                continue  # 单个电路失败不影响其余
            raw += bits
            measurements += spec["shots"]
            r_eff = meta["r"] or 2
            minent += spec["shots"] * 2 * log2(r_eff)  # A 与 B 各贡献 log2(r)
            circuits.append(meta)
        if not circuits or len(raw) < 64 * 8:
            raise RuntimeError("Shor 电路全部失败")
        hbits, hruns = hadamard_bits(qvm, nbytes * 8, batch)
        minent += nbytes * 8
    finally:
        qvm.finalize()
    return raw, hbits, circuits, measurements, minent, hruns


def main():
    if len(sys.argv) < 2:
        print(json.dumps({"ok": False, "error": "缺少参数"}))
        return 1
    try:
        nbytes = int(sys.argv[1])
    except ValueError:
        print(json.dumps({"ok": False, "error": "参数错误"}))
        return 1
    if nbytes <= 0 or nbytes > 1024:
        print(json.dumps({"ok": False, "error": "字节数超界"}))
        return 1
    mode = "shor"
    batch = 16  # Hadamard 批次宽度: 对均匀随机无影响, 小寄存器远快于 25 比特
    for extra in sys.argv[2:]:
        if extra in ("shor", "h"):
            mode = extra
        else:
            try:
                batch = max(1, min(25, int(extra)))
            except ValueError:
                pass
    rng = SystemRandom()

    if mode == "shor":
        try:
            raw, hbits, circuits, measurements, minent, hruns = run_shor(nbytes, batch, rng)
            print(json.dumps({
                "ok": True,
                "mode": "shor",
                "hex": pack_bits(raw),
                "hhex": pack_bits(hbits),
                "circuits": circuits,
                "measurements": measurements,
                "rawbits": len(raw) + len(hbits),
                "minent": round(minent, 1),
                "runs": hruns,
            }))
            return 0
        except Exception:
            mode = "h"  # constModExp/QFT 不可用等, 回退 Hadamard 模式

    try:
        from pyqpanda import CPUQVM
        qvm = CPUQVM()
        qvm.init_qvm()
        try:
            bits, runs = hadamard_bits(qvm, nbytes * 8, batch)
        finally:
            qvm.finalize()
    except Exception as exc:  # noqa: BLE001
        print(json.dumps({"ok": False, "error": str(exc)}))
        return 1

    print(json.dumps({
        "ok": True,
        "mode": "h",
        "hex": pack_bits(bits),
        "qubits": batch,
        "runs": runs,
        "source": "qpanda",
    }))
    return 0


if __name__ == "__main__":
    sys.exit(main())
