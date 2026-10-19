#!/usr/bin/env python3
"""Inspect a built macOS scaffold; this does not assert VPN readiness."""

import argparse
import hashlib
import json
import plistlib
import re
import subprocess
import sys
from pathlib import Path

from bundle_paths import MACHO_MAGICS, resolve_rpath, rpaths

ROOT = Path(__file__).resolve().parents[2]


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

    diagnostic_id = 'eu.nagavpn.nagaNetwork.DiagnosticHelper'
    diagnostic_program = 'Contents/Helpers/NagaDiagnosticHelper'
    daemon = plistlib.loads((bundle / 'Contents/Library/LaunchDaemons' /
                            (diagnostic_id + '.plist')).read_bytes())
    if daemon != {'Label': diagnostic_id, 'BundleProgram': diagnostic_program,
                  'MachServices': {diagnostic_id: True}}:
        raise RuntimeError('unexpected diagnostic daemon configuration')
    if not (bundle / diagnostic_program).is_file():
        raise RuntimeError('diagnostic helper is missing')

    vpn_id = 'eu.nagavpn.nagaNetwork.VPNHelper'
    vpn_daemon = plistlib.loads((bundle / 'Contents/Library/LaunchDaemons' /
                                (vpn_id + '.plist')).read_bytes())
    if vpn_daemon != {'Label': vpn_id, 'BundleProgram': 'Contents/Helpers/NagaVPNHelper',
                      'MachServices': {vpn_id: True}}:
        raise RuntimeError('unexpected VPN daemon configuration')
    helpers = bundle / 'Contents/Helpers'
    for name in ('naga-control', 'sing-box', 'NagaVPNBridge', 'NagaVPNHelper'):
        path = helpers / name
        if path.is_symlink() or not path.is_file() or path.stat().st_mode & 0o111 != 0o111:
            raise RuntimeError(f'VPN component is missing or not executable: {name}')
    if not (bundle / 'Contents/Resources/licenses/LICENSE.sing-box').is_file():
        raise RuntimeError('bundled sing-box license is missing')
    runtime = json.loads((bundle / 'Contents/Resources/vpn-runtime.json').read_text())
    if runtime['schema_version'] != 1 or runtime['protocol_version'] != 1 or runtime['build'] != 'vpn-1':
        raise RuntimeError('incompatible VPN runtime manifest')
    engine_digest = hashlib.sha256((helpers / 'sing-box').read_bytes()).hexdigest()
    bridge_details = subprocess.check_output(['codesign', '-dvvv', str(helpers / 'NagaVPNBridge')],
                                             stderr=subprocess.STDOUT, text=True)
    bridge_digest = re.search(r'^CDHash=([a-f0-9]{40})$', bridge_details, re.M).group(1)
    check = json.loads(output(str(helpers / 'NagaVPNHelper'), '--self-check'))
    for key, value in {'engine_sha256': engine_digest, 'bridge_cdhash': bridge_digest,
                       'protocol_version': 1, 'build': 'vpn-1'}.items():
        if runtime.get(key) != value or check.get(key) != value:
            raise RuntimeError(f'VPN helper and bundled runtime disagree: {key}')

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
        if path.stat().st_mode & 0o111 != 0o111:
            raise RuntimeError(f'Mach-O lacks executable permissions: {path}')
        search_paths = rpaths(path)
        for value in search_paths:
            if resolve_rpath(value, path, executable, bundle) is None:
                raise RuntimeError(f'nonportable LC_RPATH in {path}: {value}')
        subprocess.run(['codesign', '--verify', '--strict', str(path)], check=True)
        dependencies = []
        for line in output('otool', '-arch', 'arm64', '-L', str(path)).splitlines()[1:]:
            # Preserve spaces in bundle/executable names.
            name = line.strip().split(' (compatibility version', 1)[0]
            if name.startswith('/') and not name.startswith(('/System/Library/', '/usr/lib/')):
                raise RuntimeError(f'non-system absolute dependency in {path}: {name}')
            dependencies.append(name)
        code.append({'path': str(path.relative_to(bundle)),
                     'architectures': architectures, 'dependencies': dependencies,
                     'rpaths': search_paths})
    if not any(item['path'] == str(executable.relative_to(bundle)) for item in code):
        raise RuntimeError('main Mach-O executable not found')
    # Static signature validity and an unquarantined launch do not establish
    # browser-download acceptance. Keep the actual OS distribution verdict.
    policy = subprocess.run(['syspolicy_check', 'distribution', str(bundle)],
                            capture_output=True, text=True, timeout=60)
    return {'schema_version': 1, 'source_commit': output('git', '-C', str(ROOT), 'rev-parse', 'HEAD'),
            'bundle': expected, 'code': code, 'signature_verification': 'PASS',
            'app_sandbox': False, 'vpn_runtime': runtime, 'vpn_device_acceptance': 'UNAVAILABLE',
            'distribution_assessment': {
                'result': 'PASS' if policy.returncode == 0 else 'REJECTED',
                'returncode': policy.returncode,
                'output': policy.stdout + policy.stderr,
            }}


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
