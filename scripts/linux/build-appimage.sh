#!/usr/bin/env bash
set -euo pipefail

# Keep nested engine modules on the same patched compiler as naga-control.
export GOTOOLCHAIN=go1.27.2

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
app_dir="${project_dir}/app"
dist_dir="${project_dir}/dist"
work_dir="${project_dir}/build/appimage"
desktop_src="${project_dir}/packaging/linux/naga-network.desktop"
icon_src="${app_dir}/assets/brand/naga-mark-red.png"
flutter_bin="${FLUTTER_BIN:-flutter}"

if ! command -v "${flutter_bin}" >/dev/null 2>&1; then
  echo "Flutter не найден. Задай FLUTTER_BIN или установи Flutter в PATH." >&2
  exit 1
fi

if [[ ! -f "${desktop_src}" ]]; then
  echo "Не найден desktop-файл: ${desktop_src}" >&2
  exit 1
fi

if [[ ! -f "${icon_src}" ]]; then
  echo "Не найдена иконка: ${icon_src}" >&2
  exit 1
fi

appimagetool=""
if command -v appimagetool >/dev/null 2>&1; then
  appimagetool="$(command -v appimagetool)"
elif [[ -x "${HOME}/.local/bin/appimagetool" ]]; then
  appimagetool="${HOME}/.local/bin/appimagetool"
fi

if [[ -z "${appimagetool}" ]]; then
  echo "Не найден appimagetool. Установи appimagetool в PATH и повтори." >&2
  exit 1
fi

version="$(awk '/^version:/ { print $2; exit }' "${app_dir}/pubspec.yaml" | cut -d+ -f1)"
if [[ -z "${version}" ]]; then
  version="0.0.0"
fi

if ! command -v go >/dev/null 2>&1; then
  echo "Go не найден. AppImage содержит встроенный naga-control." >&2
  exit 1
fi

echo "Сборка Flutter Linux release (version ${version})..."
(
  cd "${app_dir}"
  "${flutter_bin}" build linux --release --build-name="${version}" --dart-define="NAGA_VERSION=${version}"
)

bundle="${app_dir}/build/linux/x64/release/bundle"
if [[ ! -x "${bundle}/naga_network" ]]; then
  echo "Не найден бандл: ${bundle}/naga_network" >&2
  exit 1
fi

rm -rf "${work_dir}"
appdir="${work_dir}/NagaNetwork.AppDir"
mkdir -p "${appdir}"
cp -a "${bundle}/." "${appdir}/"
cp "${desktop_src}" "${appdir}/naga-network.desktop"
cp "${icon_src}" "${appdir}/naga-mark-red.png"

runtime_dir="${appdir}/runtime"
mkdir -p "${runtime_dir}"
echo "Сборка встроенного naga-control (version ${version})..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false \
  -ldflags "-s -w -X naga.network/core/version.Version=${version}" \
  -o "${runtime_dir}/naga-control" "${project_dir}/cmd/naga-control"
bash "${project_dir}/scripts/linux/build-runtime.sh" "${runtime_dir}"
cp "${project_dir}/packaging/linux/AppRun" "${appdir}/AppRun"
cp "${project_dir}/packaging/linux/runtime-lib.sh" "${runtime_dir}/"
cp "${project_dir}/packaging/linux/install-system.sh" "${runtime_dir}/"
cp "${project_dir}/platform/linux/systemd/naga-network-control@.service" "${runtime_dir}/"
cp "${project_dir}/platform/linux/polkit/49-naga-network-dns.rules.in" "${runtime_dir}/"
cp "${project_dir}/platform/linux/udev/70-naga-network-tun.rules" "${runtime_dir}/"
printf '%s\n' "${version}" >"${runtime_dir}/runtime.version"
chmod 755 "${runtime_dir}/naga-control" "${runtime_dir}/install-system.sh"
(
  cd "${runtime_dir}"
  find . -maxdepth 1 -type f ! -name runtime.sha256 ! -name runtime.build-id -print0 |
    sort -z | xargs -0 sha256sum >runtime.sha256
  sha256sum runtime.sha256 | cut -d' ' -f1 >runtime.build-id
)
chmod +x "${appdir}/AppRun" "${appdir}/naga_network"

mkdir -p "${dist_dir}"
output="${dist_dir}/NagaNetwork-${version}-x86_64.AppImage"
echo "Упаковка ${output}..."
ARCH=x86_64 "${appimagetool}" "${appdir}" "${output}"
chmod +x "${output}"

echo "Готово:"
echo "  ${output}"
echo "AppImage содержит UI, naga-control, sing-box, olcRTC и hev. При первом запуске он подготовит системную службу."
