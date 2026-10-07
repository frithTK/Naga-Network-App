#!/usr/bin/env bash
set -euo pipefail

SDK="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-${HOME}/Android/Sdk}}"
AVD="${ANDROID_AVD:-Naga_API36}"
emulator_bin="${SDK}/emulator/emulator"

if [[ ! -x "${emulator_bin}" ]]; then
  echo "Эмулятор не найден: ${emulator_bin}" >&2
  echo "Поставь пакет emulator в ${SDK}." >&2
  exit 1
fi

export ANDROID_HOME="${SDK}"
export ANDROID_SDK_ROOT="${SDK}"
export PATH="${SDK}/emulator:${SDK}/platform-tools:${PATH}"
# Qt в эмуляторе Google не умеет native Wayland.
unset WAYLAND_DISPLAY
export QT_QPA_PLATFORM="${QT_QPA_PLATFORM:-xcb}"

if ! "${emulator_bin}" -list-avds | grep -qx "${AVD}"; then
  echo "Нет AVD «${AVD}». Есть:" >&2
  "${emulator_bin}" -list-avds >&2
  exit 1
fi

echo "Запуск ${AVD} (KVM)..."
exec "${emulator_bin}" -avd "${AVD}" -gpu auto -no-snapshot-save "$@"
