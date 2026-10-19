#!/usr/bin/env bash
set -euo pipefail

[[ "$(uname -s)" == Darwin ]] || { echo 'macOS hdiutil is required' >&2; exit 2; }
project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
bundle="${1:-${project_dir}/app/build/macos/Build/Products/Release/Naga Network.app}"
output_dir="${project_dir}/build/macos-dmg"
macos_app_version="$(/usr/libexec/PlistBuddy -c 'Print CFBundleShortVersionString' "${bundle}/Contents/Info.plist")"
filename="NagaNetwork-${macos_app_version}-macos-arm64.dmg"
mkdir -p "${output_dir}"
staging="$(mktemp -d "${output_dir}/staging.XXXXXX")"
trap 'rm -rf "${staging}"' EXIT
ditto "${bundle}" "${staging}/Naga Network.app"
ln -s /Applications "${staging}/Applications"
cp "${project_dir}/packaging/macos/README-test.txt" "${staging}/ПРОЧИТАЙТЕ.txt"
hdiutil create -ov -fs HFS+ -format UDZO -volname 'Naga Network Test' \
  -srcfolder "${staging}" "${output_dir}/${filename}"
cd "${output_dir}"
shasum -a 256 "${filename}" > SHA256SUMS.txt
echo "Development DMG: ${output_dir}/${filename}"
