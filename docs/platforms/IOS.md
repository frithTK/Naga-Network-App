# iOS

v1 — свой iPhone: development, Ad Hoc или внутренний TestFlight. App Store не открываем.

Имя ассета: `NagaNetwork-<ver>-ios.ipa`. Сборка **с Linux, без Mac**: SwiftPM-приложение в [`ios/`](../../ios) через [xtool](https://github.com/xtool-org/xtool).

```bash
./scripts/ios/install-xtool.sh   # один раз: AppImage → ~/.local/bin/xtool
xtool setup                      # Apple ID / API Key + Xcode.xip → Darwin SDK
./scripts/ios/dev.sh             # сборка, подпись, установка на iPhone
./scripts/ios/build-ipa.sh       # dist/NagaNetwork-<ver>-ios.ipa
```

Пока ADP не активен, `ios/xtool.yml` — оболочка без PacketTunnel (bundle `eu.nagavpn.naganetwork`, 7-дневный сертификат). Полный VPN: `cp ios/xtool.vpn.yml ios/xtool.yml`.

## Текущая реализация

В оболочке пять вкладок: Главная, Профили, Серверы, Статистика, Настройки.
SwiftUI-экраны, тема Inter и маскоты лежат в `ios/Sources/NagaUI` и
`ios/Resources`. Помощь, диагностика и выбор сервера открываются из настроек.

`NagaControlClient` — отдельный модуль Codable-моделей и HTTP API, который
тестируется на Linux. URLSession обращается только к локальному control-plane,
передаёт Bearer-токен, не следует редиректам и не загружает подписки напрямую.
ATS разрешает локальные обращения; исключение HTTP задано для `127.0.0.1`
([описание Apple](https://developer.apple.com/documentation/bundleresources/information-property-list/nsapptransportsecurity/nsallowslocalnetworking)).

Go-ядро ещё не слинковано. Поэтому реальный импорт пока недоступен, а Connect
показывает сообщение об отсутствии ядра без системного VPN-диалога.
Статус опрашивается раз в 2 секунды только при активной сцене; удаление профиля
блокируется при `starting`, `connected` и `stopping`.

Проверка интерфейса без Active ADP:

```bash
cd ios
swift test
xtool dev build -c debug --sign --ipa
xtool install xtool/NagaNetwork.ipa
```

Фикстуры тестов получают из настоящего Go API, без внешних подписок и без
запуска VPN. Обновить из корня проекта:

```bash
go run ./scripts/ios/export-api-fixtures.go
```

Установка и ручной запуск оболочки проверены на Nikita. Пользователь подтвердил
работу кнопок; на снимке главной видны все пять вкладок, логотип и маскот.
Обрезанный подзаголовок исправлен переносом текста. 22 теста API прошли на Linux.
`xtool launch` на этой машине вернул `DebugserverClient.Error.unknown`.
Проверка остальных экранов и состояний остаётся в `SWIFTUI_CLIENT_PLAN.md`;
снимки всех вкладок пока не получены.

Development-подпись на iOS 16+ требует **Режим разработчика**, это не то же самое, что «Доверять»: Настройки → Конфиденциальность и безопасность → Режим разработчика → включить и перезагрузить. Ad Hoc / TestFlight этого не просят.

`flutter build ipa` на Linux не работает: AOT snapshot для iOS собирается только на macOS. Каталог `app/ios` — запасной Flutter/Xcode каркас, не путь релиза.

## Идентификаторы

- Приложение: `eu.nagavpn.naga_network`
- Extension: `eu.nagavpn.naga_network.PacketTunnel`
- App Group: `group.eu.nagavpn.naga_network`
- min iOS 16. Нужен **Apple Developer Program** (не Enterprise, не Personal Team).

На оба App ID: Network Extensions → Packet Tunnel, App Groups. Четыре профиля: Development и Distribution × приложение и extension. UDID устройства — в Devices для development/Ad Hoc. После `xtool setup` это можно завести через `xtool ds`.

## Как устроено

xtool собирает два SwiftPM-продукта из `ios/xtool.yml`: UI `NagaNetwork` и Foundation-extension `PacketTunnel` (`PlugIns/*.appex`). PacketTunnel — отдельный процесс: пока libbox не слинкован, `startTunnel` сразу возвращает ошибку (fail-closed, мёртвый VPN не остаётся).

Go control-plane (`cmd/naga-ios`) пока не входит в Linux-сборку: CGO под iOS нужен clang из Darwin SDK. `NagaControl` готовит каталог в App Group, а в оболочке без entitlement — Application Support. До линковки ядра HTTP API в приложении не запускается. После перехода с оболочки на VPN-bundle профили нужно будет импортировать заново: это другой контейнер приложения.

Системный прокси, olcRTC и AmneziaWG в v1 недоступны. Per-app VPN на iOS — MDM, не делаем. Кнопки обновления IPA нет.

## Проверка

Симулятор PacketTunnel не проверяет. Нужен физический iPhone. Первый development-install обычно по USB (доверие компьютеру, pairing, `usbmuxd`). Дальше `xtool dev` умеет и по сети.

`ITSAppUsesNonExemptEncryption` = true: sing-box использует собственный TLS/QUIC. Перед TestFlight ответить на опросник App Store Connect по [доке Apple](https://developer.apple.com/documentation/security/complying-with-encryption-export-regulations).

Guideline 5.4 (магазин): VPN только у organization. Individual годится для своего устройства.
