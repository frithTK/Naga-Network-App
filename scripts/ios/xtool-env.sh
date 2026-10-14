# Shared helpers for xtool-based iOS scripts. Source from other scripts.

naga_find_xtool() {
  if [[ -n "${XTOOL:-}" && -x "${XTOOL}" ]]; then
    printf '%s\n' "${XTOOL}"
    return 0
  fi
  if command -v xtool >/dev/null 2>&1; then
    command -v xtool
    return 0
  fi
  if [[ -x "${HOME}/.local/bin/xtool" ]]; then
    printf '%s\n' "${HOME}/.local/bin/xtool"
    return 0
  fi
  return 1
}

naga_require_xtool() {
  local path
  if ! path="$(naga_find_xtool)"; then
    echo "xtool не найден. Запусти ./scripts/ios/install-xtool.sh" >&2
    exit 1
  fi
  printf '%s\n' "${path}"
}

naga_darwin_sdk() {
  printf '%s\n' "${HOME}/.swiftpm/swift-sdks/darwin.artifactbundle"
}

# The old workaround modified the shared SDK linker. Refuse that SDK rather
# than executing an unknown binary patch during a new signed build.
naga_require_clean_linker() {
  local trampoline="$(naga_darwin_sdk)/toolset/bin/ld64.lld"
  if [[ -f "${trampoline}" ]] && grep -q 'naga-keep-text' "${trampoline}"; then
    echo "Darwin SDK содержит старый naga linker hook. Восстановите исходный SDK перед подписанной сборкой." >&2
    return 1
  fi
}
