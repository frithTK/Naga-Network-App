#!/usr/bin/env python3
"""Remove builder-specific library paths before the final inside-out signing."""

import argparse
import plistlib
import subprocess

from bundle_paths import binaries, resolve_rpath, rpaths


def main():
    from pathlib import Path
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('bundle', type=Path)
    args = parser.parse_args()
    bundle = args.bundle.resolve()
    info = plistlib.loads((bundle / 'Contents/Info.plist').read_bytes())
    executable = bundle / 'Contents/MacOS' / info['CFBundleExecutable']
    for binary in binaries(bundle):
        architectures = subprocess.check_output(['lipo', '-archs', str(binary)], text=True).split()
        paths = {arch: rpaths(binary, arch) for arch in architectures}
        invalid = {value for values in paths.values() for value in values
                   if resolve_rpath(value, binary, executable, bundle) is None}
        for value in sorted(invalid):
            if any(value not in values for values in paths.values()):
                raise RuntimeError(f'architecture-specific LC_RPATH requires explicit handling: {binary}')
            subprocess.run(['install_name_tool', '-delete_rpath', value, str(binary)], check=True)
            print(f'Removed nonportable LC_RPATH {value} from {binary.relative_to(bundle)}')
        # Framework CFBundleExecutable files should have standard executable
        # permissions as well, including engines unpacked from SDK archives.
        binary.chmod(0o755)
    # Flutter initially signs these before packaging. Editing load commands
    # invalidates those signatures, so sign each framework before the outer app.
    for framework in sorted((bundle / 'Contents/Frameworks').glob('*.framework')):
        subprocess.run(['codesign', '--force', '--sign', '-', str(framework)], check=True)


if __name__ == '__main__':
    main()
