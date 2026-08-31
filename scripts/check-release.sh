#!/usr/bin/env bash
# 发布前检查脚本：
#   1. go test ./...
#   2. go vet ./...
#   3. go build ./...
#   4. make release
#
# 任一检查失败时以非零状态退出，并汇总输出结果。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

LOG_FILE="$(mktemp "${TMPDIR:-/tmp}/dsh-check-release.XXXXXX")"
trap 'rm -f "$LOG_FILE"' EXIT

passed=0
failed=0

run_check() {
  local name="$1"
  shift

  echo "==> $name"
  if "$@" >"$LOG_FILE" 2>&1; then
    echo "    ok: $name"
    passed=$((passed + 1))
  else
    echo "    FAIL: $name"
    echo "    ---- output (tail) ----"
    tail -n 40 "$LOG_FILE" | sed 's/^/    /'
    echo "    -----------------------"
    failed=$((failed + 1))
  fi
}

run_check "go test ./..." go test ./...
run_check "go vet ./..." go vet ./...
run_check "go build ./..." go build ./...
run_check "make release" make release

echo
if (( failed > 0 )); then
  echo "==> check-release: FAILED ($passed passed, $failed failed)"
  exit 1
fi

echo "==> check-release: PASSED ($passed checks passed)"
exit 0
