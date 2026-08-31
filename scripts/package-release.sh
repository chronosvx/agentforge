#!/usr/bin/env bash
# 将 dist/ 下已构建的多平台二进制打包到 dist/packages/。
#
# 产物：
#   Linux:   dist/packages/agentforge-linux-<arch>.tar.gz
#   macOS:   dist/packages/agentforge-darwin-<arch>.dmg
#            （本机没有 hdiutil 时回退为 .zip）
#   Windows: dist/packages/agentforge-windows-<arch>.zip
#
# 用法：
#   make release          # 先生成 dist/ 下的二进制
#   ./scripts/package-release.sh
#   或
#   make package
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DIST_DIR="$ROOT/dist"
PACKAGES_DIR="$DIST_DIR/packages"
STAGING_DIR="$PACKAGES_DIR/.staging"
mkdir -p "$PACKAGES_DIR"
trap 'rm -rf "$STAGING_DIR"' EXIT

packaged=0

# 创建 zip：优先使用系统 zip，缺失时回退 python3。
make_zip() {
  local archive="$1"
  local dir="$2"
  local target="$3"

  if command -v zip >/dev/null 2>&1; then
    (cd "$dir" && zip -qr "$archive" "$target")
  elif command -v python3 >/dev/null 2>&1; then
    (cd "$dir" && python3 -m zipfile -c "$archive" "$target")
  else
    echo "ERROR: neither 'zip' nor 'python3' found; cannot create $archive" >&2
    return 1
  fi
}

# 准备统一目录结构：agentforge-<os>-<arch>/agentforge[.exe]
prepare_staging() {
  local os="$1"
  local arch="$2"
  local base="agentforge-${os}-${arch}"
  local bin="$DIST_DIR/${base}"
  [[ "$os" == "windows" ]] && bin+=".exe"

  local stage_dir="$STAGING_DIR/$base"
  rm -rf "$STAGING_DIR"
  mkdir -p "$stage_dir"

  local target_name="agentforge"
  [[ "$os" == "windows" ]] && target_name="agentforge.exe"
  cp "$bin" "$stage_dir/$target_name"

  # 附带基础文档（存在时），便于分发。
  cp "$ROOT/README.md" "$stage_dir/README.md" 2>/dev/null || true
  cp "$ROOT/LICENSE" "$stage_dir/LICENSE" 2>/dev/null || true
}

for os in darwin linux windows; do
  for arch in amd64 arm64; do
    base="agentforge-${os}-${arch}"
    bin="$DIST_DIR/${base}"
    [[ "$os" == "windows" ]] && bin+=".exe"

    if [[ ! -f "$bin" ]]; then
      echo "==> skip missing $bin"
      continue
    fi

    case "$os" in
      linux)
        archive="$PACKAGES_DIR/${base}.tar.gz"
        echo "==> packaging ${base}.tar.gz"
        prepare_staging "$os" "$arch"
        tar -czf "$archive" -C "$STAGING_DIR" "$base"
        ;;

      darwin)
        if command -v hdiutil >/dev/null 2>&1; then
          archive="$PACKAGES_DIR/${base}.dmg"
          echo "==> packaging ${base}.dmg (hdiutil)"
          prepare_staging "$os" "$arch"
          hdiutil create \
            -volname "$base" \
            -srcfolder "$STAGING_DIR/$base" \
            -ov \
            -format UDZO \
            "$archive" >/dev/null
        else
          archive="$PACKAGES_DIR/${base}.zip"
          echo "==> hdiutil not found, packaging ${base}.zip"
          prepare_staging "$os" "$arch"
          make_zip "$archive" "$STAGING_DIR" "$base"
        fi
        ;;

      windows)
        archive="$PACKAGES_DIR/${base}.zip"
        echo "==> packaging ${base}.zip"
        prepare_staging "$os" "$arch"
        make_zip "$archive" "$STAGING_DIR" "$base"
        ;;
    esac

    packaged=$((packaged + 1))
  done
done

if (( packaged == 0 )); then
  echo "==> no binaries found in dist/; run 'make release' first" >&2
  exit 1
fi

echo "==> packages in $PACKAGES_DIR/"
ls -lh "$PACKAGES_DIR"
