# Результат проверки стенда

Заполнить отдельную копию для каждой платформы. Пароли, токены, private keys и настоящие subscription URLs не включать.

| Поле | Значение |
|---|---|
| Дата / платформа / OS / SDK | |
| Снимок исходников / commit + diff | |
| Artifact filename / SHA256 | |
| Установленная version / package или bundle ID | |
| Signer / account token level | |
| Android PAGE_SIZE, если применимо | |
| Test fixture / engine и версия | |
| Исходный proxy / DNS / routes snapshot | |
| Сценарий из NATIVE_SECURITY_TESTS.md | |
| Команды / последовательность действий | |
| Ожидаемый результат | |
| Наблюдаемый результат | |
| Статус: PASS / FAIL / PENDING | |
| Logs / screenshot / packet observations | |
| FD / процессы / ресурсы до и после | |
| Состояние после Stop / cleanup / restart | |
| Недостающая предпосылка для PENDING | |

При FAIL указать минимальные шаги, повторяемость и непосредственное последствие. Connected без packet evidence не подтверждает маршрут; успешная компиляция не подтверждает установку/подпись; отказ собственного test harness отделяется от дефекта приложения.
