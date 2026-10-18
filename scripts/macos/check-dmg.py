#!/usr/bin/env python3
"""Mount a development DMG read-only and verify the installed package contents."""

import argparse
import hashlib
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('image', type=Path)
    parser.add_argument('--report', required=True, type=Path)
    args = parser.parse_args()
    if sys.platform != 'darwin':
        parser.error('DMG verification requires native macOS hdiutil')
    image = args.image.resolve()
    subprocess.run(['hdiutil', 'verify', str(image)], check=True)
    mount = Path(tempfile.mkdtemp(prefix='naga-dmg-check-'))
    mounted = False
    try:
        subprocess.run(['hdiutil', 'attach', '-readonly', '-nobrowse', '-mountpoint', str(mount), str(image)], check=True)
        mounted = True
        applications = mount / 'Applications'
        if not applications.is_symlink() or os.readlink(applications) != '/Applications':
            raise RuntimeError('DMG must contain an Applications link')
        if not (mount / 'ПРОЧИТАЙТЕ.txt').is_file():
            raise RuntimeError('development installation instructions are missing')
        bundle = mount / 'Naga Network.app'
        if not (bundle / 'Contents/Helpers/naga-control').is_file():
            raise RuntimeError('packaged user backend is missing')
        subprocess.run([sys.executable, str(ROOT / 'scripts/macos/check-app.py'), str(bundle),
                        '--report', str(args.report.resolve())], check=True)
        subprocess.run([sys.executable, str(ROOT / 'scripts/macos/test-app-session.py'), str(bundle)], check=True)
        report = json.loads(args.report.read_text())
        hasher = hashlib.sha256()
        with image.open('rb') as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b''):
                hasher.update(chunk)
        digest = hasher.hexdigest()
        report['image'] = {'filename': image.name, 'sha256': digest, 'read_only_verification': 'PASS',
                           'channel': 'development', 'apple_notarization': 'UNAVAILABLE'}
        args.report.write_text(json.dumps(report, indent=2) + '\n')
        print('PASS: read-only DMG, Applications link, signed ARM64 code and packaged backend startup')
    finally:
        if mounted or os.path.ismount(mount):
            subprocess.run(['hdiutil', 'detach', str(mount)], check=True)
        mount.rmdir()


if __name__ == '__main__':
    main()
