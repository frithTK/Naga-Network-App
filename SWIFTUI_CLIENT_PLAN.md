# План iOS-клиента на SwiftUI

Документ для сборки **полного** клиента Naga Network на SwiftUI + xtool, без Flutter и без Mac на каждый билд. Это второй клиент: Android / Linux / Windows остаются на Flutter.

Реализация начата 2026-10-11 по запросу пользователя. Пока ADP ожидает активации, текущий объём — интерфейс и API-клиент (фазы 1–2). План можно править, если меняется ADP, ядро или список экранов.

Связанные файлы: [`docs/platforms/IOS.md`](docs/platforms/IOS.md), [`ios/`](ios/), [`cmd/naga-ios/`](cmd/naga-ios/), [`app/lib/main.dart`](app/lib/main.dart), [`app/lib/control_plane_client.dart`](app/lib/control_plane_client.dart), [`core/control/server.go`](core/control/server.go).

---

## Статус реализации (2026-10-11)

- **Фаза 1 — код реализован:** пять вкладок, главная со статусами и маскотом, пустые состояния, профили, серверы, статистика, настройки, помощь и диагностика. Экранов в одном `ContentView.swift` больше нет: приложение → `NagaUI` → отдельные файлы экранов. Connect в оболочке показывает русское сообщение и не вызывает NetworkExtension или HTTP `/v1/runtime/start`.
- **Фаза 2 — код реализован:** `NagaControlClient` без SwiftUI/NetworkExtension, Codable-модели, протокол `NagaControling`, клиент на URLSession, Bearer на всех маршрутах, только loopback без редиректов. Форма импорта URL/буфера/файла использует этот API. `NagaAppModel` опрашивает статус каждые 2 с только при активной сцене, блокирует удаление активного профиля и сохраняет настройку обновления подписки.
- **Проверки:** 22 Swift-теста на Linux прошли. Фикстуры экспортированы из настоящего `core/control.Server` скриптом `scripts/ios/export-api-fixtures.go`, а не придуманы под Swift-клиент. Сборка `xtool dev build -c debug --sign --ipa` прошла; шрифты и маскоты присутствуют в `.app` и IPA.
- **Устройство:** оболочка установлена на Nikita и открыта вручную. Пользователь подтвердил работу кнопок; на снимке главной видны все пять вкладок, бренд, маскот и форма добавления профиля. Обрезанный подзаголовок исправлен переносом текста. Автоматический запуск через `xtool launch` завершился `DebugserverClient.Error.unknown`. Остальные экраны, состояния профилей и сообщение Connect требуют отдельной проверки; снимки всех пяти вкладок не получены.
- **Дальше:** фаза 3 (CGO control-plane) ещё не начата; реальный импорт в оболочке пока недоступен. Фазы 4–6 ждут Active ADP. Текущий `xtool.yml` остаётся оболочкой без extension.

Чекбоксы ниже сохраняют исходный смысл: отмечаем их после проверки на устройстве. Успешная компиляция и тесты Linux сами по себе не закрывают критерии фаз 1–2.

---

## 0. Зачем это вообще

На Linux `flutter build ipa` не работает: Dart AOT для iOS (`gen_snapshot`) есть только на macOS. [xtool](https://github.com/xtool-org/xtool) собирает SwiftPM. Значит нативный iOS-клиент без Mac — это SwiftUI-приложение + Foundation Packet Tunnel, не Runner из `app/ios`.

На iPhone установлена **оболочка** SwiftUI с пятью вкладками. Go control-plane и VPN-ядро ещё не подключены. Эталон поведения и внешнего вида — Flutter; VPN-путь должен работать через `NEPacketTunnelProvider` и Go-ядро.

Не цель плана:

- macOS-десктоп;
- App Store в v1 (свой iPhone, Ad Hoc, внутренний TestFlight);
- per-app VPN через MDM;
- кнопка «обновить IPA» как у Linux AppImage;
- порт olcRTC и системного прокси на iPhone.

---

## 1. Что уже есть и что считать источником правды

### 1.1 Репозиторий

| Путь | Роль |
|---|---|
| `ios/` | SwiftPM + `xtool.yml`. SwiftUI-клиент фаз 1–2 и fail-closed `PacketTunnel`. |
| `ios/Sources/NagaUI/` | Пять вкладок, вложенные экраны, тема, компоненты и `NagaAppModel`. |
| `ios/Sources/NagaControlClient/` | HTTP-клиент, модели и представление данных без Apple UI-фреймворков. |
| `ios/Tests/NagaControlClientTests/` | Тесты API и фикстуры из Go server; запускаются на Linux. |
| `ios/xtool.yml` | Оболочка без VPN (`eu.nagavpn.naganetwork`), 7-дневный сертификат. |
| `ios/xtool.vpn.yml` | Полный клиент с PacketTunnel. Копировать поверх `xtool.yml` после ADP. |
| `app/lib/` | Эталон UI и клиентовского состояния. Не удалять. |
| `app/ios/` | Запасной Flutter/Xcode каркас. Не путь релиза на Linux. |
| `cmd/naga-ios/` | C-exported `NagaStart` / `NagaStop` / `NagaStopRuntime`, слушатель `127.0.0.1:8765`. |
| `core/engine/singbox/tun_ios.go` | `adaptTUN` без `naga-tun0` и без hev. |
| `scripts/ios/dev.sh` | Сборка/подпись/установка через xtool; патч Mach-O `__text` для zsign. |

### 1.2 Эталон экранов Flutter (переносить 1:1 по смыслу)

Нижняя навигация, compact-layout как телефон:

0. **Главная** — статус, профиль, узел, трафик, кнопка подключить, импорт.
1. **Профили** — список, импорт URL/файла/буфера, обновление, шаринг, удаление, автообновление подписки.
7. **Серверы** — unified nodes, ручной выбор, авто, probe latency.
2. **Статистика** — runtime snapshot, график трафика.
3. **Настройки** — автообновление подписки, версии, диагностика, справка, выбор сервера, **без** split-tunnel приложений и без system proxy.

Вложенные (сейчас индексы 5, 6, 8, 9):

- 5 справка;
- 6 диагностика / журнал;
- 8 маршрутизация приложений — **на iOS не делаем** (нет `listPackages`, per-app VPN это MDM);
- 9 выбор сети / политика подключения — только TUN, без `system_proxy`.

Токены: фон `#080808`, поверхность `#1A1C20`, акцент `#F72F38`, успех `#27C46A`, текст `#FFFFFF` / `#ABB0B9`. Шрифт Inter из `app/assets/fonts`. Знак — `assets/brand/naga-mark-red.svg` на непрозрачном `#080808`. Маскот — `app/assets/brand/states/{welcome,idle,connecting,connected,error,offline}.png`.

### 1.3 Control-plane API (не изобретать другой)

Локальный HTTP на `127.0.0.1:8765`, токен из `control.token` в App Group. Сырой JSON профиля наружу не отдавать. Маршруты из `core/control/server.go`:

```
GET  /v1/health
GET  /v1/diagnostics/logs
POST /v1/diagnostics/client
POST /v1/subscriptions/preview
POST /v1/profiles/import
GET  /v1/profiles
GET  /v1/profiles/{id}/share
GET  /v1/profiles/{id}/nodes
POST /v1/profiles/{id}/refresh
POST /v1/profiles/{id}/mode
DELETE /v1/profiles/{id}
GET  /v1/nodes
POST /v1/nodes/probe
GET  /v1/policy
PUT  /v1/policy
GET  /v1/routing
PUT  /v1/routing
GET  /v1/network
GET  /v1/apps
PUT  /v1/apps/{id}
DELETE /v1/apps/{id}
GET  /v1/apps/discovered          # на iOS пустой список
GET  /v1/runtime
GET  /v1/runtime/inspect
POST /v1/runtime/start
POST /v1/runtime/select
POST /v1/runtime/stop
```

Нет `GET /v1/profiles/{id}` без суффикса. Профиль берут из списка. `listNodes` в Dart — это `GET /v1/profiles/{id}/nodes`.

Swift-клиент повторяет `ControlPlaneClient` в Dart: те же пути, те же JSON-поля, заголовок токена как на десктопе.

### 1.4 Жёсткие ограничения Apple и ядра

- VPN на iOS — только `NEPacketTunnelProvider` в **отдельном процессе** (`appex`). Не hev-fd, не `naga-tun0`.
- Entitlement `packet-tunnel-provider` — **только платный ADP**. Free team / 7 дней оболочку ставят, туннель — нет.
- Guideline 5.4 для магазина: VPN у organization. Individual годится для своего устройства.
- `startTunnel` без готового ядра — **fail-closed** (сразу ошибка, мёртвый TUN не оставлять).
- `excludeLocalNetworks` на `NEVPNProtocol`. `excludeAPNs` / `excludeCellularServices` — с iOS 16.4, через `#available`.
- `tunnelRemoteAddress` в `NEPacketTunnelNetworkSettings` обязателен.
- Dart `http` не ходит в Unix-сокет. Пока control-plane HTTP — оставляем loopback. Не тащить Dart на iOS.
- Fetch подписки — uTLS в Go, не `URLSession` и не stock Go `net/http` без того же пути, что desktop.
- Rule-set не качать из Swift. Это делает ядро.
- `ITSAppUsesNonExemptEncryption` = true (sing-box TLS/QUIC).
- min iOS 16.
- Канон в репо: app `eu.nagavpn.naga_network`, extension `eu.nagavpn.naga_network.PacketTunnel`, group `group.eu.nagavpn.naga_network`. Оболочка сейчас `eu.nagavpn.naganetwork` (без `_` — иначе xtool собирает имя `XTool eu nagavpn naga_network` и Apple отвечает 409).
- **xtool 1.21 при подписи переписывает ID в `XTL-<8 hex>.<bundle>`.** В портале заводить нужно **уже с префиксом**, Description латиницей без точек (`NagaNetwork`). Установленное приложение на телефоне будет `XTL-….eu.nagavpn.naga_network`, не голый канон. `NagaVPN.tunnelBundleID` в Swift должен совпасть с тем, что xtool проставил в appex, а не с константой из репо вслепую.

---

## 2. Архитектура целевого клиента

```
┌─────────────────────────────────────────────┐
│  NagaNetwork.app  (SwiftUI, процесс UI)     │
│  • экраны, навигация, импорт URL            │
│  • NagaControl: NagaStart() → :8765         │
│  • HTTP клиент → 127.0.0.1:8765             │
│  • NagaVPN: NETunnelProviderManager         │
│  • данные: App Group контейнер              │
└──────────────────────┬──────────────────────┘
                       │ startVPNTunnel()
                       ▼
┌─────────────────────────────────────────────┐
│  PacketTunnel.appex  (отдельный процесс)    │
│  • PacketTunnelProvider                     │
│  • in-process libbox + RuntimeController    │
│  • packetFlow → движок                      │
│  • те же файлы профиля из App Group         │
│  • fail-closed, если ядро не поднялось      │
│  • IPC наружу: sendProviderMessage /        │
│    UNIX-сокет в контейнере группы,          │
│    не HTTP на loopback UI                   │
└─────────────────────────────────────────────┘
```

Правила разделения:

- **UI-процесс** умеет импортировать профиль и ходить в API **без** включённого VPN (как Android: HTTP control-plane живёт в приложении).
- **Туннель** не импортирует подписку и не рисует UI. Читает envelope из App Group, поднимает datapath.
- **iOS запрещает `exec` постороннего бинаря.** Сейчас `singbox.Adapter.Start` делает `exec.Command(BinaryPath, …)` — на iPhone это не путь. Android обходит это, запуская `libsingbox.so` как ELF из jniLibs. На iOS нужен **in-process** libbox (CGO/gomobile в extension), не «положить sing-box в бандл и exec». Пока этого адаптера нет, `RuntimeReady` у `StartLocal` будет `false` (`binaryPath == ""`).
- **Автовыбор и health ~45 с живут в `RuntimeController`, не внутри stock sing-box.** Если движок только в extension, а контроллер только в UI — после убийства приложения авто-прыжок узлов остановится. Значит в extension должен быть либо тот же контроллер, либо эквивалент (select + probe). Не оставлять health только в UI.
- **127.0.0.1 приложения и extension — разные песочницы.** HTTP из appex на `:8765` UI рассчитывать нельзя. Связь: App Group + `NETunnelProviderSession` messages.
- Конфиг: envelope `v:1` / `app:naga-network`. Не инжектить `naga-tun0`. `CheckTUN` на iOS сейчас `preflight_other.go` → «TUN пока не реализован» — для Connect нужен **no-op** preflight (`//go:build ios`), иначе старт даже с libbox упадёт.
- `StartLocal` зашивает `User-Agent: NagaNetwork/android`. Перед iOS-импортом сменить на `NagaNetwork/ios` (подписки и uTLS).
- **Connect на iOS ≠ Flutter `startRuntime()`.** Кнопка → `startVPNTunnel()` → `startTunnel` в appex поднимает engine. `POST /v1/runtime/start` в UI-процессе либо no-op, либо прокси в extension. Второй sing-box в UI запрещён.
- **После свайпа UI control-plane в приложении умирает, туннель может жить.** Новый `NagaStart()` не знает про движок в appex. Статус, статистика, select, probe, auto — только через IPC в extension (или control-plane **внутри** appex + канал, который UI реально достаёт). Иначе после возврата в приложение «подключено» в Настройках и пустой `/v1/runtime`.

Модули SwiftPM (предложение, править `Package.swift` когда начнётся код):

```
NagaNetwork          — @main App, навигация
NagaUI               — токены, компоненты, экраны без NetworkExtension
NagaControlClient    — HTTP + модели JSON
NagaVPNKit           — NETunnelProviderManager, статусы
PacketTunnel         — NEPacketTunnelProvider
CNaga / libnaga      — c-archive / xcframework из cmd/naga-ios
```

На Linux xtool пакует два product из `xtool.yml`: приложение и extension. Не плодить третий appex.

---

## 3. Принципы, которые нельзя нарушать

1. Ответы пользователю по-русски; код и идентификаторы — как в репо (английские символы, bundle id без смены префикса `eu.nagavpn`).
2. GPL-3.0-or-later. Не копировать UI/строки Karing, ClashMi и т.п.
3. Не коммитить `naga beh/`, секреты, `.env`, профили.
4. Коммиты только по просьбе; сообщения EN+RU, без `Co-authored-by: Cursor`.
5. Не перезапускать уже работающий Linux-VPN пользователя.
6. Не править plan-файлы, кроме этого, если явно не итерируем план.
7. iOS Connect: после `establish`/поднятия tunnel — fail-closed. До выдачи fd/tunnel — как на десктопе fail-open в control-plane **не** повторять слепо: на iPhone мёртвый VPN в Настройках хуже, чем ошибка в кнопке.
8. Автовыбор узлов — как desktop: лимит 3 VLESS, TUIC последний, health ~45 с, без гистерезиса. Не «улучшать» в Swift, пока не поменяли ядро.
9. Один визуальный язык с Flutter. Не «нативный iOS look» в ущерб бренду.

---

## 3.1 Ревизия плана (2026-10-11)

Проверка по коду. Ниже — что в черновике было неверно; в фазах уже поправлено.

| Было | Как на самом деле |
|---|---|
| `GET /v1/profiles/{id}` | Такого маршрута нет. Есть list, `/share`, `/nodes`, refresh, mode, delete. |
| Завести в портале `eu.nagavpn.naga_network` и xtool его найдёт | xtool ищет/создаёт `XTL-<8 hex>.eu.nagavpn.naga_network` и **подменяет** CFBundleIdentifier. 409 был на **name** `XTool eu nagavpn naga_network`, не на identifier. |
| Слинковать `cmd/naga-ios` = появится VPN | `Adapter` **exec**'ает sing-box. На iOS exec нельзя. Import/list без бинаря работают; Connect/idle-probe — нет. |
| Health/auto в UI, datapath в extension | Автопрыжок узлов — код `RuntimeController` (лимит 3, 45 с). Если он не в том же процессе, что engine, после свайпа приложения авто пропадёт. |
| Extension ходит на `127.0.0.1:8765` | Loopback UI ≠ loopback appex. |
| Автообновление подписки «флаг ядра» | Во Flutter `_autoUpdate` — память сессии, default `true`, **не** API и не SharedPreferences. |
| Статистика: не показывать `0 B` | Flutter как раз показывает `_formatBytes(0)` и «Нет данных» только у **rate**. |
| Probe latency без VPN как на Linux | `idleSingBoxProbeSupported()` на ios/android = false. Ping узлов без поднятого VPN не работает (как Android). |
| `swift test` всего пакета на Linux | `NetworkExtension` на Linux не собрать. Модели/HTTP — отдельный target без UIKit. |
| Inter «просто скопировать» | TTF доступны локально. Их iOS-копиям исправлены внутренние имена начертаний: иначе все четыре регистрируются как `Inter-Regular`. Глифы и веса не менялись. |
| `CheckTUN` можно игнорировать | На старте TUN `runtime_awg.go` зовёт `c.TUN.CheckTUN()`. iOS сейчас unsupported → Connect сломан даже с libbox. |
| Фаза 1: кнопка может звать `NagaVPN.start` | Без NE entitlements это шум/краш. До фазы 4 **не** вызывать `NETunnelProviderManager`. |
| `preflight_ios.go` с `//go:build ios` поверх текущего `preflight_other.go` | `preflight_other.go` уже `!linux && !windows` — на `GOOS=ios` соберутся **оба**. Нужно исключить ios из other (`&& !ios`), иначе duplicate `CheckTUN`. |
| Control-plane только в UI, engine в appex | После убийства приложения `/v1/runtime` пустой при живом VPN. Нужен IPC (см. архитектуру). |
| `selectMode` забыли в клиенте | Dart: `POST /v1/profiles/{id}/mode`. Вкладка серверов/профиля его зовёт. |
| Inter TTF «лежат в `app/assets/fonts`» | В текущем рабочем дереве все четыре TTF есть; iOS-копии лежат в `ios/Resources/Fonts` вместе с OFL. |
| App Group = `group.XTL-<hex>.…` наверняка | Префикс XTL для **bundle id** xtool делает точно. Для group — **проверить** фактический entitlement после первой vpn-подписи, не гадать. |
| v1 = AmneziaWG Connect | Импорт `.conf` в профиль — да. Datapath AWG на iOS без exec sidecar — отдельное решение (4.4.8). Не обещать Connect AWG в v1. |

Не дыры, а развилки, которые план уже выбрал и надо не размыть: SwiftUI+xtool, не Flutter-на-Mac; магазин не в v1; без olcRTC/proxy/split-tunnel.

---

## 4. Фазы — подробные шаги

Нумерация: `Ф<фаза>.<блок>.<шаг>`. Не переходить к следующей фазе, пока не закрыт её критерий. Симулятор PacketTunnel не считается проверкой. Оценка — порядок величины, не дедлайн.

Параллелить можно только **0 (ожидание ADP)** с **1–2**. Фазы **4–6** без Active ADP не начинать.

Как отмечать прогресс: в конце каждого блока чекбокс в этом файле менять на `[x]`, когда шаг реально сделан на устройстве, не «в голове».

---

### Фаза 0. Допуски и учётная запись

**Зачем.** Без ADP туннель не подписать. Без Developer Mode development-сборку iOS 16+ не запустит.

**Не делать в этой фазе:** писать Swift, менять bundle id оболочки, `xtool.vpn.yml` → `xtool.yml`.

#### 0.1 Членство

- [ ] **0.1.1** Открыть [developer.apple.com/account](https://developer.apple.com/account) тем Apple ID, которым делали `xtool setup`.
- [ ] **0.1.2** Проверить, что это **Apple Developer Program**, не Enterprise и не Personal Team.
- [ ] **0.1.3** Дождаться статуса **Active** (оплата + verification паспорта). Письма ждать дни. Пока «Access Unavailable» на Identifiers — стоп.
- [ ] **0.1.4** Сохранить Team ID (10 символов) в парольнице, не в git.
- [ ] **0.1.5** Если логин xtool был Password (free): после Active лучше `xtool auth` заново через **API Key** (App Store Connect → Users and Access → Integrations → Keys, роль App Manager). Не коммитить `.p8`.

#### 0.2 Identifiers (вручную, до `xtool dev`)

xtool 1.21 шлёт имя `XTool eu nagavpn naga_network` и ловит Apple 409. Имена ниже — латиница без точек в Description.

- [ ] **0.2.1** [Identifiers](https://developer.apple.com/account/resources/identifiers/list) → `+` → App IDs → App.
- [ ] **0.2.2** Узнать 8 hex xtool: первая группа `identityID` до `-`, UPPER (из лога auth или `xtool ds identifiers list` после неудачного create). Пример: `XTL-A5EBDAB5`.
- [ ] **0.2.3** App ID приложения:
  - Description: `NagaNetwork` (без точек и слова XTool)
  - Bundle ID Explicit: `XTL-<hex>.eu.nagavpn.naga_network`
- [ ] **0.2.4** App ID extension:
  - Description: `NagaNetworkTunnel`
  - Bundle ID Explicit: `XTL-<hex>.eu.nagavpn.naga_network.PacketTunnel`
- [ ] **0.2.5** Голый `eu.nagavpn.naga_network` в портале xtool **не подхватит** — он сравнивает с `XTL-…` идентификатором. Канон без префикса оставить в исходниках `xtool.vpn.yml`; на устройстве будет префикс.
- [ ] **0.2.6** App Group: `group.XTL-<hex>.eu.nagavpn.naga_network`, Description `NagaNetworkGroup`. В Swift `NagaAppIdentity.appGroup` выровнять с тем, что проставит сайнер, либо читать из entitlements.

#### 0.3 Capabilities

На **каждый** App ID из 0.2:

- [ ] **0.3.1** Edit → App Groups → галка, выбрать группу из **0.2.6**.
- [ ] **0.3.2** Network Extensions → **Packet Tunnel**.
- [ ] **0.3.3** Сохранить. Если Packet Tunnel серый — ADP ещё не Active, вернуться к 0.1.

#### 0.4 Устройство

- [ ] **0.4.1** UDID: `idevice_id -l` или `xtool devices`. Сейчас Nikita: `00008120-000171243E44A01E`.
- [ ] **0.4.2** [Devices](https://developer.apple.com/account/resources/devices/list) → iPhone → этот UDID, имя `Nikita`.
- [ ] **0.4.3** Кабель, на телефоне «Доверять этому компьютеру».
- [ ] **0.4.4** Настройки → Конфиденциальность и безопасность → **Режим разработчика** → вкл → ребут → подтвердить. Это не «Доверять разработчику».
- [ ] **0.4.5** Если пункта нет: `./scripts/ios/dev.sh` оболочкой, снова открыть Настройки.

#### 0.5 Профили (Development)

xtool часто выпускает профиль сам на шаге Provisioning. Если 409 повторится — руками:

- [ ] **0.5.1** Profiles → Development → iOS App Development → App ID `NagaNetwork` → сертификат этой машины → устройство Nikita.
- [ ] **0.5.2** Второй профиль — App ID `NagaNetworkTunnel`, то же устройство и сертификат.
- [ ] **0.5.3** Ad Hoc / App Store профили **не** делать в фазе 0.

#### 0.6 Проверка

- [ ] **0.6.1** Identifiers открывается без модалки Access Unavailable.
- [ ] **0.6.2** `xtool ds identifiers list` показывает оба ID (с префиксом XTL или без — как завели).
- [ ] **0.6.3** `xtool devices` видит Nikita.
- [ ] **0.6.4** Оболочку `eu.nagavpn.naganetwork` с телефона можно оставить до фазы 4. Два bundle в одном `xtool.yml` не мешать.

**Критерий фазы 0:** 0.6.1–0.6.3 зелёные. Только тогда трогать `xtool.vpn.yml`.

---

### Фаза 1. Каркас SwiftUI без ядра

**Зачем.** На телефоне те же пять вкладок, что compact Flutter. VPN ещё не нужен.

**Ставить как:** текущий `ios/xtool.yml` (оболочка `eu.nagavpn.naganetwork`). Entitlements VPN не подключать.

Эталон: `app/lib/ui/home.dart`, `app/lib/main.dart` (`NagaHomeView`, `_ProfilesPage`, `_NodesPage`, `_StatisticsPage`, `_SettingsPage`, `_HelpPage`).

#### 1.1 Модули и навигация

- [x] **1.1.1** Разрезать `ios/Sources/NagaNetwork/ContentView.swift`. Не оставлять все экраны в одном файле.
- [x] **1.1.2** Файлы (создавать по мере кода, не пустые заранее):
  - `RootView.swift` — контейнер вкладок
  - `HomeView.swift`
  - `ProfilesView.swift`
  - `ServersView.swift`
  - `StatsView.swift`
  - `SettingsView.swift`
  - `HelpView.swift` (push со настроек, не вкладка)
- [x] **1.1.3** Нижний бар, порядок как Flutter `[0, 1, 7, 2, 3]`:
  1. Главная (`Icons.home`)
  2. Профили
  3. Серверы
  4. Статистика
  5. Настройки
- [x] **1.1.4** Не `TabView` системного стиля, если он ломает бренд. Кастомный бар высотой ~76, подписи «Главная / Профили / Серверы / Статистика / Настройки».
- [ ] **1.1.5** Eyebrow на секциях: `NAGA NETWORK`. Заголовки и subtitle — дословно из Flutter (`Профили` / `Ваши подписки и доступ к VPN.` и т.д.).

#### 1.2 Тема

- [ ] **1.2.1** `NagaTheme.swift`: `_background 080808`, `_surfaceLow 111215`, `_surface 1A1C20`, `_accent F72F38`, `_success 27C46A`, `_warning FFB020`, `_error FF454D`, `_text FFFFFF`, `_muted ABB0B9`, `_tertiary 9299A5`, `_line 282E36`.
- [ ] **1.2.2** Радиусы 10 / 14, отступы 4, 8, 12, 16, 24, 32, 48 как `NagaSpacing`.
- [ ] **1.2.3** Шрифт Inter: взять `app/assets/fonts/Inter-{Regular,Medium,SemiBold,Bold}.ttf` (если TTF нет в дереве — восстановить по `app/assets/fonts/README.md`). В `ios/Resources/Fonts`, ключ `UIAppFonts`. Не SF Pro как основной.
- [x] **1.2.4** Кнопка Connect: красная заливка, текст `#080808`, corner 14, высота как Flutter (~52–56).
- [x] **1.2.5** Карточки: фон `_surfaceLow`, без системного `InsetGrouped`.

#### 1.3 Ассеты

- [ ] **1.3.1** Иконка уже `ios/Resources/AppIcon.png` — RGB, `#080808`, знак с полем. Не возвращать прозрачный SVG.
- [ ] **1.3.2** Положить маскот в `ios/Resources/Mascot/`: `welcome`, `idle`, `connecting`, `connected`, `error`, `offline` из `app/assets/brand/states/`.
- [x] **1.3.3** Знак `naga-mark-red` для шапки, если на главной он есть отдельно от иконки.
- [ ] **1.3.4** `xtool.yml` `resources:` — шрифты и маскот, не только PrivacyInfo.

#### 1.4 Главная (статический макет)

Тексты из `NagaHomeView`:

- [ ] **1.4.1** Статусы: `Готов к подключению` / `Подключаемся…` / `VPN подключён` / `Не удалось подключиться` / `Статус неизвестен` / `Отключаемся…` / `Подписка истекла` / `Подключение недоступно`.
- [ ] **1.4.2** Маскот по состоянию (enum как Flutter `NagaMascotMood`).
- [ ] **1.4.3** Блок профиля: имя или CTA «Добавить профиль»; провайдер; выбранный узел.
- [ ] **1.4.4** Кнопка «Подключить» / «Отключить». Пока **не** звать `NETunnelProviderManager` (в текущем `NagaVPN.swift` это ломает оболочку). Показ русской ошибки в UI, без системного VPN-диалога.
- [ ] **1.4.5** Ограничение старта: `Обновите подписку, чтобы подключиться` / `Проверьте срок и лимит подписки` — показать на моке с `canConnect=false`.
- [x] **1.4.6** Не рисовать переключатель TUN / системный прокси.

#### 1.5 Остальные вкладки (макет + пустые состояния)

- [ ] **1.5.1** Профили: subtitle `Ваши подписки и доступ к VPN.`; пустое — как Flutter (добавить URL / вставить / файл). Кнопки без API пока no-op с snackbar «Ядро ещё не собрано».
- [ ] **1.5.2** Серверы: subtitle `Общий пул узлов из сохранённых профилей.` (disconnected) / `Выберите сервер или доверьте выбор Naga.` (connected). Пустой список не «Нет данных 0 ms».
- [ ] **1.5.3** Статистика: subtitle как Flutter; плитки **Загрузка / Отдача / Сессия**. Байты как `_formatBytes(downloadBytes ?? 0)` (нуль допустим). «Нет данных» — у **rate**, когда `trafficAvailable != true`. Есть график `_TrafficChart`.
- [ ] **1.5.4** Настройки, группы:
  - Подключение: **без** «Какие приложения через VPN», **без** traffic mode proxy; оставить «Как выбирать сервер» (заглушка), «Диагностика» (заглушка).
  - Подписка: автообновление — как Flutter: default `true`, **не** API. Можно `@AppStorage` (даже чуть лучше Flutter, там флаг не переживает рестарт).
  - О приложении: версия UI из `CFBundleShortVersionString`; control-plane `ядро не собрано`.
  - **Без** «Проверить обновление» / автообновление IPA.
- [ ] **1.5.5** Помощь: заголовок `Помощь`, subtitle `Быстрые ответы и подсказки по подключению.`, карточка `VPN не работает?` / `Проверьте интернет, профиль и срок подписки.` + те же ExpansionTile, что в `_HelpPage` (как добавить профиль и т.д. — скопировать строки из Flutter, не выдумывать).

#### 1.6 Info.plist и сборка

- [ ] **1.6.1** Не снимать `ITSAppUsesNonExemptEncryption`.
- [ ] **1.6.2** Не трогать `PrivacyInfo.xcprivacy`.
- [x] **1.6.3** Ориентация: portrait iPhone как сейчас.
- [ ] **1.6.4** `./scripts/ios/dev.sh` → на телефоне пять вкладок, иконка с полем, Developer Mode уже из 0.4.
- [ ] **1.6.5** Скриншоты всех пяти вкладок сохранить локально (не в git), сверить с Android compact.

**Критерий фазы 1:** на iPhone видны все вкладки, тексты совпадают с Flutter, Connect не крашит, VPN-диалог системы **не** появляется.

---

### Фаза 2. Swift-клиент control-plane (без CGO)

**Зачем.** UI ходит в тот же JSON, что Dart. Ядро ещё можно мокать.

Эталон: `app/lib/control_plane_client.dart`. База `http://127.0.0.1:8765`. Заголовок `Authorization: Bearer <token>`.

#### 2.1 Модели Codable

Поля 1:1, имена JSON как в Go, не camelCase наружу.

- [ ] **2.1.1** `ProfilePreview`: `profile_id?`, `profile_title`, `provider_name`, `engine`, `upload_bytes`, `download_bytes`, `total_bytes`, `unlimited`, `expire_utc`, `update_interval_seconds`, `can_connect`, `olcrtc_available`.
- [ ] **2.1.2** `StoredProfileSummary`: `id`, `name`, `provider_name`, `engine`, `imported_at`, `updated_at`, байты, `unlimited`, `update_interval_seconds`, `expire_utc`, `can_connect`, `has_source_url`, `olcrtc_available`.
- [ ] **2.1.3** `ProfileShare`: `url`, `name`.
- [ ] **2.1.4** `UnifiedNode`: `id`, `profile_id`, `tag`, `runtime_tag`, `country`, `protocol`, `latency_ms?`, `probe_status`.
- [ ] **2.1.5** `RuntimeSnapshot` — все поля, которые читает `_StatisticsPage` / home (`status`, `engine`, `download_bytes`, `upload_bytes`, `download_rate_bytes`, `traffic_available`, selected outbound, …). Сверить с Dart-классом целиком, не урезать.
- [ ] **2.1.6** `ConnectionPolicy`, `RoutingPolicy` (для фазы 5; routing.apps на iOS не показывать).
- [ ] **2.1.7** `DiagnosticLogEntry`.
- [ ] **2.1.8** `ControlHealth`: `service`, `status`, `version`, `runtime_ready`, `olcrtc_ready`.
- [ ] **2.1.9** Ошибка API: JSON `{ "error": "..." }` → `NagaAPIError`.

#### 2.2 Клиент

- [ ] **2.2.1** Протокол `NagaControling` + реализация на `URLSession`.
- [ ] **2.2.2** Методы = Dart: `health`, `healthInfo`, `preview`, `previewConfig`, `importProfile`, `importConfig`, `listProfiles`, `shareProfile`, `listNodes` (`GET /v1/profiles/{id}/nodes`), `selectMode` (`POST /v1/profiles/{id}/mode`), `deleteProfile`, `refreshProfile`, `listUnifiedNodes` (`GET /v1/nodes`), `probeUnifiedNodes`, `runtimeStatus`, `startRuntime` (на iOS в фазе 4 станет IPC/no-op, не второй engine), `selectRuntime`, `selectRuntimeAuto` (`POST /v1/runtime/select` + `{"auto":true}`), `stopRuntime`, `connectionPolicy`, `saveConnectionPolicy`, `diagnosticLogs`, `recordClientEvent`. Нет get-profile-by-id.
- [ ] **2.2.3** Не вызывать `URLSession` на URL подписки. Только loopback.
- [ ] **2.2.4** Таймауты разумные (как Dart http). Preview/import могут быть долгими.
- [ ] **2.2.5** `updateToken(_:)`. `StartLocal` всегда ставит токен: **все** пути, включая `/v1/health`, требуют `Authorization: Bearer`. Без токена — 401, не «гость». UI ждёт файл, как Android.

#### 2.3 Тесты на Linux

- [ ] **2.3.1** Вынести модели+HTTP в target **без** `NetworkExtension`/`SwiftUI` (иначе `swift test` на Linux не соберётся). Тесты — только этот target.
- [ ] **2.3.2** Фикстуры из ответов `core/control` тестов: health, list profiles, nodes, runtime, error.
- [ ] **2.3.3** Mock URLProtocol / stub session.
- [ ] **2.3.4** `cd ios && swift test`.

#### 2.4 AppModel

- [ ] **2.4.1** `@MainActor` модель: `controlReachable`, `profiles`, `preview`, `nodes`, `runtime`, `status`, `lastError`.
- [ ] **2.4.2** min iOS 16: `ObservableObject`, не требовать iOS 17 Observation.
- [ ] **2.4.3** Поллинг `runtimeStatus` каждые **2 с**, пока сцена активна (`Timer.periodic` как Flutter). На `scenePhase == .background` останавливать.
- [ ] **2.4.4** `runtimeBlocksProfileDelete`: `starting` / `connected` / `stopping` → нельзя delete.
- [ ] **2.4.5** Snackbar/banner: текст `error` с сервера, если он уже по-русски; иначе короткий русский fallback, не raw JSON.

#### 2.5 Импорт в UI (ещё бьёт в мок)

- [ ] **2.5.1** Sheet: поле URL, кнопка «Вставить» (`UIPasteboard`), «Файл» (`fileImporter` json/text/conf).
- [ ] **2.5.2** Файл → строка → `importConfig`, не URL файла.
- [ ] **2.5.3** HTTPS URL → `importProfile`.
- [ ] **2.5.4** На моке: успех наполняет список; 4xx показывает `error`.
- [ ] **2.5.5** QR шаринга — фаза 5. Сканер камеры — не в v1 (или отдельный шаг 5.x с `NSCameraUsageDescription`).

**Критерий фазы 2:** `swift test` зелёный. На устройстве без CGO вкладки не падают; health красный/«Статус неизвестен», как `_controlPlaneReachable == false`.

---

### Фаза 3. Go control-plane в UI-процессе

**Зачем.** Импорт и узлы на телефоне без VPN.

Darwin SDK уже стоит (`swift sdk list` → `darwin`). Clang для CGO — **Swift 6.4 Linux** (`~/.local/share/swiftly/bin/clang`), не `Xcode.app/.../clang` (это Mach-O под Mac).

#### 3.1 Скрипт CGO

- [ ] **3.1.1** Файл `scripts/ios/build-naga-cgo.sh`.
- [ ] **3.1.2** Переменные:
  ```
  GOOS=ios GOARCH=arm64 CGO_ENABLED=1
  CC=$HOME/.local/share/swiftly/bin/clang
  SDK=$HOME/.swiftpm/swift-sdks/darwin.artifactbundle/Developer/Platforms/iPhoneOS.platform/Developer/SDKs/iPhoneOS.sdk
  CGO_CFLAGS="-isysroot $SDK -arch arm64 -miphoneos-version-min=16.0"
  CGO_LDFLAGS="-isysroot $SDK -arch arm64"
  ```
- [ ] **3.1.3** `go build -buildmode=c-archive -buildvcs=false -o ios/.build/libnaga.a ./cmd/naga-ios`
- [ ] **3.1.4** `-ldflags "-X naga.network/core/version.Version=<pubspec>"`
- [ ] **3.1.5** Проверить `libnaga.h` рядом с `.a`, символы `NagaStart`, `NagaStop`, `NagaStopRuntime`.
- [ ] **3.1.6** Вызывать скрипт из `dev.sh` / `build-ipa.sh` до `xtool dev`.
- [ ] **3.1.7** `.a` в `.gitignore` (как xcframework).

#### 3.2 Линковка в xtool

- [ ] **3.2.1** Попробовать `linkerSettings` у target `NagaNetwork`: `-lnaga` + `-L ios/.build`.
- [ ] **3.2.2** Если SwiftPM/xtool игнорирует — дописать trampoline `ld64.lld` (рядом с `naga-ensure-text.py`) чтобы подмешивать `libnaga.a` в линк **приложения**, не extension.
- [ ] **3.2.3** Не линковать полный control-plane в PacketTunnel на этом шаге (extension ещё fail-closed).
- [ ] **3.2.4** Неопределённые символы (`resolv`, `network`, `sqlite3`, …) — добавлять `.linkedLibrary`, не выкидывать Go.
- [ ] **3.2.5** После линка проверить `nm`/`llvm-nm` на `NagaNetwork` бинарнике: есть `_NagaStart`.

#### 3.3 Старт в Swift

- [ ] **3.3.1** Bridging: `NagaStart(UnsafePointer<CChar>) -> UnsafeMutablePointer<CChar>?`, `NagaStop()`, `NagaStopRuntime()`. Строку ошибки с C освобождать (`free`).
- [ ] **3.3.2** `NagaControl.start()`:
  - dataDir = App Group container, если entitlement есть;
  - иначе Application Support `naga-network` (только оболочка);
  - `NagaStart(path)`;
  - ждать `control.token` до 40×250 мс как Android.
- [ ] **3.3.3** Передать токен в клиент, затем `waitUntilHealthy` (health loop).
- [ ] **3.3.4** Не вызывать `NagaStop` при сворачивании — убьёт HTTP во время импорта. `NagaStop` — при terminate UI, если VPN выключен.
- [ ] **3.3.5** `NagaStopRuntime` не звать из UI, пока datapath в extension (фаза 4). Сейчас `StartLocal` без sing-box binary: `RuntimeReady=false`, `Adapter.Start` вернёт `ErrBinaryNotConfigured`. Импорт/list от этого не зависят. Connect в фазе 3 **не** ожидать.

#### 3.4 Поведение ядра на iOS

- [ ] **3.4.1** Idle probe на ios уже выключен (`idleSingBoxProbeSupported`). Не включать. `POST /v1/nodes/probe` без VPN на iPhone **не** даст latency (как Android) — не считать это багом фазы 3.
- [ ] **3.4.2** Fetch подписки — uTLS в Go. В `serve_local.go` сменить `User-Agent` с `NagaNetwork/android` на `NagaNetwork/ios` при `GOOS=ios`.
- [ ] **3.4.3** Импорт `.conf` AmneziaWG — как desktop, через `config` body. Парсера в Swift нет.
- [ ] **3.4.4** Узлы `protocol == olcRTC` не показывать / не выбирать. Сообщение `olcRTC недоступен на iPhone.`
- [ ] **3.4.5** `GET /v1/apps/discovered` может быть пустым — UI не падает.

#### 3.5 Проверка на телефоне (VPN выключен)

- [ ] **3.5.1** `./scripts/ios/dev.sh`
- [ ] **3.5.2** Health 200 после токена; `runtime_ready` может быть **false** — это норма до in-process engine. Версия control-plane = semver.
- [ ] **3.5.3** Импорт живой HTTPS-подписки.
- [ ] **3.5.4** Импорт битого URL — русская ошибка.
- [ ] **3.5.5** Импорт файла JSON.
- [ ] **3.5.6** Список профилей и узлов заполнен.
- [ ] **3.5.7** Refresh профиля.
- [ ] **3.5.8** Delete когда disconnected разрешён; во время starting — нет (пока starting не будет без VPN — проверить блокировку кодом).
- [ ] **3.5.9** Данные в контейнере group, когда entitlements группы уже в оболочке… Если оболочки без group — Application Support ок до фазы 4, но **задокументировать**, что профили придётся импортировать заново после смены bundle/group.

**Критерий фазы 3:** импорт + список узлов на iPhone без системного VPN. `runtime_ready` может быть false. Connect / probe-ms **не** входят в критерий.

---

### Фаза 4. PacketTunnel datapath

**Зачем.** Системный VPN. Без фазы 0 не начинать.

**Развилка (закрыть письменно в `docs/platforms/IOS.md` до кода 4.3):**

> Datapath sing-box/libbox работает **только** в extension. UI-процесс держит HTTP control-plane для импорта. `POST /v1/runtime/start` в UI **не** поднимает второй TUN. Extension читает envelope из App Group и стартует engine сам.

Если после прототипа это невозможно — переписать абзац здесь же, не держать два движка.

#### 4.1 Включение VPN-packaging

- [ ] **4.1.1** `cp ios/xtool.vpn.yml ios/xtool.yml` (bundle снова `eu.nagavpn.naga_network` + extension).
- [ ] **4.1.2** Entitlements app и appex: `packet-tunnel-provider` + app group.
- [ ] **4.1.3** Удалить с телефона оболочку `eu.nagavpn.naganetwork`, чтобы не путать иконки.
- [ ] **4.1.4** `NSExtensionPrincipalClass` = `PacketTunnel.PacketTunnelProvider` (уже).
- [ ] **4.1.5** Профили из фазы 0 должны совпасть с этими bundle. Если xtool префиксит XTL — Info.plist/bundleID в yml должны совпасть с порталом, не наоборот «как удобнее».

#### 4.2 UI-сторона VPN

- [ ] **4.2.1** Расширить `NagaVPN.prepare`: `providerConfiguration` с `profile_id`, display name.
- [ ] **4.2.2** `proto.serverAddress` = имя профиля или `Naga Network` (не IP).
- [ ] **4.2.3** `excludeLocalNetworks = true`; APN/cellular с `#available(iOS 16.4, *)`.
- [ ] **4.2.4** Подписка на `NEVPNStatusDidChange`.
- [ ] **4.2.5** Маппинг статуса → UI. `invalid` после prepare не должен оставаться.
- [ ] **4.2.6** Connect: prepare → save → `startVPNTunnel()`. **Не** `POST /v1/runtime/start` в UI-процессе (это Flutter/Android путь). Не звать startVPNTunnel, если `canConnect == false` / expired.
- [ ] **4.2.7** Системный диалог VPN — первый раз; отказ = ошибка в UI, не ретрай-шторм.
- [ ] **4.2.8** Disconnect: `stopVPNTunnel()` + UI disconnected. Не `NagaStop()` всего control-plane (импорт после Disconnect должен жить).
- [ ] **4.2.9** IPC: пока VPN up, `GET /v1/runtime`, select, probe, auto, stats идут в **extension**, не в локальный пустой `StartLocal`. Канал зафиксировать до кода 4.3: `NETunnelProviderSession.sendProviderMessage` и/или UNIX-сокет в App Group. Лимит размера message учесть (не тащить весь envelope JSON в одну datagram, если не влезает).

#### 4.3 Extension start/stop

- [ ] **4.3.1** `startTunnel`: прочитать envelope/профиль из App Group. Нет файла → `completionHandler(NSError)` **без** `setTunnelNetworkSettings`.
- [ ] **4.3.2** Поднять engine (libbox). Ошибка → fail-closed, тот же путь.
- [ ] **4.3.3** Собрать `NEPacketTunnelNetworkSettings`:
  - IPv4/IPv6 из ядра (адреса/маски/includedRoutes);
  - DNS servers из ядра;
  - `tunnelRemoteAddress` = адрес исходящего узла (не выдумывать `127.0.0.1` без проверки, что Apple принимает; зафиксировать выбранное значение тестом/комментарием в IOS.md).
- [ ] **4.3.4** `setTunnelNetworkSettings` успех **потом** `packetFlow` read loop.
- [ ] **4.3.5** Пакеты: `NEPacketTunnelFlow.readPackets` → libbox → `writePackets`. Не user-space TUN fd как Linux.
- [ ] **4.3.6** `stopTunnel`: engine stop, flow cancel, `completionHandler()`.
- [ ] **4.3.7** `sleep`/`wake`: не рвать сессию зря.
- [ ] **4.3.8** Memory: никаких картинок/SwiftUI в appex.

#### 4.4 In-process engine (это отдельный Go-эпик, не «линк того же libnaga.a»)

`cmd/naga-ios` + текущий `Adapter` **не** поднимают VPN на iPhone: они ищут бинарь и делают `exec`. Пока нет in-process backend, фаза 4 не закрывается.

- [ ] **4.4.1** Новый `engine.Adapter` для ios: libbox/sing-box **в процессе** extension (CGO или gomobile). Не `exec`, не hev, не mixed, не `libsingbox.so` как на Android.
- [ ] **4.4.2** Тот же процесс должен гонять **auto/health** `RuntimeController` (лимит 3, 45 с). Не оставлять контроллер только в UI — после свайпа приложения hops умрут.
- [ ] **4.4.3** `preflight_ios.go`: `CheckTUN() error { return nil }` (`//go:build ios`). В `preflight_other.go` добавить `&& !ios` (сейчас тег `!linux && !windows` — ios попадает туда же → duplicate symbol). Иначе старт TUN падает с «TUN пока не реализован» или не собирается.
- [ ] **4.4.4** `adaptTUN` passthrough (`tun_ios.go`). Не вернуть `naga-tun0`.
- [ ] **4.4.5** Доку sing-box / Apple NE: `inet4_route_address`, DNS. Не копировать Linux `auto_detect_interface=true` вслепую. Вывод — в IOS.md.
- [ ] **4.4.6** DNS — `NEDNSSettings`, не resolvectl. Rule-set качает ядро.
- [ ] **4.4.7** Два архива, если один не влезает в appex: `naga-ios` (HTTP UI: import/list) и `naga-ios-tunnel` (engine+`RuntimeController`). UI **не** вызывает `POST /v1/runtime/start` так, чтобы поднять второй sing-box. Учесть лимит памяти Network Extension (десятки МБ): картинок в appex нет, Go GC не раздувать.
- [ ] **4.4.7a** Два процесса Go на одном App Group store: запись профиля из UI во время `startTunnel` — гонка. Файловый lock / «пишем envelope, потом start».
- [ ] **4.4.8** AmneziaWG sidecar на iOS: отдельное решение (netstack in-process или скрыть AWG в v1, если exec неизбежен). Не молча обещать `.conf` Connect.

#### 4.5 Чего не включать в v1 туннеля

- [ ] **4.5.1** `isOnDemandEnabled` = false.
- [ ] **4.5.2** `includeAllNetworks` / kill switch — не в первой итерации.
- [ ] **4.5.3** Per-app VPN / `routing.apps`.
- [ ] **4.5.4** Connect on demand по Safari.

#### 4.6 Проверка туннеля

- [ ] **4.6.1** Сборка **без** libbox: Connect → ошибка в UI, в Настройках → VPN нет «подключено».
- [ ] **4.6.2** С libbox: VLESS из подписки, Safari открывает сайт, внешний IP ≠ домашний.
- [ ] **4.6.3** Hysteria2 (если есть в профиле).
- [ ] **4.6.4** Disconnect из приложения.
- [ ] **4.6.5** Disconnect из Настроек iOS → UI disconnected (не зомби).
- [ ] **4.6.6** Повторный Connect.
- [ ] **4.6.7** Airplane → ошибка/reconnect по факту ядра, UI не врёт «подключено».
- [ ] **4.6.8** Wi‑Fi ↔ LTE.
- [ ] **4.6.9** Убить UI свайпом: системный VPN жив; **автовыбор узлов тоже жив** (контроллер в extension). UI после возврата читает `NEVPNStatus` **и** runtime/stats через IPC (4.2.9), не кэш и не свежий пустой `NagaStart()`.
- [ ] **4.6.10** Не тестировать на Linux naga-tun0 пользователя.

**Критерий фазы 4:** живой TUN на Nikita, fail-closed без ядра, один движок, после свайпа UI hops и статистика не умирают.

---

### Фаза 5. Паритет с Flutter (iOS-срез)

Каждый блок — свои шаги и свой мини-критерий. Эталон всегда Flutter compact на Android.

#### 5.1 Главная

- [ ] **5.1.1** Маскот: welcome (нет профиля) / idle / connecting / connected / error / offline.
- [ ] **5.1.2** Имя профиля, провайдер, узел, latency если probe был.
- [ ] **5.1.3** Спарк трафика из runtime samples (как home chart), не фейковые нули.
- [ ] **5.1.4** Импорт URL / буфер / файл с живым API.
- [ ] **5.1.5** Тап по узлу на главной → вкладка Серверы или picker как Flutter `onOpenServerPicker`.
- [ ] **5.1.6** Нет traffic mode switch.

**Мини-критерий:** сценарии home.dart на телефоне.

#### 5.2 Профили

- [ ] **5.2.1** Список карточек: имя, провайдер, engine, квота, expiry.
- [ ] **5.2.2** Активный профиль визуально выделен.
- [ ] **5.2.3** Refresh → `POST /v1/profiles/{id}/refresh`.
- [ ] **5.2.4** Share → `GET .../share`: копировать URL; QR (CoreImage / простой генератор), без камеры.
- [ ] **5.2.5** Delete с подтверждением ««{name}» будет убран из списка.» (как Flutter). Блок по `runtimeBlocksProfileDelete`.
- [ ] **5.2.6** Автообновление подписки — UI-таймер как Flutter (`update_interval_seconds`, default вкл). Не API. Имеет смысл persist через `@AppStorage` (во Flutter флаг сессии теряется).

**Мини-критерий:** два профиля, переключение, шаринг QR читается другим телефоном.

#### 5.3 Серверы

- [ ] **5.3.1** Список **unified** `GET /v1/nodes` (`listUnifiedNodes`), не `GET /v1/profiles/{id}/nodes`. Поля: tag, country, protocol, latency, selected.
- [ ] **5.3.2** Ручной select → `POST /v1/runtime/select` с outbound/runtime_tag как Dart.
- [ ] **5.3.3** Авто → `selectRuntimeAuto`. UI не фильтрует top-3 — это ядро (`AutoCandidateLimit=3`, TUIC last).
- [ ] **5.3.4** Probe → `POST /v1/nodes/probe`. На iOS **без** подключённого VPN idle sing-box не стартует — как Android. Latency ждём после Connect (Clash API). Не требовать ms на disconnected.
- [ ] **5.3.5** Не добавлять гистерезис. Обновление списка с поллингом 2 с, health узлов как ядро (~45 с).
- [ ] **5.3.6** Скрыть olcRTC.

**Мини-критерий:** ручной узел и авто при **включённом** VPN; probe даёт ms после Connect.

#### 5.4 Статистика

- [ ] **5.4.1** Три плитки: Загрузка, Отдача, Сессия. Байты могут быть `0 B`. Rate «Нет данных», пока `trafficAvailable != true`. График `_TrafficChart`. Failover message жёлтым.
- [ ] **5.4.2** Формат байт и rate как `_formatBytes` / `_formatRate`.
- [ ] **5.4.3** `trafficAvailable != true` → «Нет данных».
- [ ] **5.4.4** График samples, если он на странице статистики есть; если только на home — не дублировать зря.

#### 5.5 Настройки и вложенные

- [ ] **5.5.1** «Как выбирать сервер» → экран политики (`_ServerChoicePage`): авто/ручной, без proxy. `GET/PUT /v1/policy`.
- [ ] **5.5.2** Диагностика: health, runtime status, `GET /v1/diagnostics/logs` (limit 200), копировать всё. Поллинг 2 с пока экран открыт.
- [ ] **5.5.3** `POST /v1/diagnostics/client` на ключевые ошибки UI (как Dart `recordClientEvent`), без PII.
- [ ] **5.5.4** Помощь — полный текст ExpansionTile из Flutter.
- [ ] **5.5.5** Версии UI и control-plane.
- [ ] **5.5.6** Нет split-tunnel, нет check-for-IPA, нет autoAppUpdate.
- [ ] **5.5.7** Routing LAN/RU (`GET/PUT /v1/routing`) — только если на Flutter compact это доступно без страницы приложений. Если это спрятано в split-tunnel — **не** тащить. Сверить UI Android. Скорее вынести «обход LAN/RU» простыми тоглами, `apps` игнорировать.

#### 5.6 Чего не переносить (явный отказ)

- [ ] **5.6.1** Tray, Windows titlebar, pkexec.
- [ ] **5.6.2** AppImage sidecar / кнопка обновления бинаря.
- [ ] **5.6.3** `listPackages` / discovered apps.
- [ ] **5.6.4** olcRTC комнаты.
- [ ] **5.6.5** Редактор raw JSON.
- [ ] **5.6.6** Виджеты, Live Activities, Siri, iPad split.

**Критерий фазы 5:** чеклист 5.1–5.5 на живой подписке. Скриншоты рядом с Android. Расхождения только из 5.6.

---

### Фаза 6. Подпись, релизы, устойчивость

#### 6.1 Версии и IPA

- [ ] **6.1.1** Один semver с `app/pubspec.yaml`. Скрипт пишет `CFBundleShortVersionString` (до `+`) и `CFBundleVersion` (после `+`, сейчас `10`).
- [ ] **6.1.2** `./scripts/ios/build-ipa.sh` → `dist/NagaNetwork-<ver>-ios.ipa`.
- [ ] **6.1.3** Не класть IPA в git.

#### 6.2 Профили релиза

- [ ] **6.2.1** Development — ежедневная работа.
- [ ] **6.2.2** Ad Hoc — второе устройство без Developer Mode (нужен UDID в портале).
- [ ] **6.2.3** TestFlight — только после опросника encryption в App Store Connect (`ITSAppUsesNonExemptEncryption` уже true).
- [ ] **6.2.4** App Store / Guideline 5.4 — **не** в v1. Нужна organization.

#### 6.3 Документация

- [ ] **6.3.1** `docs/platforms/IOS.md`: SwiftUI + xtool, фазы 3–4 развилка, идентификаторы, Developer Mode vs Trust.
- [ ] **6.3.2** `README.md` / `README.ru.md`: iOS клиент SwiftUI, не «Flutter когда-нибудь».
- [ ] **6.3.3** `scripts/README.md` и `platform/apple/README.md` актуализировать.
- [ ] **6.3.4** `RELEASES.md` уже содержит имя IPA — не менять имя файла.

#### 6.4 CI и секреты

- [ ] **6.4.1** Linux job: `swift sdk` уже в кэше или skip; сборка unsigned при наличии SDK.
- [ ] **6.4.2** Подпись только на машине с `xtool auth`. API Key / `.p8` / issuer в GitHub Secrets, не в репо.
- [ ] **6.4.3** Не логировать токен control-plane и содержимое профиля.

#### 6.5 Устойчивость после v1-функций

- [ ] **6.5.1** Перезагрузка iPhone: VPN сам не обязан подниматься; после ручного Connect работает.
- [ ] **6.5.2** IPv6 / dual-stack узел.
- [ ] **6.5.3** Captive portal (гостиница): документировать поведение, не ломать fail-closed.
- [ ] **6.5.4** Память extension: Instruments на Mac опционально; на Linux хотя бы не грузить картинки.
- [ ] **6.5.5** 7-дневная оболочка: оставить рецепт `xtool.yml` без extension для UI-превью. Релизный IPA — только vpn yml.
- [ ] **6.5.6** Журнал диагностики достаточен, Crashlytics нет.

**Критерий фазы 6:** IPA с ADP ставится на чистый iPhone, переживает ребут, Disconnect из системы не оставляет зомби, документы совпадают с кодом.

---

## 5. Порядок файлов, которые появятся (ориентир)

Не создавать заранее пустые заготовки «на будущее». Когда фаза началась:

```
ios/
  Package.swift
  xtool.yml                 # активный (оболочка или vpn)
  xtool.vpn.yml
  Info.plist
  PacketTunnel-Info.plist
  *.entitlements
  Resources/AppIcon.png
  Sources/NagaNetwork/      # App, навигация
  Sources/NagaUI/           # тема, компоненты
  Sources/NagaControlClient/
  Sources/NagaVPNKit/
  Sources/PacketTunnel/
  packaging/keep-text.c
  packaging/ensure-macho-text.py
scripts/ios/dev.sh
scripts/ios/build-ipa.sh
scripts/ios/build-naga-cgo.sh   # фаза 3
cmd/naga-ios/
core/engine/singbox/tun_ios.go
```

Flutter `app/` не удалять: Android и десктоп от него зависят.

---

## 6. Проверки на каждом шаге

На Linux:

```bash
go test ./core/engine/singbox ./core/control ./core/profile
# после фазы 2:
cd ios && swift test
./scripts/ios/dev.sh
```

На iPhone:

1. Установка, иконка, Developer Mode.
2. Холодный старт, фон, возврат.
3. Импорт URL, импорт файла, плохой URL.
4. Список узлов, ручной выбор, авто, probe (**probe ms — после Connect**).
5. Connect / Disconnect / повторный Connect.
6. Потеря сети, airplane, смена Wi‑Fi ↔ LTE.
7. Убить приложение свайпом при включённом VPN — туннель должен остаться или корректно погаснуть, но не «висит подключён» в UI при мёртвом extension.
8. Не проверять симулятором туннель.

Не гонять Connect, пока пользователь не согласился: на Linux не трогать его naga-tun0.

---

## 7. Риски и развилки

| Риск | Что делать |
|---|---|
| ADP не одобряют / не тот аккаунт | Оболочка живёт на 7-дневном сертификате. Туннель не начинать. |
| xtool 409 на имени App ID | Ручные Identifiers с человеческим Description; не ждать фикса xtool. |
| zsign `LoadCommands` / нет `__text` | Уже патчится скриптом. Если сломается после линка Go — расширить патч, не переходить на Mac-only codesign без нужды. |
| CGO не линкуется в SwiftPM Linux | Класть `libnaga.a` через `ld64.lld` trampoline, по аналогии с keep-text. |
| Два sing-box (UI + extension) | Одна запись в IOS.md до кода фазы 4. Connect = `startVPNTunnel`, не HTTP start. |
| UI убит, VPN жив, `/v1/runtime` пустой | IPC в extension обязателен (4.2.9). Не читать loopback нового `NagaStart`. |
| Duplicate `CheckTUN` на ios | `preflight_other.go` исключить ios. |
| Память appex / два Go runtime | Резать архив; не грузить UI-ассеты в extension. |
| Flutter и SwiftUI разъедутся | Чеклист экранов фазы 5; новые фичи сначала в ядре, потом оба UI. |
| Нужен магазин | Organization + 5.4; этот план v1 магазин не открывает. |
| Хочется «просто Flutter» | Нужен Mac/CI macos. Это другой план, не этот файл. |

Развилка «оставить Flutter на Mac vs SwiftUI на Linux» уже выбрана этим документом: **SwiftUI + xtool**. Если появится стабильный Mac в сборке — не смешивать два iOS UI в одном релизе.

---

## 8. Что считается v1 на iPhone

Входит:

- импорт подписки и sing-box JSON через ядро; импорт AmneziaWG `.conf` в список профилей — да, **Connect AWG** — только если закрыт 4.4.8, иначе скрыть;
- TUN VPN (VLESS, Hysteria2, TUIC, SS, Trojan, VMess — что отдаёт профиль и умеет in-process libbox);
- автовыбор как на desktop;
- пять вкладок, диагностика, QR шаринга профиля;
- IPA `NagaNetwork-<ver>-ios.ipa`;
- сборка с Linux после ADP.

Не входит:

- App Store;
- olcRTC;
- system proxy;
- split-tunnel по приложениям;
- автообновление IPA;
- виджеты, Live Activities, Siri;
- iPad-специфичный layout (должен запускаться, но приоритет iPhone compact);
- переписывание Android/Linux на Swift.

---

## 9. Как работать по плану

1. Сначала фаза 0 (ADP). Без неё фазы 4–6 не начинать.
2. Фазы 1–2 можно делать параллельно с ожиданием ADP: UI и HTTP-клиент ставятся оболочкой.
3. Фаза 3 начинается, когда Darwin SDK уже стоит (`swift sdk list` → `darwin`) — это уже выполнено на CachyOS.
4. Не пропускать критерий выхода. Особенно fail-closed туннеля.
5. Коммит — только по просьбе, одним смысловым куском на фазу или подфазу, сообщение EN+RU.
6. Этот файл — живой. Ошибки плана править здесь же, не плодить `SWIFTUI_CLIENT_PLAN2.md`.
