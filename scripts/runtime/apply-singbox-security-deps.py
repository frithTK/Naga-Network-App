#!/usr/bin/env python3
"""Apply dependency fixes and import compatibility to the pinned source."""
import json
import os
import subprocess
import sys
from pathlib import Path


def add_chrome_120_fingerprint(source):
    path = source / 'common/tls/utls_client.go'
    content = path.read_text()
    case = '\tcase "hellochrome_120":\n\t\treturn utls.HelloChrome_120, nil\n'
    if case in content:
        return
    location = '\tcase "firefox":\n\t\treturn utls.HelloFirefox_Auto, nil\n'
    if content.count(location) != 1:
        raise SystemExit('Cannot locate the pinned sing-box fingerprint switch')
    path.write_text(content.replace(location, case + location, 1))

if __name__ == '__main__':
    source = Path(sys.argv[1]).resolve()
    pins = json.loads(Path(__file__).with_name('singbox-security-deps.json').read_text())
    env = dict(os.environ, GOTOOLCHAIN='go1.27.2')
    result = subprocess.run(['go', '-C', str(source), 'get', *[f'{name}@{version}' for name, version in pins.items()]], env=env)
    if result.returncode:
        sys.exit(result.returncode)
    add_chrome_120_fingerprint(source)
