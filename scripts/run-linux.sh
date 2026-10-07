#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=linux/run-linux-lib.sh
source "${project_dir}/scripts/linux/run-linux-lib.sh"
app_dir="${project_dir}/app"
flutter_bin="${FLUTTER_BIN:-flutter}"
control_url="${NAGA_CONTROL_URL:-http://127.0.0.1:8765}"
control_listen="${NAGA_CONTROL_LISTEN:-127.0.0.1:8765}"
control_token_file="${NAGA_CONTROL_TOKEN_FILE:-${HOME}/.local/share/naga-network/control.token}"
control_pid=""
control_log="${TMPDIR:-/tmp}/naga-network-control.log"
dev_control="${NAGA_DEV_CONTROL:-}" 

if ! command -v "${flutter_bin}" >/dev/null 2>&1; then
  echo "Flutter не найден. Задай FLUTTER_BIN или установи Flutter в PATH." >&2
  exit 1
fi

cleanup() {
  if [[ -n "${control_pid}" ]]; then
    kill "${control_pid}" 2>/dev/null || true
    wait "${control_pid}" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

health() {
	local -a curl_args=(--fail --silent --max-time 1)
  if [[ -n "${NAGA_CONTROL_TOKEN:-}" ]]; then
    curl_args+=(-H "Authorization: Bearer ${NAGA_CONTROL_TOKEN}")
  elif [[ -n "${control_token_file}" && -s "${control_token_file}" ]]; then
    curl_args+=(-H "Authorization: Bearer $(<"${control_token_file}")")
  fi
	local response
	response="$(curl "${curl_args[@]}" "${control_url}/v1/health" 2>/dev/null)" || return 1
	[[ "${response}" == *'"service":"naga-control"'* ]]
}

health_ok=0
if health; then
	health_ok=1
fi
url_explicit=0
if [[ "${NAGA_CONTROL_URL+x}" == x ]]; then
	url_explicit=1
fi
listen_explicit=0
if [[ "${NAGA_CONTROL_LISTEN+x}" == x ]]; then
	listen_explicit=1
fi
action="$(naga_plan_control_launch "${health_ok}" "${control_token_file}" "${dev_control}" "${url_explicit}" "${control_url}" "${listen_explicit}")"

if [[ "${action}" == "reuse" ]]; then
	echo "Использую уже работающий control-plane."
elif [[ "${action}" == "fail_prod" ]]; then
	echo "Production control-plane недоступен или это не naga-control. Второй control-plane не запускаю." >&2
	echo "Проверь: sudo systemctl status naga-network-control@$(id -un).service" >&2
	exit 1
elif [[ "${action}" == "fail_listen" ]]; then
	echo "NAGA_CONTROL_URL задан, но NAGA_CONTROL_LISTEN не указан — control-plane не запускаю автоматически." >&2
	exit 1
else
  if ! command -v go >/dev/null 2>&1; then
    echo "Go не найден в PATH — не удалось запустить control-plane." >&2
    exit 1
  fi
  echo "Запускаю локальный control-plane..."
  mapfile -t control_args < <(naga_control_args "${control_listen}" "${control_token_file}" "${dev_control}" "${NAGA_CONTROL_TOKEN:-}")
  (cd "${project_dir}" && go run ./cmd/naga-control "${control_args[@]}") >"${control_log}" 2>&1 &
  control_pid=$!
  ready=0
  for _ in {1..40}; do
    if health; then
      ready=1
      break
    fi
    sleep 0.25
  done
  if [[ "${ready}" -ne 1 ]]; then
    echo "control-plane не запустился. Лог: ${control_log}" >&2
    tail -40 "${control_log}" >&2 || true
    exit 1
  fi
fi

if [[ -z "${NAGA_CONTROL_TOKEN:-}" && -s "${control_token_file}" ]]; then
	export NAGA_CONTROL_TOKEN_FILE="${control_token_file}"
fi

if ! command -v "${flutter_bin}" >/dev/null 2>&1 && [[ ! -x "${flutter_bin}" ]]; then
  echo "Flutter не найден. Установи Flutter или укажи FLUTTER_BIN=/путь/к/flutter." >&2
  exit 1
fi

cd "${app_dir}"
echo "Запускаю Naga Network UI..."
"${flutter_bin}" pub get
"${flutter_bin}" run -d linux "$@"
