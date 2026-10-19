#!/usr/bin/env bash
set -euo pipefail

[[ "$(uname -s)" == Darwin && "$(uname -m)" == arm64 ]] || {
  echo 'Native ARM64 macOS with Xcode is required' >&2; exit 2;
}
project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
flutter_command="${FLUTTER_BIN:-flutter}"
cd "${project_dir}"
macos_app_version="$(python3 -c 'import pathlib,re; print(re.search(r"^version:\s*([^+\s]+)", pathlib.Path("app/pubspec.yaml").read_text(), re.M).group(1))')"
mkdir -p build/macos-app-runtime
python3 scripts/macos/bootstrap.py --component sing-box --export-dir build/macos-app-runtime
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -buildvcs=false \
  -ldflags "-s -w -X naga.network/core/version.Version=${macos_app_version}" \
  -o build/macos-app-runtime/naga-control ./cmd/naga-control
cd app
"${flutter_command}" build macos --release --dart-define="NAGA_VERSION=${macos_app_version}"
bundle="${project_dir}/app/build/macos/Build/Products/Release/Naga Network.app"
mkdir -p "${bundle}/Contents/Helpers"
cp "${project_dir}/build/macos-app-runtime/naga-control" "${bundle}/Contents/Helpers/naga-control"
chmod 755 "${bundle}/Contents/Helpers/naga-control"
cp "${project_dir}/build/macos-app-runtime/sing-box" "${bundle}/Contents/Helpers/sing-box"
chmod 755 "${bundle}/Contents/Helpers/sing-box"
mkdir -p "${bundle}/Contents/Resources/licenses"
cp "${project_dir}/build/macos-app-runtime/LICENSE.sing-box" "${bundle}/Contents/Resources/licenses/LICENSE.sing-box"
swiftc -parse-as-library -swift-version 5 -O -target arm64-apple-macos14.0 \
  "${project_dir}/native/macos/vpn/VPNProtocol.swift" \
  "${project_dir}/native/macos/vpn/VPNBridge.swift" \
  -o "${bundle}/Contents/Helpers/NagaVPNBridge"
swiftc -parse-as-library -swift-version 5 -O -target arm64-apple-macos14.0 -D INTEGRATED_DIAGNOSTIC \
  "${project_dir}/native/macos/service-probe/ProbeProtocol.swift" \
  "${project_dir}/native/macos/service-probe/ProbeHelper.swift" \
  -o "${bundle}/Contents/Helpers/NagaDiagnosticHelper"
python3 - "${bundle}" <<'PY'
import plistlib
import sys
from pathlib import Path
bundle = Path(sys.argv[1])
identifier = 'eu.nagavpn.nagaNetwork.DiagnosticHelper'
directory = bundle / 'Contents/Library/LaunchDaemons'
directory.mkdir(parents=True, exist_ok=True)
(directory / (identifier + '.plist')).write_bytes(plistlib.dumps({
    'Label': identifier,
    'BundleProgram': 'Contents/Helpers/NagaDiagnosticHelper',
    'MachServices': {identifier: True},
}))
identifier = 'eu.nagavpn.nagaNetwork.VPNHelper'
(directory / (identifier + '.plist')).write_bytes(plistlib.dumps({
    'Label': identifier,
    'BundleProgram': 'Contents/Helpers/NagaVPNHelper',
    'MachServices': {identifier: True},
}))
PY
python3 "${project_dir}/scripts/macos/prepare-app.py" "${bundle}"
# Pin final bytes only after all load-command/permission normalization.
codesign --force --sign - --identifier eu.nagavpn.nagaNetwork.SingBox "${bundle}/Contents/Helpers/sing-box"
codesign --force --sign - --identifier eu.nagavpn.nagaNetwork.VPNBridge "${bundle}/Contents/Helpers/NagaVPNBridge"
python3 - "${bundle}" "${project_dir}/build/macos-app-runtime/VPNBuild.swift" <<'PY'
import hashlib, json, re, subprocess, sys
from pathlib import Path
bundle = Path(sys.argv[1])
helpers = bundle / 'Contents/Helpers'
details = subprocess.check_output(['codesign', '-dvvv', str(helpers / 'NagaVPNBridge')], stderr=subprocess.STDOUT, text=True)
digest = re.search(r'^CDHash=([a-f0-9]{40})$', details, re.M).group(1)
engine = hashlib.sha256((helpers / 'sing-box').read_bytes()).hexdigest()
Path(sys.argv[2]).write_text(f'let vpnBridgeCDHash = "{digest}"\nlet vpnEngineSHA256 = "{engine}"\n')
(bundle / 'Contents/Resources/vpn-runtime.json').write_text(json.dumps({
    'schema_version': 1, 'protocol_version': 1, 'build': 'vpn-1',
    'engine_sha256': engine, 'bridge_cdhash': digest,
    'source_commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip(),
    'vpn_device_acceptance': 'UNAVAILABLE',
}, indent=2) + '\n')
PY
swiftc -parse-as-library -swift-version 5 -O -target arm64-apple-macos14.0 \
  "${project_dir}/native/macos/vpn/VPNProtocol.swift" \
  "${project_dir}/native/macos/vpn/VPNConfig.swift" \
  "${project_dir}/native/macos/vpn/VPNHelper.swift" \
  "${project_dir}/build/macos-app-runtime/VPNBuild.swift" \
  -o "${bundle}/Contents/Helpers/NagaVPNHelper"
codesign --force --sign - --identifier eu.nagavpn.nagaNetwork.VPNHelper "${bundle}/Contents/Helpers/NagaVPNHelper"
codesign --force --sign - --identifier eu.nagavpn.nagaNetwork.Control "${bundle}/Contents/Helpers/naga-control"
codesign --force --sign - --identifier eu.nagavpn.nagaNetwork.DiagnosticHelper "${bundle}/Contents/Helpers/NagaDiagnosticHelper"
codesign --force --sign - --entitlements macos/Runner/Release.entitlements "${bundle}"
echo "Application with user backend: ${bundle}"
