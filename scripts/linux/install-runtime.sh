#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
binary_path="${HOME}/.local/bin/naga-control"
data_dir="${HOME}/.local/share/naga-network"
token_file="${data_dir}/control.token"
unit_source="${project_dir}/platform/linux/systemd/naga-network-control@.service"
unit_target="/etc/systemd/system/naga-network-control@.service"
polkit_source="${project_dir}/platform/linux/polkit/49-naga-network-dns.rules.in"
polkit_target="/etc/polkit-1/rules.d/49-naga-network-dns.rules"
udev_source="${project_dir}/platform/linux/udev/70-naga-network-tun.rules"
udev_target="/etc/udev/rules.d/70-naga-network-tun.rules"
tun_group="naga-network"
user_name="$(id -un)"
user_home="$(getent passwd "${user_name}" | cut -d: -f6)"

if [[ -z "${user_home}" || ! -d "${user_home}" ]]; then
	echo "Не удалось определить домашний каталог пользователя ${user_name}." >&2
	exit 1
fi

if [[ ! -f "${unit_source}" ]]; then
  echo "Не найден systemd-шаблон: ${unit_source}" >&2
  exit 1
fi

if [[ ! -f "${polkit_source}" ]]; then
	echo "Не найден polkit rule: ${polkit_source}" >&2
	exit 1
fi

if [[ ! -f "${udev_source}" ]]; then
	echo "Не найден udev rule: ${udev_source}" >&2
	exit 1
fi

if [[ ! -e /dev/net/tun ]]; then
  echo "Не найден /dev/net/tun. Сначала выполни: sudo modprobe tun" >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "Go не найден в PATH." >&2
  exit 1
fi

sing_box_path="$(command -v sing-box || true)"
if [[ -z "${sing_box_path}" ]]; then
	echo "sing-box не найден в PATH." >&2
	exit 1
fi

if ! command -v setcap >/dev/null 2>&1; then
	echo "setcap не найден. Установи пакет libcap и повтори установку." >&2
	exit 1
fi

echo "Собираю naga-control..."
mkdir -p "$(dirname "${binary_path}")"
mkdir -p "${data_dir}"
chmod 700 "${data_dir}"
naga_version="$(awk '/^version:/ { print $2; exit }' "${project_dir}/app/pubspec.yaml" | cut -d+ -f1)"
if [[ -z "${naga_version}" ]]; then
  naga_version="dev"
fi
go build -buildvcs=false -ldflags "-X naga.network/core/version.Version=${naga_version}" -o "${binary_path}" "${project_dir}/cmd/naga-control"

if [[ ! -s "${token_file}" ]]; then
	token_tmp="$(mktemp "${token_file}.tmp.XXXXXX")"
	trap 'rm -f "${polkit_tmp-}" "${token_tmp-}"' EXIT
	if command -v openssl >/dev/null 2>&1; then
		openssl rand -hex 32 >"${token_tmp}"
	else
		od -An -N32 -tx1 /dev/urandom | tr -d ' \n' >"${token_tmp}"
	fi
	chmod 600 "${token_tmp}"
	mv -f "${token_tmp}" "${token_file}"
fi
chmod 600 "${token_file}"

echo "Устанавливаю systemd-сервис с доступом только к TUN..."
if ! getent group "${tun_group}" >/dev/null 2>&1; then
  sudo groupadd --system "${tun_group}"
fi
sudo install -Dm644 "${udev_source}" "${udev_target}"
sudo udevadm control --reload-rules
sudo udevadm trigger --subsystem-match=misc --sysname-match=tun
if [[ ! -e /dev/net/tun ]]; then
	echo "После применения udev rule не найден /dev/net/tun." >&2
	exit 1
fi
if [[ "$(stat -c '%a' /dev/net/tun)" != "660" ]] || [[ "$(stat -c '%G' /dev/net/tun)" != "${tun_group}" ]]; then
	echo "Права /dev/net/tun не стали ${tun_group}:660; установка остановлена." >&2
	stat -c '%A %U:%G %n' /dev/net/tun >&2 || true
	exit 1
fi
unit_tmp="$(mktemp)"
trap 'rm -f "${polkit_tmp-}" "${token_tmp-}" "${unit_tmp-}"' EXIT
sed "s|@NAGA_HOME@|${user_home}|g" "${unit_source}" >"${unit_tmp}"
sudo install -Dm644 "${unit_tmp}" "${unit_target}"
polkit_tmp="$(mktemp)"
trap 'rm -f "${polkit_tmp}" "${token_tmp-}" "${unit_tmp-}"' EXIT
sed "s/@NAGA_USER@/${user_name}/g" "${polkit_source}" > "${polkit_tmp}"
sudo install -Dm644 "${polkit_tmp}" "${polkit_target}"
# Do not leave a partial file capability on sing-box: it would clear the
# service's inherited CAP_NET_BIND_SERVICE when the binary is exec'd.
# setcap exits non-zero when the file already has no capabilities; that is
# the desired final state and must not abort the installation.
sudo setcap -r "${sing_box_path}" 2>/dev/null || true
systemctl --user disable --now naga-network-control.service 2>/dev/null || true
sudo systemctl daemon-reload
sudo systemctl enable "naga-network-control@${user_name}.service"
sudo systemctl restart "naga-network-control@${user_name}.service" 2>/dev/null || \
  sudo systemctl start "naga-network-control@${user_name}.service"

echo
echo "Пользователь ${user_name} не добавляется в группу ${tun_group} автоматически."
echo "Production: systemd SupplementaryGroups=${tun_group} + DeviceAllow=/dev/net/tun."
echo "Пользователь не обязан быть в группе, чтобы VPN работал через system service."
if [[ "${NAGA_ADD_USER_TO_TUN_GROUP:-}" == "1" ]]; then
	echo "NAGA_ADD_USER_TO_TUN_GROUP=1: добавляю ${user_name} в ${tun_group}."
	echo "После этого любой процесс этого uid сможет открыть TUN. Нужен повторный вход."
	sudo usermod -aG "${tun_group}" "${user_name}"
else
	echo "Opt-in fallback (не нужен для systemd-сервиса):"
	echo "  sudo usermod -aG ${tun_group} ${user_name}"
	echo "Предупреждение: после usermod любой процесс этого uid откроет /dev/net/tun."
fi

echo
echo "Проверка control-plane:"
for _ in {1..20}; do
  if curl --fail --silent --max-time 1 \
    -H "Authorization: Bearer $(<"${token_file}")" \
    http://127.0.0.1:8765/v1/health; then
    echo
    echo "Готово. Сервис naga-network-control@${user_name} запущен."
    exit 0
  fi
  sleep 0.25
done

echo "Сервис запущен, но control-plane не ответил за 5 секунд." >&2
sudo systemctl status "naga-network-control@${user_name}.service" --no-pager -n 20 >&2 || true
exit 1
