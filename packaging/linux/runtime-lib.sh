#!/usr/bin/env bash
# Shared by AppRun and the privileged system-service installer.

naga_render_service() {
  local template="$1" user_home="$2" user_gid="$3" output="$4"
  local command_home working_home
  [[ "${user_home}" == /* && "${user_gid}" =~ ^[0-9]+$ ]] || return 1
  # ExecStart parses quoted words; WorkingDirectory takes a literal path.
  command_home="${user_home//\\/\\\\}"
  command_home="${command_home//\"/\\\"}"
  command_home="${command_home//%/%%}"
  working_home="${user_home//%/%%}"
  # Escape the replacement text separately for sed, preserving systemd escapes.
  command_home="${command_home//\\/\\\\}"
  command_home="${command_home//&/\\&}"
  command_home="${command_home//|/\\|}"
  working_home="${working_home//\\/\\\\}"
  working_home="${working_home//&/\\&}"
  working_home="${working_home//|/\\|}"
  sed -e "s|@NAGA_HOME@|${command_home}|g" \
    -e "s|@NAGA_WORKING_HOME@|${working_home}|g" \
    -e "s|^Group=%i$|Group=${user_gid}|" "${template}" >"${output}"
}

naga_stage_runtime() (
  set -euo pipefail
  local source_dir="$1" cache_root="$2" build_id cached stage=""
  build_id="$(<"${source_dir}/runtime.build-id")"
  [[ "${build_id}" =~ ^[0-9a-f]{64}$ ]] || return 1
  [[ "$(sha256sum "${source_dir}/runtime.sha256" | cut -d' ' -f1)" == "${build_id}" ]] || return 1
  for name in naga-control sing-box olcrtc hev-socks5-tunnel olcrtc.version install-system.sh; do
    [[ -f "${source_dir}/${name}" && ! -L "${source_dir}/${name}" ]] || return 1
  done
  (cd "${source_dir}" && sha256sum --check --status runtime.sha256)
  mkdir -p "${cache_root}"
  chmod 700 "${cache_root}"
  cached="${cache_root}/${build_id}"
  if [[ -d "${cached}" && ! -L "${cached}" ]] &&
     (cd "${cached}" && sha256sum --check --status "${source_dir}/runtime.sha256"); then
    printf '%s\n' "${cached}"
    return
  fi
  # A changed existing cache is an error, never silently trusted or reused.
  [[ ! -e "${cached}" && ! -L "${cached}" ]] || return 1
  stage="$(mktemp -d "${cache_root}/.stage-XXXXXXXX")"
  trap '[[ -z "${stage}" ]] || rm -rf -- "${stage}"' EXIT
  cp -a "${source_dir}/." "${stage}/"
  (cd "${stage}" && sha256sum --check --status runtime.sha256)
  mv -T -- "${stage}" "${cached}"
  stage=""
  printf '%s\n' "${cached}"
)

naga_wait_control() {
  local token_file="$1" version="$2" token response
  [[ -s "${token_file}" ]] || return 1
  token="$(<"${token_file}")"
  token="${token//\\/\\\\}"
  token="${token//\"/\\\"}"
  for _ in {1..40}; do
    # The token is passed through stdin, not exposed in curl's process argv.
    response="$(printf 'header = "Authorization: Bearer %s"\n' "${token}" |
      curl --config - --noproxy '*' --fail --silent --max-time 1 \
        http://127.0.0.1:8765/v1/health 2>/dev/null)" || response=""
    if [[ "${response}" == *'"service":"naga-control"'* &&
          "${response}" == *"\"version\":\"${version}\""* ]]; then
      return 0
    fi
    sleep 0.25
  done
  return 1
}

naga_runtime_active() {
  local unit="$1" binary="$2" data_dir="$3"
  systemctl is-active --quiet "${unit}" 2>/dev/null || return 1
  [[ -f "${data_dir}/runtime.build-id" ]] || return 1
  # A service with ambient capabilities may deny /proc/PID/exe to its own uid.
  # The privileged installer records the verified build; health is also checked.
  [[ "$(<"${data_dir}/runtime.build-id")" == "$(<"$(dirname "${binary}")/runtime.build-id")" ]]
}

naga_prepare_runtime() (
  set -euo pipefail
  local source_dir="$1" cache_root="$2" bin_dir="$3" data_dir="$4" user_name="$5"
  local cached target previous="" swap="" token_tmp="" version
  mkdir -p "${data_dir}" "${bin_dir}"
  chmod 700 "${data_dir}"
  exec 9>"${data_dir}/runtime-install.lock"
  flock -x 9
  cached="$(naga_stage_runtime "${source_dir}" "${cache_root}")"
  version="$(<"${source_dir}/runtime.version")"
  target="${bin_dir}/naga-control"
  if [[ "$(readlink -f "${target}" 2>/dev/null || true)" == "${cached}/naga-control" ]] &&
     naga_runtime_active "naga-network-control@${user_name}.service" "${cached}/naga-control" "${data_dir}" &&
     naga_wait_control "${data_dir}/control.token" "${version}"; then
    return
  fi
  if [[ ! -s "${data_dir}/control.token" ]]; then
    token_tmp="$(mktemp "${data_dir}/.token-XXXXXXXX")"
    chmod 600 "${token_tmp}"
    od -An -N32 -tx1 /dev/urandom | tr -d ' \n' >"${token_tmp}"
    printf '\n' >>"${token_tmp}"
    mv -T -- "${token_tmp}" "${data_dir}/control.token"
  fi
  chmod 600 "${data_dir}/control.token"
  swap="$(mktemp -d "${bin_dir}/.naga-runtime-XXXXXXXX")"
  trap '[[ -z "${swap}" ]] || rm -rf -- "${swap}"' EXIT
  if [[ -e "${target}" || -L "${target}" ]]; then
    cp -a --no-dereference "${target}" "${swap}/previous"
    previous="${swap}/previous"
  fi
  ln -s "${cached}/naga-control" "${swap}/control"
  mv -Tf -- "${swap}/control" "${target}"
  if ! pkexec /bin/bash "${cached}/install-system.sh"; then
    if [[ -n "${previous}" ]]; then
      mv -Tf -- "${previous}" "${target}"
    else
      rm -f -- "${target}"
    fi
    echo "Не удалось подготовить службу Naga. Установка отменена или завершилась ошибкой." >&2
    return 1
  fi
  naga_wait_control "${data_dir}/control.token" "${version}"
)
