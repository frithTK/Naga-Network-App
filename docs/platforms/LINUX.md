# Linux

TUN через sing-box, systemd unit и AppImage.

## Установка

1. Скачать один AppImage из [релиза](https://github.com/frithTK/Naga-Network-App/releases), разрешить исполнение и запустить.
2. При первом запуске подтвердить диалог ОС: встроенный установщик подготовит udev/polkit и unit `naga-network-control@$(id -un)`. При отмене прежний control сохраняется.
3. Сборка из исходников: `./scripts/linux/build-appimage.sh`; результат — `dist/NagaNetwork-<version>-x86_64.AppImage`. Для разработки — `./scripts/linux/install-runtime.sh` и `./run.sh`.

AppImage содержит Flutter UI, naga-control, sing-box, olcRTC и hev. Встроенный runtime копируется в `~/.local/lib/naga-network/<build-id>`, control доступен через `~/.local/bin/naga-control`. Предыдущий runtime сохраняется. TUN и `CAP_NET_ADMIN` остаются у systemd unit; UI запускается от обычного пользователя. Движки находятся рядом с control, системный sing-box не заменяется.

Обновление из приложения (только AppImage, не `flutter run`): скачивается **один** AppImage, проверяется SHA256, затем новый файл запускается с `--prepare-runtime`. После подготовки встроенного runtime и проверки его версии новое окно ждёт завершения прежнего процесса. Автообновление проверяет GitHub при запуске не чаще раза в 12 часов; ручная кнопка сохраняется. Старым клиентам с двухфайловым updater требуется один раз скачать новый AppImage вручную. Имена: [RELEASES.md](RELEASES.md).

Токен control-plane: `~/.local/share/naga-network/control.token` (права `0600`).

Проверка окружения без изменений системы:

```bash
./scripts/linux/check-runtime.sh
```

AppImage включает движки; нужны systemd/polkit, curl, стандартные Linux utilities и GTK / AppIndicator для tray. Установщик настраивает `/dev/net/tun` (`0660 root:naga-network`). CLI `AppImage --install-runtime` повторяет подготовку службы; `--version` показывает версию; `--control` передаёт аргументы встроенному control для диагностики.

## Режимы

- TUN — системный VPN.
- Системный прокси — loopback `127.0.0.1:2080` (GNOME `gsettings`).

AmneziaWG 3.1 идёт userspace-sidecar, без DKMS. olcRTC — отдельный процесс поверх SOCKS и TUN `naga-olc0`.

После закрытия окна приложение может остаться в tray. На Wayland без StatusNotifier host окно скрывается, runtime продолжает работать. «Выход» останавливает VPN.

Второй запуск того же бинаря показывает уже открытое окно (`eu.nagavpn.naga_network`).

## Журнал

```text
~/.local/share/naga-network/logs/events.jsonl
```

Без URL подписки, ключей, UUID и полного runtime-конфига.

Подробности runtime: [platform/linux/README.md](../../platform/linux/README.md).
