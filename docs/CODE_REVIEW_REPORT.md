# Code review Naga Network — 2026-10-13

Это исходный отчёт до исправлений. Актуальное состояние: [FIX_AND_SECURITY_PROGRESS.md](FIX_AND_SECURITY_PROGRESS.md).

**Обзор доступного кода и компонентных проверок завершён. Найдены 17 дефектов/отклонений: 8 P1 и 9 P2. Готовность релиза не подтверждена: текущий Windows updater не компилируется, открыты проблемы обновлений, TUN lifecycle и конфигурации. Сквозные проверки сети и устройств остаются UNAVAILABLE.**

База: `2fe0f50043bdafe8cc59ee0e9a673f1a4ede31de` (`main`) плюс незакоммиченные изменения и новые исходники. [План](CODE_REVIEW_PLAN.md) содержит 121 шаг; [выполнение каждого шага](CODE_REVIEW_PROGRESS.md) фиксирует PASS/FAIL/UNAVAILABLE/N/A. Исправления production-кода, установка приложений, публикация релиза и изменения рабочей сети не выполнялись.

## Охват и пределы

В снимке 489 исходных файлов, тестов, конфигураций, документов и ресурсов; 490-я строка реестра — исключённая пользовательская фотография. Все файлы распределены по этапам. Для 156 файлов явно учтено чтение критических ветвей; остальные охвачены инвентаризацией и применимыми анализаторами/тестами/сборками. Это не заявление о ручном построчном чтении каждого файла или отсутствии иных дефектов. Native ветви учитываются отдельно.

[Baseline и SHA256](testing/audit/20261013T100903Z-full-review/baseline.json), [исходный diff](testing/audit/20261013T100903Z-full-review/working-tree.patch), [реестр по файлам](testing/audit/20261013T100903Z-full-review/coverage.csv), [архитектура](testing/audit/20261013T100903Z-full-review/architecture.md), [владение ресурсами и состояния](testing/audit/20261013T100903Z-full-review/resources-and-states.md), [маршруты API](testing/audit/20261013T100903Z-full-review/route-matrix.csv), [применимость платформ/движков](testing/audit/20261013T100903Z-full-review/matrix.csv). Все проверки выполнялись в защищённой копии, новые regression proofs добавлены только в её тестовые каталоги.

## Автоматические результаты

| Проверка | Результат | Ограничение |
|---|---|---|
| Go tests | PASS: 397 test/subtest pass, 3 skip | Windows-specific runtime tests пропущены; это не 397 независимых пользовательских сценариев |
| Go vet / expanded race | PASS | Только host-применимые ветви; дополнительный ruleset proof воспроизводит F01 |
| Dart analyze / Flutter tests | PASS: 106 пользовательских tests; 118 runner events с loading | Goldens не обновлялись; export-only ветви disabled, VPN/OS channels mocked |
| Swift tests | PASS: 22 tests, 3 suites | Linux-совместимый API module; SwiftUI background не исполнялся |
| Go modules verify / offline Flutter restore | PASS | Lockfiles исходного workspace не менялись |
| Flutter Linux release | PASS | Собраны Dart и Linux C++ runner; настоящий tray/TUN не исполнялись |
| Go Android arm64 shared library | PASS; PT_LOAD 4×16384 | Это не полный APK и не 16KB device execution |
| iOS SwiftUI shell, unsigned xtool build | PASS | Go framework, PacketTunnel и release signing этим не подтверждены |
| Windows Go cross compile | FAIL F07 | Dummy executor использован только для компиляции, Windows tests не запускались |
| Fuzz | PASS: 7 целей × 60 с; 5,916,819 выполнений | Нет crash/hang в этом бюджете; дефекты вне него не исключаются |
| Syntax/resources | PASS: 146 проверок | Bash -n, Python AST, JSON/XML/plist/image; ShellCheck/PowerShell отсутствуют |
| Controller lifetime | PASS: 200 mock Start/Stop, race; goro 2→2, FD 6→6 | Нет реальных engine/TUN/DNS resources |
| Storage write failure | PASS | Read-only directory: старый config сохранился; реальный ENOSPC/crash не воспроизводились |
| API fixtures | PASS: 11 fixtures семантически совпали | Единственное отличие exporter — updated_at; originals не перезаписаны |
| Vulnerability scans | Выполнены; имеются alerts D01–D03 | JSON exit 0 означает завершение сканера, а не отсутствие уязвимостей |

Coverage baseline: Go 5411/8398 statements (64.4%), Dart 2769/4355 executable lines (63.6%). Это разные метрики и разные наборы native exclusions; общий процент «покрытия проекта» не вычисляется. Обычные тесты проходят при наличии воспроизведённых дефектов — новые proofs выявляют недостающие ветви.

Точные argv, версии, cwd, exit codes и logs находятся в [checks/](testing/audit/20261013T100903Z-full-review/checks/). Go `go1.27.1-X:nodwarf5`, Flutter `3.47.1`, Dart `3.13.1`, Swift `6.4`, sing-box host `1.14.3`, Android NDK `28.2.13676358`, govulncheck `v1.9.0`. Первые Go/Swift sandbox отказы отнесены к окружению и повторены вне sandbox; ошибки самого harness не включены в находки.

## Реестр находок

P1 — исправить до выпуска затронутой функции/платформы. P2 — существенная ошибка корректности, устойчивости или границы файлов, которую следует исправить следующим блоком. Приоритет не означает, что проведена эксплуатация или что affected path доступен любому удалённому субъекту. У каждой записи ниже отдельно указаны предпосылки и доказательство.

| ID | Приоритет | Проблема |
|---|---|---|
| F01 | P1 | Гонка данных при загрузке rule sets |
| F02 | P1 | Updater перезаписывает файл через заранее созданную symlink |
| F03 | P1 | Падение Android hev оставляет TUN FD открытым |
| F07 | P1 | Windows updater не компилируется |
| F08 | P1 | Android updater игнорирует подтверждение PackageInstaller |
| F09 | P1 | Неудачная сборка iOS упаковывает старое приложение |
| F12 | P1 | Преобразование system proxy удаляет готовый listener |
| F17 | P1 | Elevated Windows host доверяет произвольному пути журнала из IPC |
| F04 | P2 | Восстановление GNOME proxy затирает чужие настройки |
| F05 | P2 | ZIP updater пишет за каталог установки через symlink |
| F06 | P2 | UI навсегда остаётся в Connecting после потери control |
| F10 | P2 | Profile-Update-Interval: часы интерпретируются как дни |
| F11 | P2 | Mach-O patcher повреждает существующую data section |
| F13 | P2 | Квадратичная подготовка больших подписок |
| F14 | P2 | AmneziaWG молча принимает повреждённые числовые поля |
| F15 | P2 | Rule-set cache никогда не обновляется по update_interval |
| F16 | P2 | HTTP Stop не может отменить незавершённый Start |

### F01 · P1 · Гонка данных при загрузке rule sets

**Код:** [core/ruleset/ruleset.go:231](../core/ruleset/ruleset.go:231).

После cold-wait timeout/отмены Materialize читает paths и возвращает Stats, пока late worker изменяет их. Дополнительный тест с медленным fetcher и коротким cold wait дал сообщения race detector. Аварийное завершение процесса не воспроизводилось; риск конкурентного доступа к map установлен.

**Условие и предел:** Cache miss и загрузка, которая заканчивается после ожидания или отмены. Тест ускоряет реальное условие таймаута; production cold wait равен 8 секундам.

**Подтверждение:** Подтверждено race detector. [checks/proof-ruleset-race-1.log](testing/audit/20261013T100903Z-full-review/checks/proof-ruleset-race-1.log), [proofs/go/core/ruleset/review_audit_test.go](testing/audit/20261013T100903Z-full-review/proofs/go/core/ruleset/review_audit_test.go).

**Происхождение:** Существует в HEAD; файл совпадает с базовым commit.

**Исправление:** Получать неизменяемый snapshot paths/Stats под mutex либо отменять и дожидаться workers. Проверить также смешанные cache hits/misses и ownership результатов.

**Проверка исправления:** Race-тест late fetch, cancellation и смешанного cache; отсутствие гонок и стабильный итоговый config.

### F02 · P1 · Updater перезаписывает файл через заранее созданную symlink

**Код:** [app/lib/app_update.dart:546](../app/lib/app_update.dart:546).

Имя временного каталога вычисляется из timestamp; create(recursive:true) принимает уже существующий каталог. dest.openWrite следует symlink. В тесте sentinel вне каталога обновления заменён скачанными байтами даже при правильном SHA256.

**Условие и предел:** Локальный процесс может заранее создать каталог/ссылку в используемом temp root. На Linux общий /tmp делает это существенным; приватные temp roots других платформ меняют предпосылки. Это не доказательство удалённого RCE.

**Подтверждение:** Подтверждено filesystem test с внешним sentinel. [checks/proof-flutter-2.log](testing/audit/20261013T100903Z-full-review/checks/proof-flutter-2.log), [proofs/flutter/test/review_audit_test.dart](testing/audit/20261013T100903Z-full-review/proofs/flutter/test/review_audit_test.dart).

**Происхождение:** Существует в HEAD; файл совпадает с базовым commit.

**Исправление:** Directory.createTemp со случайным именем и приватными правами; эксклюзивное создание файлов и защищённый staging. Проверять тот же файл, который затем устанавливается, и удалять staging при отказе.

**Проверка исправления:** Предсозданный каталог/symlink, замена между download/check/install, digest mismatch и отмена; внешний sentinel неизменен.

### F03 · P1 · Падение Android hev оставляет TUN FD открытым

**Код:** [core/androidvpn/fd.go:100](../core/androidvpn/fd.go:100), [core/control/runtime_awg.go](../core/control/runtime_awg.go), [core/control/runtime_hevfd.go](../core/control/runtime_hevfd.go).

Результат Wait дочернего hev отбрасывается; состояние bridge и detached FD не сбрасываются. Тестовый child завершился с exit 17, но TunFile остался открытым. Runtime следит за sing-box/SOCKS, что само по себе не проверяет оставшийся bridge.

**Условие и предел:** hev неожиданно завершается после успешного старта. Владение FD подтверждено общим кодом на Linux; симптомы реального Android datapath ещё требуют устройства.

**Подтверждение:** Подтверждено общим native lifecycle на host; Android device pending. [checks/proof-go-1.log](testing/audit/20261013T100903Z-full-review/checks/proof-go-1.log), [proofs/go/core/androidvpn/review_audit_test.go](testing/audit/20261013T100903Z-full-review/proofs/go/core/androidvpn/review_audit_test.go).

**Происхождение:** Существует в HEAD; файл совпадает с базовым commit.

**Исправление:** Один владелец Wait и явное событие failure bridge; закрытие FD, согласованное завершение VpnService/runtime и наблюдаемый reconnect либо failed.

**Проверка исправления:** Early/late child exit, Stop одновременно с exit, повторный Start; затем Android-тест с принудительным завершением только test hev.

### F04 · P2 · Восстановление GNOME proxy затирает чужие настройки

**Код:** [core/control/system_proxy_linux.go:86](../core/control/system_proxy_linux.go:86).

proxyPointsAt проверяет substring порта и игнорирует host. Настройка корпоративного proxy.example:2080 или порта 32080 считается прежним Naga proxy; сохранённый режим заменяется none. Проверено подменой gsettings без изменения рабочего desktop.

**Условие и предел:** До запуска у пользователя задан независимый proxy с совпадающим портом/подстрокой.

**Подтверждение:** Подтверждено тестом с подменой gsettings. [checks/proof-go-1.log](testing/audit/20261013T100903Z-full-review/checks/proof-go-1.log), [proofs/go/core/control/review_audit_test.go](testing/audit/20261013T100903Z-full-review/proofs/go/core/control/review_audit_test.go).

**Происхождение:** Существует в HEAD; файл совпадает с базовым commit.

**Исправление:** Сравнивать точный host и числовой port; сохранять полный исходный набор. Restore должен учитывать настройки, изменённые пользователем после Configure.

**Проверка исправления:** Чужой host:2080, порт 32080, реальный Naga loopback и изменение настроек между Configure/Restore.

### F05 · P2 · ZIP updater пишет за каталог установки через symlink

**Код:** [cmd/naga-update/zip.go:46](../cmd/naga-update/zip.go:46).

Проверка filepath.Rel защищает от лексического ../, но не от существующего symlink родителя. Архив linked/sentinel перезаписал sentinel вне назначения. Тест выполнен на Linux; Windows reparse-path исполнение отдельно.

**Условие и предел:** В каталоге назначения уже существует подконтрольная ссылка либо путь меняется во время распаковки. Обычно нужны права локальной записи; повышение прав этим тестом не установлено.

**Подтверждение:** Подтверждено на Linux filesystem; Windows reparse execution pending. [checks/proof-go-1.log](testing/audit/20261013T100903Z-full-review/checks/proof-go-1.log), [proofs/go/cmd/naga-update/review_audit_test.go](testing/audit/20261013T100903Z-full-review/proofs/go/cmd/naga-update/review_audit_test.go).

**Происхождение:** Существует в HEAD; файл совпадает с базовым commit.

**Исправление:** Распаковывать в новый staging, запрещать ссылки/reparse в цепочке пути, применять безопасное эксклюзивное создание; ограничить количество и общий распакованный размер.

**Проверка исправления:** Traversal, symlink/reparse parent, race замены каталога и oversized archive; обновление остаётся восстанавливаемым.

### F06 · P2 · UI навсегда остаётся в Connecting после потери control

**Код:** [app/lib/main.dart:2785](../app/lib/main.dart:2785).

В error branch счётчик пропусков увеличивается, затем connecting возвращается раньше установки unreachable на третьем пропуске. Widget test: starting snapshot и четыре отказа polling не показывают «Статус неизвестен».

**Условие и предел:** Control-plane перестал отвечать во время подключения. Не утверждается, что сам VPN обязательно отключён.

**Подтверждение:** Подтверждено widget test. [checks/proof-flutter-2.log](testing/audit/20261013T100903Z-full-review/checks/proof-flutter-2.log), [proofs/flutter/test/review_audit_test.dart](testing/audit/20261013T100903Z-full-review/proofs/flutter/test/review_audit_test.dart).

**Происхождение:** Проблемный фрагмент существует в HEAD; вокруг него есть незакоммиченные изменения.

**Исправление:** Обрабатывать потерю control до ветвей отображения runtime, выставлять unknown после порога независимо от starting; восстановление связи возвращает фактический status.

**Проверка исправления:** Starting → repeated API errors → unknown → восстановленный ответ; не показывать ложный disconnected.

### F07 · P1 · Windows updater не компилируется

**Код:** [cmd/naga-update/main.go:64](../cmd/naga-update/main.go:64).

Кросс-компиляция на зависимостях снимка: invalid operation event == windows.WAIT_TIMEOUT (mismatched types uint32 and syscall.Errno). Windows build.ps1 собирает naga-update.exe и останавливается на этой ошибке; portable/setup release заблокирован.

**Условие и предел:** Windows сборка текущего снимка. Наличие настоящей Windows для ошибки типов не требуется.

**Подтверждение:** Подтверждено Windows cross compilation. [checks/windows-compile-2.log](testing/audit/20261013T100903Z-full-review/checks/windows-compile-2.log).

**Происхождение:** Существует в HEAD; файл совпадает с базовым commit.

**Исправление:** Согласовать тип константы/значения при сравнении. Добавить обязательную Windows compile проверку всех cmd в обычный CI.

**Проверка исправления:** GOOS=windows compilation ./... и реальная Windows portable/setup сборка; Windows wait/timeout тест.

### F08 · P1 · Android updater игнорирует подтверждение PackageInstaller

**Код:** [app/android/app/src/main/kotlin/eu/nagavpn/naga_network/MainActivity.kt:28](../app/android/app/src/main/kotlin/eu/nagavpn/naga_network/MainActivity.kt:28).

installReceiver.onReceive пуст. Session.commit передаёт broadcast callback, а MethodChannel сразу сообщает успех запуска. Если приходит STATUS_PENDING_USER_ACTION, intent подтверждения отбрасывается и установка не продолжается.

**Условие и предел:** PackageInstaller требует действия пользователя. Контракт проверен по официальному Android API; поведение на конкретном устройстве пока не запускалось.

**Подтверждение:** Подтверждено кодом и официальным API-контрактом; device execution pending. [первичный источник](https://developer.android.com/reference/android/content/pm/PackageInstaller#STATUS_PENDING_USER_ACTION).

**Происхождение:** Существует в HEAD; файл совпадает с базовым commit.

**Исправление:** Обрабатывать pending/success/failure: запустить Intent.EXTRA_INTENT либо уведомление, сохранить результат с учётом lifecycle и показывать завершение только по финальному статусу.

**Проверка исправления:** Устройство: подтверждение, отказ, запрещённые unknown sources, background и recreation Activity; old app/version сохраняются при отказе.

### F09 · P1 · Неудачная сборка iOS упаковывает старое приложение

**Код:** [scripts/ios/build-ipa.sh:35](../scripts/ios/build-ipa.sh:35), [scripts/ios/dev.sh](../scripts/ios/dev.sh).

Любая ошибка build --sign трактуется как проблема подписи. Если старый .app существует, fallback подписывает/упаковывает его. Детерминированный тест неизменённого скрипта: имя версии 1.2.6, внутри Info.plist 1.2.5, старый marker, exit 0. Аналогичное допущение есть в dev.sh.

**Условие и предел:** Предыдущая сборка оставила .app; следующая компиляция завершается ошибкой. Внешние xtool/sign действия заменены stub; телефон и настоящая подпись не использовались.

**Подтверждение:** Подтверждено неизменённым скриптом с детерминированными build/sign stubs. [checks/ipa-stale-proof.json](testing/audit/20261013T100903Z-full-review/checks/ipa-stale-proof.json), [checks/ipa-stale-proof.log](testing/audit/20261013T100903Z-full-review/checks/ipa-stale-proof.log).

**Происхождение:** Новый файл текущего рабочего дерева; в HEAD отсутствует.

**Исправление:** Отделить compile failure от signing failure; собирать в свежий каталог/проверять marker сборки и bundle version. Не выбирать старый IPA по glob после произвольной ошибки.

**Проверка исправления:** Старый .app + compile error → exit != 0 и отсутствие нового release artifact; отдельные signing-only failure и успешная свежая сборка.

### F10 · P2 · Profile-Update-Interval: часы интерпретируются как дни

**Код:** [core/subscription/client.go:207](../core/subscription/client.go:207).

Для документированного Hiddify header interval в часах код умножает число на 24h. Header 12 даёт 288 часов. Existing tests проекта закрепляют дни; это подтверждённое расхождение совместимости, а не универсальный стандарт HTTP.

**Условие и предел:** Подписка использует часовую конвенцию Hiddify/совместимых клиентов. Перед миграцией нужно проверить собственный provider Naga и старые metadata.

**Подтверждение:** Подтверждено расхождение с документированным Hiddify header convention. [checks/proof-header-interval-1.log](testing/audit/20261013T100903Z-full-review/checks/proof-header-interval-1.log), [первичный источник](https://hiddify.com/app/URL-Scheme/#profile-update-interval).

**Происхождение:** Существует в HEAD; файл совпадает с базовым commit.

**Исправление:** Явно определить единицы общего контракта, поддержать совместимость существующих собственных подписок и миграцию сохранённых интервалов.

**Проверка исправления:** Header 1/12/24, некорректные/переполненные числа, собственный provider и persisted metadata после обновления.

### F11 · P2 · Mach-O patcher повреждает существующую data section

**Код:** [ios/packaging/ensure-macho-text.py:61](../ios/packaging/ensure-macho-text.py:61).

Фиксированный dummy offset от 0x1000 проверяется по границам segment, но не по пересечениям sections. В синтетическом pure-data Mach-O существующий __const по этому адресу заменён четырьмя ARM64 RET.

**Условие и предел:** Расположение sections пересекается с выбранным offset. Такой layout в фактически поставляемом PacketTunnel пока не установлен.

**Подтверждение:** Подтверждено на синтетическом Mach-O layout; наличие такого layout в поставляемом extension не установлено. [checks/macho-proof.json](testing/audit/20261013T100903Z-full-review/checks/macho-proof.json).

**Происхождение:** Новый файл текущего рабочего дерева; в HEAD отсутствует.

**Исправление:** Линковать настоящий keep-text object либо выбирать доказанно свободный диапазон с проверкой всех sections/load commands. Проверять результат до подписания.

**Проверка исправления:** Чистый pure-data layout, занятый offset 0x1000, недостаток padding и повторный patch; исходные data bytes неизменны.

### F12 · P1 · Преобразование system proxy удаляет готовый listener

**Код:** [core/profile/profile.go:544](../core/profile/profile.go:544).

Существующий inbound naga-system-proxy пропускается при построении списка, одновременно выставляется haveProxy=true. Новый inbound добавляется только при !haveProxy. В тесте готовый mixed 127.0.0.1:2080 превращён в пустые inbounds, хотя ОС должна использовать этот port.

**Условие и предел:** Импортируемый config уже содержит inbound с зарезервированным tag; повторная нормализация может дать тот же эффект. Все AWG подключения безусловно сломанными не объявляются.

**Подтверждение:** Подтверждено тестом преобразования конфигурации. [checks/proof-system-proxy-1.log](testing/audit/20261013T100903Z-full-review/checks/proof-system-proxy-1.log), [proofs/go/core/profile/review_audit_test.go](testing/audit/20261013T100903Z-full-review/proofs/go/core/profile/review_audit_test.go).

**Происхождение:** Существует в HEAD; файл совпадает с базовым commit.

**Исправление:** Всегда формировать один канонический proxy inbound либо сохранять и нормализовать существующий; обеспечить идемпотентность преобразования.

**Проверка исправления:** Нет inbound / один / несколько с reserved tag; ApplyTrafficMode дважды даёт тот же рабочий listener и корректные route references.

### F13 · P2 · Квадратичная подготовка больших подписок

**Код:** [core/engine/singbox/adapter.go:932](../core/engine/singbox/adapter.go:932).

prepareRuntimeConfig для каждого outbound повторно вызывает SelectionPath, которая декодирует весь JSON и строит индекс. Для 10/100/1000 узлов: 3,2/139,8/13117,4 мс и 0,36/16,22/1521,62 МБ TotalAlloc. Это суммарные выделения, а не пиковый RSS. Крупная подписка задерживает запуск и создаёт сильное давление на GC.

**Условие и предел:** Подписка с большим количеством узлов; измерение касается подготовки конфигурации, без сети и TUN.

**Подтверждение:** Измерено на синтетических конфигурациях в изолированной копии. [checks/profile-scaling-1.log](testing/audit/20261013T100903Z-full-review/checks/profile-scaling-1.log), [proofs/go/core/engine/singbox/review_audit_test.go](testing/audit/20261013T100903Z-full-review/proofs/go/core/engine/singbox/review_audit_test.go).

**Происхождение:** Существует в HEAD; файл совпадает с базовым commit.

**Исправление:** Один раз разобрать граф outbounds и вычислить пути выбора из общего индекса. Сохранить поддержку вложенных selector/urltest и циклов.

**Проверка исправления:** Повторить одинаковый benchmark 10/100/1000 узлов; проверить идентичность selection paths на вложенных селекторах.

### F14 · P2 · AmneziaWG молча принимает повреждённые числовые поля

**Код:** [core/profile/amneziawg/parse.go:152](../core/profile/amneziawg/parse.go:152).

Ошибки atoi отбрасываются. Jc=broken, MTU=broken и PersistentKeepalive=broken принимаются без ошибки и становятся нулями. IPC затем пропускает нулевые параметры, поэтому повреждение конфигурации меняет её смысл вместо явного отказа.

**Условие и предел:** Импортируемый AWG conf содержит неверное число. Конкретное влияние на соединение зависит от параметра и сервера; отключение всей обфускации этим тестом не доказывается.

**Подтверждение:** Подтверждено тремя минимальными примерами парсера. [checks/proof-awg-numbers-1.log](testing/audit/20261013T100903Z-full-review/checks/proof-awg-numbers-1.log), [proofs/go/core/profile/amneziawg/review_audit_test.go](testing/audit/20261013T100903Z-full-review/proofs/go/core/profile/amneziawg/review_audit_test.go).

**Происхождение:** Существует в HEAD; файл совпадает с базовым commit.

**Исправление:** Отклонять присутствующие, но некорректные числовые значения; проверять диапазоны. Сообщать имя поля без значения и секретов.

**Проверка исправления:** Табличные тесты пустых, неверных, отрицательных, переполненных и корректных значений; совместимость существующих валидных fixtures.

### F15 · P2 · Rule-set cache никогда не обновляется по update_interval

**Код:** [core/ruleset/ruleset.go:176](../core/ruleset/ruleset.go:176).

Любой синтаксически допустимый cache используется без проверки возраста; fetcher не вызывается. ApplyLocal удаляет update_interval. Повторные соединения и импорт того же URL не получают новые правила провайдера.

**Условие и предел:** Провайдер меняет rule set по прежнему URL; пользователь ожидает обновления согласно импортированному interval. Ручное удаление cache пока является обходным путём.

**Подтверждение:** Подтверждено cache возрастом 30 дней и конфигурацией update_interval=1h. [checks/proof-cache-ttl-1.log](testing/audit/20261013T100903Z-full-review/checks/proof-cache-ttl-1.log), [proofs/go/core/ruleset/review_cache_audit_test.go](testing/audit/20261013T100903Z-full-review/proofs/go/core/ruleset/review_cache_audit_test.go).

**Происхождение:** Существует в HEAD; файл совпадает с базовым commit.

**Исправление:** Сохранить metadata времени загрузки и interval; обновлять просроченный cache атомарно, сохраняя пригодный старый вариант при временном сетевом отказе.

**Проверка исправления:** Свежий/просроченный cache, новая версия payload, ошибка fetch, concurrent refresh и отмена; учитывать исправление F01.

### F16 · P2 · HTTP Stop не может отменить незавершённый Start

**Код:** [core/control/server.go:227](../core/control/server.go:227).

Server держит MutationMu весь runtime/start. runtime/stop ждёт тот же mutex и не доходит до Controller.Stop, хотя контроллер умеет отменять запуск. В тесте Stop не завершился за 150 мс и продолжился только после разблокировки probe. Ожидание определяется длительностью Start, а не числом 150 мс.

**Условие и предел:** Start ещё ждёт probe либо другую длительную стадию, одновременно приходит Stop. Физический VPN в воспроизведении не запускался.

**Подтверждение:** Подтверждено блокирующим mock probe через реальные обработчики HTTP. [checks/proof-http-cancel-3.log](testing/audit/20261013T100903Z-full-review/checks/proof-http-cancel-3.log), [proofs/go/core/control/review_cancel_audit_test.go](testing/audit/20261013T100903Z-full-review/proofs/go/core/control/review_cancel_audit_test.go).

**Происхождение:** Существует в HEAD; файл совпадает с базовым commit.

**Исправление:** Выделить путь отмены запуска из общей сериализации мутаций либо ввести операцию запуска с контекстом/ID и отдельным cancel. Сохранить атомарность остальных изменений.

**Проверка исправления:** HTTP Start–Stop на заблокированном probe, Stop–Stop, отмена до/после acquire ресурса; итог stopped и отсутствие оставшихся процессов.

### F17 · P1 · Elevated Windows host доверяет произвольному пути журнала из IPC

**Код:** [core/engine/singbox/host_windows.go:493](../core/engine/singbox/host_windows.go:493).

Helper pipe разрешает процессы того же SID и принимает допустимый mode/pipe без проверки конкретного клиента. Запущенный elevated host читает tunHostRequest из указанного pipe. BinaryPath ограничен bundled sing-box, но LogPath открывается с CREATE|TRUNC без ограничения каталога до запуска движка. Таким образом, привилегированный процесс использует путь, полученный от процесса с меньшими правами.

**Условие и предел:** Установлен elevated helper у пользователя с административным split token; локальный процесс того же SID может обращаться к pipe. Реальный доступ к защищённому файлу и Windows ACL требуют отдельного стенда. Удалённый доступ или доступ другого пользователя не доказаны.

**Подтверждение:** Подтверждено сквозным чтением границы привилегий; эксплуатация на Windows не выполнялась. [core/winhelper/helper_windows.go:152](../core/winhelper/helper_windows.go:152), [core/winhelper/helper_windows.go:285](../core/winhelper/helper_windows.go:285), [core/engine/singbox/host_windows.go:422](../core/engine/singbox/host_windows.go:422), [core/engine/singbox/host_windows.go:473](../core/engine/singbox/host_windows.go:473).

**Происхождение:** Существует в HEAD; файл совпадает с базовым commit.

**Исправление:** Генерировать путь журнала внутри доверенного host в фиксированном защищённом каталоге; исключить пользовательский LogPath из протокола. Проверить reparse points и идентичность подключённого клиента.

**Проверка исправления:** Windows VM: запрос от разрешённого UI и стороннего процесса того же SID, абсолютный чужой путь, reparse parent; защищённый sentinel должен остаться неизменным.

## Зависимости, инструменты и открытые гипотезы

**D01 — Go dependencies.** Прямой source scan на исходном имени compiler показал 7 module-level advisory без symbol chains: compress/s2, x/net/http2 и unmaintained openpgp. Это не доказывает безопасную сборку, поскольку stdlib версия распознавалась неполно.

**D02 — installed sing-box.** Binary scan `/usr/bin/sing-box` 1.14.3 нашёл совпадения с 22 unique advisory в grpc, chi, go-jose, DHCP, x/crypto, x/net, x/mod и x/text. 1556 finding events отражают уровни module/package/symbol и повторные symbols, а не 1556 независимых уязвимостей. Binary scan не устанавливает достижимость через конфигурацию приложения. Например, HTTP/2 alerts требуют проверки реально включённого transport/server; SSH alerts относятся к SSH paths. [Реестр версий, fixes и symbols](testing/audit/20261013T100903Z-full-review/checks/binary-advisory-summary.json). Официальные описания: [HTTP/2 CPU consumption](https://pkg.go.dev/vuln/GO-2026-6611), [SSH channel DoS](https://pkg.go.dev/vuln/GO-2026-6354).

**D03 — версия compiler.** `go1.27.1-X:nodwarf5` не разбирается GoTagToSemver установленного govulncheck как обычный release tag. Повтор с явно записанным `GOVERSION=go1.27.1` (compiler и исходники не менялись) дал 15 unique advisory, из них 9 со static call chains в stdlib. Они зависят от протокола: обычный loopback HTTP server не превращается в HTTP/2 server только из-за импорта net/http. До release сборки нужно использовать исправленный toolchain и повторить scan фактических артефактов; для этих stdlib записей fix — Go 1.27.2. [Source scan summary](testing/audit/20261013T100903Z-full-review/checks/source-advisory-summary.json), [официальный affected-version report](https://pkg.go.dev/vuln/GO-2026-6611). Alerts toolchain относятся к проверенному окружению, а не автоматически к уже опубликованным релизам.

**D04 — сохранение оригинала.** docs/engines/AMNEZIAWG.md обещает `.conf` байт-в-байт; core/control/server.go:snapshotFromConfig удаляет BOM и краевые пробелы. Значимые AWG поля этим наблюдением не потеряны, но буквальное требование R03 не выполнено. Нужно хранить original отдельно от parsing buffer либо уточнить документированную гарантию.

**D05 — release acceptance.** Android build допускает debug-signing fallback без key.properties, а native page-size checker не является обязательным failing gate. Собранный libnaga соответствует alignment, но это не проверка sing-box/hev/olc/Flutter libraries конечного APK. Публичный APK debug-signed этим ревью не объявляется: реальные signing inputs и конечный APK не исследованы.

| Гипотеза | Что известно | Чего не хватает |
|---|---|---|
| H01 resolved-IP filtering | Go direct transport фильтрует resolved IP; Dart download и proxy fetch имеют другую границу | Trusted synthetic TLS/DNS/proxy стенд; доказать обращение к private destination и определить ожидаемую policy |
| H02 Android service readiness | MethodChannel start подтверждается после startForegroundService, до установления detached FD | Device trace первого connect, readiness handshake и отказа establish; не считать race воспроизведённой |
| H03 writable elevated bundle | Bundled executable validator лексический; installer/task path и writable dirs влияют на boundary | Windows ACL/MIC/reparse и фактические install locations; не объявлять proven escalation |
| H04 installer taskkill | Windows installer использует завершение по engine image name | Два независимых VPN клиента в VM; подтвердить unintended process termination |

Не подтверждены и не включены в дефекты: переполнение Swift quota (код saturates), внешний доступ к generated Clash API (принудительные random loopback/secret), отсутствие iOS deep link (onOpenURL есть). Неподключённый iOS datapath и ограничения IPv6/kill switch не выдаются за неожиданные дефекты без соответствующей гарантии.

## Очередь исправлений и необходимые проверки

1. **Разблокировать поставку:** F07 Windows updater; F09/F11 iOS packaging; добавить Windows compilation на push/PR и проверку свежести embedded bundle version.
2. **Закрыть границы файлов и привилегий:** F02/F05/F17. Windows защищённый sentinel/ACL/reparse проверять в VM после исправления; перед поставкой пересобрать на исправленном compiler и повторить scans D01–D03.
3. **Восстановить достоверность сети и статуса:** F01/F03/F12/F16, затем F04/F06. F15 делать вместе с resolver ownership F01; Android bridge failure проверять на устройстве.
4. **Исправить совместимость и нагрузку:** F10 с миграцией provider metadata, F14 numeric parsing, F13 single graph index. Для каждого patch сохранить минимальный regression из карточки и проверить затронутый native path.

Структурный backlog: выделить polling/runtime orchestration, profile flows, updater, OS bridges и settings из 7362-строчного main.dart; сокращать общий shared state UI parts. Делать после поведенческих regression tests, малыми изменениями. Для SwiftUI нужен тестируемый AppModel transport/polling lifecycle и device tests; API fixture tests уже полезны, но не заменяют их. Для CI нужны expanded race с ruleset proof, bounded fuzz jobs, native compilation/packaging gates и artifact vulnerability scans. Автоматическое обновление pinned dependencies без проверки engine compatibility не предлагается.

## Итог платформ и дальнейший native прогон

| Платформа | Проверено | Не проверено / стенд для продолжения |
|---|---|---|
| Linux | Go tests/vet/race, настоящий sing-box config check, Flutter tests и release сборка C++/Dart, fake DNS/proxy/process, 200 mock sessions | Выделенная VM/network namespace + доступный desktop; реальный TUN/proxy traffic, resolved/GNOME restore, suspend, tray. Рабочая сеть пользователя не менялась |
| Windows | Compile всех Go packages/tests с dummy executor; updater compilation FAIL F07; source helper/host/C++/installer | Windows 10/11 VM с snapshot, medium/elevated test users: ACL/MIC, Wintun/routes/DNS/helper, C++ Flutter build, portable/setup install/update, Explorer restart. Wine не приравнивается к Windows VM |
| Android | JNI/Go arm64 libnaga.so c-shared build; все 4 PT_LOAD align=16384; Kotlin/service/channel source и Flutter mocked tests | Изолированные устройства/emulator с 4KB и 16KB pages: полный release APK, consent/revoke/process death, IPv6/split/DNS, bridge exit, PackageInstaller confirmation. Подпись и полный набор native libs APK не проверены |
| iOS | Swift API 22 tests и Go fixture comparison; unsigned SwiftUI shell сборка iOS SDK/xtool | iPhone iOS16+ для shell lifecycle/UI; paid ADP profile/NetworkExtension для extension/signing. Go framework/datapath пока не подключены: работающий VPN не заявляется. Flutter iOS scaffold не собирался |

Сборки этого ревью — локальные проверочные артефакты, не release acceptance. Нет универсальной гарантии kill switch/IPv6; семантика system proxy отличается от TUN. Применимость сочетаний перечислена в matrix.csv.

Для R01/R02/R06/R07 получены component/static подтверждения в пределах исходного контракта. R03 имеет D04, R04 — F04 и native restore pending, R05 — F03/F06 и packet-path pending, R08 — F03/device pending, R09 — updater component доказательства без реальной Linux install/update, R10 — подтверждён shell fail-path по коду без iPhone execution. [Исходные требования](testing/audit/20261013T100903Z-full-review/requirements.md).

Обязательный следующий стендовый прогон: clean install/import/connect/traffic/stop/restart; DNS и IPv4/IPv6 capture; split/app bypass; engine/bridge death и crash cleanup; refresh/policy/update во время polling; consent/revoke и background/suspend. Windows отдельно: medium/elevated IPC, Wintun/routes/ACL, setup/portable update и protected sentinel. Android отдельно: полный release APK 4KB/16KB, PackageInstaller callbacks и revoke/recreate. iPhone: SwiftUI shell lifecycle сейчас; NetworkExtension только после подключения ядра и необходимых provisioning inputs.

## Сохранность результатов

Хеши всех 489 исходных файлов рабочего дерева сверены с baseline; production-файлы не изменены ревью. Новый код proofs находится только в ignored audit directory. Итоговые документы сохраняются в docs/CODE_REVIEW_REPORT.md и docs/CODE_REVIEW_PROGRESS.md; подробные локальные artifacts excluded from git. [Проверка целостности](testing/audit/20261013T100903Z-full-review/integrity.json). Релиз не публиковался; наличие отчёта означает завершение доступного обзора, а не устранение находок или успешное прохождение UNAVAILABLE шагов.
