#!/usr/bin/env bash
set -euo pipefail

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
  echo "Go не найден. Релиз кладёт AppImage и naga-control-linux-x86_64 вместе." >&2
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

cat >"${appdir}/AppRun" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
HERE="$(dirname "$(readlink -f "$0")")"
export LD_LIBRARY_PATH="${HERE}/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
exec "${HERE}/naga_network" "$@"
EOF
chmod +x "${appdir}/AppRun" "${appdir}/naga_network"

mkdir -p "${dist_dir}"
output="${dist_dir}/NagaNetwork-${version}-x86_64.AppImage"
echo "Упаковка ${output}..."
ARCH=x86_64 "${appimagetool}" "${appdir}" "${output}"
chmod +x "${output}"

control_output="${dist_dir}/naga-control-linux-x86_64"
echo "Сборка ${control_output} (version ${version})..."
CGO_ENABLED=0 go build -buildvcs=false \
  -ldflags "-s -w -X naga.network/core/version.Version=${version}" \
  -o "${control_output}" "${project_dir}/cmd/naga-control"
chmod +x "${control_output}"

echo "Готово:"
echo "  ${output}"
echo "  ${control_output}"
echo "AppImage — только UI. Кнопка обновления ставит оба файла; systemd unit по-прежнему указывает на ~/.local/bin/naga-control."
