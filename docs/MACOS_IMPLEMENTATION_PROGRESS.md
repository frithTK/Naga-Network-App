# Разработка macOS: результаты

Начало: 2026-10-18. План: [этапы](MACOS_DEVELOPMENT_PLAN.md), [подшаги](MACOS_IMPLEMENTATION_STEPS.md).

Рабочая ветка: `feat/macos-arm64-bootstrap`, исходный commit `d2e618a0b84fa24228c4aa92b2ead502a9cb418d`. До перехода на актуальную main сохранены Git bundle, patch и копии четырёх изменённых/новых документов в локальном backup `/tmp/naga-macos-start-byq9ux43`.

## Доступные среды

| Среда | Состояние |
|---|---|
| Linux x86_64 | Доступен; Go 1.27.2, Flutter 3.47.1 / Dart 3.13.1, Swift 6.4 для Linux |
| GitHub Actions | Доступ подтверждён, Actions включены; добавлен macOS ARM64 bootstrap workflow |
| MacBook M2 | Пользователь подтвердил ручную проверку скачиваемых сборок; конкретный macOS build ещё не записан |
| Signing identity | Не предоставлена; Developer ID/native helper registration остаются отдельными проверками |
| Тестовые VPN-серверы | Для этого этапа не предоставлены; CLI smoke не использует сетевой профиль |

## Выполненная подготовка

- **M00.01.01 PASS:** сохранены локальная история, refs и изменённые/untracked документы.
- **M00.01.02 PASS:** получена актуальная `origin/main`, создана отдельная ветка, документы сохранены.
- **M00.01.03 PARTIAL:** записаны инструменты Linux и CI: macOS 14.8.9 (23J631), Xcode 15.4 (15F31d); версия пользовательского M2 ожидается.
- **M00.01.04 PASS:** доступность Linux/Actions/ручного M2 и недостающих signing inputs зафиксирована.
- **M00.01.05 PASS:** [bootstrap run 39137379893](https://github.com/frithTK/Naga-Network-App/actions/runs/39137379893), commit `5bf9d53b4f9a8650620983572006881e12ba6c7f`: все пять jobs завершились успешно. Общие [Quality checks](https://github.com/frithTK/Naga-Network-App/actions/runs/39137379808) также прошли.

## Локальные ARM64 build proofs

| Компонент | Кросс-компиляция | Native execution | Результат |
|---|---|---|---|
| naga-control | PASS | UNAVAILABLE на Linux | Получен thin Mach-O ARM64, Go 1.27.2, app version 1.2.9 |
| sing-box | PASS | UNAVAILABLE на Linux | Получен thin Mach-O ARM64 из закреплённого исходника с security dependency fixes |
| olcRTC | PASS | UNAVAILABLE на Linux | Получен thin Mach-O ARM64 из закреплённого исходника с dependency fixes |
| hev | UNAVAILABLE на Linux | UNAVAILABLE | Требуются macOS SDK и native job |

Каждая bootstrap-сборка сохраняет собственный manifest, source commit, builder digest, признак dirty tree, SHA-256 бинарника, лицензию, Go build info и журнал в `build/macos-bootstrap/`. Native CLI smoke и VPN datapath учитываются отдельно.

## macOS runner

Штатным Flutter 3.47.1 создан `app/macos`. Перенесены только macOS scaffold и запись платформы в Flutter metadata. Настроены macOS 14, ARM64, product name `Naga Network` и ad-hoc signing; App Sandbox отключён согласно принятой внешней архитектуре. Проверены синтаксис workflow, plist/entitlements и Python bootstrap.

Release `.app` build, Dart analyzer и Flutter tests прошли на native macOS ARM64 runner. Внутри скачанного bundle подтверждены bundle ID `eu.nagavpn.nagaNetwork`, версия `1.2.9`, build `14`, minimum OS `14.0` и thin Mach-O ARM64. Внешний вид на M2 ещё не подтверждён; системный helper основного приложения и VPN не реализованы. Иконки scaffold заменены растеризацией существующего `assets/brand/naga-mark-red.svg`; `MainMenu.xib` согласован с именем Swift-модуля. Эти изменения прошли [native build и проверку bundle](https://github.com/frithTK/Naga-Network-App/actions/runs/39138706941/job/117708457962): `check-app.py` сверяет версии с pubspec, strict codesign, отсутствие App Sandbox, ARM64 slice каждого Mach-O и отсутствие абсолютных зависимостей вне системных каталогов. Инструкции для артефактов: [scripts/macos/README.md](../scripts/macos/README.md).

## Подтверждённые native build proofs

В run 39137379893 все четыре компонента собраны как thin Mach-O ARM64 и выполнены на native runner. Скачанные manifests проверены локально; исходники чистые, Go 1.27.2, версия приложения 1.2.9.

| Компонент | Сборка | Native CLI smoke | Ограничение |
|---|---|---|---|
| naga-control | PASS | PASS: `-help` | Backend ещё не встроен в `.app` |
| sing-box 1.13.20 | PASS | PASS: `version`, `check` | Без TUN и VPN-профиля |
| olcRTC | PASS | PASS: ожидаемый usage | Реальная сессия не проверена |
| hev | PASS | PASS: зависимости и usage/version | Системный туннель не проверен |

**M00.02.01/.02 PASS.** Остальные подшаги M00.02 остаются частичными до проверки runtime-конфигурации hev, полного manifest окружения/зависимостей и интеграционных сценариев. Успешный CLI smoke не подтверждает VPN datapath; во всех manifests `vpn_device_acceptance` остаётся `UNAVAILABLE`.

## Прототип SMAppService

Добавлены исходники отдельного пробника A/B и сборочный job: [native/macos/service-probe](../native/macos/service-probe), [build-service-probe.sh](../scripts/macos/build-service-probe.sh). Helper предоставляет только диагностический status. [Native job 117707484054](https://github.com/frithTK/Naga-Network-App/actions/runs/39138371628/job/117707484054) прошёл: обе версии скомпилированы, проверены `codesign --verify --deep --strict` и локальный `--self-check` без регистрации. Скачанный [артефакт A/B](https://github.com/frithTK/Naga-Network-App/actions/runs/39138371628/artifacts/12140592911) проверен локально: SHA-256 совпадают, app/helper — ARM64, минимальная ОС 14, версии 0.0.1/0.0.2. Установка и XPC к root-процессу требуют ручной проверки M2 по [инструкции](MACOS_M2_SERVICE_PROBE.md).

**M00.04.01/.02 PASS для реализации пробника.** **M00.07.01 PARTIAL:** A/B собраны и сохранены; состояние установленной A пока не проверено. Проверки системного разрешения, перезагрузки, удаления регистрации и замены A/B остаются открытыми. Подписанная Apple identity ветвь не проверена. Наличие ad-hoc подписи и успешная компиляция не доказывают возможность регистрации daemon.

В UI macOS определяется отдельно от Linux. Updater выбирает имя ARM64 DMG и исключает установку до реализации macOS lifecycle: Linux installer не запускается, VPN не отключается. Это подготовка M07.06, а не готовый механизм обновления.

Локально прошли 19 updater tests и Dart analyzer. Два новых сценария проверяют выбор DMG даже при оставшемся `APPIMAGE` и отказ установки до скачивания, создания staging, запуска процесса и отключения VPN.

## Нативные Go tests

Первый полный macOS-прогон выявил две платформенные ошибки тестов: проверка olcRTC ожидала Linux routing на неподдерживаемой Darwin-платформе, а проверка рабочего каталога sing-box сравнивала строки путей через `/var` и `/private/var`. Для первой явно ограничены поддерживаемые платформы Linux/Windows; для второй директории сравниваются через `os.SameFile`. Сами проверки rollback и размещения временного config сохранены. Повторный [run 39139085027](https://github.com/frithTK/Naga-Network-App/actions/runs/39139085027), commit `82ed1f77b00f096361b0c0cf20aaa935d9d8d310`: все шесть native jobs прошли, включая полный `go test ./...` и `go vet ./...`. Сборочный proof выполняется перед тестами, чтобы оставлять артефакт и при тестовой ошибке.

## M02: пользовательский backend

Следующая реализация добавляет в пакет `Contents/Helpers/naga-control`, запускает его от пользователя и передаёт readiness по отдельному stdout pipe: версия IPC, версия приложения, собственный случайный loopback endpoint, token и data directory. Dashboard создаётся только после handshake и авторизованного health. Фиксированный порт и чужой HTTP health для bootstrap не используются.

Один data directory защищён POSIX lock; повторный backend получает отказ. Входной pipe определяет жизнь дочернего процесса: EOF/SIGTERM вызывает cleanup, native host ограничивает ожидание остановки восемью секундами. Ранний выход и несовместимый handshake дают экран ошибки с повторным запуском. Принудительное завершение UI также закрывает pipe; поведение окон/процессов на физическом M2 ещё надо проверить.

Профили/настройки/token находятся в `~/Library/Application Support/Naga Network`, журналы — в `~/Library/Logs/Naga Network`. Путь backend передаётся Dart для `app-update.json`; token не проходит через argv или журналы. Проверяются владелец/права каталога, symlink/public token и второй экземпляр. Отдельный cache/helper-state и миграция прежних dev-данных ещё не реализованы.

Добавлены `NSOpenPanel` и ограниченное чтение config-файла: размер проверяется до чтения, поток ограничен 16 MiB + 1, UTF-8 декодируется строго. Поддержаны Unicode и пробелы. VPN API в этой сборке возвращает конкретную причину недоступности до запуска runtime; подключение системного helper остаётся следующим этапом.

Локально прошли Go tests `cmd/naga-control`, `core/control`, `core/storage`, Darwin ARM64 build backend, Swift syntax parse, Dart analyzer и 31 целевой Flutter test. Проверки session выполняют настоящий дочерний процесс и проверяют readiness/auth/origin, отказ второго экземпляра, EOF cleanup, повторный запуск и сохранение token. Добавлен native smoke именно упакованного CLI. Все шесть jobs [run 39148942922](https://github.com/frithTK/Naga-Network-App/actions/runs/39148942922), commit `e6ac79f1014a54ad976bce71dcd1cdc6bc1f5f9b`, прошли. Новая Swift-интеграция скомпилирована, `.app` содержит подписанный ARM64 backend, packaged CLI прошёл readiness/auth/origin/EOF smoke нативно на macOS. Общие Quality checks Linux/Windows/Flutter также прошли. Запуск окон и физические M2-сценарии ещё ожидаются. Контракт: [MACOS_PROCESS_CONTRACT.md](MACOS_PROCESS_CONTRACT.md).

## Тестовый DMG

Добавлены `build-dmg.sh` и `check-dmg.py`: `.app` + ссылка `/Applications` + инструкция о доступном объёме тестовой сборки. Проверка использует `hdiutil verify`, read-only mount, повторную проверку bundle/signatures/ARM64 и запуск встроенного backend прямо со смонтированного образа. Сохраняются SHA-256 и отчёт inspection. DMG — development/ad-hoc, без Apple notarization; не публикуется как рабочий VPN-релиз. Все шесть native jobs [run 39150201059](https://github.com/frithTK/Naga-Network-App/actions/runs/39150201059), commit `9c3f5ed5f8b8c42509c7053c40a4f867d307fd69`, прошли. DMG смонтирован read-only; прошли проверка подписей/ARM64/зависимостей и запуск встроенного backend со смонтированного образа. Дополнительно перед финальной сборкой добавлен drain текущих HTTP-запросов до пяти секунд при завершении backend; его локальные session tests проходят, повтор native CI ожидается.
