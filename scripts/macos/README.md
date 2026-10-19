# macOS ARM64: bootstrap

Текущий исходный код включает экспериментальный sing-box TUN через отдельный VPN helper,
пользовательский backend, кнопку «Проверить Mac» и DMG. Первая сетевая проверка описана
в [MACOS_VPN_TEST.md](../../docs/MACOS_VPN_TEST.md); физический результат ещё не получен.
Ранее опубликованный `check1` содержит только диагностическую службу и не включает VPN.
Основной план: [MACOS_DEVELOPMENT_PLAN.md](../../docs/MACOS_DEVELOPMENT_PLAN.md).

Пакет для первой проверки TUN: [DMG `vpn1`](https://github.com/frithTK/Naga-Network-App/releases/download/macos-dev-1.2.9-vpn1/NagaNetwork-1.2.9-macos-arm64.dmg),
source `a157c952936171585a56220e23ab3a05bf46ff24`, [native CI PASS](https://github.com/frithTK/Naga-Network-App/actions/runs/39240394882).
Сценарий физической проверки приведён в [MACOS_VPN_TEST.md](../../docs/MACOS_VPN_TEST.md).

```sh
python3 scripts/macos/bootstrap.py --component all
```

Можно выбрать `naga-control`, `sing-box`, `olcrtc` или `hev-socks5-tunnel`. Каждый запуск создаёт отдельную директорию в `build/macos-bootstrap/` с manifest, бинарником, лицензией и журналом. Источники engines получаются в новый checkout из закреплённого commit; существующие Linux source caches используются только как источник Git objects. Исправления зависимостей совпадают с Linux runtime.

На Linux Go-компоненты кросс-компилируются, native execution и сборка hev остаются `UNAVAILABLE`. На ARM64 macOS дополнительно проверяется запуск CLI: help/version, а для sing-box — `check` конфига без TUN. Эти операции не устанавливают службу и не меняют маршруты/DNS. `vpn_device_acceptance` остаётся `UNAVAILABLE` до физического прогона на M2.

Workflow [macos-bootstrap.yml](../../.github/workflows/macos-bootstrap.yml) запускает независимые
native jobs для четырёх компонентов и собирает Flutter `.app` с backend, sing-box, bridge,
VPN helper и отдельным diagnostic helper. Validator проверяется без root/сетевых изменений;
проверка `.app` сверяет manifest и встроенные engine hash/CDHash. A/B probe сохранён отдельно.

Native CI также запускает Go tests/vet и проверяет готовый `.app` через `check-app.py`: версии из pubspec, deployment target, strict code signature, отсутствие App Sandbox, ARM64 и динамические зависимости. Отчёт `macos-app-inspection.json` сохраняется вместе с ZIP приложения.

Для подключения TUN используйте [первую проверку VPN](../../docs/MACOS_VPN_TEST.md).
Отдельная [быстрая диагностика](../../docs/MACOS_QUICK_CHECK.md) проверяет регистрацию/XPC/удаление
status-only службы через кнопку «Проверить Mac». A/B, Терминал и перезагрузка для неё не нужны.

Дополнительные инженерные проверки приложения на M2:

1. Скачать [DMG тестовой сборки](https://github.com/frithTK/Naga-Network-App/releases/download/macos-dev-1.2.9-check1/NagaNetwork-1.2.9-macos-arm64.dmg) напрямую, без входа в GitHub. [Остальные файлы и checksum](https://github.com/frithTK/Naga-Network-App/releases/tag/macos-dev-1.2.9-check1).
2. Открыть скачанный DMG, перетащить `Naga Network.app` в Applications и запустить установленную копию. В сборках до добавления DMG используется вложенный `macos-application.zip`. Если macOS блокирует открытие ad-hoc приложения, записать сообщение и использовать штатное разрешение через «Конфиденциальность и безопасность», если оно доступно.
3. Записать версию macOS, поведение первого запуска и наличие основного экрана.
4. Дождаться основного экрана после запуска встроенного backend; при ошибке записать сообщение. Проверить импорт небольшого JSON/CONF через «Из файла», отмену выбора, Unicode в имени файла и сохранение профиля/настроек после `⌘Q` и повторного открытия.
5. После `⌘Q` в «Мониторинге системы» не должен оставаться дочерний `naga-control`. При принудительном завершении UI backend также должен закончить работу по EOF. Проверить повторное открытие приложения и отсутствие второго backend для тех же данных.
6. Рабочее VPN-подключение ещё не является критерием этого артефакта: UI показывает тестовый статус, а API отказывает в старте до подключения системной службы.

Каждый следующий артефакт будет сопровождаться перечнем добавленных функций и соответствующих сценариев M2.

## Диагностическая служба

```sh
bash scripts/macos/build-service-probe.sh A
bash scripts/macos/build-service-probe.sh B
```

Сборка требует macOS с Xcode. Получаются два ad-hoc приложения с отдельным bundle ID `eu.nagavpn.nagaNetwork.ServiceProbe`; версии A/B используют protocol 1/2. CI проверяет компиляцию, подпись и локальный ответ helper без регистрации службы. Установка через `SMAppService`, системное разрешение, XPC с UID 0, reboot и замена проверяются вручную по [инструкции M2](../../docs/MACOS_M2_SERVICE_PROBE.md).

Иконки macOS получены из существующего `assets/brand/naga-mark-red.svg`. Для повторного экспорта нужен `rsvg-convert`: `bash scripts/macos/generate-icons.sh`. Сборка Xcode использует уже сохранённые PNG и не требует librsvg.

Проверенный пакет с кнопкой «Проверить Mac»: [DMG](https://github.com/frithTK/Naga-Network-App/releases/download/macos-dev-1.2.9-check1/NagaNetwork-1.2.9-macos-arm64.dmg), source `6f4e2bf382d2d21d293e0cb2b6de5d9abc233506`. Прямые файлы и SHA256SUMS опубликованы в [отдельном pre-release](https://github.com/frithTK/Naga-Network-App/releases/tag/macos-dev-1.2.9-check1). Actions artifacts остаются дополнительным способом скачивания и требуют входа в GitHub. Все шесть native jobs прошли: [run 39152885679](https://github.com/frithTK/Naga-Network-App/actions/runs/39152885679).

## Приложение с пользовательским backend

На ARM64 Mac с Go/Flutter/Xcode: `bash scripts/macos/build-app.sh`. Скрипт собирает backend,
закреплённый sing-box с Darwin watchdog/API restrictions и Release UI. Встраивает runtime
в `Contents/Helpers`, нормализует bundle, подписывает bridge/engine и встраивает их hashes
в VPN helper. Последним подписывается внешний app. Запуск простого `flutter build macos`
создаёт UI без этого шага упаковки; для установки нужен полный результат скрипта.

`test-app-session.py <путь-к-app>` проверяет упакованный CLI в изолированном временном домашнем каталоге: readiness, версию, обязательный token, отказ browser origin и штатную остановку по закрытию parent pipe. Программа не включает VPN. Нельзя запускать `--app-session` с выводом в терминал/журнал: этот режим требует приватных stdin/stdout pipes.

## Тестовый DMG

После `build-app.sh`: `bash scripts/macos/build-dmg.sh`. В `build/macos-dmg/` сохраняются
`NagaNetwork-<ver>-macos-arm64.dmg` и `SHA256SUMS.txt`. Это development-образ с ad-hoc
приложением для первой проверки TUN, не production release.

`python3 scripts/macos/check-dmg.py <image> --report build/macos-dmg/dmg-inspection.json` проверяет checksum образа, монтирует его read-only, проверяет Applications link/инструкцию/состав/подписи/ARM64 и запускает private session упакованного backend без VPN. После проверки образ отмонтируется. Сценарии процессов и каналов описаны в [контракте](../../docs/MACOS_PROCESS_CONTRACT.md).
