# Scripts

Скрипты сборки, проверки конфигураций, подготовки runtime и локальной диагностики.

## Быстрый запуск Linux

Из корня проекта:

```bash
./run.sh
```

Скрипт переиспользует уже работающий control-plane или запустит временный
локальный экземпляр, затем выполнит `flutter pub get` и запустит UI.

## AppImage (только UI)

Из корня проекта:

```bash
./scripts/linux/build-appimage.sh
```

Нужны Flutter, `appimagetool` в PATH и системные `gtk3` + `libayatana-appindicator`.
Артефакты: `dist/NagaNetwork-<version>-x86_64.AppImage` и
`dist/naga-control-linux-x86_64`. AppImage — только UI; бинарь control-plane
для кнопки обновления и для `~/.local/bin` после `install-runtime.sh`.
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

Артефакт: `dist/NagaNetwork-<version>-arm64-v8a.apk`. Нужен Android SDK.
Подпись release-keystore — до первой выкладки на GitHub Releases.

