#!/usr/bin/env bash
set -euo pipefail

# Build the routing experiment without modifying either upstream checkout or
# the currently installed engine. This artifact is not production-ready.
source_dir="$(realpath "${1:?pass an upstream olcRTC checkout}")"
output="${2:?pass a new output path}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
commit="b8a6e12e3e3afb0666e38f9668c7d81d8aff948f"
[[ "$(git -C "${source_dir}" rev-parse HEAD)" == "${commit}" ]] || {
  echo 'Unexpected upstream commit.' >&2; exit 1;
}
[[ -z "$(git -C "${source_dir}" status --porcelain --untracked-files=all)" ]] || {
  echo 'Upstream checkout has uncommitted or untracked files.' >&2; exit 1;
}
[[ ! -e "${output}" ]] || { echo 'Output already exists.' >&2; exit 1; }
[[ ! -e "${source_dir}/cmd/olcrtc/zz_naga_mark_linux.go" ]] || {
  echo 'Upstream checkout already contains the hook.' >&2; exit 1;
}
work="$(mktemp -d)"
trap 'rm -rf -- "${work}"' EXIT
python3 - "${source_dir}" "${script_dir}" "${work}/overlay.json" <<'PY'
import json, pathlib, sys
source, scripts, output = map(pathlib.Path, sys.argv[1:])
output.write_text(json.dumps({"Replace": {
    str(source / "cmd/olcrtc/zz_naga_mark_linux.go"):
        str(scripts / "naga_mark_linux.go.txt")
}}))
PY
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go -C "${source_dir}" build \
  -overlay "${work}/overlay.json" -trimpath -buildvcs=false \
  -o "${work}/olcrtc" ./cmd/olcrtc
# noclobber also rejects a concurrently created output.
set -o noclobber
cat "${work}/olcrtc" > "${output}"
chmod 755 "${output}"
echo "Built experimental olcRTC ${commit} with the socket-mark hook: ${output}"
echo 'Do not use for system VPN until routing and recovery are integrated.'
