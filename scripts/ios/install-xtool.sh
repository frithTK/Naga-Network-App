#!/usr/bin/env bash
set -euo pipefail

# Installs the xtool AppImage (Linux iOS toolchain). Setup itself is
# interactive: Apple login + Xcode.xip → Darwin Swift SDK.

version="${XTOOL_VERSION:-1.21.0}"
arch="$(uname -m)"
case "${arch}" in
  x86_64|aarch64) ;;
  *)
    echo "Неподдерживаемая архитектура: ${arch}" >&2
    exit 1
    ;;
esac

dest="${XTOOL_DEST:-${HOME}/.local/bin/xtool}"
mkdir -p "$(dirname "${dest}")"
url="https://github.com/xtool-org/xtool/releases/download/${version}/xtool-${arch}.AppImage"
tmp="$(mktemp)"
trap 'rm -f "${tmp}"' EXIT

echo "Скачиваю xtool ${version} (${arch})…"
curl -fL --retry 3 -o "${tmp}" "${url}"
chmod +x "${tmp}"
mv -f "${tmp}" "${dest}"
trap - EXIT

echo "xtool: ${dest}"
"${dest}" --version || "${dest}" --help | head -5

if ! command -v swift >/dev/null 2>&1; then
  echo >&2
  echo "Swift не найден. Нужен Swift 6.4+: https://www.swift.org/install/linux/" >&2
fi

if ! command -v usbmuxd >/dev/null 2>&1 && [[ ! -x /usr/bin/usbmuxd ]]; then
  echo "usbmuxd не найден. Поставь пакет usbmuxd для установки по USB." >&2
fi

cat <<'EOF'

Дальше один раз, интерактивно:

  xtool setup

Это запросит Apple ID / API Key (платный ADP для packet-tunnel-provider)
и Xcode.xip с https://developer.apple.com/download/all/ — из него xtool
соберёт Darwin Swift SDK. После этого:

  ./scripts/ios/dev.sh
  ./scripts/ios/build-ipa.sh

Документация: docs/platforms/IOS.md
EOF
