#!/usr/bin/env python3
"""Run the actual helper's config validator without root or network changes."""

import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
with tempfile.TemporaryDirectory(prefix='naga-vpn-config-') as temporary:
    directory = Path(temporary)
    sources = ROOT / 'native/macos/vpn'
    binary = directory / 'vpn-config-tests'
    subprocess.run(['swiftc', '-parse-as-library', '-swift-version', '5',
                    '-module-cache-path', str(directory / 'cache'),
                    str(sources / 'VPNProtocol.swift'), str(sources / 'VPNConfig.swift'),
                    str(sources / 'tests/VPNConfigTests.swift'), '-o', str(binary)], check=True)
    subprocess.run([str(binary), *sys.argv[1:]], check=True)
