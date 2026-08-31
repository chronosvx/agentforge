#!/usr/bin/env bash
# 构建多平台发布二进制到 dist/
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

mkdir -p dist

for os in darwin linux windows; do
  for arch in amd64 arm64; do
    out="dist/agentforge-${os}-${arch}"
    if [[ "$os" == "windows" ]]; then
      out+=".exe"
    fi
    echo "==> building ${os}/${arch} -> ${out}"
    GOOS="$os" GOARCH="$arch" go build -o "$out" ./cmd/agentforge
  done
done

echo "==> generating checksums"
(cd dist && shasum -a 256 agentforge-* > checksums.txt)

echo "==> release binaries in dist/"
ls -lh dist/
cat dist/checksums.txt
