# Выполнение плана code review

Снимок: `2fe0f50043bdafe8cc59ee0e9a673f1a4ede31de` плюс рабочее дерево из baseline.json. Дата: 2026-10-13. План: [CODE_REVIEW_PLAN.md](CODE_REVIEW_PLAN.md); выводы: [CODE_REVIEW_REPORT.md](CODE_REVIEW_REPORT.md).

PASS означает результат указанной проверки в пределах перечисленных доказательств. FAIL означает обнаруженное отклонение. UNAVAILABLE означает, что остаётся требуемое выполнение на ОС/устройстве либо отсутствует инструмент; выполненная часть описана в той же строке. N/A — проверка неприменима. Частичные component результаты не закрывают native-сценарий.

| Шаг | Проверка | Статус | Фактический результат и пределы |
|---|---|---|---|
| 00.01 | База | PASS | Commit, diff, untracked и SHA256 зафиксированы; все исходные файлы зарегистрированы. |
| 00.02 | Незакоммиченный код | PASS | Commit, diff, untracked и SHA256 зафиксированы; все исходные файлы зарегистрированы. |
| 00.03 | Исключения | PASS | Commit, diff, untracked и SHA256 зафиксированы; все исходные файлы зарегистрированы. |
| 00.04 | Реестр | PASS | Commit, diff, untracked и SHA256 зафиксированы; все исходные файлы зарегистрированы. |
| 00.05 | Происхождение изменений | PASS | Commit, diff, untracked и SHA256 зафиксированы; все исходные файлы зарегистрированы. |
| 00.06 | Изменение базы | PASS | Commit, diff, untracked и SHA256 зафиксированы; все исходные файлы зарегистрированы. |
| 01.01 | Точки входа | PASS | Процессы, границы доверия, требования и изолированные fixtures описаны; реальные сетевые изменения исключены из component стенда. |
| 01.02 | Данные и IPC | PASS | Процессы, границы доверия, требования и изолированные fixtures описаны; реальные сетевые изменения исключены из component стенда. |
| 01.03 | Ресурсы | PASS | Процессы, границы доверия, требования и изолированные fixtures описаны; реальные сетевые изменения исключены из component стенда. |
| 01.04 | Требования | PASS | Процессы, границы доверия, требования и изолированные fixtures описаны; реальные сетевые изменения исключены из component стенда. |
| 01.05 | Неопределённости | PASS | Процессы, границы доверия, требования и изолированные fixtures описаны; реальные сетевые изменения исключены из component стенда. |
| 01.06 | Применимость | PASS | Процессы, границы доверия, требования и изолированные fixtures описаны; реальные сетевые изменения исключены из component стенда. |
| 01.07 | Побочные эффекты тестов | PASS | Процессы, границы доверия, требования и изолированные fixtures описаны; реальные сетевые изменения исключены из component стенда. |
| 01.08 | Набор входов | PASS | Процессы, границы доверия, требования и изолированные fixtures описаны; реальные сетевые изменения исключены из component стенда. |
| 01.09 | Стенды | PASS | Процессы, границы доверия, требования и изолированные fixtures описаны; реальные сетевые изменения исключены из component стенда. |
| 01.10 | Модель угроз | PASS | Процессы, границы доверия, требования и изолированные fixtures описаны; реальные сетевые изменения исключены из component стенда. |
| 02.01 | Окружение | PASS | Доступные baseline проверки выполнены; host анализ не заменяет проверки native build tags. |
| 02.02 | Зависимости | PASS | Доступные baseline проверки выполнены; host анализ не заменяет проверки native build tags. |
| 02.03 | Go | PASS | Доступные baseline проверки выполнены; host анализ не заменяет проверки native build tags. |
| 02.04 | Конкурентность | PASS | Доступные baseline проверки выполнены; host анализ не заменяет проверки native build tags. |
| 02.05 | Flutter | PASS | Доступные baseline проверки выполнены; host анализ не заменяет проверки native build tags. |
| 02.06 | Swift | PASS | Доступные baseline проверки выполнены; host анализ не заменяет проверки native build tags. |
| 02.07 | Исключённые ветви | PASS | Список platform exclusions отражён в coverage.csv; три Windows runtime skips записаны отдельно. |
| 02.08 | Качество тестов | PASS | Доступные baseline проверки выполнены; host анализ не заменяет проверки native build tags. |
| 02.09 | Дополнительные анализаторы | UNAVAILABLE | ShellCheck и PowerShell отсутствуют. Bash -n, Python AST, XML/JSON/plist/image parsers доступны и прошли; native lint для Kotlin/Windows не выполнен. |
| 02.10 | Первичный скан зависимостей | FAIL | Сканирование выполнено; исходный суффикс Go скрывал stdlib alerts. Нормализованный scan и binary scan выявили advisory, см. D01–D03. |
| 03.01 | Маршруты | PASS | Route matrix включает literal и dynamic маршруты; mutation lock и timeout ownership прослежены по исходникам. |
| 03.02 | Режимы доступа | PASS | Проверены handlers, клиенты и контрактные тесты; права ОС отдельно ограничены стендом. |
| 03.03 | Токен | UNAVAILABLE | Token/auth на host проверены; Windows ACL и доступ другого OS пользователя не воспроизводились. |
| 03.04 | HTTP-границы | PASS | Проверены handlers, клиенты и контрактные тесты; права ОС отдельно ограничены стендом. |
| 03.05 | Входные запросы | PASS | Проверены handlers, клиенты и контрактные тесты; права ОС отдельно ограничены стендом. |
| 03.06 | Согласованность | PASS | Go/Dart/Swift DTO и 11 Swift fixtures сопоставлены; exporter меняет только updated_at. |
| 03.07 | Отмена и ошибки | FAIL | F16: HTTP stop блокируется общей сериализацией start. |
| 03.08 | Фикстуры | PASS | Экспорт в отдельной копии; структурные значения совпали, единственное отличие — updated_at. |
| 03.09 | Приватность | PASS | Проверены handlers, клиенты и контрактные тесты; права ОС отдельно ограничены стендом. |
| 03.10 | Журнал | PASS | Logger rotation/redaction проверены штатными tests; concurrent access входит в host race. Настоящий ENOSPC журнала не воспроизводился. |
| 04.01 | URL и TLS | PASS | HTTPS/redirect/IP literal и Go resolved-IP guard проверены. Различия Dart/proxy resolved-IP фильтрации оставлены H01 с недостающим TLS proof. |
| 04.02 | Загрузка | PASS | Парсеры, transforms и storage проверены исходниками, штатными тестами и дополнительными входами. |
| 04.03 | Metadata | FAIL | F10: часы интервала Hiddify трактуются как дни; quota/expiry штатные и fuzz tests прошли. |
| 04.04 | Парсеры | FAIL | F14: ошибочные числовые AWG поля превращаются в ноль. Остальные парсеры прошли unit/seed tests; семь fuzz целей запущены отдельно. |
| 04.05 | Преобразования | FAIL | F12: system-proxy transform удаляет существующий listener; merge/detour/selection tests прошли. |
| 04.06 | Исходные байты | FAIL | Документ обещает AWG байт-в-байт, snapshotFromConfig удаляет BOM и краевые пробелы. D04: нужно уточнить сохранение оригинала либо документацию. |
| 04.07 | Storage | PASS | ID containment, private files и atomic write проверены на Linux. Windows ACL/reparse не закрыты host тестами. |
| 04.08 | Сбои данных | PASS | Versioned save/rollback/schema tests прошли; дополнительный отказ записи в read-only directory сохранил старый config. Реальный ENOSPC и crash во время rename не воспроизводились. |
| 04.09 | Refresh и delete | PASS | Парсеры, transforms и storage проверены исходниками, штатными тестами и дополнительными входами. |
| 04.10 | Rule sets | FAIL | F01 и F15: late-worker data race и вечный cache; format/cache-hit/fallback штатные tests прошли. |
| 05.01 | Привилегии | PASS | Прослежены привилегии и updater; действия на реальных ОС не выполнялись. |
| 05.02 | Windows IPC | FAIL | F17: LogPath от IPC используется elevated host без containment. Доступность по Windows MIC/ACL и эксплуатация остаются device pending. |
| 05.03 | Пути исполнения | UNAVAILABLE | Пути и validators прочитаны; F02/F05 подтверждены POSIX tests. Writable DLL/reparse/elevated TOCTOU требуют Windows VM. |
| 05.04 | Linux policy | PASS | Прослежены привилегии и updater; действия на реальных ОС не выполнялись. |
| 05.05 | Metadata обновления | PASS | Прослежены привилегии и updater; действия на реальных ОС не выполнялись. |
| 05.06 | Целостность | PASS | Digest present/missing/mismatch и asset validators покрыты Flutter tests; обязательной подписи desktop в требованиях нет. Сам release certificate не проверялся. |
| 05.07 | Архивы | FAIL | F05: lexical ZIP containment допускает parent symlink; Windows reparse испытание не выполнено. |
| 05.08 | Транзакция | FAIL | F02/F07/F09 препятствуют безопасной/достоверной транзакции обновления; реальный rollback после установки требует платформенных стендов. |
| 05.09 | Установщики | FAIL | F08: PackageInstaller user-action callback игнорируется; подтверждён контрактом API, установка на устройстве не выполнялась. |
| 05.10 | Секреты и файлы | FAIL | F02 temp symlink и F17 privileged log path. argv/token planner tests прошли; завершение сторонних процессов installer оставлено H04. |
| 06.01 | Состояния | PASS | Проверены state transitions, синхронизация и component rollback; реальные маршруты/DNS остаются вне доказательства. |
| 06.02 | Синхронизация | FAIL | Baseline race PASS, но дополнительный ruleset race F01 и HTTP cancellation F16 подтверждены. |
| 06.03 | Пересечения | FAIL | F16 на HTTP Start–Stop; controller concurrency/health tests прошли. Полное пересечение native-операций требует S09. |
| 06.04 | Ошибка каждого шага | PASS | Component failures sidecar/adapter/DNS/reconnect покрыты штатными tests; crash в настоящем privileged route apply не выполнялся. |
| 06.05 | Stop и recovery | FAIL | F03 bridge child exit; обычный recovery/Stop component tests прошли. 200 mock sessions: goroutines 2→2, FD 6→6. |
| 06.06 | DNS/proxy/routes | FAIL | F04 GNOME restore стирает независимый proxy; host mocks подтвердили. Реальный resolved/Windows route restore не исполнялся. |
| 06.07 | Маршрутизация | UNAVAILABLE | Config policy assertions прочитаны и прошли; фактический direct/tunnel/reject, DNS и IPv6 packet path не наблюдались. |
| 06.08 | Выбор узлов | PASS | Проверены state transitions, синхронизация и component rollback; реальные маршруты/DNS остаются вне доказательства. |
| 06.09 | Discovery приложений | UNAVAILABLE | Linux discovery parser tests прошли; реальные Android package/Windows process identities и persisted rules на устройствах не проверены. |
| 06.10 | Статистика | UNAVAILABLE | Quit/update cleanup прочитан и component lifetime проверен; suspend/reboot/user session shutdown требуют отдельного desktop/device стенда. |
| 07.01 | Контракт адаптера | PASS | Адаптеры проверены доступными tests/config check/compile; реальный VPN datapath не запускался. |
| 07.02 | sing-box config | PASS | Адаптеры проверены доступными tests/config check/compile; реальный VPN datapath не запускался. |
| 07.03 | sing-box процесс | PASS | Адаптеры проверены доступными tests/config check/compile; реальный VPN datapath не запускался. |
| 07.04 | AmneziaWG | PASS | Адаптеры проверены доступными tests/config check/compile; реальный VPN datapath не запускался. |
| 07.05 | olcRTC | PASS | Адаптеры проверены доступными tests/config check/compile; реальный VPN datapath не запускался. |
| 07.06 | hev и datapath | FAIL | F03: выход hev не передаётся runtime как failure; внешний TUN↔SOCKS трафик не проверен. |
| 07.07 | JNI/CGO | PASS | JNI C-string/free и FD transfer прочитаны; Android arm64 c-shared собран. iOS framework не собирался/не линковался. |
| 07.08 | FD | FAIL | F03 ownership после child exit; normal ownership и Stop tests прошли на host, Android recreate pending. |
| 07.09 | Приватность движков | PASS | Адаптеры проверены доступными tests/config check/compile; реальный VPN datapath не запускался. |
| 07.10 | Память native-кода | UNAVAILABLE | ASan/UBSan/MTE native runtime не запускались; Go race не заменяет native memory sanitizers. |
| 08.01 | Сценарии сборки | FAIL | F09 shell fallback и F11 Mach-O corruption; синтаксические проверки доступных scripts прошли. |
| 08.02 | Входы сборки | PASS | Pins/checksums/overrides и patches учтены; sing-box 1.14.3 host и 1.13.20 mobile/Windows проверяются как разные зависимости. |
| 08.03 | Уязвимости | FAIL | Source normalized-Go scan: 15 unique advisory, 9 с static call-chain. Installed sing-box binary: 22 advisory matches; runtime applicability см. D01–D03. |
| 08.04 | Release contracts | FAIL | F07 Windows build blocker, F09 stale embedded version; остальная naming/asset selection покрыта updater tests. |
| 08.05 | CI | PASS | Workflow permissions/steps изучены. Windows workflow dispatch-only и отсутствие native test gates объясняют пропуск F07; предложены gates в отчёте. |
| 08.06 | Packaging | UNAVAILABLE | Manifests/plists/entitlements разобраны; реальная release signature/installer не проверены. Debug-key fallback и non-fatal page-size guard требуют release gate. |
| 08.07 | Чистая сборка | FAIL | Linux release, iOS unsigned shell и Android native сборки PASS; Windows updater FAIL. Полный APK, Windows Flutter/setup и Go iOS framework недоступны в этом прогоне. |
| 08.08 | Ресурсы и лицензии | PASS | 146 syntax/resource checks и 8 font validations; font hashes/weights/Ё/ё согласованы, iOS PostScript имена различны. Полный third-party notice комплект конечных APK/IPA/setup не проверен. |
| 08.09 | Документация и scaffold | PASS | Изучены сборщики и CI; доступны Linux release, unsigned iOS shell и Android native build. |
| 09.01 | Build tags | PASS | Host, Windows compile, Android build tags и iOS не подключённый Go код разнесены в coverage.csv. |
| 09.02 | Linux runtime | UNAVAILABLE | Отдельный управляемый Linux desktop/network, Windows VM, Android device или iPhone стенд не предоставлен; component/compile доказательства указаны в platform-results.md. |
| 09.03 | Linux UI | UNAVAILABLE | Отдельный управляемый Linux desktop/network, Windows VM, Android device или iPhone стенд не предоставлен; component/compile доказательства указаны в platform-results.md. |
| 09.04 | Windows helper | UNAVAILABLE | Отдельный управляемый Linux desktop/network, Windows VM, Android device или iPhone стенд не предоставлен; component/compile доказательства указаны в platform-results.md. |
| 09.05 | Windows сеть и установка | UNAVAILABLE | Отдельный управляемый Linux desktop/network, Windows VM, Android device или iPhone стенд не предоставлен; component/compile доказательства указаны в platform-results.md. |
| 09.06 | Android компоненты | UNAVAILABLE | Отдельный управляемый Linux desktop/network, Windows VM, Android device или iPhone стенд не предоставлен; component/compile доказательства указаны в platform-results.md. |
| 09.07 | Android сеть и release | UNAVAILABLE | Отдельный управляемый Linux desktop/network, Windows VM, Android device или iPhone стенд не предоставлен; component/compile доказательства указаны в platform-results.md. |
| 09.08 | iOS оболочка | UNAVAILABLE | Отдельный управляемый Linux desktop/network, Windows VM, Android device или iPhone стенд не предоставлен; component/compile доказательства указаны в platform-results.md. |
| 09.09 | iOS native/scaffold | UNAVAILABLE | Scaffold/entitlements/fail path прочитаны; Go framework, платная подпись NetworkExtension и real PacketTunnel исполнение отсутствуют. |
| 09.10 | Остаточные ограничения | PASS | Для каждой платформы составлена матрица compile/component/device и перечень требуемых стендов. |
| 10.01 | Асинхронность | PASS | Polling/timers/dispose/MainActor просмотрены; Flutter асинхронные tests прошли, SwiftUI background lifecycle pending. |
| 10.02 | Состояния UI | FAIL | F06: starting→control offline остаётся connecting; остальные fixture/widget состояния PASS. |
| 10.03 | Пользовательские действия | UNAVAILABLE | Доступные client/widget import/refresh/select/settings tests PASS. OS file picker, clipboard/QR/camera/native dialogs и SwiftUI touch flows требуют GUI/device. |
| 10.04 | Защита от повторов | PASS | Busy/reentry guards и client tests изучены; native installer recreation/consent ещё не проверены. |
| 10.05 | Доступность | UNAVAILABLE | Flutter accessibility/text scale/golden tests PASS; настоящий screen reader, contrast измерения и iOS reduced-motion/device проверки не выполнены. |
| 10.06 | Golden/snapshots | PASS | Goldens и штатные widget fixtures прошли без обновления; export-only branches отключены, native VPN этим не проверяется. |
| 10.07 | Структура | PASS | main.dart 7362 строк; UI parts имеют общий shared state. Структурный backlog отделён от функциональных находок. |
| 10.08 | Native UI bridges | UNAVAILABLE | Linux C++ runner собран и прочитан, Windows runner прочитан; tray/titlebar/Explorer restart/single instance действия не исполнялись. |
| 11.01 | Набор сценариев | PASS | Матрица applicability заполнена с конкретными пределами; native-сценарии требуют отдельного стенда. |
| 11.02 | Позитивный путь | UNAVAILABLE | Unit/component import/start/stop/restart PASS; полный поток с реальным VPN traffic не выполнялся. |
| 11.03 | Конкурентные изменения | UNAVAILABLE | Mock rollback/health/concurrency tests PASS; полный UI/persisted/runtime сценарий refresh/update/reconnect на устройствах не выполнялся. |
| 11.04 | Аварии | FAIL | Управляемый test child exit подтвердил F03; реальный системный DNS/proxy/route crash recovery требует стенда. |
| 11.05 | Проверка трафика | UNAVAILABLE | Нет packet capture из изолированного VPN-стенда. DNS/IPv4/IPv6/endpoint bypass физически не подтверждены. |
| 11.06 | Смена окружения | UNAVAILABLE | Class/cooldown/recovery component tests PASS; captive portal/suspend/background/потеря физической сети не воспроизводились. |
| 11.07 | Fuzzing | PASS | 7 fuzz целей по 60 секунд; 5,916,819 выполнений, crash/hang не найден. Это ограниченный прогон. |
| 11.08 | Нагрузка | FAIL | F13: 1000 узлов, 13,1 с и 1,52 ГБ TotalAlloc. 200 mock sessions без роста FD/goroutines; CPU/RSS/UI latency и полный native probe fanout не измерялись. |
| 11.09 | Бюджет и повтор | PASS | Fuzz budget 60с/цель; performance измерен без придуманного SLA. Повторы ограничены воспроизведением находок/новыми данными. |
| 11.10 | Подтверждение находок | PASS | Минимальные filesystem/widget/race/compile/script proofs сохранены. F08/F17 подтверждены по коду/контракту и явно требуют native execution. |
| 12.01 | Полнота файлов | PASS | Находки, доказательства, охват и ограничения сведены в отчёт. |
| 12.02 | Полнота требований | PASS | Находки, доказательства, охват и ограничения сведены в отчёт. |
| 12.03 | Дедупликация | PASS | Находки, доказательства, охват и ограничения сведены в отчёт. |
| 12.04 | Приоритет и уверенность | PASS | Находки, доказательства, охват и ограничения сведены в отчёт. |
| 12.05 | Очередь исправлений | PASS | Находки, доказательства, охват и ограничения сведены в отчёт. |
| 12.06 | Итог платформ | PASS | Находки, доказательства, охват и ограничения сведены в отчёт. |
| 12.07 | Повторная проверка правок | N/A | Исправления production-кода не входят в текущую работу; повторная проверка выполняется после отдельного patch. |
| 12.08 | Завершение | PASS | Находки, доказательства, охват и ограничения сведены в отчёт. |

Подробная связь с командами и снимком: [step-status.csv](testing/audit/20261013T100903Z-full-review/step-status.csv). Реестр файлов: [coverage.csv](testing/audit/20261013T100903Z-full-review/coverage.csv). Локальные evidence файлы исключены из git; отчёт и эта таблица предназначены для хранения в репозитории.
