#!/usr/bin/env python3
"""Inspect a built macOS scaffold; this does not assert VPN readiness."""

import argparse
import json
import plistlib
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
MACHO_MAGICS = {
    bytes.fromhex(value) for value in
    ('feedface', 'cefaedfe', 'feedfacf', 'cffaedfe',
     'cafebabe', 'bebafeca', 'cafebabf', 'bfbafeca')
}


def output(*command):
    return subprocess.check_output(command, text=True).strip()


def inspect(bundle):
    info = plistlib.loads((bundle / 'Contents/Info.plist').read_bytes())
    pubspec = (ROOT / 'app/pubspec.yaml').read_text()
    version, build = re.search(r'^version:\s*(\S+)', pubspec, re.M).group(1).split('+')
    expected = {
        'CFBundleIdentifier': 'eu.nagavpn.nagaNetwork',
        'CFBundleShortVersionString': version,
        'CFBundleVersion': build,
        'LSMinimumSystemVersion': '14.0',
    }
    for key, value in expected.items():
        if info.get(key) != value:
            raise RuntimeError(f'{key}: expected {value!r}, got {info.get(key)!r}')
    subprocess.run(['codesign', '--verify', '--deep', '--strict', str(bundle)], check=True)
    entitlements = subprocess.check_output(
        ['codesign', '-d', '--entitlements', ':-', str(bundle)])
    entitlement_values = plistlib.loads(entitlements) if entitlements.strip() else {}
    if entitlement_values.get('com.apple.security.app-sandbox', False):
        raise RuntimeError('external macOS channel must not enable App Sandbox')

    executable = bundle / 'Contents/MacOS' / info['CFBundleExecutable']
    code = []
    for path in sorted(bundle.rglob('*')):
        if not path.is_file() or path.is_symlink():
            continue
        with path.open('rb') as stream:
            if stream.read(4) not in MACHO_MAGICS:
                continue
        architectures = output('lipo', '-archs', str(path)).split()
        if 'arm64' not in architectures:
            raise RuntimeError(f'no ARM64 slice: {path}')
        if path == executable and architectures != ['arm64']:
            raise RuntimeError(f'main executable is not native ARM64 only: {architectures}')
        dependencies = []
        for line in output('otool', '-arch', 'arm64', '-L', str(path)).splitlines()[1:]:
            # Preserve spaces in bundle/executable names.
            name = line.strip().split(' (compatibility version', 1)[0]
            if name.startswith('/') and not name.startswith(('/System/Library/', '/usr/lib/')):
                raise RuntimeError(f'non-system absolute dependency in {path}: {name}')
            dependencies.append(name)
        code.append({'path': str(path.relative_to(bundle)),
                     'architectures': architectures, 'dependencies': dependencies})
    if not any(item['path'] == str(executable.relative_to(bundle)) for item in code):
        raise RuntimeError('main Mach-O executable not found')
    return {'schema_version': 1, 'source_commit': output('git', '-C', str(ROOT), 'rev-parse', 'HEAD'),
            'bundle': expected, 'code': code, 'signature_verification': 'PASS',
            'app_sandbox': False, 'vpn_device_acceptance': 'UNAVAILABLE'}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('bundle', type=Path)
    parser.add_argument('--report', type=Path, required=True)
    args = parser.parse_args()
    if sys.platform != 'darwin':
        parser.error('native macOS inspection requires codesign, lipo and otool')
    report = inspect(args.bundle.resolve())
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(report, indent=2) + '\n')
    print(f'PASS: app metadata, signatures, ARM64 code and dependencies ({len(report["code"])} binaries)')


if __name__ == '__main__':
    main()
