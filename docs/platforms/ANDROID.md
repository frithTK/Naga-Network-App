# Android

Sideload APK из [Releases](https://github.com/frithTK/Naga-Network-App/releases). Play Store в текущем контуре нет.

Имя ассета: `NagaNetwork-<ver>-arm64-v8a.apk` (см. [контракт релизов](RELEASES.md)). Сборка: `./scripts/android/build-apk.sh`. `minSdk` 26, ABI `arm64-v8a`. `versionCode` — число после `+` в `app/pubspec.yaml`; его нужно поднимать на каждый публичный APK.

Обновление: тумблер «Автообновление приложения» (проверка при запуске) или кнопка «Проверить обновление» → скачать APK → системный диалог PackageInstaller. Нужно разрешение «установка неизвестных приложений» для Naga Network. Первый публичный APK подписывай **release** keystore (не debug). Keystore не в git. Другой ключ обновление не примет — только удалить и поставить заново.

VPN-клиент (VpnService, TUN) — отдельный план; этот документ фиксирует контракт обновления APK.
