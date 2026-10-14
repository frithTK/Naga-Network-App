#!/usr/bin/env python3
"""Fail the APK release gate unless every native library supports 16 KiB pages."""
import struct
import sys
import zipfile


def verify_elf(data):
    if len(data) < 64 or data[:6] != b'\x7fELF\x02\x01':
        raise ValueError('expected little-endian ELF64')
    phoff = struct.unpack_from('<Q', data, 32)[0]
    entsize, count = struct.unpack_from('<HH', data, 54)
    if entsize < 56 or phoff + entsize * count > len(data):
        raise ValueError('invalid ELF program headers')
    loads = 0
    for index in range(count):
        offset = phoff + index * entsize
        segment = struct.unpack_from('<I', data, offset)[0]
        if segment != 1:
            continue
        loads += 1
        file_offset, address = struct.unpack_from('<QQ', data, offset + 8)
        alignment = struct.unpack_from('<Q', data, offset + 48)[0]
        if alignment < 16384 or alignment & (alignment - 1) or (file_offset - address) % 16384:
            raise ValueError('PT_LOAD does not support 16 KiB pages')
    if not loads:
        raise ValueError('ELF contains no loadable segment')


def verify_apk(path):
    with zipfile.ZipFile(path) as archive, open(path, 'rb') as file:
        libraries = [name for name in archive.namelist() if name.startswith('lib/') and name.endswith('.so')]
        if not libraries:
            raise ValueError('APK contains no native libraries')
        for name in libraries:
            try:
                entry = archive.getinfo(name)
                if entry.compress_type == zipfile.ZIP_STORED:
                    file.seek(entry.header_offset)
                    header = file.read(30)
                    if len(header) != 30 or header[:4] != b'PK\x03\x04':
                        raise ValueError('invalid ZIP local header')
                    name_length, extra_length = struct.unpack_from('<HH', header, 26)
                    if (entry.header_offset + 30 + name_length + extra_length) % 16384:
                        raise ValueError('uncompressed library is not ZIP aligned to 16 KiB')
                verify_elf(archive.read(name))
            except ValueError as error:
                raise ValueError(f'{name}: {error}') from error
        print(f'16 KiB ELF gate: {len(libraries)} libraries passed')


if __name__ == '__main__':
    try:
        verify_apk(sys.argv[1])
    except (ValueError, OSError, zipfile.BadZipFile, struct.error, IndexError) as error:
        print(f'Native library validation failed: {error}', file=sys.stderr)
        sys.exit(1)
