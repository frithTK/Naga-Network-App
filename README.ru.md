# Naga Network

[English](README.md) | **Русский**

Десктоп-клиент VPN для Linux и Windows. Добавляешь подписку или файл конфигурации — выбираешь узел — подключаешься. Свои серверы приложение не поднимает: работает с тем профилем, который ты импортировал.

Сборки: [Releases](https://github.com/frithTK/Naga-Network-App/releases). Лицензия: [GPL-3.0-or-later](LICENSE).

## Возможности

- системный VPN (TUN) или системный прокси;
- список узлов с ручным выбором, автовыбором и замером задержки;
- трафик, лимит и срок действия из заголовков подписки;
- русский интерфейс, трей, подключение без правки JSON вручную;
- встроенные правила LAN/РФ и опциональные списки из подписки.

Профиль можно добавить по HTTPS-ссылке, файлом sing-box JSON или AmneziaWG `.conf`.

## Протоколы

Ядро — [sing-box](https://sing-box.sagernet.org). Через него ходят обычные узлы подписки:

| Протокол | Как обычно используется |
|---|---|
| **VLESS** | TLS, часто с Reality |
| **Hysteria2** | QUIC, устойчив к потере пакетов |
| **TUIC** | QUIC, в клиенте запасной вариант при автовыборе |
| **Shadowsocks** | в том числе со ShadowTLS |
| **Trojan** | TLS |
| **VMess** | если есть в профиле |

Отдельно от stock sing-box:

- **AmneziaWG** — обфусцированный WireGuard (голый WireGuard без полей Amnezia не принимается);
- **olcRTC** — туннель поверх WebRTC-комнат (Jitsi, Telemost, WB). Пока он выбран, остальные ядра не запускаются.

## Платформы

Linux, Windows, Android. iOS собирается с Linux через [xtool](https://github.com/xtool-org/xtool), без Mac. Для Packet Tunnel нужен платный Apple Developer Program. macOS-десктопа нет.

Linux поставляется одним AppImage: внутри UI, naga-control, sing-box, olcRTC и hev. При первом запуске он подготовит системную службу через диалог авторизации ОС. На Windows в установщике и portable zip есть UI, control-plane, sing-box и Wintun.

В настройках кнопка **«Проверить обновление»** скачивает нужный файл с GitHub Releases и ставит его. Тумблер **«Автообновление приложения»** (включён по умолчанию) проверяет GitHub при запуске не чаще раза в 12 часов. Диалоги ОС (pkexec, установщик, PackageInstaller) остаются. Linux скачивает один AppImage и устанавливает runtime из него. Старые клиенты, требующие отдельный control-ассет, переходят на этот формат однократным ручным скачиванием нового AppImage.

Имена файлов:

| Платформа | Файл |
|---|---|
| Linux | `NagaNetwork-<ver>-x86_64.AppImage` |
| Windows portable | `NagaNetwork-<ver>-windows-x64.zip` |
| Windows setup | `NagaNetwork-<ver>-windows-x64-setup.exe` |
| Android | `NagaNetwork-<ver>-arm64-v8a.apk` |
| iOS | `NagaNetwork-<ver>-ios.ipa` |

Подробности: [RELEASES.md](docs/platforms/RELEASES.md).

## Документация

- [Linux](docs/platforms/LINUX.md)
- [Windows](docs/platforms/WINDOWS.md)
- [Android](docs/platforms/ANDROID.md)
- [iOS](docs/platforms/IOS.md)
- [Ассеты релизов](docs/platforms/RELEASES.md)
- [sing-box](docs/engines/SING_BOX.md), [AmneziaWG](docs/engines/AMNEZIAWG.md), [olcRTC](docs/engines/OLCRTC.md)
