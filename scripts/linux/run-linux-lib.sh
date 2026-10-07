# Shared helpers for scripts/run-linux.sh. Sourced by the launcher and tests.
# Do not execute this file directly.

naga_default_control_url() {
	echo "http://127.0.0.1:8765"
}

naga_plan_control_launch() {
	local health_ok="${1}"
	local token_file="${2}"
	local dev_control="${3}"
	local url_explicit="${4}"
	local control_url="${5}"
	local listen_explicit="${6}"

	if [[ "${health_ok}" == "1" ]]; then
		echo "reuse"
		return 0
	fi
	if [[ -s "${token_file}" && "${dev_control}" != "1" ]]; then
		echo "fail_prod"
		return 0
	fi
	if [[ "${url_explicit}" == "1" && "${control_url}" != "$(naga_default_control_url)" && "${listen_explicit}" != "1" ]]; then
		echo "fail_listen"
		return 0
	fi
	echo "start"
}

naga_control_args() {
	local listen="${1}"
	local token_file="${2}"
	local dev_control="${3}"
	local env_token="${4}"
	local -a args=(--listen "${listen}")
	if [[ "${dev_control}" != "1" && -n "${token_file}" && -s "${token_file}" ]]; then
		args+=(--production --token-file "${token_file}")
	elif [[ -n "${env_token}" ]]; then
		args+=(--production)
	fi
	printf '%s\n' "${args[@]}"
}

naga_command_leaks_token() {
	local command_line="${1}"
	local token="${2}"
	[[ -n "${token}" && "${command_line}" == *"${token}"* ]]
}
