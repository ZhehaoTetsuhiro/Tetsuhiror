#!/usr/bin/env python3
"""从纯文本 RFC 3526 提取 group 14 (2048-bit) 素数并验证。"""
import re, random, sys

text = open('rfc3526.txt', encoding='utf-8', errors='replace').read()

i3 = text.find('3.  2048-bit MODP Group')
i4 = text.find('4.  3072-bit MODP Group')
if i3 < 0 or i4 < 0:
    print('section not found')
    sys.exit(1)
seg = text[i3:i4]
a = seg.find('Its hexadecimal value is:')
b = seg.find('The generator is:')
prime_block = seg[a:b]

rows = []
for line in prime_block.splitlines():
    s = line.strip()
    if not s:
        continue
    if re.fullmatch(r'(?:[0-9A-F]{8} ){1,6}[0-9A-F]{8}', s):
        rows.append(s.replace(' ', ''))
hexdigits = ''.join(rows)
print('len', len(hexdigits), 'bits', int(hexdigits, 16).bit_length())

p = int(hexdigits, 16)
q = (p - 1) // 2

def fermat(n, k=16):
    if n < 4:
        return n == 2 or n == 3
    for _ in range(k):
        a = random.randrange(2, n - 1)
        if pow(a, n - 1, n) != 1:
            return False
    return True

ok = fermat(p) and fermat(q)
print('safe prime (fermat):', ok)
print('g=2 order == q:', pow(2, q, p) == 1)
print('starts:', hexdigits[:32])
print('ends:  ', hexdigits[-32:])

if not ok or len(hexdigits) != 512:
    sys.exit(1)
open('p14.hex', 'w').write(hexdigits)
print('SAVED')
