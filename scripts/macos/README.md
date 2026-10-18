# macOS ARM64: bootstrap

Первый этап проверяет сборку закреплённых компонентов и запуск CLI на macOS. Основной план: [MACOS_DEVELOPMENT_PLAN.md](../../docs/MACOS_DEVELOPMENT_PLAN.md).

```sh
python3 scripts/macos/bootstrap.py --component all
```

Можно выбрать `naga-control`, `sing-box`, `olcrtc` или `hev-socks5-tunnel`. Каждый запуск создаёт отдельную директорию в `build/macos-bootstrap/` с manifest, бинарником, лицензией и журналом. Источники engines получаются в новый checkout из закреплённого commit; существующие Linux source caches используются только как источник Git objects. Исправления зависимостей совпадают с Linux runtime.

На Linux Go-компоненты кросс-компилируются, native execution и сборка hev остаются `UNAVAILABLE`. На ARM64 macOS дополнительно проверяется запуск CLI: help/version, а для sing-box — `check` конфига без TUN. Эти операции не устанавливают службу и не меняют маршруты/DNS. `vpn_device_acceptance` остаётся `UNAVAILABLE` до физического прогона на M2.

Workflow [macos-bootstrap.yml](../../.github/workflows/macos-bootstrap.yml) запускает независимые native jobs для четырёх компонентов и собирает Flutter `.app`. macOS runner пока является каркасом приложения; установка helper и полноценный VPN относятся к следующим задачам плана.

Для ручной проверки артефакта приложения на M2:

1. Скачать ZIP из завершившегося Actions run и распаковать его штатным архиватором.
2. Перенести `Naga Network.app` в `/Applications` и открыть.
3. Записать версию macOS, поведение первого запуска и наличие основного экрана.
4. На этом этапе проверять только интерфейс и ресурсы; рабочее VPN-подключение ещё не является критерием этого артефакта.

Каждый следующий артефакт будет сопровождаться перечнем добавленных функций и соответствующих сценариев M2.

## Диагностическая служба

```sh
bash scripts/macos/build-service-probe.sh A
bash scripts/macos/build-service-probe.sh B
```

Сборка требует macOS с Xcode. Получаются два ad-hoc приложения с отдельным bundle ID `eu.nagavpn.nagaNetwork.ServiceProbe`; версии A/B используют protocol 1/2. CI проверяет компиляцию, подпись и локальный ответ helper без регистрации службы. Установка через `SMAppService`, системное разрешение, XPC с UID 0, reboot и замена проверяются вручную по [инструкции M2](../../docs/MACOS_M2_SERVICE_PROBE.md).
