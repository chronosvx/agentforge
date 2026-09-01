#!/usr/bin/env bash
# 安装 agentforge 到 PREFIX/bin（默认 ~/.local/bin）
#
# 默认优先从 GitHub Release 下载预编译二进制；如果下载失败或设置
# AGENTFORGE_INSTALL_METHOD=source，则回退到本地源码构建。
#
# 环境变量：
#   PREFIX                     安装前缀，默认 $HOME/.local
#   AGENTFORGE_REPO            GitHub 仓库，默认 chronosvx/agentforge
#   AGENTFORGE_VERSION         要安装的版本，默认 latest
#   AGENTFORGE_INSTALL_METHOD  source 或 release（默认 release，失败回退 source）
#   AGENTFORGE_SKIP_CHECKSUM   设为 1 时跳过 checksum 校验
set -euo pipefail

REPO="${AGENTFORGE_REPO:-chronosvx/agentforge}"
VERSION="${AGENTFORGE_VERSION:-latest}"
PREFIX="${PREFIX:-$HOME/.local}"
METHOD="${AGENTFORGE_INSTALL_METHOD:-release}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

mkdir -p "$PREFIX/bin"
BIN_DIR="$PREFIX/bin"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

detect_os_arch() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    darwin) os="darwin" ;;
    linux) os="linux" ;;
    mingw*|msys*|cygwin*) os="windows" ;;
  esac

  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
  esac

  echo "${os}_${arch}"
}

install_from_source() {
  echo "==> building agentforge from source"
  cd "$ROOT"
  go build -o "$BIN_DIR/agentforge" ./cmd/agentforge
  echo "==> installed to $BIN_DIR/agentforge"
}

install_from_release() {
  local os_arch
  os_arch="$(detect_os_arch)"
  local os="${os_arch%_*}"
  local arch="${os_arch#*_}"
  local exe=""
  [[ "$os" == "windows" ]] && exe=".exe"

  local asset="agentforge-${os}-${arch}${exe}"
  local download_version="$VERSION"

  echo "==> downloading ${asset} from ${REPO} ${VERSION}"
  if command -v gh >/dev/null 2>&1; then
    if [[ "$download_version" == "latest" ]]; then
      download_version="$(gh release view -R "$REPO" --json tagName -q .tagName 2>/dev/null)"
    fi
    if [[ -z "$download_version" ]]; then
      echo "ERROR: unable to resolve latest release for $REPO" >&2
      return 1
    fi
    gh release download "$download_version" -R "$REPO" -p "$asset" -D "$TMP_DIR" --clobber || return 1
  elif command -v curl >/dev/null 2>&1; then
    local url="https://github.com/${REPO}/releases/${download_version}/download/${asset}"
    curl -fL --retry 3 -o "$TMP_DIR/$asset" "$url" || return 1
  elif command -v wget >/dev/null 2>&1; then
    local url="https://github.com/${REPO}/releases/${download_version}/download/${asset}"
    wget -O "$TMP_DIR/$asset" "$url" || return 1
  else
    echo "ERROR: neither gh, curl nor wget found" >&2
    return 1
  fi

  if [[ "${AGENTFORGE_SKIP_CHECKSUM:-}" != "1" ]]; then
    local checksum_ok=1
    if command -v gh >/dev/null 2>&1 && gh release download "$download_version" -R "$REPO" -p checksums.txt -D "$TMP_DIR" --clobber >/dev/null 2>&1; then
      checksum_ok=0
    elif command -v curl >/dev/null 2>&1 && curl -fsSL --retry 2 -o "$TMP_DIR/checksums.txt" "https://github.com/${REPO}/releases/${download_version}/download/checksums.txt" 2>/dev/null; then
      checksum_ok=0
    fi
    if [[ "$checksum_ok" == "0" ]]; then
      (cd "$TMP_DIR" && shasum -a 256 -c <(grep "$asset" checksums.txt) >/dev/null 2>&1) || {
        echo "ERROR: checksum verification failed for $asset" >&2
        return 1
      }
      echo "==> checksum verified"
    else
      echo "==> warning: checksums.txt not found; skipping verification"
    fi
  fi

  install -m 0755 "$TMP_DIR/$asset" "$BIN_DIR/agentforge${exe}"
  echo "==> installed to $BIN_DIR/agentforge${exe}"
}

if [[ "$METHOD" == "source" ]]; then
  install_from_source
else
  if install_from_release; then
    :
  else
    echo "==> release download failed; falling back to source build"
    install_from_source
  fi
fi

echo "    请确保 $PREFIX/bin 在 PATH 中"
