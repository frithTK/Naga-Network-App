# Имена файлов в GitHub Releases

Кнопка «Проверить обновление» ищет ассеты **по точному имени**, не «первый zip в списке». Тег релиза `vX.Y.Z` совпадает с semver в `app/pubspec.yaml` до `+`. В именах файлов `<ver>` — это `X.Y.Z` **без** префикса `v`.

| Платформа | Файл |
|---|---|
| Linux UI | `NagaNetwork-<ver>-x86_64.AppImage` |
| Linux runtime | `naga-control-linux-x86_64` |
| Windows portable | `NagaNetwork-<ver>-windows-x64.zip` |
| Windows setup | `NagaNetwork-<ver>-windows-x64-setup.exe` |
| Android | `NagaNetwork-<ver>-arm64-v8a.apk` |

Linux-кнопка не ставит AppImage, пока в том же релизе нет `naga-control-linux-x86_64`. Сборка: `./scripts/linux/build-appimage.sh` (кладёт оба файла в `dist/`).

Каждый публичный APK увеличивает число после `+` в pubspec (`versionCode`). Подпись APK — тем же keystore, что у первой публичной сборки.

После скачивания клиент сверяет SHA-256 с полем `digest` ассета в GitHub API, если оно есть.
