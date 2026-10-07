# AmneziaWG 3.1

Отдельный профиль, не outbound `wireguard` в stock sing-box. Исходный `.conf` хранится байт-в-байт.

В конверте Naga Network тот же текст лежит в `amnezia.conf`. Голый WireGuard без полей Amnezia отклоняется.

Runtime: userspace `amneziawg-go` + netstack, SOCKS5 на localhost, трафик в тот же TUN, что и остальные узлы, если рядом есть sing-box профиль. Ядро Linux, DKMS и `awg-quick` не используются.

После обновления бинарника:

```bash
./scripts/linux/install-runtime.sh
sudo systemctl restart naga-network-control@$(id -un).service
```

| Компонент | Лицензия |
|-----------|----------|
| [amneziawg-go](https://github.com/amnezia-vpn/amneziawg-go) | MIT |
| [wireguard-go netstack](https://git.zx2c4.com/wireguard-go) | MIT |
| gVisor | Apache-2.0 |
