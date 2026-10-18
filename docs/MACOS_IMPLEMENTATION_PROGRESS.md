# Разработка macOS: результаты

Начало: 2026-10-18. План: [этапы](MACOS_DEVELOPMENT_PLAN.md), [подшаги](MACOS_IMPLEMENTATION_STEPS.md).

Рабочая ветка: `feat/macos-arm64-bootstrap`, исходный commit `d2e618a0b84fa24228c4aa92b2ead502a9cb418d`. До перехода на актуальную main сохранены Git bundle, patch и копии четырёх изменённых/новых документов в локальном backup `/tmp/naga-macos-start-byq9ux43`.

## Доступные среды

| Среда | Состояние |
|---|---|
| Linux x86_64 | Доступен; Go 1.27.2, Flutter 3.47.1 / Dart 3.13.1, Swift 6.4 для Linux |
| GitHub Actions | Доступ подтверждён, Actions включены; добавлен macOS ARM64 bootstrap workflow |
| MacBook M2 | Пользователь подтвердил ручную проверку скачиваемых сборок; конкретный macOS build ещё не записан |
| Signing identity | Не предоставлена; Developer ID/native helper registration остаются отдельными проверками |
| Тестовые VPN-серверы | Для этого этапа не предоставлены; CLI smoke не использует сетевой профиль |

## Выполненная подготовка

- **M00.01.01 PASS:** сохранены локальная история, refs и изменённые/untracked документы.
- **M00.01.02 PASS:** получена актуальная `origin/main`, создана отдельная ветка, документы сохранены.
- **M00.01.03 TODO:** записаны инструменты Linux; версии macOS/Xcode предоставит native job и пользовательский M2.
- **M00.01.04 PASS:** доступность Linux/Actions/ручного M2 и недостающих signing inputs зафиксирована.
- **M00.01.05 TODO:** реализован bootstrap workflow; результат первого remote run будет добавлен отдельно.

## Локальные ARM64 build proofs

| Компонент | Кросс-компиляция | Native execution | Результат |
|---|---|---|---|
| naga-control | PASS | UNAVAILABLE на Linux | Получен thin Mach-O ARM64, Go 1.27.2, app version 1.2.9 |
| sing-box | PASS | UNAVAILABLE на Linux | Получен thin Mach-O ARM64 из закреплённого исходника с security dependency fixes |
| olcRTC | PASS | UNAVAILABLE на Linux | Получен thin Mach-O ARM64 из закреплённого исходника с dependency fixes |
| hev | UNAVAILABLE на Linux | UNAVAILABLE | Требуются macOS SDK и native job |

Каждая bootstrap-сборка сохраняет собственный manifest, source commit, builder digest, признак dirty tree, SHA-256 бинарника, лицензию, Go build info и журнал в `build/macos-bootstrap/`. Native CLI smoke и VPN datapath учитываются отдельно.

## macOS runner

Штатным Flutter 3.47.1 создан `app/macos`. Перенесены только macOS scaffold и запись платформы в Flutter metadata. Настроены macOS 14, ARM64, product name `Naga Network` и ad-hoc signing; App Sandbox отключён согласно принятой внешней архитектуре. Проверены синтаксис workflow, plist/entitlements и Python bootstrap.

Native `.app` build и внешний вид на M2 ещё не подтверждены; системный helper и VPN не реализованы. Default scaffold icons будут заменены существующими бренд-ресурсами на этапе оформления runner. Инструкции для артефактов: [scripts/macos/README.md](../scripts/macos/README.md).
