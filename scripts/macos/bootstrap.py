#!/usr/bin/env python3
"""Build isolated Darwin ARM64 proofs; native execution is reported separately."""

import argparse
import hashlib
import json
import os
import platform
import re
import shutil
import struct
import subprocess
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
COMPONENTS = ('naga-control', 'sing-box', 'olcrtc', 'hev-socks5-tunnel')


def capture(*args):
    return subprocess.check_output(args, cwd=ROOT, text=True).strip()


def run(command, *, cwd, env, log, allowed=(0,)):
    print(f'[{log.stem}] {command[0]} {command[1] if len(command) > 1 else ""}', flush=True)
    with log.open('ab') as stream:
        stream.write(('\n$ ' + ' '.join(map(str, command)) + '\n').encode())
        stream.flush()
        result = subprocess.run(command, cwd=cwd, env=env, stdout=stream, stderr=subprocess.STDOUT)
    if result.returncode not in allowed:
        raise RuntimeError(f'command failed ({result.returncode}); see {log}')


def prepare_source(name, spec, directory, env, log):
    source = directory / 'source'
    source.mkdir()
    run(['git', 'init', '--quiet', str(source)], cwd=ROOT, env=env, log=log)
    # A fresh checkout uses cached objects, never the cached tree's local patches.
    cache = ROOT / 'build' / 'linux-runtime' / name
    remote = spec['url']
    if (cache / '.git').exists():
        exists = subprocess.run(['git', '-C', str(cache), 'cat-file', '-e', spec['commit']],
                                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if exists.returncode == 0:
            remote = str(cache)
    run(['git', 'fetch', '--depth', '1', remote, spec['commit']], cwd=source, env=env, log=log)
    run(['git', '-c', 'advice.detachedHead=false', 'checkout', '--detach', spec['commit']],
        cwd=source, env=env, log=log)
    actual = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=source, text=True).strip()
    if actual != spec['commit']:
        raise RuntimeError(f'wrong source commit: {actual}')
    return source


def inspect_binary(binary):
    with binary.open('rb') as stream:
        header = stream.read(8)
    if len(header) != 8 or struct.unpack('<II', header) != (0xFEEDFACF, 0x0100000C):
        raise RuntimeError('expected a thin Mach-O ARM64 executable')
    return {'format': 'Mach-O 64-bit', 'architecture': 'arm64',
            'sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
            'size': binary.stat().st_size}


def build_component(name, spec, directory, env, version):
    directory.mkdir()
    log = directory / f'{name}.log'
    binary = directory / name
    result = {'component': name, 'build': 'FAIL', 'native_execution': 'UNAVAILABLE', 'native_checks': [],
              'log': log.name, 'source_commit': spec.get('commit')}
    try:
        source = ROOT if name == 'naga-control' else prepare_source(name, spec, directory, env, log)
        if name == 'sing-box':
            run([sys.executable, str(ROOT / 'scripts/runtime/apply-singbox-security-deps.py'), str(source)],
                cwd=ROOT, env=env, log=log)
            shutil.copy2(ROOT / 'scripts/macos/singbox-parent-watchdog.go.txt',
                         source / 'cmd/sing-box/naga_parent_watchdog_darwin.go')
            for suffix in ('', '_test'):
                shutil.copy2(ROOT / f'scripts/macos/singbox-vpn-api{suffix}.go.txt',
                             source / f'experimental/clashapi/naga_vpn_api{suffix}.go')
            server = source / 'experimental/clashapi/server.go'
            text = server.read_text()
            anchor = 'chiRouter := chi.NewRouter()'
            if text.count(anchor) != 1:
                raise RuntimeError('sing-box API layout changed; recheck privileged API restrictions')
            server.write_text(text.replace(anchor, anchor + '\n\tchiRouter.Use(nagaVPNAPI)'))
            result['local_overlays'] = {
                path.name: hashlib.sha256(path.read_bytes()).hexdigest()
                for path in (ROOT / 'scripts/macos').glob('singbox-*.go.txt')
            }
        elif name == 'olcrtc':
            fixes = [f'{module}@{pin}' for module, pin in spec['dependency_fixes'].items()]
            run(['go', 'get', *fixes], cwd=source, env=env, log=log)

        if name == 'hev-socks5-tunnel':
            if platform.system() != 'Darwin':
                result['build'] = 'UNAVAILABLE'
                result['reason'] = 'hev requires the macOS SDK; run the native macOS job'
                return result
            run(['git', 'submodule', 'update', '--init', '--recursive'], cwd=source, env=env, log=log)
            run(['make', '-j2', 'CC=clang', 'PP=clang -E',
                 'CFLAGS=-arch arm64 -mmacosx-version-min=14.0',
                 'LFLAGS=-arch arm64 -mmacosx-version-min=14.0'], cwd=source, env=env, log=log)
            shutil.copy2(source / 'bin/hev-socks5-tunnel', binary)
        else:
            command = ['go', 'build', '-p', '2', '-trimpath', '-buildvcs=false']
            ldflags = '-s -w'
            if name == 'naga-control':
                ldflags += f' -X naga.network/core/version.Version={version}'
            elif name == 'sing-box':
                command += ['-tags', 'with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_clash_api,http2legacy']
                ldflags += f' -checklinkname=0 -X github.com/sagernet/sing-box/constant.Version={spec["version"]}'
            package = './cmd/naga-control' if name == 'naga-control' else f'./cmd/{name}'
            run([*command, '-ldflags', ldflags, '-o', str(binary), package], cwd=source, env=env, log=log)

        result.update(inspect_binary(binary))
        shutil.copy2(source / spec.get('license', 'LICENSE'), directory / f'LICENSE.{name}')
        if name != 'hev-socks5-tunnel':
            (directory / 'go-build-info.txt').write_text(capture('go', 'version', '-m', str(binary)) + '\n')
        result['build'] = 'PASS'
        if platform.system() == 'Darwin' and platform.machine() == 'arm64':
            if name == 'sing-box':
                run(['go', 'test', '-tags', 'with_clash_api', './experimental/clashapi',
                     '-run', '^TestNagaVPNAPI$', '-count=1'], cwd=source, env=env, log=log)
                config = directory / 'check.json'
                config.write_text('{"outbounds":[{"type":"direct","tag":"direct"}]}\n')
                run([str(binary), 'version'], cwd=directory, env=env, log=log)
                run([str(binary), 'check', '-c', str(config)], cwd=directory, env=env, log=log)
                vpn_config = directory / 'helper-check.json'
                run([sys.executable, str(ROOT / 'scripts/macos/test-vpn-config.py'), str(vpn_config)],
                    cwd=ROOT, env=env, log=log)
                run([str(binary), 'check', '-c', str(vpn_config)], cwd=directory, env=env, log=log)
                result['native_checks'] = ['privileged API restrictions', 'version',
                                          'check without a TUN interface', 'check helper-prepared TUN config (no start)']
            elif name == 'naga-control':
                run([str(binary), '-help'], cwd=directory, env=env, log=log)
                result['native_checks'] = ['CLI help without starting the service']
            elif name == 'olcrtc':
                run([str(binary), '--help'], cwd=directory, env=env, log=log, allowed=(1,))
                if 'usage: olcrtc' not in log.read_text(errors='replace'):
                    raise RuntimeError('olcrtc did not report its expected usage')
                result['native_checks'] = ['expected CLI usage error without a session']
            else:
                run(['otool', '-L', str(binary)], cwd=directory, env=env, log=log)
                run([str(binary)], cwd=directory, env=env, log=log, allowed=(255,))
                if 'Version:' not in log.read_text(errors='replace'):
                    raise RuntimeError('hev did not report its expected usage/version')
                result['native_checks'] = ['otool dependencies', 'expected usage/version without a tunnel']
            result['native_execution'] = 'PASS'
        return result
    except (OSError, RuntimeError, subprocess.CalledProcessError) as error:
        result['error'] = str(error)
        if result['build'] == 'PASS':
            result['native_execution'] = 'FAIL'
        return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--component', choices=(*COMPONENTS, 'all'), default='all')
    parser.add_argument('--output-root', type=Path, default=ROOT / 'build/macos-bootstrap')
    parser.add_argument('--export-dir', type=Path, help='Copy a successful single component and its license here')
    args = parser.parse_args()
    if args.export_dir and args.component == 'all':
        parser.error('--export-dir requires one component')
    args.output_root.mkdir(parents=True, exist_ok=True)
    output = Path(tempfile.mkdtemp(prefix=f'{args.component}-', dir=args.output_root)).resolve()
    pins = json.loads(Path(__file__).with_name('runtime-pins.json').read_text())
    toolchain = re.search(r'^toolchain\s+(\S+)', (ROOT / 'go.mod').read_text(), re.M).group(1)
    version = re.search(r'^version:\s*([^+\s]+)', (ROOT / 'app/pubspec.yaml').read_text(), re.M).group(1)
    env = dict(os.environ, GOTOOLCHAIN=toolchain, GOOS='darwin', GOARCH='arm64',
               CGO_ENABLED='0', MACOSX_DEPLOYMENT_TARGET='14.0')
    manifest = {'schema_version': 1, 'created_utc': datetime.now(timezone.utc).isoformat(),
                'source_commit': capture('git', 'rev-parse', 'HEAD'), 'app_version': version,
                'source_dirty': bool(capture('git', 'status', '--porcelain')),
                'builder_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                'host': {'system': platform.system(), 'machine': platform.machine()},
                'target': 'darwin/arm64', 'go_toolchain': toolchain, 'components': [],
                'vpn_device_acceptance': 'UNAVAILABLE'}
    for command, key in [(('sw_vers',), 'macos'), (('xcodebuild', '-version'), 'xcode')]:
        if platform.system() == 'Darwin':
            manifest['host'][key] = capture(*command)
    for name in COMPONENTS if args.component == 'all' else (args.component,):
        spec = pins.get(name, {'commit': manifest['source_commit']})
        result = build_component(name, spec, output / name, env, version)
        manifest['components'].append(result)
        (output / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
        print(f'{name}: build={result["build"]}, native={result["native_execution"]}', flush=True)
        if args.export_dir and result['build'] == 'PASS' and result['native_execution'] != 'FAIL':
            args.export_dir.mkdir(parents=True, exist_ok=True)
            for file in (name, f'LICENSE.{name}'):
                shutil.copy2(output / name / file, args.export_dir / file)
    print(f'Manifest: {output / "manifest.json"}', flush=True)
    return int(any(item['build'] == 'FAIL' or item['native_execution'] == 'FAIL'
                   for item in manifest['components']))


if __name__ == '__main__':
    raise SystemExit(main())
