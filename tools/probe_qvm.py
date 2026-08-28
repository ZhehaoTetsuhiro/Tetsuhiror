#!/usr/bin/env python3
"""探测 CPUQVM 的能力与测量 API 形态。"""
from pyqpanda import CPUQVM, QProg, H, measure_all

qvm = CPUQVM()
qvm.init_qvm()

for n in (8, 16, 24, 25, 32, 64, 100, 128, 256):
    try:
        q = qvm.qAlloc_many(n)
        c = qvm.cAlloc_many(n)
        qvm.qFree_all(q)
        qvm.cFree_all(c)
        print(n, 'OK')
    except Exception as exc:
        print(n, 'FAIL:', exc)
        break
qvm.finalize()
