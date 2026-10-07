# Linux platform

sing-box, TUN, systemd, системный прокси и Wayland status notifier.

```bash
./scripts/linux/install-runtime.sh
./scripts/linux/check-runtime.sh
./run.sh
```

Production unit — `naga-network-control@$(id -un)` с `CAP_NET_ADMIN` и `DeviceAllow=/dev/net/tun`. Не делай `/dev/net/tun` world-writable.

`./run.sh` переиспользует уже запущенный control-plane. Dev без systemd: `NAGA_DEV_CONTROL=1 ./run.sh`.

Журнал: `~/.local/share/naga-network/logs/events.jsonl`.
