# macOS: контракт пользовательских процессов

Статус: реализован пользовательский backend; контракт привилегированного helper остаётся provisional до проверок M00.04–M00.06. Физические сценарии M2 учитываются отдельно от native CI.

## Процессы и данные

`Naga Network.app` запускается от пользователя. Swift host находит `Contents/Helpers/naga-control` относительно собственного bundle и запускает его без shell с аргументом `--app-session`. Go backend работает с тем же UID; запуск этого режима с UID 0 на macOS запрещён. Обычный UI не повышает права и не регистрирует daemon.

Профили, настройки, `control.token` и `backend.lock` находятся в `~/Library/Application Support/Naga Network`; журналы — в `~/Library/Logs/Naga Network`. Native host передаёт каталог данных, полученный через macOS home provider. Go подтверждает его в readiness, а Dart использует тот же путь для настроек обновления. `NAGA_DATA_DIR` остаётся отдельным входом для CLI-тестов/embedding. Cache, root-owned helper-state и миграция прежних dev-каталогов ещё не реализованы.

Каталог пользователя должен быть настоящей директорией текущего UID, с правами 0700. Lock открывается с `O_NOFOLLOW` и проверкой владельца/типа; неблокирующий POSIX lock запрещает второй backend с теми же данными. Файл токена должен быть обычным, принадлежащим пользователю и недоступным группе/остальным. Новый token — 32 случайных байта в hex, файл 0600. Lock удерживается открытым descriptor до окончания cleanup.

## Readiness v1

Режим `--app-session` требует stdin/stdout pipes. Stdout содержит одну JSON-строку, а не журнал. После привязки собственного listener к `127.0.0.1:0` Go возвращает:

```json
{
  "type": "ready",
  "protocol_version": 1,
  "endpoint": "http://127.0.0.1:<assigned-port>",
  "token": "<private 64-character hex token>",
  "version": "<version from app/pubspec.yaml>",
  "data_dir": "<absolute Application Support path>"
}
```

Ошибка до готовности: `{"type":"error","message":"<startup failure>"}` и exit 1. Token не передаётся в argv или журналы. Native host удаляет унаследованные control-token и engine overrides; engine discovery для этой сессии использует только явный путь, а не PATH/Homebrew.

Swift ограничивает startup frame до 16 KiB и ожидание до 15 секунд. Он проверяет тип/версию handshake, точный IPv4 loopback endpoint без userinfo/path/query/fragment, token и совпадение bundle/backend versions. Через `eu.nagavpn.naga_network/macos` метод `startBackend` возвращает этот ответ Flutter. Dart повторно валидирует контракт, создаёт client с полученными endpoint/token и делает авторизованный health; только после этого создаётся Dashboard. UI имеет внешний timeout 20 секунд. Неуспешный запуск показывает ошибку и действие повтора без fallback к порту 8765.

Управляющие HTTP routes требуют bearer token. Browser Origin отклоняется для этой нативной сессии. Readiness относится к конкретному дочернему процессу; чужой HTTP health не используется для выбора endpoint или получения полномочий.

## Остановка и восстановление

Закрытие входного pipe родительским приложением или SIGTERM прекращает приём HTTP-запросов. Backend ждёт завершения текущих запросов сохранения профилей/настроек до пяти секунд, затем выполняет cleanup и закрывает оставшиеся соединения. Это также связывает жизнь backend с принудительно завершённым UI. Native host ждёт максимум 8 секунд, затем завершает свой дочерний процесс. Обычное завершение проходит через Flutter termination handler и асинхронный ответ AppKit. Неожиданный выход готового backend отправляет `backendExited`; Dart освобождает client и предлагает новый запуск. Новый процесс заново получает readiness; token и профили сохраняются.

Сейчас сессия сообщает `vpn_connect: false` и причину недоступности. `runtime/start` отклоняется до запуска runtime. Это позволяет проверять UI/профили, сохраняя отдельной задачей подключение системного helper и его network cleanup. Остановка пользовательского процесса не считается доказательством восстановления сети после будущего привилегированного туннеля.

## Отдельный service probe

`Naga Service Probe.app` использует другой bundle ID и диагностический root helper только с операцией status. Его версии A/B имеют протоколы 1/2, независимые от readiness v1. Он нужен для проверки SMAppService, разрешений, XPC, reboot и замены службы на M2. Проверка ad-hoc identifiers в пробнике не является production policy для privileged API. Инструкция: [MACOS_M2_SERVICE_PROBE.md](MACOS_M2_SERVICE_PROBE.md).
