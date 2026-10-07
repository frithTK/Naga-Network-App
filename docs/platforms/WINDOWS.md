# Windows

Portable zip и установщик: Flutter UI, `naga-control`, MSVC CRT, `sing-box.exe`, `wintun.dll`.

Native TUN — через sing-box и Wintun. Установщик один раз запрашивает UAC и регистрирует elevated-helper; дальше «Подключить» без запроса. Portable zip helper не ставит — там UAC на каждое подключение, либо один раз `naga-control.exe --install-helper` от администратора.

## Установка

Скачай `NagaNetwork-<ver>-windows-x64-setup.exe` или `NagaNetwork-<ver>-windows-x64.zip` из [релиза](https://github.com/frithTK/Naga-Network-App/releases). Запускай **`NagaNetwork.exe`**. Имена файлов: [RELEASES.md](RELEASES.md).

Обновление из приложения: тумблер «Автообновление приложения» или кнопка «Проверить обновление». Portable распаковывает zip через `naga-update.exe` после выхода UI (запущенный exe Windows не перезапишет). Setup запускает новый установщик.

Данные: `%LOCALAPPDATA%\naga-network\`. Программа: `%LOCALAPPDATA%\Programs\Naga Network\`. Setup просит права администратора один раз.

## Сборка

Нужна машина Windows 10/11 x64 (Flutter Windows с Linux не кросс-компилируется). Либо workflow [Windows portable](../../.github/workflows/windows-portable.yml).

1. Git, Go (версия из `go.mod`), Flutter 3.47 stable.
2. Visual Studio 2022 Build Tools: Desktop development with C++.
3. Inno Setup 6 (`ISCC.exe`) для setup.exe.
4. Опционально Authenticode: `NAGA_SIGN_PFX` или `NAGA_SIGN_SHA1`.

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\windows\build.ps1
```

Артефакты в `dist\windows\`: portable-папка, zip и `*-setup.exe` (UAC на установку, helper без UAC на «Подключить»).

Host-бинарники с Linux: `./scripts/windows/cross-compile-host.sh` (без Flutter UI и без иконки лаунчера).

## Поведение

- UI и control-plane — отдельные процессы на `127.0.0.1:8765` с Bearer.
- Крестик скрывает окно в NotifyIcon; «Выход» останавливает runtime и UI. Portable `naga-control` при выходе может остаться — `taskkill /IM naga-control.exe /F`.
- Один экземпляр UI. Не запускай portable и установленную копию одновременно.
- После setup TUN и olcRTC идут через scheduled task `NagaNetworkElevatedHelper` (права администратора, без UAC). Portable без helper — UAC на «Подключить». System proxy пишет WinINET (`127.0.0.1:2080`) без UAC.
