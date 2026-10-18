#!/usr/bin/env python3
"""Exercise the packaged CLI handshake and parent-bound lifetime, without VPN."""

import json
import os
import plistlib
import selectors
import subprocess
import sys
import tempfile
import urllib.error
import urllib.request
from pathlib import Path


def main():
    bundle = Path(sys.argv[1]).resolve()
    info = plistlib.loads((bundle / 'Contents/Info.plist').read_bytes())
    binary = bundle / 'Contents/Helpers/naga-control'
    diagnostic = bundle / 'Contents/Helpers/NagaDiagnosticHelper'
    diagnostic_status = json.loads(subprocess.check_output([str(diagnostic), '--self-check']))
    assert diagnostic_status['build'] == 'integrated-1'
    assert diagnostic_status['protocol_version'] == 1
    assert diagnostic_status['euid'] == os.geteuid()
    with tempfile.TemporaryDirectory(prefix='naga-macos-session-') as temp:
        home = Path(temp) / 'Пользователь с пробелами'
        root = home / 'Library/Application Support/Naga Network'
        home.mkdir()
        env = dict(os.environ, HOME=str(home), NAGA_DATA_DIR=str(root),
                   NAGA_CONTROL_TOKEN='must-not-be-used-from-the-shell')
        process = subprocess.Popen([str(binary), '--app-session'], env=env,
                                   stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        try:
            with selectors.DefaultSelector() as selector:
                selector.register(process.stdout, selectors.EVENT_READ)
                if not selector.select(timeout=15):
                    raise RuntimeError('packaged backend readiness timed out')
            line = process.stdout.readline(16 * 1024)
            value = json.loads(line)
            assert value['type'] == 'ready', 'backend rejected startup'
            assert value['protocol_version'] == 1
            assert value['version'] == info['CFBundleShortVersionString']
            assert value['data_dir'] == str(root)
            assert value['token'] != env['NAGA_CONTROL_TOKEN'] and len(value['token']) == 64
            endpoint = value['endpoint']
            assert endpoint.startswith('http://127.0.0.1:')
            for token, origin, expected in (
                    ('', '', 401), ('incorrect', '', 401),
                    (value['token'], 'https://untrusted.example', 403),
                    (value['token'], '', 200)):
                headers = {'Authorization': 'Bearer ' + token} if token else {}
                if origin:
                    headers['Origin'] = origin
                request = urllib.request.Request(endpoint + '/v1/health', headers=headers)
                try:
                    response = urllib.request.urlopen(request, timeout=3)
                except urllib.error.HTTPError as error:
                    response = error
                with response:
                    assert response.code == expected, f'HTTP status {response.code}, expected {expected}'
                    if expected == 200:
                        health = json.load(response)
                        assert health['version'] == value['version']
                        assert health['capabilities']['vpn_connect'] is False
            process.stdin.close()
            assert process.wait(timeout=10) == 0, 'parent EOF did not shut down cleanly'
            assert (root / 'control.token').stat().st_mode & 0o077 == 0
            print('PASS: packaged backend version, private readiness, authentication, browser rejection and parent EOF cleanup')
        finally:
            if process.poll() is None:
                process.stdin.close()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=3)
            process.stdout.close()
            process.stderr.close()


if __name__ == '__main__':
    main()
