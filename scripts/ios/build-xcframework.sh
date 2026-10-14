#!/usr/bin/env bash
set -euo pipefail

# Keep nested engine modules on the same patched compiler as naga-control.
export GOTOOLCHAIN=go1.27.2

# Optional Go archive for a future Flutter/Xcode Runner. The Linux iOS client
# in ios/ is built with xtool and does not need this yet.

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "libnaga.xcframework собирается на Mac (Xcode + iOS SDK)." >&2
  echo "Linux-клиент: ./scripts/ios/build-ipa.sh через xtool (ios/)." >&2
  exit 1
fi

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
out_dir="${project_dir}/app/ios/Frameworks"
mkdir -p "${out_dir}"
work="$(mktemp -d)"
trap 'rm -rf "${work}"' EXIT

sdk="$(xcrun --sdk iphoneos --show-sdk-path)"
cc="$(xcrun --sdk iphoneos --find clang)"
naga_version="$(awk '/^version:/ { print $2; exit }' "${project_dir}/app/pubspec.yaml" | cut -d+ -f1)"

CGO_ENABLED=1 \
GOOS=ios \
GOARCH=arm64 \
CC="${cc}" \
CGO_CFLAGS="-isysroot ${sdk} -arch arm64 -miphoneos-version-min=16.0" \
CGO_LDFLAGS="-isysroot ${sdk} -arch arm64" \
go build -buildmode=c-archive -buildvcs=false \
  -ldflags "-X naga.network/core/version.Version=${naga_version:-dev}" \
  -o "${work}/libnaga.a" \
  "${project_dir}/cmd/naga-ios"

rm -rf "${out_dir}/libnaga.xcframework"
xcodebuild -create-xcframework \
  -library "${work}/libnaga.a" \
  -headers "${work}" \
  -output "${out_dir}/libnaga.xcframework"

echo "xcframework: ${out_dir}/libnaga.xcframework"
