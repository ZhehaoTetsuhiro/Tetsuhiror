#!/usr/bin/env python3
"""把 p14.hex 格式化为 Go 字符串字面量片段。"""
hexdigits = open('p14.hex').read().strip()
lines = []
for i in range(0, len(hexdigits), 64):
    lines.append('\t\t"' + hexdigits[i:i+64] + '" +')
golit = '\n'.join(lines).rstrip(' +')
open('p14_go.txt', 'w').write(golit)
print(len(golit), 'chars written')
