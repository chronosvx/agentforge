#!/usr/bin/env bash
# 安装 agentforge 到 PREFIX/bin（默认 ~/.local/bin）
#
# 默认优先从 GitHub Release 下载预编译二进制；如果下载失败或设置
# AGENTFORGE_INSTALL_METHOD=source，则回退到本地源码构建。
#
# Release 安装默认强制 SHA256 校验：
#   - 下载与二进制同目录的 checksums.txt；
#   - checksums.txt 下载失败/缺失、其中没有对应条目、或哈希不匹配时，
#     以非零状态中止安装，不会回退到源码构建。
#
# 环境变量：
#   PREFIX                     安装前缀，默认 $HOME/.local
#   AGENTFORGE_REPO            GitHub 仓库，默认 chronosvx/agentforge
#   AGENTFORGE_VERSION         要安装的版本，默认 latest
#   AGENTFORGE_INSTALL_METHOD  source 或 release（默认 release，失败回退 source）
#   AGENTFORGE_SKIP_CHECKSUM   单独设置不再跳过校验；必须同时设置
#                              AGENTFORGE_CONFIRM_SKIP_CHECKSUM=1 才会显式跳过。
#   AGENTFORGE_CONFIRM_SKIP_CHECKSUM
#                              与 AGENTFORGE_SKIP_CHECKSUM=1 同时设置时才允许
#                              跳过 SHA256 校验（不推荐，存在安全风险）。
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

SKIP_CHECKSUM=0
if [[ "${AGENTFORGE_SKIP_CHECKSUM:-}" == "1" ]]; then
  if [[ "${AGENTFORGE_CONFIRM_SKIP_CHECKSUM:-}" == "1" ]]; then
    SKIP_CHECKSUM=1
  elif [[ "$METHOD" != "source" ]]; then
    echo "ERROR: AGENTFORGE_SKIP_CHECKSUM=1 no longer silently disables checksum verification." >&2
    echo "       To explicitly skip verification, also set AGENTFORGE_CONFIRM_SKIP_CHECKSUM=1." >&2
    exit 1
  fi
fi

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

# 从 GitHub Release 下载指定 asset。返回 0 成功，1 失败（网络/文件不存在/无下载工具）。
download_release_file() {
  local asset="$1"
  local release_ref="$2"
  local url=""

  if command -v gh >/dev/null 2>&1; then
    local gh_ref="$release_ref"
    if [[ "$gh_ref" == "latest" ]]; then
      gh_ref="$(gh release view -R "$REPO" --json tagName -q .tagName 2>/dev/null || true)"
      if [[ -z "$gh_ref" ]]; then
        echo "ERROR: unable to resolve latest release for $REPO" >&2
        return 1
      fi
    fi
    gh release download "$gh_ref" -R "$REPO" -p "$asset" -D "$TMP_DIR" --clobber || return 1
  elif command -v curl >/dev/null 2>&1; then
    url="https://github.com/${REPO}/releases/${release_ref}/download/${asset}"
    curl -fL --retry 3 -o "$TMP_DIR/$asset" "$url" || return 1
  elif command -v wget >/dev/null 2>&1; then
    url="https://github.com/${REPO}/releases/${release_ref}/download/${asset}"
    wget -O "$TMP_DIR/$asset" "$url" || return 1
  else
    echo "ERROR: neither gh, curl nor wget found" >&2
    return 1
  fi
}

# Release 安装：返回 1 表示普通下载失败（可由外层回退源码构建）；
# 返回 2 表示 checksum 校验相关失败（必须中止，不可回退）。
install_from_release() {
  local os_arch os arch exe asset
  os_arch="$(detect_os_arch)"
  os="${os_arch%_*}"
  arch="${os_arch#*_}"
  exe=""
  [[ "$os" == "windows" ]] && exe=".exe"

  asset="agentforge-${os}-${arch}${exe}"

  echo "==> downloading ${asset} from ${REPO} ${VERSION}"
  download_release_file "$asset" "$VERSION" || return 1

  if [[ "$SKIP_CHECKSUM" == "1" ]]; then
    echo "==> warning: checksum verification disabled by AGENTFORGE_SKIP_CHECKSUM=1 and AGENTFORGE_CONFIRM_SKIP_CHECKSUM=1" >&2
  else
    echo "==> downloading checksums.txt"
    if ! download_release_file "checksums.txt" "$VERSION"; then
      echo "ERROR: failed to download checksums.txt; refusing to install without checksum verification" >&2
      return 2
    fi

    local expected_hash actual_hash actual_lower expected_lower
    expected_hash="$(awk -v asset="$asset" '
      {
        file = $2
        sub(/\r$/, "", file)
        if (file == asset || file == "*" asset) {
          print $1
          exit
        }
      }
    ' "$TMP_DIR/checksums.txt")" || {
      echo "ERROR: failed to read checksums.txt" >&2
      return 2
    }

    if [[ -z "$expected_hash" ]]; then
      echo "ERROR: checksums.txt does not contain an entry for $asset" >&2
      return 2
    fi

    if command -v sha256sum >/dev/null 2>&1; then
      actual_hash="$(sha256sum "$TMP_DIR/$asset" | awk '{print $1}')" || {
        echo "ERROR: failed to compute SHA256 of $asset" >&2
        return 2
      }
    elif command -v shasum >/dev/null 2>&1; then
      actual_hash="$(shasum -a 256 "$TMP_DIR/$asset" | awk '{print $1}')" || {
        echo "ERROR: failed to compute SHA256 of $asset" >&2
        return 2
      }
    else
      echo "ERROR: neither sha256sum nor shasum found; cannot verify checksum" >&2
      return 2
    fi

    actual_lower="$(printf '%s' "$actual_hash" | tr '[:upper:]' '[:lower:]')"
    expected_lower="$(printf '%s' "$expected_hash" | tr '[:upper:]' '[:lower:]')"
    if [[ "$actual_lower" != "$expected_lower" ]]; then
      echo "ERROR: SHA256 mismatch for $asset" >&2
      echo "  expected: $expected_hash" >&2
      echo "  actual:   $actual_hash" >&2
      return 2
    fi

    echo "==> checksum verified"
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
    status=$?
    if [[ "$status" -eq 2 ]]; then
      echo "ERROR: release checksum verification failed; aborting installation" >&2
      exit 1
    fi
    echo "==> release download failed; falling back to source build"
    install_from_source
  fi
fi

echo "    请确保 $PREFIX/bin 在 PATH 中"
