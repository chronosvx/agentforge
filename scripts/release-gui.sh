#!/usr/bin/env bash
# 为 macOS GUI 发布产物生成校验和（M5 release helper）。
# 依赖：先运行 scripts/build-gui-macos.sh（或 make gui-macos）。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APP="$ROOT/dist/agentforge_gui.app"
CHECKSUM_FILE="$ROOT/dist/gui-checksums.txt"

if [[ ! -d "$APP" ]]; then
  echo "ERROR: $APP not found; run 'make gui-macos' first." >&2
  exit 1
fi

: > "$CHECKSUM_FILE"

# DMG 文件（build-gui-macos.sh 在缺少 hdiutil 时会跳过 DMG，因此这里可选）。
if compgen -G "$ROOT/dist/agentforge_gui-*.dmg" >/dev/null; then
  (cd "$ROOT/dist" && shasum -a 256 agentforge_gui-*.dmg) >> "$CHECKSUM_FILE"
fi

# App 是目录，逐个计算内部文件校验和，覆盖整个 .app bundle。
(cd "$ROOT/dist" && find agentforge_gui.app -type f -print0 | LC_ALL=C sort -z | xargs -0 shasum -a 256) >> "$CHECKSUM_FILE"

echo "==> GUI checksums written to $CHECKSUM_FILE"
cat "$CHECKSUM_FILE"
