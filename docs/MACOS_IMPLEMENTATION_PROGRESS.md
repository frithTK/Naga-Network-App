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

Добавлены `build-dmg.sh` и `check-dmg.py`: `.app` + ссылка `/Applications` + инструкция о доступном объёме тестовой сборки. Проверка использует `hdiutil verify`, read-only mount, повторную проверку bundle/signatures/ARM64 и запуск встроенного backend прямо со смонтированного образа. Сохраняются SHA-256 и отчёт inspection. DMG — development/ad-hoc, без Apple notarization; не публикуется как рабочий VPN-релиз. Все шесть native jobs [run 39150201059](https://github.com/frithTK/Naga-Network-App/actions/runs/39150201059), commit `9c3f5ed5f8b8c42509c7053c40a4f867d307fd69`, прошли. DMG смонтирован read-only; прошли проверка подписей/ARM64/зависимостей и запуск встроенного backend со смонтированного образа. Дополнительно перед финальной сборкой добавлен drain текущих HTTP-запросов до пяти секунд при завершении backend; его локальные session tests и повторный native CI прошли.

## Проверенный пакет для M2

Исходный commit `87bcd7b5fd66cb58cc8ff64417356ebac4acb7f9`, [run 39150715018](https://github.com/frithTK/Naga-Network-App/actions/runs/39150715018): все шесть native jobs **PASS**. Оба запуска общих Quality checks для push/PR также **PASS**. На macOS прошли Go tests/vet, Dart analyzer, 127 Flutter tests; три Linux golden tests штатно пропущены на macOS и проверены общим Linux job.

- [Прямой DMG](https://github.com/frithTK/Naga-Network-App/releases/download/macos-dev-1.2.9-b14/NagaNetwork-1.2.9-macos-arm64.dmg): публичное скачивание без входа в GitHub; остальные файлы и SHA256SUMS — в [тестовом релизе](https://github.com/frithTK/Naga-Network-App/releases/tag/macos-dev-1.2.9-b14). Исходные inspection reports и альтернативный ZIP `.app` сохранены в Actions run.
- [Диагностический service probe A/B](https://github.com/frithTK/Naga-Network-App/releases/tag/macos-dev-1.2.9-b14): отдельные версии с протоколами 1/2; инструкция [M2 service probe](MACOS_M2_SERVICE_PROBE.md).
- DMG: 31 463 784 байта; SHA-256 `c634e5e6f19215f90ae29f92d39880560091af12a7c773ba3720a8ed076cd548`.

Финальные артефакты скачаны и проверены локально: checksum образа совпадает с SHA256SUMS и native inspection report, source commit совпадает; оба ZIP пробника имеют совпадающие checksum и ARM64 app/helper. Во время native CI образ прошёл `hdiutil verify`, read-only mount, strict signature/ARM64/dependency inspection четырёх Mach-O, startup/auth/origin/EOF smoke backend и отмонтирование.

**Открытые проверки:** пользователь сообщил, что service probe на M2 ещё не запускал. Физическое открытие окна/NSOpenPanel, сохранение профилей после переустановки и lifecycle процессов остаются непроверенными. Для privileged VPN нужны результаты регистрации/XPC A и дальнейшие M00.03–M00.07; системная интеграция, TUN/DNS и рабочее VPN-подключение не объявляются готовыми. Cache/helper-state paths, миграция dev-данных и Darwin network detector остаются открытыми задачами M02. Полный статус плана отражён в подшагах; этот DMG является тестовой поставкой текущей реализации.

## Исправление доступа к скачиванию

Пользователь получил 404 по ссылке Actions artifact. Проверка API подтвердила, что artifact существует и не истёк. Для скачивания через браузер опубликован отдельный публичный pre-release `macos-dev-1.2.9-b14` из проверенного commit; DMG и A/B probe перенесены без изменения файлов, опубликованы общие checksum и инструкция. GitHub asset digest DMG совпадает с проверенным SHA-256. Прямой DMG URL проверен без авторизации: HTTP 200. Production latest остаётся `v1.2.9`.

## Встроенная проверка Mac

По просьбе пользователя обычная проверка перенесена в основное приложение. Кнопка **«Проверить Mac»** автоматически проверяет backend health, регистрирует status-only diagnostic helper, ожидает разрешение macOS, проверяет root XPC status и удаляет регистрацию. Отмена и штатное завершение запускают cleanup; при ошибке удаления есть отдельная кнопка повтора. Успех требует health, root status и `notRegistered` после удаления. Отчёт копируется явно пользователем, с фильтрацией полей и без профилей, токенов, журналов и личных путей. [Краткая инструкция](MACOS_QUICK_CHECK.md), [контракт](MACOS_PROCESS_CONTRACT.md).

Проверенный source `6f4e2bf382d2d21d293e0cb2b6de5d9abc233506`: все шесть jobs [native run 39152885679](https://github.com/frithTK/Naga-Network-App/actions/runs/39152885679) **PASS**, оба общих Quality checks **PASS**. Локально прошли Dart analyzer и 136 Flutter tests; на macOS — 133 tests и три штатных пропуска Linux goldens. Новые проверки покрывают один клик/копирование, приватность отчёта, ожидание разрешения, отмену с ожиданием cleanup, отказ cleanup с повтором, channel error и закрытие UI. Native CI скомпилировал app/helper, проверил пять Mach-O, точный daemon plist, локальный `--self-check` helper и packaged backend; DMG прошёл read-only mount, проверки кода/запуска и отмонтирование. Регистрация службы в CI не выполняется.

Опубликован отдельный [pre-release с кнопкой проверки](https://github.com/frithTK/Naga-Network-App/releases/tag/macos-dev-1.2.9-check1). [Прямой DMG](https://github.com/frithTK/Naga-Network-App/releases/download/macos-dev-1.2.9-check1/NagaNetwork-1.2.9-macos-arm64.dmg) проверен без авторизации: HTTP 200. Размер 31 521 525 байт; SHA-256 `a49a532785649ed871bbaebd04709c0ea49eedfcc116400b299eaee9bdf13d26` совпадает с локальным образом, checksum, inspection report и GitHub asset digest. Основной Latest остаётся `v1.2.9`.

**Остаётся проверить на M2:** первое открытие и фактические SMAppService authorization/XPC/unregister. Новая кнопка собирает первоначальное доказательство без отдельных A/B, Терминала и перезагрузки. A/B, reboot и upgrade остаются инженерными сценариями; production helper, TUN/DNS и VPN ещё не реализованы. При принудительном завершении UI watchdog ограничивает жизнь диагностического процесса; запись регистрации удаляется при следующем открытии. Новая реализация не объявляет закрытыми все M00.03–M00.07.

## Отказ первого открытия, 2026-10-19

Пользователь получил отказ Finder до запуска встроенной проверки. Точный опубликованный DMG проверен в [run 39207970950](https://github.com/frithTK/Naga-Network-App/actions/runs/39207970950): на macOS 14/26 **PASS** для LaunchServices, окна, backend, стабильного запуска и штатной остановки. Браузерный карантин этим не проверяется. `syspolicy_check distribution` на обеих ОС **REJECTED**: отсутствует notarization ticket, есть internal Xprotect error; macOS 26 также сообщает, что ad-hoc подпись не подходит для распространения. Причина конкретного отказа на пользовательском Mac пока не установлена.

Исправлен пробел CI: добавлен фактический запуск GUI и отдельно учитывается distribution assessment. Удаляются непереносимые пути поиска библиотек Flutter, нормализуются executable permissions, framework заново подписывается перед app. Новые regression tests проверяют реальные проблемные пути, пробелы и symlink escape. Первое macOS выполнение нового path test обнаружило сравнение `/var` и `/private/var` в fixture; корень теста исправлен через `resolve()`.

Подробности и условия штатной поставки: [отказ запуска](MACOS_LAUNCH_FAILURE.md). Developer ID/notarization не предоставлены, установка скачанного пакета остаётся неподтверждённой. Повторная ad-hoc сборка сама по себе не закрывает проблему распространения.

## Успешная проверка устройства, 2026-10-19

Пользователь подтвердил macOS **26.2** и установленную копию в «Программах», затем передал [исходный JSON](macos-test-results/2026-10-19-macos-26.2-quick-check.json): версия 1.2.9+14, diagnostic `integrated-1`, `overall_result: pass`. Приложение запущено от обычного пользователя; backend health прошёл. Служба зарегистрирована (`enabled`), XPC вернул root status (`effective_uid: 0`), автоматический unregister завершился `not_registered` / cleanup pass.

Это первое подтверждение регистрации/root XPC/unregister на устройстве для встроенного status-only helper. Способ первого открытия, source commit и checksum установленного DMG в отчёте отсутствуют. Пользователь не хочет ручных команд в Терминале и не имеет Apple Developer Program; командный обход не считается выполненным. Воспроизводимый способ GUI-установки ещё уточняется. PKG пока только рассмотрен, не создан и не проверен.

M00.04.03/.05 частично подтверждены, но отказ разрешения, повторная регистрация, наблюдение завершения процесса helper, reboot/A–B/upgrade и Apple-issued signing не закрыты. Физический результат macOS 26.2 не подтверждает регистрацию на macOS 14. `vpn_tested: false`; сетевой трафик/TUN/DNS не проверялись. Для Go → bridge → helper и production authority нужны следующие задачи M00.05/M03.

Все шесть native jobs [run 39208827007](https://github.com/frithTK/Naga-Network-App/actions/runs/39208827007), source `907e71b91e705e55807436f8bab940942b76421a`, **PASS**, оба Quality checks **PASS**. Включены новые path regression tests, actual GUI launch/quit и отдельный distribution assessment. Последний остаётся `REJECTED` для ad-hoc пакета; исправление RPATH не объявляется исправлением первоначального отказа Finder.
