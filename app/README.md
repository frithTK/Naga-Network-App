# app

Flutter UI Naga Network.

- подключение, профили, узлы, статистика, диагностика, настройки;
- Linux tray (Ayatana AppIndicator) и Windows NotifyIcon;
- TUN и системный прокси.

Сборка Linux: из корня репозитория `./run.sh` или `./scripts/linux/build-appimage.sh`. Нужен Flutter 3.47 (`FLUTTER_BIN`, если не в PATH). Для tray на Linux — `libayatana-appindicator`.

Windows: см. [docs/platforms/WINDOWS.md](../docs/platforms/WINDOWS.md).
