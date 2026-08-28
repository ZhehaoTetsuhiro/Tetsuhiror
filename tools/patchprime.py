#!/usr/bin/env python3
"""把 p14.hex 替换进 internal/tac/asym.go 的 tIESP 常量。"""
hexdigits = open('p14.hex').read().strip()
lines = []
for i in range(0, len(hexdigits), 64):
    chunk = hexdigits[i:i+64]
    if i + 64 < len(hexdigits):
        lines.append('\t\t"' + chunk + '" +')
    else:
        lines.append('\t\t"' + chunk + '", 16)')
newprime = chr(10).join(lines)

src = open('internal/tac/asym.go').read()
start = src.find('tIESP, _ = new(big.Int).SetString(')
end = src.find('tIESG')
assert start > 0 and end > start, (start, end)
src = src[:start] + 'tIESP, _ = new(big.Int).SetString(' + chr(10) + newprime + chr(10) + chr(9) + src[end:]
open('internal/tac/asym.go', 'w').write(src)
print('patched asym.go prime')
