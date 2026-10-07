# Sing-box

Основное ядро клиента.

Клиент принимает JSON sing-box напрямую или поле `singbox` из конверта Naga Network. Исходный файл на диске не переписывается. Перед запуском на Linux runtime нормализует схему под sing-box 1.14+ (inbound sniff/DNS, без Android-only полей).

Поддерживаются VLESS Reality, TUIC, Hysteria2, selector/urltest, статистика и `sing-box check`.

Встроенные правила (локальная сеть и зоны `.ru` / `.su` / `.рф`) включаются тумблерами. Списки из подписки клиент скачивает сам и подкладывает sing-box как local-файл; runtime эти URL сам не качает.

AmneziaWG stock sing-box не терминирует — см. [AMNEZIAWG.md](AMNEZIAWG.md).
