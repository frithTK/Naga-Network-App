#!/usr/bin/env python3
"""Copy a published DMG app and exercise a real LaunchServices GUI launch."""

import argparse
import hashlib
import json
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('image', type=Path)
    parser.add_argument('--sha256', required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    image = args.image.resolve()
    if hashlib.sha256(image.read_bytes()).hexdigest() != args.sha256:
        raise RuntimeError('published DMG checksum mismatch')
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='naga-gui-') as temp:
        root = Path(temp)
        mount = root / 'mounted'
        mount.mkdir()
        subprocess.run(['hdiutil', 'attach', '-readonly', '-nobrowse', '-mountpoint', str(mount), str(image)], check=True)
        try:
            app = root / 'Naga Network.app'
            subprocess.run(['ditto', str(mount / app.name), str(app)], check=True)
        finally:
            subprocess.run(['hdiutil', 'detach', str(mount)], check=True)
        subprocess.run(['codesign', '--verify', '--deep', '--strict', str(app)], check=True)
        policy = subprocess.run(['syspolicy_check', 'distribution', str(app)], capture_output=True, text=True, timeout=60)
        (output / 'distribution-policy.log').write_text(policy.stdout + policy.stderr)
        # Ad-hoc development code is not a notarized distribution. Record this
        # separately; an ordinary unquarantined GUI launch is a different check.
        (output / 'distribution-policy.json').write_text(json.dumps({
            'returncode': policy.returncode, 'channel': 'development-ad-hoc',
            'notarized_browser_download_acceptance': 'UNAVAILABLE',
        }, indent=2) + '\n')
        result = subprocess.run(['swift', str(ROOT / 'scripts/macos/test-gui-launch.swift'),
                                 str(app), str(output / 'gui-launch.json')], timeout=100)
        if result.returncode or policy.returncode:
            logs = subprocess.run(['log', 'show', '--last', '2m', '--style', 'compact', '--predicate',
                                   'process == "Naga Network" OR process == "syspolicyd" OR process == "amfid" OR subsystem == "com.apple.xprotect"'],
                                  capture_output=True, text=True, timeout=20)
            (output / 'launch-failure.log').write_text((logs.stdout + logs.stderr)[-50000:])
        raise SystemExit(result.returncode)


if __name__ == '__main__':
    main()
