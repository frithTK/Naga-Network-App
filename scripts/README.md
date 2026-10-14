# Scripts

Скрипты сборки, проверки конфигураций, подготовки runtime и локальной диагностики.

## Быстрый запуск Linux

Из корня проекта:

```bash
./run.sh
```

Скрипт переиспользует уже работающий control-plane или запустит временный
локальный экземпляр, затем выполнит `flutter pub get` и запустит UI.

## Единый Linux AppImage

Из корня проекта:

```bash
./scripts/linux/build-appimage.sh
```

Нужны Flutter, Go 1.27.2, Python 3, Git, make/GCC, appimagetool и системные
gtk3 + libayatana-appindicator. Движки собираются из закреплённых commits;
первой сборке нужен доступ к их исходникам и зависимостям.
Артефакт: `dist/NagaNetwork-<version>-x86_64.AppImage`. Внутри UI, control,
sing-box, olcRTC, hev и установщик systemd-службы. Отдельный runtime не скачивается.
Имена на GitHub: [RELEASES.md](../docs/platforms/RELEASES.md).

## Portable Windows (UI + control-plane)

Flutter Windows exe **не** собирается с Linux. На ВМ Windows 10/11 x64 нужен
ещё [Inno Setup 6](https://jrsoftware.org/isinfo.php) (`ISCC.exe`), либо
`INNO_SETUP_ISCC`:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\windows\build.ps1
```

Артефакты: `dist/windows/NagaNetwork/`, zip и `*-setup.exe`. В бандл копируется
MSVC CRT из VS 2022, отдельный VC++ Redistributable не нужен. Подпись:
`NAGA_SIGN_PFX` или `NAGA_SIGN_SHA1`. Без ISCC zip собирается, установщик — нет.
Либо workflow `Windows portable` в GitHub Actions (там Inno Setup ставит
chocolatey). Подробности: [WINDOWS.md](../docs/platforms/WINDOWS.md).
В бандл входят `sing-box.exe` 1.13.20 и `wintun.dll`. Setup просит UAC один
раз и регистрирует elevated-helper; дальше «Подключить» без запроса. Portable
без helper — UAC на подключение. В zip также `naga-update.exe` для кнопки
обновления portable-сборки.

## Android APK (arm64)

```bash
./scripts/android/build-apk.sh
```

Артефакт: `dist/NagaNetwork-<version>-arm64-v8a.apk`. Нужны Flutter, Go с CGO и Android NDK (API 26, `aarch64-linux-android26-clang`, `ANDROID_NDK_HOME`). Скрипт кладёт `libnaga.so`, `libsingbox.so`, `libhevfd.so` (и olcRTC, если NDK-сборка прошла) в `jniLibs/arm64-v8a`. Подпись release-keystore (`app/android/key.properties`) — до первой выкладки на GitHub Releases. Подробности: [ANDROID.md](../docs/platforms/ANDROID.md).

## iOS IPA (Linux, xtool)

```bash
./scripts/ios/install-xtool.sh
xtool setup
./scripts/ios/dev.sh
./scripts/ios/build-ipa.sh
```

Сборка без Mac: [xtool](https://github.com/xtool-org/xtool) + SwiftPM-проект [`ios/`](../ios). Нужны Swift 6.4+, `usbmuxd`, Xcode.xip (Darwin SDK) и платный Apple Developer Program для Packet Tunnel. Артефакт: `dist/NagaNetwork-<version>-ios.ipa`. Подробности: [IOS.md](../docs/platforms/IOS.md).

`scripts/ios/build-xcframework.sh` — только macOS, для запасного Flutter-каркаса в `app/ios`.

Тесты Swift API-клиента без Apple UI-фреймворков: из `ios/` — `swift test`.
`go run ./scripts/ios/export-api-fixtures.go` из корня обновляет тестовые JSON
на основе настоящего Go API (изолированное хранилище, без подключения VPN).

Локальный эмулятор (Pixel 7, API 36, x86_64):

```bash
./scripts/android/run-emulator.sh
```

