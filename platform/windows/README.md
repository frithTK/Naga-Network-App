# Windows platform

Portable zip и установщик UI+control-plane:
[docs/platforms/WINDOWS.md](../../docs/platforms/WINDOWS.md),
`scripts/windows/build.ps1`, `packaging/windows/naga-network.iss`.

Нужен Inno Setup 6 (`ISCC.exe`) для `*-setup.exe`. Данные пользователя —
`%LOCALAPPDATA%\naga-network`, программа —
`%LOCALAPPDATA%\Programs\Naga Network`. Не запускай portable и установленную
копию параллельно. VC++ runtime копируется в бандл из VS Build Tools. Подпись
Authenticode — через `NAGA_SIGN_PFX` / `NAGA_SIGN_SHA1`.

Native runtime: в бандле `sing-box.exe` и `wintun.dll`. Setup один раз просит
UAC и ставит scheduled task `NagaNetworkElevatedHelper`; «Подключить» после
этого без запроса. Portable без helper — UAC на подключение. Системный прокси —
WinINET текущего пользователя на `127.0.0.1:2080`. Пустой `core/engine/wintun`
не заводить.
