#!/usr/bin/env bash
# 构建 macOS 桌面 GUI（Flutter）并打包到 dist/。
# 仅用于发布准备；需要本机已安装 Flutter SDK。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if ! command -v flutter >/dev/null 2>&1; then
  echo "ERROR: flutter not found on PATH." >&2
  echo "Install the Flutter SDK first, then add 'flutter' to your PATH." >&2
  echo "Example: export PATH=\"\$PATH:\$HOME/flutter/bin\"" >&2
  exit 1
fi

if ! xcodebuild -version >/dev/null 2>&1; then
  echo "ERROR: full Xcode is required to build a macOS Flutter app." >&2
  echo "Current active developer directory: $(xcode-select -p 2>/dev/null || true)" >&2
  echo "Install Xcode from the App Store, then run:" >&2
  echo "  sudo xcode-select --switch /Applications/Xcode.app/Contents/Developer" >&2
  echo "Alternatively, use the manual GitHub Actions workflow: .github/workflows/gui-macos.yml" >&2
  exit 1
fi

echo "==> fetching Flutter dependencies"
(cd gui && flutter pub get)

echo "==> building macOS release app"
(cd gui && flutter build macos --release)

APP_SRC="gui/build/macos/Build/Products/Release/agentforge_gui.app"
if [[ ! -d "$APP_SRC" ]]; then
  echo "ERROR: built app not found at $APP_SRC" >&2
  exit 1
fi

mkdir -p dist
APP_DEST="dist/agentforge_gui.app"
echo "==> copying app to $APP_DEST"
rm -rf "$APP_DEST"
cp -R "$APP_SRC" "$APP_DEST"

VERSION="$(sed -n 's/^version:[[:space:]]*//p' gui/pubspec.yaml | head -n 1 | tr -d '[:space:]')"
if [[ -z "$VERSION" ]]; then
  echo "WARNING: could not read version from gui/pubspec.yaml; using 0.0.0" >&2
  VERSION="0.0.0"
fi

DMG="dist/agentforge_gui-${VERSION}.dmg"

if command -v hdiutil >/dev/null 2>&1; then
  STAGING="$(mktemp -d "${TMPDIR:-/tmp}/agentforge_gui.XXXXXX")"
  trap 'rm -rf "$STAGING"' EXIT
  echo "==> creating DMG with hdiutil"
  cp -R "$APP_DEST" "$STAGING/"
  ln -s /Applications "$STAGING/Applications"
  hdiutil create \
    -volname "AgentForge GUI ${VERSION}" \
    -srcfolder "$STAGING" \
    -ov \
    -format UDZO \
    "$DMG" >/dev/null
else
  echo "WARNING: hdiutil not found; skipping DMG creation" >&2
  DMG=""
fi

echo
echo "==> output:"
echo "  App: $APP_DEST"
if [[ -n "${DMG:-}" ]]; then
  echo "  DMG: $DMG"
fi
