# macOS ARM64: bootstrap

Первый этап проверяет сборку закреплённых компонентов и запуск CLI на macOS. Основной план: [MACOS_DEVELOPMENT_PLAN.md](../../docs/MACOS_DEVELOPMENT_PLAN.md).

```sh
python3 scripts/macos/bootstrap.py --component all
```

Можно выбрать `naga-control`, `sing-box`, `olcrtc` или `hev-socks5-tunnel`. Каждый запуск создаёт отдельную директорию в `build/macos-bootstrap/` с manifest, бинарником, лицензией и журналом. Источники engines получаются в новый checkout из закреплённого commit; существующие Linux source caches используются только как источник Git objects. Исправления зависимостей совпадают с Linux runtime.

На Linux Go-компоненты кросс-компилируются, native execution и сборка hev остаются `UNAVAILABLE`. На ARM64 macOS дополнительно проверяется запуск CLI: help/version, а для sing-box — `check` конфига без TUN. Эти операции не устанавливают службу и не меняют маршруты/DNS. `vpn_device_acceptance` остаётся `UNAVAILABLE` до физического прогона на M2.

Workflow [macos-bootstrap.yml](../../.github/workflows/macos-bootstrap.yml) запускает независимые native jobs для четырёх компонентов и собирает Flutter `.app`. macOS `.app` включает пользовательский backend; установка системного helper и полноценный VPN относятся к следующим задачам плана. Diagnostic helper поставляется отдельно.

Native CI также запускает Go tests/vet и проверяет готовый `.app` через `check-app.py`: версии из pubspec, deployment target, strict code signature, отсутствие App Sandbox, ARM64 и динамические зависимости. Отчёт `macos-app-inspection.json` сохраняется вместе с ZIP приложения.

Для ручной проверки артефакта приложения на M2:

1. Скачать ZIP из завершившегося Actions run и распаковать его штатным архиватором.
2. Перенести `Naga Network.app` в `/Applications` и открыть.
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

## Приложение с пользовательским backend

На ARM64 Mac с Go/Flutter/Xcode: `bash scripts/macos/build-app.sh`. Скрипт собирает backend и Release UI из одной версии pubspec, встраивает `naga-control` в `Contents/Helpers` и подписывает вложенный code перед внешним app bundle. Запуск простого `flutter build macos` создаёт UI без этого шага упаковки; для установки нужен полный результат скрипта.

`test-app-session.py <путь-к-app>` проверяет упакованный CLI в изолированном временном домашнем каталоге: readiness, версию, обязательный token, отказ browser origin и штатную остановку по закрытию parent pipe. Программа не включает VPN. Нельзя запускать `--app-session` с выводом в терминал/журнал: этот режим требует приватных stdin/stdout pipes.
