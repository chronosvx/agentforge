#!/usr/bin/env bash
# 签名并发布一个插件市场条目。
#
# 用法：
#   ./scripts/publish-plugin.sh <插件名> <版本> <包URL> <私钥PEM路径> <市场索引JSON路径> [描述]
#
# 包 URL 支持 http(s)://、file:// 或本地文件路径。
# 脚本会调用 cmd/agentforge-plugin-publish 计算 SHA-256、生成 ECDSA P-256 签名并更新市场索引。
set -euo pipefail

if [[ $# -lt 5 || $# -gt 6 ]]; then
  echo "用法: $0 <插件名> <版本> <包URL> <私钥PEM路径> <市场索引JSON路径> [描述]" >&2
  exit 1
fi

NAME="$1"
VERSION="$2"
URL="$3"
KEY_PATH="$4"
INDEX_PATH="$5"
DESCRIPTION="${6:-$NAME}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ ! -f "$KEY_PATH" ]]; then
  echo "错误: 找不到私钥文件: $KEY_PATH" >&2
  exit 1
fi

echo "==> 发布插件: $NAME@$VERSION"
echo "    包 URL:  $URL"
echo "    索引:    $INDEX_PATH"

go run ./cmd/agentforge-plugin-publish \
  "$NAME" \
  "$VERSION" \
  "$URL" \
  "$KEY_PATH" \
  "$INDEX_PATH" \
  "$DESCRIPTION"
