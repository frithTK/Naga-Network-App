#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
app_dir="${project_dir}/app"
dist_dir="${project_dir}/dist"
flutter_bin="${FLUTTER_BIN:-flutter}"

if ! command -v "${flutter_bin}" >/dev/null 2>&1; then
  echo "Flutter не найден. Задай FLUTTER_BIN или установи Flutter в PATH." >&2
  exit 1
fi

version="$(awk '/^version:/ { print $2; exit }' "${app_dir}/pubspec.yaml" | cut -d+ -f1)"
if [[ -z "${version}" ]]; then
  version="0.0.0"
fi

echo "Сборка Android APK arm64-v8a (version ${version})..."
(
  cd "${app_dir}"
  "${flutter_bin}" build apk --release --target-platform android-arm64 --dart-define="NAGA_VERSION=${version}"
)

apk="${app_dir}/build/app/outputs/flutter-apk/app-release.apk"
if [[ ! -f "${apk}" ]]; then
  echo "Не найден APK: ${apk}" >&2
  exit 1
fi

mkdir -p "${dist_dir}"
output="${dist_dir}/NagaNetwork-${version}-arm64-v8a.apk"
cp -f "${apk}" "${output}"
echo "Готово: ${output}"
echo "Подпиши release keystore до первой публичной выкладки. Другой ключ обновление не примет."
echo "versionCode берётся из числа после + в pubspec.yaml и должен расти на каждый APK."
