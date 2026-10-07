# olcRTC

Третий режим рядом с sing-box и AmneziaWG. Ядро — `ghostlane-project/olcrtc`, ветка `proofkit`, коммит `7f849e08`, лицензия Apache-2.0. В Go-модуль оно не входит. Клиент пишет YAML `mode: cnc` и запускает `olcrtc /path/to/client.yaml`. Флага `-config` нет.

Пока выбран olcRTC, sing-box и AmneziaWG не запускаются. Режим не записывается outbound-ом в sing-box.

## Профиль

Строка:

```text
olcrtc://<provider>?<transport>@<room>#<key>$<name>
```

После транспорта может быть блок `k=v&k=v`. Нет `?`, `@` или `#` на своих местах — строка битая. `key` — 64 hex. Провайдеры: `jitsi`, `telemost`, `wbstream`. В рантайм входят `datachannel` и `vp8channel`. У Telemost и WB входной `datachannel` записывается как `vp8channel`. `seichannel` и `videochannel` не запускаются.

В конверте Naga Network поле `olcrtc` добавочное, `v` остаётся `1`. Нет поля или `null` — пункта меню нет. Битый объект не отменяет sing-box и Amnezia. Исходные байты подписки не переписываются.

В списке серверов каждый профиль — отдельная строка с именем из подписки, в той же группе страны, что и остальные узлы. Выбор строки запускает только эту комнату. Сохранённый режим `olcRTC` по-прежнему поднимает все профили сразу.

## Запуск

SOCKS `127.0.0.1:10808`, DNS `1.1.1.1:53`, UDP `max_flows: 256`, сессия `6h`. Несколько профилей — failover `retry_delay: 2s`, `max_cycles: 0`. YAML лежит в `.olcrtc/<id>/client.yaml` с правами `0600` и удаляется вместе с профилем. В журнал не пишутся ключ и комната.

Linux поднимает TUN `naga-olc0` через hev-socks5-tunnel поверх SOCKS `127.0.0.1:10808`. Windows делает то же через отдельный elevated-host `naga-control --olc-host` (не `--tun-host` sing-box): после setup через helper без UAC, в portable — UAC, `hev-socks5-tunnel.exe`, маршруты `route.exe`. Системный прокси WinINET на `:10808` не включается. На Android hev читает fd `VpnService` (`libhevfd.so`), без имени `naga-olc0`; нет NDK-сборки olcrtc — `olcrtc_ready=false`, профиль не удаляется. Бинарник `hev-socks5-tunnel` ищется в `NAGA_HEV_PATH`, рядом с `naga-control` (`hev-socks5-tunnel.exe` на Windows, `libhevfd.so` на Android) и в `PATH`.

Через транспорт `datachannel` коммита `7f849e08` часть адресов может не открываться, даже когда туннель поднят. Это ограничение ядра и комнаты, не признак того, что VPN выключен.

Бинарник `olcrtc` ищется в `NAGA_OLCRTC_PATH`, рядом с `naga-control` (`olcrtc.exe` на Windows), затем в `PATH`. Рядом с ним должен лежать файл `olcrtc.version` со строкой `7f849e08`. Команды `version` у бинарника нет. Windows-сборщик клонирует `ghostlane-project/olcrtc` на этот коммит и кладёт `olcrtc.exe` в бандл. `GET /v1/health` отдаёт `olcrtc_ready: true`, если бинарник и version-файл на месте. Остановка на Windows шлёт процессу `Kill`. Если TUN не поднялся после SOCKS, обходы комнаты снимаются. На Windows IPv6 default и DNS через `resolvectl` не трогаются.

Обновление конверта при активном режиме — не реже раза в час и перед стартом, если сохранённый профиль старше часа. Смена ключа перезапускает процесс. Неудачная загрузка старый процесс не гасит.
