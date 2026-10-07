# Naga brand assets

Канон исходников брендпака — каталог `source/` в этом дереве; дубль экспорта в `design/` снят.

- `source/` — исходные SVG-страницы и варианты логотипа;
- `naga-mark-red.svg` — подготовленный вариант знака для интерфейса;
- `raster/` — растровые превью для документации и ручной проверки;
- `app/assets/brand/naga-mark-red.png` — знак, который использует Flutter UI.
- `naga-network.ico` — Windows-иконка (16/32/48/256) из красного знака для exe, taskbar и tray.
- `app/assets/brand/mascot-welcome.png` — изолированный mascot asset для welcome/onboarding и promo-состояний.
- `app/assets/brand/states/` — чиби-позы из брендпака под состояния подключения
  (`welcome`, `idle`, `connecting`, `connected`, `error`, `offline`); 320 px,
  прозрачный фон, grayscale кроме `welcome` с красным знаком.

Основной UI-цвет — `#F72F38`, базовый фон — `#080808`, светлый текст — `#FBFBF9`.
