#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
out_dir="${project_dir}/dist/windows/host"
mkdir -p "${out_dir}"

echo "Кросс-сборка Windows host-бинарников (без Flutter UI)..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -ldflags "-s -w" \
  -o "${out_dir}/naga-control.exe" "${project_dir}/cmd/naga-control"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -ldflags "-s -w -H windowsgui" \
  -o "${out_dir}/NagaNetwork.exe" "${project_dir}/cmd/naga-windows"

echo "Готово: ${out_dir}"
echo "Flutter Windows exe с Linux не собирается. На ВМ запусти scripts\\windows\\build.ps1."
echo "Лаунчер из этой кроссборки без брендовой иконки; ICO вшивается только build.ps1 через go-winres."
