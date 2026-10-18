#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
variant="${1:-A}"
case "${variant}" in A|B) ;; *) echo 'Expected probe variant A or B' >&2; exit 2 ;; esac
[[ "$(uname -s)" == Darwin ]] || { echo 'macOS with Xcode is required' >&2; exit 2; }

output_dir="${project_dir}/build/macos-service-probe/${variant}"
bundle="${output_dir}/Naga Service Probe.app"
native_dir="${project_dir}/native/macos/service-probe"
mkdir -p "${bundle}/Contents/MacOS" "${bundle}/Contents/Library/LaunchDaemons"
swift_flags=(-parse-as-library -swift-version 5 -O -target arm64-apple-macos14.0)
if [[ "${variant}" == B ]]; then swift_flags+=(-D PROBE_B); fi

swiftc "${swift_flags[@]}" "${native_dir}/ProbeProtocol.swift" "${native_dir}/ProbeHelper.swift" \
  -o "${bundle}/Contents/MacOS/NagaServiceProbeHelper"
swiftc "${swift_flags[@]}" "${native_dir}/ProbeProtocol.swift" "${native_dir}/ProbeApp.swift" \
  -o "${bundle}/Contents/MacOS/NagaServiceProbe"

python3 - "${bundle}" "${variant}" <<'PY'
import plistlib
import sys
from pathlib import Path

bundle = Path(sys.argv[1])
variant = sys.argv[2]
identifier = 'eu.nagavpn.nagaNetwork.ServiceProbe'
helper = identifier + '.Helper'
info = {
    'CFBundleIdentifier': identifier,
    'CFBundleName': 'Naga Service Probe',
    'CFBundleExecutable': 'NagaServiceProbe',
    'CFBundlePackageType': 'APPL',
    'CFBundleShortVersionString': '0.0.1' if variant == 'A' else '0.0.2',
    'CFBundleVersion': '1' if variant == 'A' else '2',
    'LSMinimumSystemVersion': '14.0',
    'NSHighResolutionCapable': True,
}
daemon = {
    'Label': helper,
    'BundleProgram': 'Contents/MacOS/NagaServiceProbeHelper',
    'MachServices': {helper: True},
}
(bundle/'Contents/Info.plist').write_bytes(plistlib.dumps(info))
(bundle/f'Contents/Library/LaunchDaemons/{helper}.plist').write_bytes(plistlib.dumps(daemon))
PY

codesign --force --sign - --identifier eu.nagavpn.nagaNetwork.ServiceProbe.Helper \
  "${bundle}/Contents/MacOS/NagaServiceProbeHelper"
codesign --force --sign - --identifier eu.nagavpn.nagaNetwork.ServiceProbe "${bundle}"
codesign --verify --deep --strict "${bundle}"
"${bundle}/Contents/MacOS/NagaServiceProbeHelper" --self-check
ditto -c -k --sequesterRsrc --keepParent "${bundle}" "${output_dir}/NagaServiceProbe-${variant}-macos-arm64.zip"
shasum -a 256 "${output_dir}/NagaServiceProbe-${variant}-macos-arm64.zip" > "${output_dir}/SHA256SUMS.txt"
echo "Probe ${variant}: ${output_dir}"
