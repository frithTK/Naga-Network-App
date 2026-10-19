#!/usr/bin/env python3
"""Typecheck native VPN sources before building Flutter or downloading engines."""

import subprocess
import sys
import tempfile
from pathlib import Path

if sys.platform != 'darwin':
    raise SystemExit('Native macOS SDK is required')
root = Path(__file__).resolve().parents[2]
sources = root / 'native/macos/vpn'
with tempfile.TemporaryDirectory(prefix='naga-vpn-typecheck-') as temporary:
    directory = Path(temporary)
    pins = directory / 'VPNBuild.swift'
    pins.write_text('let vpnBridgeCDHash = "' + '0' * 40 + '"\n'
                    'let vpnEngineSHA256 = "' + '0' * 64 + '"\n')
    base = ['swiftc', '-typecheck', '-parse-as-library', '-swift-version', '5',
            '-module-cache-path', str(directory / 'cache'), '-target', 'arm64-apple-macos14.0',
            str(sources / 'VPNProtocol.swift')]
    subprocess.run([*base, str(sources / 'VPNBridge.swift')], check=True)
    subprocess.run([*base, str(sources / 'VPNConfig.swift'), str(sources / 'VPNHelper.swift'), str(pins)], check=True)
print('PASS: native VPN bridge/helper typecheck; no registration or network changes')
