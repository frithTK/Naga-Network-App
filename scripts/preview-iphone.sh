#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../app" && pwd)"
flutter_bin="${FLUTTER_BIN:-flutter}"
base_port="${NAGA_PREVIEW_PORT:-8080}"
port="$base_port"
startup_timeout="${NAGA_PREVIEW_TIMEOUT:-120}"
browser="${NAGA_PREVIEW_BROWSER:-chromium}"
build_log="${TMPDIR:-/tmp}/naga-network-flutter-web-build.log"
server_log="${TMPDIR:-/tmp}/naga-network-flutter-static-server.log"

if ! command -v "$flutter_bin" >/dev/null 2>&1; then
  echo "Flutter не найден. Задай FLUTTER_BIN или установи Flutter в PATH." >&2
  exit 1
fi

cd "$project_dir"
server_pid=""

cleanup() {
  if [[ -n "$server_pid" ]]; then
    kill "$server_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

echo "Собираю web preview..."
if ! "$flutter_bin" build web >"$build_log" 2>&1; then
  echo "Flutter web build не удался. Лог: $build_log" >&2
  tail -40 "$build_log" >&2 || true
  exit 1
fi

server_ready=0
for offset in $(seq 0 9); do
  port=$((base_port + offset))
  : >"$server_log"
  python3 -m http.server "$port" \
    --bind 127.0.0.1 \
    --directory "$project_dir/build/web" \
    >"$server_log" 2>&1 &
  server_pid=$!

  for _ in $(seq 1 "$startup_timeout"); do
    if curl --silent --fail "http://127.0.0.1:${port}" >/dev/null 2>&1; then
      server_ready=1
      break 2
    fi
    if ! kill -0 "$server_pid" 2>/dev/null; then
      break
    fi
    sleep 1
  done

  if [[ "$server_ready" -ne 1 ]]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
done

if [[ "$server_ready" -ne 1 ]]; then
  echo "Static web server не запустился на портах ${base_port}–$((base_port + 9)). Лог: $server_log" >&2
  tail -30 "$server_log" >&2 || true
  exit 1
fi

echo "Naga Network iPhone preview: http://127.0.0.1:${port}"
echo "Окно рассчитано на viewport 390×844 (iPhone 15)."

if [[ "$browser" != "none" ]] && command -v "$browser" >/dev/null 2>&1; then
  "$browser" \
    --app="http://127.0.0.1:${port}" \
    --window-size=390,844 \
    --force-device-scale-factor=1 \
    >/dev/null 2>&1 &
  echo "Chromium запущен. Если окно не открылось, вставь ссылку выше вручную."
  echo "Для остановки preview нажми Ctrl+C в этом терминале."
  wait "$server_pid"
else
  echo "Браузер не запущен — открой ссылку вручную и включи viewport 390×844 в DevTools."
  wait "$server_pid"
fi
