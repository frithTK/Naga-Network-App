#!/usr/bin/env bash
set -euo pipefail
export GOTOOLCHAIN=go1.27.2
project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
output_dir="${1:?runtime output directory is required}"
source_root="${project_dir}/build/linux-runtime"
jobs="${NAGA_BUILD_JOBS:-4}"
mkdir -p "${output_dir}" "${source_root}"

checkout_source() {
  local name="$1" url="$2" commit="$3" source="${source_root}/$1"
  if [[ ! -d "${source}/.git" ]]; then
    git init "${source}"
  fi
  if ! git -C "${source}" rev-parse --verify HEAD >/dev/null 2>&1; then
    git -C "${source}" fetch --depth 1 "${url}" "${commit}"
    git -C "${source}" -c advice.detachedHead=false checkout --detach FETCH_HEAD
  fi
  [[ "$(git -C "${source}" rev-parse HEAD)" == "${commit}" ]] || {
    echo "Неверный source commit ${name}." >&2
    return 1
  }
}

echo "Сборка встроенного sing-box..."
checkout_source sing-box https://github.com/SagerNet/sing-box.git 56f91dfeabd6f4edbd437dfcc1e5b0ebc856b778
singbox_source="${source_root}/sing-box"
python3 "${project_dir}/scripts/runtime/apply-singbox-security-deps.py" "${singbox_source}"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -C "${singbox_source}" -p "${jobs}" \
  -trimpath -buildvcs=false \
  -tags "with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_clash_api,http2legacy" \
  -ldflags "-s -w -checklinkname=0 -X github.com/sagernet/sing-box/constant.Version=1.13.20" \
  -o "${output_dir}/sing-box" ./cmd/sing-box
cp "${singbox_source}/LICENSE" "${output_dir}/LICENSE.sing-box"

echo "Сборка встроенного olcRTC..."
checkout_source olcrtc https://github.com/ghostlane-project/olcrtc.git 7f849e08022ac4ce6dcdb1a49ad5560446daca1f
olc_source="${source_root}/olcrtc"
go -C "${olc_source}" get golang.org/x/net@v0.60.0 golang.org/x/crypto@v0.57.0 \
  golang.org/x/text@v0.42.0 github.com/klauspost/compress@v1.18.7 \
  google.golang.org/grpc@v1.83.2 github.com/google/cel-go@v0.30.0
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -C "${olc_source}" -p "${jobs}" \
  -trimpath -buildvcs=false -ldflags "-s -w" -o "${output_dir}/olcrtc" ./cmd/olcrtc
printf '7f849e08\n' >"${output_dir}/olcrtc.version"
cp "${olc_source}/LICENSE" "${output_dir}/LICENSE.olcrtc"

echo "Сборка встроенного hev-socks5-tunnel..."
checkout_source hev-socks5-tunnel https://github.com/heiher/hev-socks5-tunnel.git dec294e1402ebb0b2d78d1f6efebaba81511e960
hev_source="${source_root}/hev-socks5-tunnel"
git -C "${hev_source}" submodule update --init --recursive
make -C "${hev_source}" -j "${jobs}"
cp "${hev_source}/bin/hev-socks5-tunnel" "${output_dir}/hev-socks5-tunnel"
cp "${hev_source}/License" "${output_dir}/LICENSE.hev-socks5-tunnel"
chmod 755 "${output_dir}/sing-box" "${output_dir}/olcrtc" "${output_dir}/hev-socks5-tunnel"
