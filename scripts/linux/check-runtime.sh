#!/usr/bin/env bash
set -euo pipefail

echo "Naga Network Linux runtime check"

if command -v sing-box >/dev/null 2>&1; then
  echo "sing-box: $(command -v sing-box)"
  sing-box version | head -1
else
  echo "sing-box: NOT FOUND"
fi

tun_mode=""
tun_group=""
if [[ -e /dev/net/tun ]]; then
  tun_mode="$(stat -c '%a' /dev/net/tun 2>/dev/null || echo unknown)"
  tun_group="$(stat -c '%G' /dev/net/tun 2>/dev/null || echo unknown)"
  if [[ "${tun_mode}" == "660" && "${tun_group}" == "naga-network" ]]; then
    echo "/dev/net/tun: present (${tun_group}:${tun_mode})"
  else
    echo "/dev/net/tun: present but permissions are unsafe (${tun_group}:${tun_mode})"
    echo "Do not chmod 0666. Re-run ./scripts/linux/install-runtime.sh"
  fi
else
  echo "/dev/net/tun: NOT FOUND"
fi

if command -v ip >/dev/null 2>&1; then
  echo "iproute2: $(command -v ip)"
else
  echo "iproute2: NOT FOUND"
fi

unit="naga-network-control@$(id -un)"
if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet "${unit}" 2>/dev/null; then
  echo "system unit ${unit}: active"
else
  echo "system unit ${unit}: inactive"
  echo "Install with ./scripts/linux/install-runtime.sh"
fi

if id -nG 2>/dev/null | grep -qw naga-network; then
  echo "user in naga-network: yes (opt-in; any process of this uid can open TUN)"
else
  echo "user in naga-network: no (expected for production systemd SupplementaryGroups)"
fi

incomplete=0
if [[ ! -e /dev/net/tun ]] || [[ "${tun_mode}" != "660" ]] || [[ "${tun_group}" != "naga-network" ]] || ! command -v sing-box >/dev/null 2>&1; then
  incomplete=1
  echo "runtime prerequisites: incomplete"
else
  echo "runtime prerequisites: ready for config check"
fi

if [[ "${STRICT:-}" == "1" && "${incomplete}" -eq 1 ]]; then
  exit 1
fi
exit 0
