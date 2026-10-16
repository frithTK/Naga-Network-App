# GitHub Releases

- Тег: `vX.Y.Z`; версия пакетов — `X.Y.Z` из `app/pubspec.yaml`.
- Windows setup: `NagaNetwork-X.Y.Z-windows-x64-setup.exe`.
- Windows portable: `NagaNetwork-X.Y.Z-windows-x64.zip`.
- Linux: `NagaNetwork-X.Y.Z-x86_64.AppImage`.
- Android: `NagaNetwork-X.Y.Z-arm64-v8a.apk`.
- iOS: `NagaNetwork-X.Y.Z-ios.ipa`.

Перед `Latest`: собрать Windows setup/ZIP и Linux AppImage одной версии, пройти проверки, сверить GitHub `digest` с SHA-256 и приложить `SHA256SUMS.txt`.

Сборка и проверка desktop-пакетов: [workflow](../../.github/workflows/linux-appimage.yml), параметры `source_ref`, `build_linux`, `build_windows`.
AppImage содержит UI, runtime и установщик службы. Старый двухфайловый Linux-клиент обновляется вручную один раз.

Updater требует точное имя ассета и SHA-256 из GitHub API.
Для каждого публичного APK увеличивать `versionCode`; сохранять прежний keystore.
