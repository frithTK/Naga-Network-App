# Linux platform

Единый AppImage содержит UI, control, sing-box, olcRTC и hev. Он подготавливает
systemd-службу через pkexec при первом запуске. TUN, системный прокси и tray
используют тот же runtime; интерфейс запускается от обычного пользователя.

Сборка: `./scripts/linux/build-appimage.sh`. Диагностика: AppImage `--version`,
`--install-runtime`, `--control --help`. Runtime хранится в
`~/.local/lib/naga-network/<build-id>`; `~/.local/bin/naga-control` указывает на него.

Разработка из исходников:

```bash
./scripts/linux/install-runtime.sh
./scripts/linux/check-runtime.sh
./run.sh
```

Production unit — `naga-network-control@$(id -un)` с `CAP_NET_ADMIN` и `DeviceAllow=/dev/net/tun`. Не делай `/dev/net/tun` world-writable.

`./run.sh` переиспользует уже запущенный control-plane. Dev без systemd: `NAGA_DEV_CONTROL=1 ./run.sh`.

Журнал: `~/.local/share/naga-network/logs/events.jsonl`.
