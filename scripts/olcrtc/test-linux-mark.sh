#!/usr/bin/env bash
set -euo pipefail

# This only edits a disposable upstream checkout and an isolated network
# namespace. It does not alter the host's routes, DNS, or installed runtime.
source_dir="${1:?pass a disposable olcRTC source checkout}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
expected_commit="b8a6e12e3e3afb0666e38f9668c7d81d8aff948f"
[[ "$(git -C "${source_dir}" rev-parse HEAD)" == "${expected_commit}" ]] || {
  echo "Unexpected olcRTC checkout; refusing to install the experiment." >&2
  exit 1
}

hook="${source_dir}/cmd/olcrtc/zz_naga_mark_linux.go"
test_file="${source_dir}/cmd/olcrtc/zz_naga_mark_linux_test.go"
[[ ! -e "${hook}" && ! -e "${test_file}" ]] || {
  echo "Experiment files already exist; refusing to overwrite them." >&2
  exit 1
}
work_dir="$(mktemp -d)"
cleanup() { rm -f -- "${hook}" "${test_file}"; rm -rf -- "${work_dir}"; }
trap cleanup EXIT
cp -- "${script_dir}/naga_mark_linux.go.txt" "${hook}"
cp -- "${script_dir}/naga_mark_linux_test.go.txt" "${test_file}"

go -C "${source_dir}" test -count=1 ./cmd/olcrtc -run '^TestNaga'
go -C "${source_dir}" build -trimpath -o "${work_dir}/olcrtc" ./cmd/olcrtc
if NAGA_OLCRTC_SOCKET_MARK=invalid "${work_dir}/olcrtc" -version >/dev/null 2>&1; then
  echo "Invalid mark was accepted by the CLI." >&2
  exit 1
fi
if NAGA_OLCRTC_REQUIRE_MARK=1 NAGA_OLCRTC_SOCKET_MARK= "${work_dir}/olcrtc" -version >/dev/null 2>&1; then
  echo "Missing required mark was accepted by the CLI." >&2
  exit 1
fi
unshare --user --map-root-user --net bash -c '
  set -euo pipefail
  ip link set lo up
  ip addr add 198.18.0.2/32 dev lo
  ip rule del priority 0
  ip rule add priority 100 fwmark 0x5a table 100
  ip rule add priority 110 to 198.18.0.2/32 unreachable
  ip rule add priority 120 lookup local
  ip route add local 198.18.0.2/32 dev lo src 127.0.0.1 table 100
  ip -6 addr add 2001:db8::2/128 dev lo
  ip -6 rule del priority 0
  ip -6 rule add priority 100 fwmark 0x5a table 100
  ip -6 rule add priority 110 to 2001:db8::2/128 unreachable
  ip -6 rule add priority 120 lookup local
  ip -6 route add local 2001:db8::2/128 dev lo src ::1 table 100
  export NAGA_OLCRTC_SOCKET_MARK=0x5a
  export NAGA_OLCRTC_REQUIRE_MARK=1
  export NAGA_OLCRTC_MARK_ROUTE_TEST=1
  "$2" -version >/dev/null
  go -C "$1" test -count=1 ./cmd/olcrtc -run "^TestNaga"
  setpriv --no-new-privs --bounding-set=-net_admin,-net_raw \
    --inh-caps=-all --ambient-caps=-all env NAGA_OLCRTC_MARK_REFUSAL_TEST=1 \
    go -C "$1" test -count=1 ./cmd/olcrtc -run "^TestNagaMarkRefusal$"
' bash "${source_dir}" "${work_dir}/olcrtc"
