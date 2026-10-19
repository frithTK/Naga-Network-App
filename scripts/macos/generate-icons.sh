#!/usr/bin/env bash
set -euo pipefail

# Optional source-asset regeneration; committed PNGs are used by Xcode builds.
# Requires librsvg's rsvg-convert, not a Python/image-generation dependency.
project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source_svg="${project_dir}/assets/brand/naga-mark-red.svg"
icon_dir="${project_dir}/app/macos/Runner/Assets.xcassets/AppIcon.appiconset"
for size in 16 32 64 128 256 512 1024; do
  rsvg-convert --width "${size}" --height "${size}" \
    --output "${icon_dir}/app_icon_${size}.png" "${source_svg}"
done
