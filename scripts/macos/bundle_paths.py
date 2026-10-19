"""Inspect Mach-O search paths without depending on the builder's filesystem."""

import re
import subprocess
from pathlib import Path

MACHO_MAGICS = {
    bytes.fromhex(value) for value in
    ('feedface', 'cefaedfe', 'feedfacf', 'cffaedfe',
     'cafebabe', 'bebafeca', 'cafebabf', 'bfbafeca')
}


def binaries(bundle):
    for path in sorted(bundle.rglob('*')):
        if not path.is_file() or path.is_symlink():
            continue
        with path.open('rb') as stream:
            if stream.read(4) in MACHO_MAGICS:
                yield path


def parse_rpaths(load_commands):
    paths = []
    for command in re.split(r'^Load command \d+\s*$', load_commands, flags=re.M):
        if re.search(r'^\s*cmd LC_RPATH\s*$', command, re.M):
            value = re.search(r'^\s*path (.+) \(offset \d+\)\s*$', command, re.M)
            if value is None:
                raise RuntimeError('malformed LC_RPATH')
            paths.append(value.group(1))
    return paths


def rpaths(path, architecture='arm64'):
    return parse_rpaths(subprocess.check_output(
        ['otool', '-arch', architecture, '-l', str(path)], text=True))


def resolve_rpath(value, binary, executable, bundle):
    for prefix, directory in (
            ('@loader_path', binary.parent), ('@executable_path', executable.parent)):
        if value == prefix or value.startswith(prefix + '/'):
            target = (directory / value[len(prefix):].lstrip('/')).resolve()
            if target.is_relative_to(bundle.resolve()) and target.is_dir():
                return target
            return None
    # System libraries are backed by the OS, never by Homebrew or build paths.
    if value.startswith(('/System/Library/', '/usr/lib/')):
        return Path(value)
    return None
