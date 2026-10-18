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
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -buildvcs=false \
  -ldflags "-s -w -X naga.network/core/version.Version=${macos_app_version}" \
  -o build/macos-app-runtime/naga-control ./cmd/naga-control
cd app
"${flutter_command}" build macos --release --dart-define="NAGA_VERSION=${macos_app_version}"
bundle="${project_dir}/app/build/macos/Build/Products/Release/Naga Network.app"
mkdir -p "${bundle}/Contents/Helpers"
cp "${project_dir}/build/macos-app-runtime/naga-control" "${bundle}/Contents/Helpers/naga-control"
chmod 755 "${bundle}/Contents/Helpers/naga-control"
codesign --force --sign - --identifier eu.nagavpn.nagaNetwork.Control "${bundle}/Contents/Helpers/naga-control"
codesign --force --sign - --entitlements macos/Runner/Release.entitlements "${bundle}"
echo "Application with user backend: ${bundle}"
