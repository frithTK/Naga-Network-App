# Приёмка на тестовых стендах

На 2026-10-14 пользователь запускает проверки отдельно на каждом своём стенде. Доступ агенту не требуется. Сценарии подготовлены, результаты ещё не получены. [Общий результат](FIX_AND_SECURITY_PROGRESS.md).

Для записи результатов: [шаблон](NATIVE_SECURITY_RESULT_TEMPLATE.md), по одной копии на платформу/сценарий.

Для каждого прогона записываются OS/SDK/page size, installed artifact SHA256, version/package/bundle ID, signer/token level, fixture ID, исходная сеть, команды, expected/actual, logs и cleanup. Профили/назначения принадлежат стенду; настоящие subscription keys не нужны. Routes/DNS/proxy изменяются внутри выделенного стенда с исходным snapshot.

## Windows 10/11 VM

1. На своём стенде подготовить snapshot VM, medium-token account и возможность elevation; записать install directory/ACL и способ восстановления snapshot.
2. В отдельной копии проекта выполнить go version, `go test -count=1 -timeout=90s ./...`, go vet; записать результаты cmd/naga-update/wait_windows_test.go. Parse scripts штатным PowerShell Parser, затем запустить scripts/windows/build.ps1. Проверить версии и состав portable/setup. Cross compile на Linux этого не заменяет.
3. F05/R02: ZIP с traversal/drive/UNC/ADS/reserved names/collisions; symlink/junction parent в test install root; внешний sentinel; locked target; отказ второй replace. Expected: sentinel неизменен, старая версия доступна, отказ виден. Отдельно crash/restart и UI/control/engine version skew.
4. F17/H03/H05: разрешённый UI, отдельный medium-token клиент того же SID, другой пользователь; malformed/oversized IPC; trusted и writable/reparse bundle. Зафиксировать DACL/MIC и token host. Безвредный sentinel проверяет IPC LogPath, log.output/cache path из config. Установить возможность подмены binary/config между validation и launch. Privilege impact подтверждает только наблюдаемая операция повышенным token.
5. F04/F12/F16: чужой proxy на 2080/32080, Connect/Stop, пользовательская смена настроек во время VPN, slow probe/acquire, два Stop, policy reconnect. Expected: настройки сохраняются; старый Start не возвращает Connected после Stop.
6. H04: два независимых тестовых VPN клиента. Обновление Naga не должно завершать чужой процесс с тем же image name; использовать безвредный test engine.
7. Import → select → connect → IPv4/IPv6/DNS/TCP/UDP → stop → restart; split/LAN/RU/endpoint bypass, engine exit, suspend; routes/resources после каждого отказа.

## Android arm64

1. На своём стенде выбрать adb serial и разрешить установку test APK. Записать SDK и `adb -s SERIAL shell getconf PAGE_SIZE`. Для 16KB совместимости требуется отдельная 16KB среда.
2. APK должен иметь известную тестовую подпись, подтверждённые package/version/signer и native hashes. Private audit APK: локальный docs/testing/audit/20261013T174635Z-fixes/checks/android/fixed-apk/. Manifest/check logs определяют результат сборки и ELF gate; debug artifact не является release candidate.
3. F03/H02: первый consent/отказ, establish failure, быстрый Start/Stop, повторный Start, revoke, background/recreation. ACK только после establish+FD transfer. Завершить только test hev; проверить FD/process count, failed, старый exit после нового FD и следующий запуск.
4. Routing: all_vpn, selected_vpn с одним/нулём приложений, direct/исключённое приложение. Наблюдать IPv4/IPv6/DNS/UDP/TCP. Разделять black hole, неверный UI и доказанный обход туннеля.
5. F08: следующая APK version с той же test key; pending action → подтверждение → установленная новая версия. Отказ/unknown sources/signature mismatch/нет места → конечный отказ и старая версия. Activity onStop/recreation/restart/duplicate callback. Отдельный test app с поддельным broadcast не должен пройти receiver/session/nonce.
6. Сохранить test-app logs, ресурсы, packet observations и cleanup; source/build PASS не подтверждает callback или datapath.

## iPhone

1. На своём стенде подготовить iPhone/Mac и signing/provisioning, записать модель/OS. Credentials/private keys не включаются в отчёт.
2. В новой build directory собрать SwiftUI shell, подписать соответствующим profile; проверить bundle ID/version/build/final signature, записать IPA hash. Не принимать старый .app после compile/sign failure.
3. Shell: imports, malformed input, select/delete restrictions, API offline/recovery, polling, background/foreground, deep links и identity. Linux API-model tests этого не подтверждают.
4. PacketTunnel: наличие реализованного Go framework/datapath и NetworkExtension entitlements/profile. Unsigned extension с __text подтверждает link, но не provisioning. При отсутствии предпосылки оставить конкретный PENDING.
5. При наличии datapath проверить connect/traffic/stop/restart, revoke/background/engine exit, DNS/routes/packets и UI; executable не должен изменяться после signing.

## Linux desktop/network VM

Актуальный файл: [единый AppImage 1.2.5](../dist/NagaNetwork-1.2.5-x86_64.AppImage), [hashes/проверки сборки](../dist/linux-build-manifest.json). Внутри UI, control, sing-box, olcRTC и hev. При первом запуске требуется авторизация установки системной службы.

1. Отдельный desktop/VM, test engine/profiles; baseline proxy/DNS/routes. Уже выполненный gsettings keyfile regression не менял рабочий desktop.
2. System proxy/TUN, занятый port, чужой proxy 2080/32080, смена настроек, configure/restore failure, slow connect/Stop и child exit.
3. IPv4/IPv6/DNS/UDP/TCP, split/LAN/RU/provider rules, tray/suspend, cleanup. Проверить, что сервис использует встроенные движки из каталога соответствующего build-id.
4. На чистом стенде запуск одного AppImage → авторизация → healthy control с нужной версией и olcrtc_ready; Go/Flutter и системный sing-box не должны требоваться. Отмена авторизации сохраняет прежний control. Повторный запуск установленного build не должен снова запрашивать авторизацию. Проверить cold boot, восстановление сервиса, установку с primary group, отличной от username, и следующий single-asset update.

Результат: PASS, FAIL или PENDING с конкретной отсутствующей предпосылкой. При FAIL сначала минимальное воспроизведение, затем fix и повтор старого proof/вариантов. Установка на стенд сама по себе не закрывает R13.
