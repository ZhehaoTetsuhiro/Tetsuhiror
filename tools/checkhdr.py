#!/usr/bin/env python3
"""解析 tarchive 流头部, 验证读写两端字段布局一致性。"""
import struct, sys

data = bytes.fromhex(
    "04 00 64 6f 63 73 01 ed 01 00 00 00 00 00 00 00 00 00 00 e6 fe 90 6a "
    "00 00 00 00 00 00 00 00 0d 00 64 6f 63 73 2f 6c 69 6e 6b 2e 74 78 74 02 ff"
)

off = 0
name_len = struct.unpack_from("<H", data, off)[0]
off += 2
name = data[off:off+name_len]
off += name_len
type_ = data[off]
off += 1
mode = struct.unpack_from("<I", data, off)[0]
off += 4
size = struct.unpack_from("<Q", data, off)[0]
off += 8
mod = struct.unpack_from("<Q", data, off)[0]
off += 8
print("entry1:", name, "type", type_, "mode", oct(mode), "size", size, "mtime", mod)
print("content starts at", off, "(22)")
print("remaining:", data[off:off+10].hex())
