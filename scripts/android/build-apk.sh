#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
app_dir="${project_dir}/app"
dist_dir="${project_dir}/dist"
jni_dir="${app_dir}/android/app/src/main/jniLibs/arm64-v8a"
work_dir="${project_dir}/build/android-native"
flutter_bin="${FLUTTER_BIN:-flutter}"
api="${ANDROID_API:-26}"
page_size_ld="-Wl,-z,max-page-size=16384"
singbox_ref="${SINGBOX_REF:-v1.13.20}"
hev_ref="${HEV_REF:-2.6.8}"
olc_commit="${OLCRTC_COMMIT:-7f849e08}"

if ! command -v "${flutter_bin}" >/dev/null 2>&1; then
  echo "Flutter не найден. Задай FLUTTER_BIN или установи Flutter в PATH." >&2
  exit 1
fi
if ! command -v go >/dev/null 2>&1; then
  echo "Go не найден в PATH." >&2
  exit 1
fi

find_ndk() {
  if [[ -n "${ANDROID_NDK_HOME:-}" && -d "${ANDROID_NDK_HOME}" ]]; then
    printf '%s\n' "${ANDROID_NDK_HOME}"
    return
  fi
  if [[ -n "${ANDROID_NDK_ROOT:-}" && -d "${ANDROID_NDK_ROOT}" ]]; then
    printf '%s\n' "${ANDROID_NDK_ROOT}"
    return
  fi
  local sdk="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-}}"
  if [[ -n "${sdk}" && -d "${sdk}/ndk" ]]; then
    ls -1d "${sdk}/ndk"/* 2>/dev/null | sort -V | tail -1
    return
  fi
  if [[ -d "${HOME}/Android/Sdk/ndk" ]]; then
    ls -1d "${HOME}/Android/Sdk/ndk"/* 2>/dev/null | sort -V | tail -1
    return
  fi
}

ndk="$(find_ndk || true)"
if [[ -z "${ndk}" || ! -d "${ndk}" ]]; then
  echo "Не найден Android NDK. Задай ANDROID_NDK_HOME (API ${api}, llvm aarch64)." >&2
  exit 1
fi

host_tag=""
for candidate in linux-x86_64 linux-aarch64 darwin-x86_64 darwin-arm64; do
  if [[ -d "${ndk}/toolchains/llvm/prebuilt/${candidate}" ]]; then
    host_tag="${candidate}"
    break
  fi
done
if [[ -z "${host_tag}" ]]; then
  echo "В NDK нет llvm prebuilt toolchain." >&2
  exit 1
fi

cc="${ndk}/toolchains/llvm/prebuilt/${host_tag}/bin/aarch64-linux-android${api}-clang"
if [[ ! -x "${cc}" ]]; then
  echo "Нет компилятора: ${cc}" >&2
  exit 1
fi

version="$(awk '/^version:/ { print $2; exit }' "${app_dir}/pubspec.yaml" | cut -d+ -f1)"
if [[ -z "${version}" ]]; then
  version="0.0.0"
fi

mkdir -p "${jni_dir}" "${work_dir}" "${dist_dir}"

export CC="${cc}"
export CXX="${ndk}/toolchains/llvm/prebuilt/${host_tag}/bin/aarch64-linux-android${api}-clang++"
export AR="${ndk}/toolchains/llvm/prebuilt/${host_tag}/bin/llvm-ar"
export CGO_ENABLED=1
export GOOS=android
export GOARCH=arm64
export CGO_CFLAGS="-O2 -fPIC"
export CGO_LDFLAGS="${page_size_ld}"

echo "Сборка libnaga.so..."
go build -buildvcs=false -buildmode=c-shared \
  -ldflags "-s -w -X naga.network/core/version.Version=${version} -linkmode=external -extldflags ${page_size_ld}" \
  -o "${jni_dir}/libnaga.so" "${project_dir}/cmd/naga-android"
rm -f "${jni_dir}/libnaga.h"

echo "Сборка libsingbox.so (${singbox_ref})..."
singbox_dir="${work_dir}/sing-box"
if [[ ! -d "${singbox_dir}/.git" ]]; then
  rm -rf "${singbox_dir}"
  git clone --depth 1 --branch "${singbox_ref}" https://github.com/SagerNet/sing-box.git "${singbox_dir}"
fi
patch_file="${project_dir}/scripts/android/patches/sing-box-android-cli.patch"
if [[ -f "${patch_file}" ]]; then
  if ! git -C "${singbox_dir}" apply --reverse --check "${patch_file}" >/dev/null 2>&1; then
    git -C "${singbox_dir}" apply "${patch_file}"
  fi
fi
(
  cd "${singbox_dir}"
  go build -trimpath -tags "with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_clash_api" \
    -ldflags "-s -w -checklinkname=0 -linkmode=external -extldflags ${page_size_ld}" \
    -o "${jni_dir}/libsingbox.so" ./cmd/sing-box
)

echo "Сборка libhevfd.so (${hev_ref})..."
hev_dir="${work_dir}/hev-socks5-tunnel"
if [[ ! -d "${hev_dir}/.git" ]]; then
  rm -rf "${hev_dir}"
  git clone --depth 1 --recurse-submodules --branch "${hev_ref}" \
    https://github.com/heiher/hev-socks5-tunnel.git "${hev_dir}"
fi
git -C "${hev_dir}" submodule update --init --recursive
(
  cd "${hev_dir}"
  make clean >/dev/null 2>&1 || true
  # NDK 28 already typedefs fd_set; lwip's unix port must not redefine it.
  make static -j"$(nproc)" \
    CC="${cc}" \
    AR="${AR}" \
    STRIP="${ndk}/toolchains/llvm/prebuilt/${host_tag}/bin/llvm-strip" \
    CFLAGS="-DFD_SET_DEFINED -fPIC -O2"
  "${cc}" -O2 -fPIC "${page_size_ld}" -o "${jni_dir}/libhevfd.so" \
    "${project_dir}/scripts/android/hev-fd-main.c" \
    -Wl,--whole-archive "${hev_dir}/bin/libhev-socks5-tunnel.a" -Wl,--no-whole-archive \
    -L"${hev_dir}/third-part/yaml/bin" -lyaml \
    -L"${hev_dir}/third-part/lwip/bin" -llwip \
    -L"${hev_dir}/third-part/hev-task-system/bin" -lhev-task-system \
    -llog
)
"${ndk}/toolchains/llvm/prebuilt/${host_tag}/bin/llvm-strip" "${jni_dir}/libhevfd.so"
chmod +x "${jni_dir}/libhevfd.so" "${jni_dir}/libsingbox.so" "${jni_dir}/libnaga.so"

echo "Spike olcRTC ${olc_commit} (не блокер APK)..."
olc_dir="${work_dir}/olcrtc"
set +e
if [[ ! -d "${olc_dir}/.git" ]]; then
  rm -rf "${olc_dir}"
  git clone --depth 50 https://github.com/ghostlane-project/olcrtc.git "${olc_dir}"
fi
(
  cd "${olc_dir}"
  git fetch --depth 50 origin "${olc_commit}" >/dev/null 2>&1
  git checkout "${olc_commit}" >/dev/null 2>&1
  if [[ -f Makefile ]]; then
    make clean >/dev/null 2>&1
    make -j"$(nproc)" CC="${cc}" CGO_ENABLED=0 GOOS=android GOARCH=arm64
  fi
  built="$(find . -maxdepth 3 -type f -name olcrtc -o -name libolcrtc.so | head -1)"
  if [[ -n "${built}" && -f "${built}" ]]; then
    cp -f "${built}" "${jni_dir}/libolcrtc.so"
    chmod +x "${jni_dir}/libolcrtc.so"
    printf '%s\n' "${olc_commit}" >"${jni_dir}/olcrtc.version"
    echo "olcRTC: ${jni_dir}/libolcrtc.so"
  else
    rm -f "${jni_dir}/libolcrtc.so" "${jni_dir}/olcrtc.version"
    echo "olcRTC: нет Android-таргета, UI покажет недоступно."
  fi
)
set -e

echo "16 KB ELF check..."
python3 - <<'PY' "${jni_dir}" || true
import pathlib, struct, sys
root = pathlib.Path(sys.argv[1])
for path in sorted(root.glob("lib*.so")):
    data = path.read_bytes()
    if data[:4] != b"\x7fELF":
        print(f"{path.name}: not ELF", file=sys.stderr)
        continue
    ei_class = data[4]
    if ei_class != 2:
        print(f"{path.name}: not ELF64", file=sys.stderr)
        continue
    e_phoff = struct.unpack_from("<Q", data, 32)[0]
    e_phentsize = struct.unpack_from("<H", data, 54)[0]
    e_phnum = struct.unpack_from("<H", data, 56)[0]
    align = 0
    for i in range(e_phnum):
        off = e_phoff + i * e_phentsize
        p_type = struct.unpack_from("<I", data, off)[0]
        if p_type != 1:
            continue
        p_align = struct.unpack_from("<Q", data, off + 48)[0]
        if p_align > align:
            align = p_align
    ok = "ok" if align >= 16384 else "NEED 16KB"
    print(f"{path.name}: p_align={align} {ok}")
PY

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

output="${dist_dir}/NagaNetwork-${version}-arm64-v8a.apk"
cp -f "${apk}" "${output}"
echo "Готово: ${output}"
if [[ ! -f "${app_dir}/android/key.properties" ]]; then
  echo "Подпиши release keystore до первой публичной выкладки (app/android/key.properties). Другой ключ обновление не примет."
fi
echo "versionCode берётся из числа после + в pubspec.yaml и должен расти на каждый APK."
