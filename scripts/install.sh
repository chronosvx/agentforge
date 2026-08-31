#!/usr/bin/env bash
# 本地安装 agentforge 到 PREFIX/bin（默认 ~/.local/bin）
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PREFIX="${PREFIX:-$HOME/.local}"

mkdir -p "$PREFIX/bin"
echo "==> building agentforge"
cd "$ROOT"
go build -o "$PREFIX/bin/agentforge" ./cmd/agentforge

echo "==> installed to $PREFIX/bin/agentforge"
echo "    请确保 $PREFIX/bin 在 PATH 中"
