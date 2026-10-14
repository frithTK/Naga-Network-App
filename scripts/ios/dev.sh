#!/usr/bin/env bash
set -euo pipefail

# Build/install exactly the result produced by this xtool invocation.
project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${project_dir}/scripts/ios/xtool-env.sh"
xtool_bin="$(naga_require_xtool)"
naga_require_clean_linker
cd "${project_dir}/ios"
exec "${xtool_bin}" dev "$@"
