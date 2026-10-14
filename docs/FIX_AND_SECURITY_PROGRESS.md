# Исправления и углублённый поиск ошибок — 2026-10-14

В рабочем дереве подготовлены исправления всех 17 исходных F-находок и семи дополнительных дефектов F18–F24. Проверки общего Go-кода, race detector, Dart, Flutter и доступные сборки проходят. Полная приёмка плана ещё не завершена: Windows privileges/update, Android datapath/installer и подписанный iPhone требуют стендов. Пользователь запускает проверки на каждом стенде самостоятельно; результаты ещё не получены.

Основа: `2fe0f50043bdafe8cc59ee0e9a673f1a4ede31de` плюс исходные изменения пользователя. Они сохранены; публикация и commit не выполнялись. [Первоначальный отчёт](CODE_REVIEW_REPORT.md) описывает состояние **до** исправлений. [План](FIX_AND_SECURITY_PLAN.md) содержит расширенные критерии, поэтому исправление отдельной причины не означает завершение всего блока.

Локальные доказательства: [снимок и исходный diff](testing/audit/20261013T174635Z-fixes/baseline.json), [реестр исправлений](testing/audit/20261013T174635Z-fixes/status.json), [результаты команд](testing/audit/20261013T174635Z-fixes/checks-summary.json), [журналы и точные argv](testing/audit/20261013T174635Z-fixes/checks/), [состояние каждого шага](testing/audit/20261013T174635Z-fixes/plan-status.csv). Эти артефакты исключены из Git; переносимые regression tests находятся в штатных test directories.

Реестр содержит все 144 шага и сохраняет PARTIAL/PENDING вместо автоматического закрытия блоков. [Прежние 23 недоступные проверки](testing/audit/20261013T174635Z-fixes/native-gaps.json) сопоставлены с самостоятельными прогонами пользователя. [Дополнительное ручное чтение](testing/audit/20261013T174635Z-fixes/additional-reading.csv) содержит 59 записей с глубиной проверки; длинные частично обрезанные чтения явно отмечены PARTIAL_READ. Инвентаризация не объявляется полным построчным чтением проекта.

## Исходные находки

«Проверено на компоненте» означает конкретное воспроизведение и regression, а не сквозной результат на всех ОС.

| ID | Что изменилось | Проверка и оставшаяся граница |
|---|---|---|
| F01 | paths/Stats защищены mutex; наружу возвращается отдельный snapshot | Старое воспроизведение выдаёт race; исправленное и общий race suite проходят |
| F02 | Новый случайный staging; HTTPS, обязательный SHA256, ограничения размера/времени, cleanup при ошибке | Внешний sentinel не меняется; пустой digest и обрыв отклоняются. Подмена того же UID между проверкой и установкой отдельно не закрыта |
| F03 | Один владелец child Wait; unexpected exit освобождает соответствующий FD; replacement сериализован; runtime видит bridge failure | Host child exit/replacement проверены; Android consent/revoke/пакеты ещё не проверены |
| F04 | Сохраняется полный прежний proxy; чужой host/порт не считается Naga; restore учитывает изменение настроек; убран соседний blind reset olcRTC | Mock и настоящий gsettings с отдельным keyfile backend проходят. Windows registry и межпроцессная гонка требуют отдельной проверки |
| F05 | ZIP полностью проверяется и распаковывается в staging через os.Root; запрещены опасные имена/ссылки; есть бюджеты и rollback заменённых файлов | Traversal, внешний symlink, коллизии и отказ второй замены проходят. Windows junction/locked files, crash journal и согласованность всего набора открыты |
| F06 | Потеря API после порога переводит также Connecting в «Статус неизвестен» | Widget regression проходит. Защита от позднего polling response нового поколения пока не реализована |
| F07 | Исправлены WAIT_TIMEOUT и ошибки OpenProcess/Wait; добавлены Windows tests и CI job | Старый код не компилируется, новый компилируется целиком. Native wait tests и portable/setup ещё не выполнялись на Windows |
| F08 | Non-exported receiver; session ID + nonce; подтверждение установки; сохранённый результат; UI читает отказ при resume/poll | Kotlin и APK собираются. Подтверждение/отказ, recreation, несовместимый signer и повторный callback ждут устройства |
| F09 | IPA строится в свежей копии; принимается только результат текущей успешной команды; проверяются версии и ZIP | Compile-failure/stale-artifact regression и unsigned builds проходят. Bundle identity и итоговая Apple signature требуют расширения проверки |
| F10 | Profile-Update-Interval трактуется в часах с проверкой диапазона | Контракт и границы проверены; старые секунды не делятся вслепую. Конвенция собственного provider отдельно не подтверждена |
| F11 | Изменяющий Mach-O patcher заменён read-only validator; __text создаёт настоящий C linker symbol | Fixtures сохраняют data bytes; реальная unsigned PacketTunnel extension проходит validator. Подписанный запуск pending |
| F12 | Transform создаёт канонический mixed listener для system proxy, даже если reserved listener уже был | Regression и существующие profile/runtime tests проходят; настоящий bind/OS proxy проверяется на стенде |
| F13 | JSON/граф индексируются один раз; пути выбора вычисляются общим обходом | Вложенные selector/urltest и cycles проверены; измерения ниже. Native UI load и responsiveness под нагрузкой ещё не закрыты |
| F14 | Ошибочные, пустые, отрицательные и переполненные AWG числа дают явный отказ; aliases проверяются так же | Табличные regression, прежние fixtures и fuzz проходят; дополнительные native границы выделены в F18/F20 |
| F15 | TTL/update_interval, refresh cache, offline fallback, singleflight и runtime-owned watcher | Fake-clock stale/fresh, concurrent fetch, cancel и formats проверены. Длительный connected refresh/eviction требуют дополнительных проверок |
| F16 | Stop отменяет Start до захвата mutation lock; cancellation проходит через запуск/probe; stopPending запрещает replacement | HTTP tests на blocked probe и engine acquire проходят; unauthorized Stop не отменяет запуск. Все policy/reconnect/native restore расписания ещё не исчерпаны |
| F17 | Из privileged IPC удалён LogPath; неизвестные поля отклоняются; размер и Env ограничены | Protocol regression и Windows compilation проходят. Общий privilege boundary остаётся открытым, см. H03/H05 |

Сопутствующие изменения: исходные AWG bytes сохраняются отдельно от parsing buffer, включая BOM и крайние пробелы (D04); VpnService сообщает готовность после establish и передачи FD, а не после startForegroundService (H02); пустой selected_vpn whitelist отклоняется вместо неявного захвата всех приложений. H02 и фактическая маршрутизация требуют device evidence.

## Дополнительные дефекты

| ID | Вход → операция → последствие | Исправление / подтверждение |
|---|---|---|
| F18 · P1 | AWG Jc > 0, Jmin > 0 при Jmax = 0 → unsigned max-min в native junk size → потенциально чрезмерное выделение | Проверяется Jmin ≤ Jmax. Underflow установлен по pinned native source; OOM на устройстве не воспроизводился |
| F19 · P1 | Небольшой DAG selectors с общими потомками → повторное раскрытие дерева → чрезмерный рост работы | Depth/work/metadata budgets, индексированный обход. Regression ограничивает 27-узловой DAG; native OOM exploit не запускался |
| F20 · P1 | AWG S4=65535 → offset=16+S4 → slicing fixed native buffer вне границ → panic | Проверяется S4 + effective MTU + overhead ≤ UDP budget до device. Цепь установлена по принятому формату и native source; panic на устройстве не запускался |
| F21 · P2 | Unexpected exit olcRTC → прежняя session остаётся видимой → ложный Connected и задержанный cleanup | Owned Done + watcher по identity, terminal Failed до Stop. Child exit 17 проверяет состояние/cleanup без изменения маршрутов |
| F22 · P2 | stderr больше Scanner limit → reader прекращает drain → child блокируется при записи → Wait не завершается | Bounded Reader продолжает drain длинной строки. Child с 1 MiB stderr завершается в regression |
| F23 · P2 | Ошибка local rule set → retry fabricates example.invalid вместо исходного remote URL → неверный набор правил/ошибка | Retry использует исходные definitions; публикуется effective config. Regression проверяет сохранение URL |
| F24 · P2 | Один URL в source и binary rule sets → общий cache key → взаимная перезапись форматов | Ключ включает format; миграция прежнего binary cache. Regression проверяет раздельные файлы |

Это не семь доказанных удалённых атак. У F18/F19/F20 доказана опасная цепь обработки входа и отказ после fix; native последствия проверены по коду и ограниченному компонентному тесту. Повышение Windows privileges, DNS leak и выход трафика мимо VPN этим проходом не доказаны.

## Проверки и зависимости

| Проверка | Результат | Ограничение |
|---|---|---|
| Go test ./..., vet, expanded race | PASS после последних Go изменений: 449 test/subtest pass events | Windows-only runtime и реальные устройства; это не 449 независимых пользовательских сценариев |
| Единый Linux AppImage 1.2.5 | PASS: release build, bundle hashes, ldd, embedded control health/auth/shutdown, olcRTC discovery, sing-box TUN config check; установка через pkexec/systemd на текущем Linux и запуск UI из меню | Визуальная проверка GUI/tray, cold boot, реальное обновление между версиями и VPN traffic остаются за стендом |
| Flutter suite | PASS: 112 tests, включая single-AppImage updater regressions | OS channels/installer и фактический datapath |
| Dart analyze | PASS после последних UI/updater изменений | Native callback delivery |
| Swift API/model | PASS: 22 tests, 3 suites | SwiftUI device/background lifecycle |
| iOS packaging | PASS: 3 tests; unsigned shell и PacketTunnel build PASS | Provisioning, Apple signature, VPN на iPhone |
| Android Kotlin + arm64 Go library | PASS | Consent/revoke и трафик устройства |
| Windows Go packages/tests compile | PASS | Dummy executor не выполняет Windows tests |
| AWG fuzz | PASS: 60 с, 223 682 executions | Ограниченный бюджет |

На одинаковом Go 1.27.2 и одном host подготовка 1000 узлов снизилась с медианы **11 283 мс до 27 мс**. TotalAlloc снизился с **1,52 GB до 4,86 MB**: суммарные временные выделения, не peak RSS. По три измерения 10/100/1000 узлов; [данные](testing/audit/20261013T174635Z-fixes/performance-comparison.json). После замера добавлены metadata limits; timings относятся к замеренному промежуточному снимку, финальный код проверен regression suite.

Официальный Go 1.27.2 подготовлен отдельно с проверенным SHA256; изменены toolchain/CI/build scripts. Обновлены root x/net и compress; sing-box 1.13.20 собирается с закреплёнными исправленными dependencies. Android и Windows private engine builds проходят. Tag http2legacy сохраняет совместимость sing-box с исправленным x/net; зависимость от private HTTP/2 internals остаётся риском и требует runtime проверки. Исправленные версии по advisory: [Go/x/net](https://pkg.go.dev/vuln/GO-2026-6611), [compress](https://pkg.go.dev/vuln/GO-2026-5841).

У исходного Android libsingbox scanner обнаружил 40 уникальных advisory; у исправленного Android/Windows binary и root source осталось **одно module-level предупреждение GO-2026-5932 об устаревшем openpgp**, без установленной static call chain в source scan. JSON exit 0 означает завершение сканера. Binary scan не доказывает protocol reachability; нестандартная версия исходного compiler ограничивает оценку stdlib. /usr/bin/sing-box не заменялся: private builds не закрывают D02 для этой Linux копии.

Android production build требует release signing configuration; local debug fallback разрешается только явным флагом. Обязательный ELF gate проверяет каждый PT_LOAD и ZIP alignment. Первые отказы debug APK по RELRO были false positive самого нового проверяющего скрипта: RELRO end не обязан быть кратен 16KB при допустимом padding. Критерий исправлен и закреплён regression. Это не проверка release signing или исполнения на 16KB устройстве. Production signing keys не публиковались; release не выпускался. Требования платформы: [Android 16KB pages](https://developer.android.com/guide/practices/page-sizes).

Отдельный [тестовый arm64 APK](testing/audit/20261013T174635Z-fixes/checks/android/fixed-apk/NagaNetwork-audit-arm64-debug.apk) собран с текущими Dart/Kotlin, versioned libnaga и исправленным libsingbox; прежний libhevfd сохранён. SHA256: `7aa7467b51d92b4e82f944f51d832f197e7069aae126ec928d7b5d23d163abd0`. Все три встроенные библиотеки совпадают с входами байт-в-байт; [manifest](testing/audit/20261013T174635Z-fixes/checks/android/fixed-apk/manifest.json). ELF/ZIP gate проходит для всех пяти native libraries, включая Flutter и debug Vulkan validation layer. APK подписан локальным Android debug key; совместимость обновления требует той же test key. Исходные ignored jniLibs не заменялись: каталог подставлен только private Gradle init script.

Скрытый Naga linker hook внешнего xtool SDK восстановлен отдельным одобренным действием; backup/hashes сохранены в локальном sdk-repair/. Новая сборка не изменяет глобальный SDK и не патчит подписанный executable.

Linux теперь поставляется [одним AppImage](../dist/NagaNetwork-1.2.5-x86_64.AppImage): UI, control, sing-box, olcRTC, hev и установка службы. Новый формат подготовлен по запросу пользователя 2026-10-14. [Manifest](../dist/linux-build-manifest.json), [build](testing/audit/20261013T174635Z-fixes/checks/linux-appimage-5.log), [single-file smoke](testing/audit/20261013T174635Z-fixes/checks/linux-bundle-smoke-3.log), [7 installer component tests](testing/audit/20261013T174635Z-fixes/checks/linux-packaging-tests-4.log). Updater требует только AppImage, берёт runtime из нового файла и ждёт освобождения application ID старым процессом. Компонентные тесты подтверждают сохранение прежнего control при отмене pkexec, отказ повреждённого cache и отсутствие повторного запроса для активного build.

По запросу пользователя AppImage установлен на текущем Linux. Реальная установка выявила недопустимые кавычки в WorkingDirectory шаблона systemd; шаблон исправлен, рендеринг путей разделён для WorkingDirectory и ExecStart, перед изменением системных файлов добавлен systemd-analyze verify. Regression использует настоящий парсер systemd, включая путь с пробелами. После пересборки служба обновлена через pkexec, включена в автозапуск, API и хеши кеша проверены; повторная подготовка завершилась без новой авторизации. [Проверка установки](testing/audit/20261013T174635Z-fixes/linux-local-install-verification.json) и [запуск UI из меню](testing/audit/20261013T174635Z-fixes/linux-installed-ui.json) сохранены. AppImage находится в ~/.local/share/naga-network/app; ярлык запускает последнюю стабильную версию из этой папки с APPIMAGE_EXTRACT_AND_RUN=1, поскольку на host отсутствует /dev/fuse. Старый control и доступные настройки службы сохранены в приватной резервной копии. Профили не заменялись, VPN при установке был отключён. Визуальная оценка окна, tray, cold boot и фактический трафик ещё не проверены.

Движки собраны из закреплённых commits на Go 1.27.2; Linux-версия olcRTC получила обновления grpc/CEL после бинарного скана. Минимальные исправленные версии определены по [CEL advisory](https://pkg.go.dev/vuln/GO-2026-6094), [gRPC OOM](https://pkg.go.dev/vuln/GO-2026-6348) и [gRPC panic](https://pkg.go.dev/vuln/GO-2026-6443). Binary scanner по-прежнему не доказывает достижимость из конкретного room/provider. Установленный /usr/bin/sing-box не изменён; обычный AppImage runtime использует свою копию рядом с control.

## Остаток плана

| Блоки | Выполненная часть | Остаток |
|---|---|---|
| R00–R01 | Snapshot, регрессии, toolchain, compile, CI changes | Фактический Windows CI/native результат |
| R02–R03 | Download/ZIP containment/rollback, LogPath removal, bounded protocol | Same-UID verify→install подмена, crash recovery/Windows reparse; privileged inputs и авторизация |
| R04–R05 | Race, TTL, format keys, dedup, Start cancellation | Worker pool/общая квота cache, глубокая SRS/schema validation, live refresh; полный concurrency matrix |
| R06–R09 | FD/process ownership, readiness, callbacks, proxy/UI, fresh IPA/read-only Mach-O | Device/OS scenarios, signer/version до install, stale polling protection, signed IPA identity/signature |
| R10–R12 | Numeric guards, original bytes, interval, graph index, dependencies/gates | Собственный provider, legacy evidence, API/native нагрузка, release ABI/signing/page-size acceptance |
| R13–R14 | [Сценарии стендов](NATIVE_SECURITY_TESTS.md), реестр причин и доказательств | Отдельные прогоны пользователя на стендах; финальная сверка и отдельные changesets рефакторинга |
| B00–B11 | Поиск соседних причин дал F18–F24; дополнительное чтение native/UI/transport | Полная reachability/privileges/traffic кампания, остаток ручного чтения и mocks/generated inputs |

Открытые гипотезы: **H01** DNS/redirect/private-destination policy разных transports; **H03** writable bundle/task → elevated launch; **H04** installer завершает чужой engine по имени; **H05** privileged sing-box принимает file-writing/file-reading options из config после удаления IPC LogPath. H05 требует проверки log.output, cache path, TLS inputs, ownership/TOCTOU config и ACL/MIC. Engine имеет файловые операции по контракту: [log](https://sing-box.sagernet.org/configuration/log/), [cache file](https://sing-box.sagernet.org/configuration/experimental/cache-file/). Последствие с повышенными правами пока не подтверждено.

Следующий проход: Windows privilege/установка → Android bridge/installer/пакеты → iPhone signing/lifecycle → оставшиеся resource limits и требования. Каждый результат привязывается к установленному artifact hash, ожидаемому свойству и наблюдаемому состоянию.
