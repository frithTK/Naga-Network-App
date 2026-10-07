# Android

Sideload APK из [Releases](https://github.com/frithTK/Naga-Network-App/releases). Play Store в текущем контуре нет.

Имя ассета: `NagaNetwork-<ver>-arm64-v8a.apk` (см. [контракт релизов](RELEASES.md)). Сборка: `./scripts/android/build-apk.sh`. `minSdk` 26, ABI `arm64-v8a`. `versionCode` — число после `+` в `app/pubspec.yaml`; его нужно поднимать на каждый публичный APK.

`applicationId` — `eu.nagavpn.naga_network`. Менять нельзя: PackageInstaller не обновит приложение с другим id.

Обновление: тумблер «Автообновление приложения» (проверка при запуске) или кнопка «Проверить обновление» → скачать APK → системный диалог PackageInstaller. Нужно разрешение «установка неизвестных приложений» для Naga Network. Первый публичный APK подписывай **release** keystore (не debug). Keystore не в git: `app/android/key.properties` и `*.jks` / `*.keystore`. Другой ключ обновление не примет — только удалить и поставить заново.

## VPN

Go control-plane живёт в том же процессе (`libnaga.so`), слушает только `127.0.0.1:8765`. Токен — `filesDir/control.token`, UI читает его через канал `dataDir`.

`NagaVpnService` — единственный foreground-сервис: тип `specialUse`, subtype `vpn`. После `VpnService.establish()` любой сбой закрывает fd (иначе интерфейс висит и нет сети). TUN принадлежит `hev-socks5-tunnel` (`libhevfd.so`, ExtraFiles fd 3). Stock sing-box и AmneziaWG netstack работают за mixed/SOCKS на loopback. Системный HTTP-прокси на Android нет.

Per-app split задаётся только `VpnService.Builder` (`addAllowedApplication` **или** `addDisallowedApplication`, никогда оба). Приложение само всегда обходит туннель, чтобы fetch подписки, GitHub и control-plane ходили снаружи.

olcRTC в APK только если NDK-сборка `ghostlane-project/olcrtc` @ `7f849e08` прошла. Иначе health `olcrtc_ready=false`, пункт в UI «недоступно на Android», профиль не удаляется.

Все ELF в APK (Flutter, `libnaga.so`, `libsingbox.so`, `libhevfd.so`, опционально `libolcrtc.so`) собираются с 16 KB max-page-size. `extractNativeLibs` / `useLegacyPackaging` включены, бинари запускаются из `nativeLibraryDir`.

На части OEM (Xiaomi, Huawei, Samsung) фон и автозапуск режутся батареей. Включи Naga Network в исключениях автозапуска вручную — в v1 это не автоматизируется.

Нужны: Android NDK (API 26, arm64-v8a), Go с CGO, Flutter stable. Без NDK скрипт не соберёт `libnaga.so` / hev.

## Эмулятор (локально)

AVD `Naga_API36` — Pixel 7, Android 16, Google APIs **x86_64** (KVM). Релизный APK по-прежнему только `arm64-v8a`; debug дополнительно собирает `x86_64`, чтобы `flutter run` ставился на этот AVD.

```bash
./scripts/android/run-emulator.sh
# в другом терминале, из app/:
flutter run
```

SDK: `~/Android/Sdk`. Без окна: `./scripts/android/run-emulator.sh -no-window`. VPN в эмуляторе работает через `VpnService`, но нужен `libnaga.so` под `x86_64` в `jniLibs` — иначе UI откроется, control-plane нет.
