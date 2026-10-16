# Windows

- [Релизы](https://github.com/frithTK/Naga-Network-App/releases): setup или portable ZIP. Запуск: `NagaNetwork.exe`.
- Setup один раз запрашивает UAC и устанавливает helper; TUN затем подключается без UAC.
- Portable без helper запрашивает UAC при подключении. Установка helper: `naga-control.exe --install-helper` от администратора.
- Обновление: «Проверить обновление» или автообновление; portable использует `naga-update.exe`.
- Данные: `%LOCALAPPDATA%\naga-network\`; программа: `%LOCALAPPDATA%\Programs\Naga Network\`.
- Запускайте одну копию приложения. Крестик скрывает окно; «Выход» останавливает UI и VPN.

## Сборка

Windows 10/11 x64; Git, Go из `go.mod`, Flutter 3.47 stable, Python 3 в PATH, Visual Studio 2022 C++ Build Tools, Inno Setup 6.

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\windows\build.ps1
```

Артефакты: `dist\windows\`. CI: [workflow](../../.github/workflows/linux-appimage.yml), `build_windows=true`.
Подпись: `NAGA_SIGN_PFX` или `NAGA_SIGN_SHA1`.

## Runtime

- API: `127.0.0.1:8765`, Bearer-токен.
- TUN/olcRTC: задача `NagaNetworkElevatedHelper`; системный прокси: WinINET `127.0.0.1:2080`.
- Сборка host-бинарников на Linux без UI: `scripts/windows/cross-compile-host.sh`.
