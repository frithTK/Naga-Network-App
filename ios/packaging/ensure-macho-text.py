#!/usr/bin/env python3
"""Read-only Mach-O __text check. Never mutate a linked or signed binary."""
import struct
import sys
from pathlib import Path


def ensure_text(path):
    data = path.read_bytes()
    if len(data) < 32 or struct.unpack_from("<I", data)[0] != 0xFEEDFACF:
        raise ValueError("expected a 64-bit little-endian Mach-O")
    ncmds, sizeofcmds = struct.unpack_from("<II", data, 16)
    end = 32 + sizeofcmds
    if end > len(data) or ncmds > sizeofcmds // 8:
        raise ValueError("invalid Mach-O command bounds")
    off = 32
    has_text = False
    for _ in range(ncmds):
        if off + 8 > end:
            raise ValueError("truncated Mach-O command")
        cmd, size = struct.unpack_from("<II", data, off)
        if size < 8 or size % 8 or off + size > end:
            raise ValueError("invalid Mach-O command size")
        if cmd == 0x19:
            if size < 72:
                raise ValueError("truncated Mach-O segment")
            sections = struct.unpack_from("<I", data, off + 64)[0]
            if 72 + 80 * sections > size:
                raise ValueError("invalid Mach-O section bounds")
            for index in range(sections):
                section = off + 72 + index * 80
                name = data[section:section+16].rstrip(b"\0")
                segment = data[section+16:section+32].rstrip(b"\0")
                if name == b"__text" and segment == b"__TEXT":
                    length = struct.unpack_from("<Q", data, section+40)[0]
                    offset = struct.unpack_from("<I", data, section+48)[0]
                    if not length or offset < end or offset + length > len(data):
                        raise ValueError("invalid __text file range")
                    has_text = True
        off += size
    if off != end or not has_text:
        raise ValueError("linker must produce a valid __TEXT,__text section before signing")
    return False


if __name__ == "__main__":
    try:
        if len(sys.argv) < 2:
            raise ValueError("usage: ensure-macho-text.py <mach-o> ...")
        for arg in sys.argv[1:]:
            ensure_text(Path(arg))
    except (ValueError, OSError, struct.error) as error:
        print(error, file=sys.stderr)
        sys.exit(1)
