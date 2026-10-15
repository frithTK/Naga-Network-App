#!/usr/bin/env bash
# Integration check for a disposable GitHub-hosted Ubuntu runner.
set -euo pipefail
[[ "${GITHUB_ACTIONS:-}" == true ]] || { echo 'This check installs a service and requires a disposable CI runner.' >&2; exit 2; }
image="$(readlink -f "${1:?AppImage path is required}")"
version="$(basename "${image}" | sed -E 's/^NagaNetwork-(.*)-x86_64.AppImage$/\1/')"
work="$(mktemp -d "${RUNNER_TEMP:?}/naga-verify-XXXXXXXX")"
cd "${work}"
"${image}" --appimage-extract >/dev/null
appdir="${work}/squashfs-root"
[[ "$("${appdir}/AppRun" --version)" == "Naga Network ${version}" ]]
for file in naga-control sing-box olcrtc hev-socks5-tunnel install-system.sh runtime-lib.sh \
  naga-network-control@.service 49-naga-network-dns.rules.in 70-naga-network-tun.rules; do
  [[ -s "${appdir}/runtime/${file}" ]]
done
(cd "${appdir}/runtime" && sha256sum --check runtime.sha256)
[[ "$(sha256sum "${appdir}/runtime/runtime.sha256" | cut -d' ' -f1)" == "$(<"${appdir}/runtime/runtime.build-id")" ]]
[[ -x "${appdir}/naga_network" && -s "${appdir}/lib/libflutter_linux_gtk.so" ]]
libraries="$(ldd "${appdir}/naga_network")"
if grep -q 'not found' <<<"${libraries}"; then echo "${libraries}"; exit 1; fi
"${appdir}/runtime/sing-box" version
cat >"${work}/tun-check.json" <<'JSON'
{"inbounds":[{"type":"tun","interface_name":"naga-smoke0","address":["172.29.254.1/30"],"auto_route":false,"stack":"gvisor"}],"outbounds":[{"type":"direct","tag":"direct"}]}
JSON
"${appdir}/runtime/sing-box" check -c "${work}/tun-check.json"

# Grant the installer its normal pkexec authorization through sudo in CI.
# The same production AppRun and install-system.sh perform all installation.
mkdir "${work}/bin"
cat >"${work}/bin/pkexec" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'authorized\n' >> "${NAGA_CI_AUTH_LOG:?}"
exec sudo env "PKEXEC_UID=$(id -u)" "$@"
SH
chmod +x "${work}/bin/pkexec"
export PATH="${work}/bin:${PATH}"
export NAGA_CI_AUTH_LOG="${work}/authorization.log"
"${appdir}/AppRun" --install-runtime
unit="naga-network-control@$(id -un).service"
trap 'sudo systemctl stop naga-tun-ci.service "${unit}" 2>/dev/null || true' EXIT
systemctl is-active --quiet "${unit}"
[[ "$(stat -c '%a:%G' /dev/net/tun)" == 660:naga-network ]]
[[ "$(stat -c '%a' "${HOME}/.local/share/naga-network/control.token")" == 600 ]]
"${appdir}/AppRun" --install-runtime
[[ "$(wc -l <"${NAGA_CI_AUTH_LOG}")" == 1 ]]
python3 - "${version}" <<'PY'
import json
from pathlib import Path
import sys
from urllib.error import HTTPError
from urllib.request import Request, urlopen
url = 'http://127.0.0.1:8765/v1/health'
token = (Path.home() / '.local/share/naga-network/control.token').read_text().strip()
request = Request(url, headers={'Authorization': f'Bearer {token}'})
with urlopen(request, timeout=5) as response:
    health = json.load(response)
assert health['version'] == sys.argv[1] and health['status'] == 'ok', health
assert health['runtime_ready'] and health['olcrtc_ready'], health
assert health['capabilities']['node_probe_jobs'], health
try:
    urlopen(url, timeout=5)
except HTTPError as error:
    assert error.code == 401, error.code
else:
    raise AssertionError('API accepted an unauthenticated request')
print(json.dumps(health))
PY
sudo systemd-run --unit=naga-tun-ci \
  --property="User=$(id -un)" --property="Group=$(id -gn)" \
  --property=SupplementaryGroups=naga-network \
  --property=AmbientCapabilities=CAP_NET_ADMIN \
  --property=CapabilityBoundingSet=CAP_NET_ADMIN \
  --property=RuntimeMaxSec=30 \
  "${appdir}/runtime/sing-box" run -c "${work}/tun-check.json"
tun_ready=false
for _ in {1..40}; do
  if ip link show naga-smoke0 >/dev/null 2>&1; then tun_ready=true; break; fi
  sleep 0.25
done
if [[ "${tun_ready}" != true ]]; then
  sudo journalctl -u naga-tun-ci --no-pager -n 50
  exit 1
fi
sudo systemctl stop naga-tun-ci.service
echo 'AppImage: checksums, service install, repeat launch, API authentication and real TUN creation passed.'
