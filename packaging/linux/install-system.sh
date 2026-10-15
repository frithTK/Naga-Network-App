#!/usr/bin/env bash
set -euo pipefail
runtime_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${runtime_dir}/runtime-lib.sh"
[[ "${EUID}" == "0" && "${PKEXEC_UID:-}" =~ ^[1-9][0-9]*$ ]] || {
  echo "Установщик службы запускается через pkexec из AppImage." >&2
  exit 1
}
user_record="$(getent passwd "${PKEXEC_UID}")"
IFS=: read -r user_name _ user_uid user_gid _ user_home _ <<<"${user_record}"
[[ "${user_uid}" == "${PKEXEC_UID}" && "${user_home}" == /* && -d "${user_home}" ]]
[[ "${user_name}" =~ ^[a-zA-Z_][a-zA-Z0-9_.@-]*\$?$ ]]
[[ "${user_home}" != *$'\n'* && "${user_home}" != *$'\r'* ]]
(cd "${runtime_dir}" && sha256sum --check --status runtime.sha256)
[[ "$(readlink -f "${user_home}/.local/bin/naga-control")" == "${runtime_dir}/naga-control" ]]
work="$(mktemp -d)"
stamp=""
trap 'rm -rf -- "${work}"; [[ -z "${stamp}" ]] || rm -f -- "${stamp}"' EXIT
naga_render_service "${runtime_dir}/naga-network-control@.service" \
  "${user_home}" "${user_gid}" "${work}/naga-network-control@.service"
# Reject invalid unit syntax before changing the running installation.
systemd-analyze verify "${work}/naga-network-control@.service"
sed "s|@NAGA_USER@|${user_name}|g" "${runtime_dir}/49-naga-network-dns.rules.in" >"${work}/polkit"
if [[ ! -e /dev/net/tun ]]; then modprobe tun; fi
[[ -e /dev/net/tun ]]
if ! getent group naga-network >/dev/null; then groupadd --system naga-network; fi
install -Dm644 "${runtime_dir}/70-naga-network-tun.rules" /etc/udev/rules.d/70-naga-network-tun.rules
udevadm control --reload-rules
udevadm trigger --subsystem-match=misc --sysname-match=tun
udevadm settle --timeout=15
if [[ "$(stat -c '%a' /dev/net/tun)" != "660" || "$(stat -c '%G' /dev/net/tun)" != "naga-network" ]]; then
  echo "Правило udev не применилось к /dev/net/tun: $(stat -c '%a %G' /dev/net/tun)" >&2
  exit 1
fi
install -Dm644 "${work}/naga-network-control@.service" /etc/systemd/system/naga-network-control@.service
install -Dm644 "${work}/polkit" /etc/polkit-1/rules.d/49-naga-network-dns.rules
# Migrate only the old service owned by this user, after authorization.
user_systemctl=(runuser -u "${user_name}" -- env \
  "XDG_RUNTIME_DIR=/run/user/${user_uid}" \
  "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/${user_uid}/bus" systemctl --user)
if "${user_systemctl[@]}" is-active --quiet naga-network-control.service 2>/dev/null; then
  "${user_systemctl[@]}" disable --now naga-network-control.service
fi
systemctl daemon-reload
systemctl enable "naga-network-control@${user_name}.service"
systemctl restart "naga-network-control@${user_name}.service"
naga_wait_control "${user_home}/.local/share/naga-network/control.token" "$(<"${runtime_dir}/runtime.version")"
main_pid="$(systemctl show --property=MainPID --value "naga-network-control@${user_name}.service")"
[[ "${main_pid}" =~ ^[1-9][0-9]*$ ]]
[[ "$(readlink -f "/proc/${main_pid}/exe")" == "${runtime_dir}/naga-control" ]]
stamp="$(mktemp "${user_home}/.local/share/naga-network/.runtime-id-XXXXXXXX")"
printf '%s\n' "$(<"${runtime_dir}/runtime.build-id")" >"${stamp}"
chmod 644 "${stamp}"
mv -Tf -- "${stamp}" "${user_home}/.local/share/naga-network/runtime.build-id"
