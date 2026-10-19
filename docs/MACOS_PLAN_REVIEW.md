# Проверка плана разработки macOS

Дата: 2026-10-18. Проверены основной план и пошаговая детализация версии 1. По результатам оба документа обновлены до версии 2.

Основа проверки исходников: `origin/main` на `d2e618a0b84fa24228c4aa92b2ead502a9cb418d`. Проверялись соответствие реальному коду, порядок/зависимости работ, границы процессов, критерии готовности и полнота сценариев. Чтение исходников и проверка документации не подтверждают сборку или выполнение VPN на macOS.

## Находки и исправления

«Высокая» означает риск неверной архитектуры, блокирующей зависимости или неподтверждённого результата поставки. «Средняя» означает недостаточную определённость процедуры или приёмки. Все исправления ниже внесены **в план**, а реализация соответствующих задач остаётся открытой.

| ID | Важность | Недостаток версии 1 | Исправление версии 2 |
|---|---|---|---|
| P01 | Высокая | M07.04 направлял к M09.01, хотя M09 зависел от M07/M08. Различие тестовой и подписанной сборок позволяло трактовать зависимости как цикл. | M07 выпускает тестовый пакет независимо от M09. Внутри M09 scripts упаковки выполняются повторно с подписью, затем принимается новый артефакт. |
| P02 | Высокая | Замена helper проверялась после почти всей разработки; даже ручная замена `.app` меняет bundle зарегистрированной службы и требует согласованного lifecycle. | Добавлен ранний A/B прототип M00.07: unregister/register, старая живая служба, IPC N/N+1, отмена и восстановление. Результат требуется перед объявлением проверенного обновления. |
| P03 | Высокая | Для root engine отсутствовал конкретный контракт config/rule-set/log файлов и административного API. Пользовательский adapter уже создаёт эти файлы, API secret и читает диагностику. | Добавлена M03.07: передача проверенных данных, ownership, ограниченные logs/telemetry и проверка полномочий Clash API. Snapshot/runtime защита уточнена в M03.03. |
| P04 | Высокая | Darwin-перенос описывался преимущественно новым `tun_darwin.go`, хотя Validate/Start/StartProbe используют общий Linux-named normalization и отдельную resolver/API подготовку. | M04.04 охватывает весь pipeline; общая schema normalization отделяется от Linux policy, validation и start используют окончательные одинаковые данные. |
| P05 | Высокая | Единый privileged starter и правило одной сессии могли захватить `StartProbe`, привести к лишним правам либо конфликту с временными замерами. | Добавлена M04.07: TUN, user proxy и user probe имеют разные execution purposes и lifecycle; проверяются stopped/connected probes и гонки connect/cancel. |
| P06 | Высокая | UID/подпись были названы, но доверие backend → bridge → helper не определялось полностью; подпись bridge не доказывает право отправителя его stdin-запроса. Проверка identity helper клиентом отсутствовала. | M00.05/M03.02 включают mutual XPC requirements, bootstrap/session authority, trusted connection context, пределы протокола и публичные API macOS 14.0. |
| P07 | Высокая | Обещание cleanup после crash не различало смерть helper и запрет его запуска. Не были закреплены fail-open/kill-switch policy и необходимость разрешения для восстановления. | Задана fail-open policy beta; добавлены lease, crash-restart проверка, recovery pending при отсутствии прав и отдельные негативные сценарии. |
| P08 | Средняя | Встроенный manifest и manifest окончательного DMG не были разделены; порядок добавления digest мог привести к изменению подписанного bundle или самоссылочному хешированию. | Встроенный build manifest формируется до подписи внешнего app. Digest DMG и сведения финальной поставки находятся во внешнем release manifest после всех преобразований. |
| P09 | Средняя | Были определены owner UID и одна сессия, но не было явной приёмки под обычной учётной записью, при fast user switching и logout. | M08.04 дополнен отдельными сценариями UID/admin authorization, смены пользователя, чужого token/stop и отзыва разрешения службы. |
| P10 | Средняя | Сборка UI зависела от всего M00, включая физический M2/signing; bootstrap macOS CI до полного M01 workflow отсутствовал. OS image label мог восприниматься как полная фиксация окружения. | M00.01 содержит bootstrap runner; UI требует baseline, а встраивание engines — соответствующие build inputs. Записываются реальные image revision/tool versions. |
| P11 | Средняя | Не были достаточно определены mandatory/optional строки матрицы и роль `UNAVAILABLE`; длительность stress-прогона не задавала измеримого критерия успеха. | M08.01 фиксирует обязательный объём по каналам; незакрытый обязательный сценарий блокирует заявление о готовности. Performance thresholds и метод сравнения записываются до теста. DEV/SIGNED учитываются отдельно. |
| P12 | Средняя | Sparkle уже был выбран при непроверенном lifecycle helper, а общий «откат» мог восприниматься как готовая транзакция пакета, службы и данных. | Sparkle остаётся плановым вариантом после M00.07. M09.04 отдельно доказывает автоматический rollback либо проверенную ручную переустановку со совместимыми данными; подписанный ручной канал имеет самостоятельный критерий готовности. |

## Подтверждение исходниками и документацией

- P03/P05: `Adapter.Start` создаёт runtime config, принимает `ConfigPath`/`LogPath` и использует `prepareRuntimeConfigMode`; `StartProbe` вызывает тот же `Start`. `runtime_probe.go` запускает временные runtimes в stopped. Это подтверждает необходимость специального контракта привилегий и файлов. [adapter.go](https://github.com/frithTK/Naga-Network-App/blob/d2e618a0b84fa24228c4aa92b2ead502a9cb418d/core/engine/singbox/adapter.go), [runtime_probe.go](https://github.com/frithTK/Naga-Network-App/blob/d2e618a0b84fa24228c4aa92b2ead502a9cb418d/core/control/runtime_probe.go).
- P04: `NormalizeLinuxConfig` вызывается в проверке/запуске и меняет TUN stack/strict_route; подготовка runtime дополнительно инжектирует API/resolver. Конкретные Darwin semantics ещё требуется проверить. [compat.go](https://github.com/frithTK/Naga-Network-App/blob/d2e618a0b84fa24228c4aa92b2ead502a9cb418d/core/engine/singbox/compat.go), [adapter.go](https://github.com/frithTK/Naga-Network-App/blob/d2e618a0b84fa24228c4aa92b2ead502a9cb418d/core/engine/singbox/adapter.go).
- P02/P07: helper находится в bundle, а `SMAppService.register()` для LaunchDaemon требует admin approval. Регистрация и разрешение запуска не являются доказательством корректной замены/восстановления нашей службы; это предмет прототипа. [SMAppService](https://developer.apple.com/documentation/servicemanagement/smappservice), [правила register](https://developer.apple.com/documentation/servicemanagement/smappservice/register%28%29), [структура helper](https://developer.apple.com/documentation/servicemanagement/updating-helper-executables-from-earlier-versions-of-macos).
- P06: для NSXPC доступен code-signing requirement на peer; Apple DTS указывает доступность этого механизма с macOS 13 и различает его с более новыми XPC API. Точная bootstrap/session policy нашего bridge остаётся проектным решением M00. [NSXPCConnection](https://developer.apple.com/documentation/foundation/nsxpcconnection/setcodesigningrequirement%28_%3A%29), [Apple DTS](https://developer.apple.com/forums/thread/681053).
- P12: Sparkle описывает подпись пакетов, appcast и установку app. Возможность атомарно согласовать наш root helper и формат пользовательских данных из этого не следует: это инженерное ограничение текущего плана, проверяемое отдельно. [Sparkle setup](https://sparkle-project.org/documentation/), [настройка lifecycle](https://sparkle-project.org/documentation/customization/).

## Проверенный порядок исполнения

1. Baseline и bootstrap macOS job позволяют выполнить build proofs и начать UI без Developer ID и подтверждённого физического helper.
2. Прототипы TUN/install/auth определяют архитектуру; до закрытия соответствующих системных проверок VPN-beta не объявляется готовой.
3. Пользовательский backend → подтверждённый helper → защищённые runtime данные → TUN/DNS → proxy/desktop UX → тестовый DMG → native-приёмка образуют основной путь.
4. Ранний A/B прототип нужен перед проверяемой заменой app/helper. Проверка DEV и SIGNED ведётся раздельно; отсутствие SIGNED результата не превращает DEV в неуспех.
5. Дополнительные движки добавляются после основного datapath; их результат приводит к новой упаковке и приёмке изменённого набора.
6. Подписанный пакет создаётся новым запуском package scripts внутри M09 и принимается как новый артефакт. M07 не требует завершения M09.
7. Автоматическое обновление имеет дополнительные gates: подтверждённый lifecycle helper, реальный переход двух версий и доказанный способ восстановления.

Контрольные зависимости для проверки графа указаны явно ниже. Это обязательный порядок основных результатов; повтор упаковки с другой подписью создаёт новый результат внутри соответствующего узла. Подписанные предусловия проверяются для SIGNED отдельно от DEV.

| Узел | Результат/задачи | Предпосылки |
|---|---|---|
| BASE | Baseline и bootstrap M00.01 | — |
| BUILD | Сборочные proofs M00.02 | BASE |
| UI | Runner M01.01–M01.02 | BASE |
| AUTH | TUN/install/auth proofs M00.03–M00.06 | BUILD |
| HANDOVER | A/B замена helper M00.07 | AUTH |
| BACKEND | Пользовательский backend M02 | UI, BUILD |
| HELPER | Helper/IPC/данные M03 | BACKEND, AUTH |
| VPN | TUN/proxy/UX M04–M05 | HELPER |
| TEST_DMG | Тестовая поставка M07 с ручным обновлением | VPN, HANDOVER |
| BETA | Обязательная DEV-приёмка M08 | TEST_DMG |
| ENGINES | Дополнительные engines M06 и их переупаковка/приёмка | VPN |
| SIGNED | Новый подписанный пакет M09.01 при доступных signing inputs | BETA |
| SIGNED_ACCEPTANCE | Подписанная приёмка M09.02 и соответствующие строки M08 | SIGNED |
| AUTO | Реализация автоматического обновления M09.03–M09.04 | SIGNED_ACCEPTANCE, HANDOVER |
| AUTO_ACCEPTANCE | Настоящее обновление двух версий M09.05 | AUTO |
| MANUAL_RELEASE | Документация/подписанная ручная поставка M09.06 | SIGNED_ACCEPTANCE, HANDOVER |

## Что остаётся проверить реализацией

- Точный путь штатной установки helper для доступной identity, включая возможности ad-hoc канала.
- Сборку закреплённых engines, Darwin pipeline, protected runtime и полноту протоколов на ARM64.
- Конкретный безопасный IPC/bootstrap контракт и семантику restart/recovery при запрете службы.
- Восстановление routes/DNS/proxy на M2, матрицу минимальной/актуальной macOS и ресурсные показатели.
- Lifecycle app/helper при ручной замене и Sparkle-обновлении, совместимость данных и форму восстановления.

Это оставленные открытыми инженерные проверки с ID задач. Их разрешение не подменяется проверкой структуры Markdown или анализом плана.

## Проверки документов версии 2

Подтверждены 61 родительская задача, 268 уникальных последовательных подшагов и 61 критерий завершения. Все исходные 58 ID сохранены. Проверены ссылки между документами и ссылки на ID задач. Явный граф 16 контрольных точек имеет топологический порядок без циклов; ручной подписанный выпуск не зависит от готовности auto-update. `git diff --check` и проверка пробелов/Markdown fences проходят.
