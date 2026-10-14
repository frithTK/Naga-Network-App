#!/usr/bin/env python3
"""Apply reviewed dependency fixes to the pinned sing-box source before build."""
import json
import os
import subprocess
import sys
from pathlib import Path

if __name__ == '__main__':
    source = Path(sys.argv[1]).resolve()
    pins = json.loads(Path(__file__).with_name('singbox-security-deps.json').read_text())
    env = dict(os.environ, GOTOOLCHAIN='go1.27.2')
    result = subprocess.run(['go', '-C', str(source), 'get', *[f'{name}@{version}' for name, version in pins.items()]], env=env)
    sys.exit(result.returncode)
