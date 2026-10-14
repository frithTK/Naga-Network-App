# Имена файлов в GitHub Releases

Кнопка «Проверить обновление» ищет ассеты **по точному имени**, не «первый zip в списке». Тег релиза `vX.Y.Z` совпадает с semver в `app/pubspec.yaml` до `+`. В именах файлов `<ver>` — это `X.Y.Z` **без** префикса `v`.

| Платформа | Файл |
|---|---|
| Linux | `NagaNetwork-<ver>-x86_64.AppImage` |
| Windows portable | `NagaNetwork-<ver>-windows-x64.zip` |
| Windows setup | `NagaNetwork-<ver>-windows-x64-setup.exe` |
| Android | `NagaNetwork-<ver>-arm64-v8a.apk` |
| iOS | `NagaNetwork-<ver>-ios.ipa` |

Linux-кнопка скачивает один AppImage. Внутри него UI, naga-control, sing-box, olcRTC, hev и установщик службы. Сборка: `./scripts/linux/build-appimage.sh`. Отдельный control-ассет больше не требуется. Старые версии с двухфайловым updater переходят на новый формат ручным скачиванием AppImage один раз.

Каждый публичный APK увеличивает число после `+` в pubspec (`versionCode`). Подпись APK — тем же keystore, что у первой публичной сборки.

После скачивания клиент обязательно сверяет SHA-256 с полем `digest` ассета в GitHub API; при отсутствии digest установка отклоняется.
