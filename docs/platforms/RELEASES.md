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

## Публикация desktop-релиза

Перед назначением релиза `Latest` загрузите Windows setup, Windows portable ZIP и Linux AppImage одной версии. Ссылка на прошлый Linux-релиз не заменяет ассет: updater ищет файл с версией текущего релиза. Старый AppImage нельзя переименовывать в новую версию.

Workflow [Linux AppImage](../../.github/workflows/linux-appimage.yml) собирает AppImage и проверяет установку службы и TUN на Ubuntu 22.04. Параметр `build_windows` также собирает Windows setup/ZIP и проверяет установку, helper, создание Wintun и удаление в отдельной Windows VM. Публикуйте файлы после успешных проверок, сверяйте GitHub `digest` с локальным SHA-256.
