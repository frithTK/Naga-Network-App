# Linux

TUN через sing-box, systemd unit и AppImage.

## Установка

1. `./scripts/linux/install-runtime.sh` — соберёт `naga-control`, поставит udev/polkit и unit `naga-network-control@$(id -un)`.
2. `sudo systemctl start naga-network-control@$(id -un)`, если install не включил сервис.
3. UI: AppImage из [релиза](https://github.com/frithTK/Naga-Network-App/releases) или `./scripts/linux/build-appimage.sh` (кладёт ещё `dist/naga-control-linux-x86_64`). Для разработки — `./run.sh`.

AppImage содержит Flutter UI. TUN и `CAP_NET_ADMIN` остаются у systemd unit.

Обновление из приложения (только AppImage, не `flutter run`): при включённом «Автообновление приложения» клиент проверяет GitHub при запуске (не чаще раза в 12 часов) и ставит релиз сам. Кнопка «Проверить обновление» остаётся. Ставятся **оба** ассета — новый AppImage рядом с текущим (`$APPIMAGE`) и `naga-control` в `~/.local/bin`, затем `pkexec systemctl restart naga-network-control@$(id -un)`. Имена файлов: [RELEASES.md](RELEASES.md).

Токен control-plane: `~/.local/share/naga-network/control.token` (права `0600`).

Проверка окружения без изменений системы:

```bash
./scripts/linux/check-runtime.sh
```

Нужны `sing-box` в PATH, `/dev/net/tun` (`0660 root:naga-network`) и пакеты GTK / AppIndicator для tray.

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
