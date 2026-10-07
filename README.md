# Naga Network

**English** | [Русский](README.ru.md)

Desktop VPN client for Linux and Windows. Import a subscription or a config file, pick a node, connect. The app does not run its own servers: it uses the profile you imported.

Builds: [Releases](https://github.com/frithTK/Naga-Network-App/releases). License: [GPL-3.0-or-later](LICENSE).

## Features

- system VPN (TUN) or system proxy;
- node list with manual pick, auto-select and latency probes;
- traffic, quota and expiry from subscription headers;
- Russian UI, tray, no hand-editing JSON;
- built-in LAN/RU rules and optional provider lists.

Add a profile from an HTTPS URL, a sing-box JSON file, or an AmneziaWG `.conf`.

## Protocols

The engine is [sing-box](https://sing-box.sagernet.org). Ordinary subscription nodes go through it:

| Protocol | Typical use |
|---|---|
| **VLESS** | TLS, often with Reality |
| **Hysteria2** | QUIC, loss-tolerant |
| **TUIC** | QUIC, auto-select fallback |
| **Shadowsocks** | including ShadowTLS |
| **Trojan** | TLS |
| **VMess** | if present in the profile |

Outside stock sing-box:

- **AmneziaWG** — obfuscated WireGuard (plain WireGuard without Amnezia fields is rejected);
- **olcRTC** — tunnel over WebRTC rooms (Jitsi, Telemost, WB). While it is selected, other engines stay down.

## Platforms

Linux and Windows. No iOS or macOS yet.

On Linux the UI is an AppImage; TUN is started by local `naga-control`. The Windows installer and portable zip already include the UI, control-plane, sing-box and Wintun.

In Settings, **Check for updates** downloads the matching GitHub Release asset and installs it. **Auto-update the app** (on by default) checks GitHub at startup at most once every 12 hours and installs when a complete release is available. OS dialogs still appear (pkexec, installer, PackageInstaller). Turn the toggle off to keep updates manual. Linux needs both the AppImage and `naga-control-linux-x86_64` in the same release.

Release file names:

| Platform | Asset |
|---|---|
| Linux UI | `NagaNetwork-<ver>-x86_64.AppImage` |
| Linux runtime | `naga-control-linux-x86_64` |
| Windows portable | `NagaNetwork-<ver>-windows-x64.zip` |
| Windows setup | `NagaNetwork-<ver>-windows-x64-setup.exe` |
| Android | `NagaNetwork-<ver>-arm64-v8a.apk` |

Details: [RELEASES.md](docs/platforms/RELEASES.md).

## Documentation

Docs are in Russian:

- [Linux](docs/platforms/LINUX.md)
- [Windows](docs/platforms/WINDOWS.md)
- [Android](docs/platforms/ANDROID.md)
- [Release assets](docs/platforms/RELEASES.md)
- [sing-box](docs/engines/SING_BOX.md), [AmneziaWG](docs/engines/AMNEZIAWG.md), [olcRTC](docs/engines/OLCRTC.md)
