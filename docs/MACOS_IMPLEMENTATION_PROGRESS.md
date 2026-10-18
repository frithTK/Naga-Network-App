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

Release `.app` build, Dart analyzer и Flutter tests прошли на native macOS ARM64 runner. Внутри скачанного bundle подтверждены bundle ID `eu.nagavpn.nagaNetwork`, версия `1.2.9`, build `14`, minimum OS `14.0` и thin Mach-O ARM64. Внешний вид на M2 ещё не подтверждён; системный helper основного приложения и VPN не реализованы. Иконки scaffold заменены растеризацией существующего `assets/brand/naga-mark-red.svg`; `MainMenu.xib` согласован с именем Swift-модуля. Для этих двух последних правок требуется следующий native build. Инструкции для артефактов: [scripts/macos/README.md](../scripts/macos/README.md).

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
