#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=run-linux-lib.sh
source "${root}/scripts/linux/run-linux-lib.sh"

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

pass() {
	echo "ok - $*"
}

default_url="$(naga_default_control_url)"
[[ "${default_url}" == "http://127.0.0.1:8765" ]] || fail "default URL"

action="$(naga_plan_control_launch 1 "" "" 0 "${default_url}" 0)"
[[ "${action}" == "reuse" ]] || fail "healthy default should reuse, got ${action}"
pass "already-running reuses control-plane"

token_file="$(mktemp)"
trap 'rm -f "${token_file}"' EXIT
printf 'super-secret-token\n' >"${token_file}"

action="$(naga_plan_control_launch 0 "${token_file}" "" 0 "${default_url}" 0)"
[[ "${action}" == "fail_prod" ]] || fail "down production API should not spawn second plane, got ${action}"
pass "foreign/down production API is fail_prod"

action="$(naga_plan_control_launch 0 "${token_file}" "1" 0 "${default_url}" 0)"
[[ "${action}" == "start" ]] || fail "dev override should start, got ${action}"
pass "NAGA_DEV_CONTROL starts local plane"

action="$(naga_plan_control_launch 0 "" "" 1 "http://127.0.0.1:9999" 0)"
[[ "${action}" == "fail_listen" ]] || fail "custom URL without listen should fail, got ${action}"
pass "custom URL without listen is fail_listen"

action="$(naga_plan_control_launch 0 "" "" 1 "http://127.0.0.1:9999" 1)"
[[ "${action}" == "start" ]] || fail "custom URL with listen should start, got ${action}"
pass "custom NAGA_CONTROL_LISTEN matching URL starts"

mapfile -t args < <(naga_control_args "127.0.0.1:8765" "${token_file}" "" "")
joined="${args[*]}"
[[ "${joined}" == *"--token-file ${token_file}"* ]] || fail "token-file missing from args: ${joined}"
[[ "${joined}" != *super-secret-token* ]] || fail "token leaked into argv: ${joined}"
naga_command_leaks_token "${joined}" "super-secret-token" && fail "token leak helper missed argv"
pass "token stays in header/file, not in argv"

empty="$(mktemp)"
: >"${empty}"
mapfile -t args < <(naga_control_args "127.0.0.1:8765" "${empty}" "" "env-token-value")
joined="${args[*]}"
[[ "${joined}" == *"--production"* ]] || fail "env token should enable production: ${joined}"
[[ "${joined}" != *env-token-value* ]] || fail "env token leaked into argv"
pass "NAGA_CONTROL_TOKEN is not copied to the command line"

echo "run-linux planner tests passed"
