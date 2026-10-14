#!/usr/bin/env bash
set -euo pipefail

# Build from a private source copy, so old app/IPA outputs cannot be published.
project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${project_dir}/scripts/ios/xtool-env.sh"
xtool_bin="$(naga_require_xtool)"
naga_require_clean_linker
work="$(mktemp -d "${TMPDIR:-/tmp}/naga-ios-build.XXXXXXXX")"
trap 'rm -rf "${work}"' EXIT
tar -C "${project_dir}/ios" --exclude='./.build' --exclude='./xtool' --exclude='./.swiftpm' -cf - . | tar -C "${work}" -xf -
version="$(awk '/^version:/ { print $2; exit }' "${project_dir}/app/pubspec.yaml")"
python3 "${project_dir}/scripts/ios/verify-ipa.py" prepare "${work}" "${version}"
cd "${work}"
# pipefail preserves compile/link/sign failure. No retry can install old output.
"${xtool_bin}" dev build -c release --sign --ipa 2>&1 | tee "${work}/build.log"
python3 "${project_dir}/scripts/ios/verify-ipa.py" publish "${work}" "${version}" "${project_dir}/dist"
