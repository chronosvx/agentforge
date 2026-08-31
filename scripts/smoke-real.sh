#!/usr/bin/env bash
# 真实 DSH Web API 全流程验收脚本：
#   1. 启动 dsh --profile web --no-open --port 0
#   2. 解析输出的端口
#   3. 第一次运行 agentforge-smoke：创建会话 -> 发送 prompt -> 观察 mux -> 拉取 History
#   4. 第二次运行 agentforge-smoke --resume：复用同一会话再发一条 -> 观察 mux -> 拉取 History
#   5. 输出“全流程验收通过”或明确失败原因
#   6. 关闭 web server
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LOG="$(mktemp -t dsh-web-smoke.XXXXXX.log)"
PID=""

cleanup() {
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  rm -f "$LOG"
}
trap cleanup EXIT

if ! command -v dsh >/dev/null 2>&1; then
  echo "验收失败：未找到 dsh 命令，请先安装/配置 dsh 并确保其在 PATH 中" >&2
  exit 1
fi

echo "==> 启动 dsh web ..."
dsh --profile web --no-open --port 0 >"$LOG" 2>&1 &
PID=$!

for _ in $(seq 1 30); do
  if grep -q "dsh web: http" "$LOG"; then
    break
  fi
  sleep 0.3
done

URL_LINE="$(grep "dsh web: http" "$LOG" | head -1)"
if [[ -z "$URL_LINE" ]]; then
  if kill -0 "$PID" 2>/dev/null; then
    echo "dsh web 启动超时（9 秒内未输出地址），日志如下：" >&2
  else
    echo "dsh web 进程提前退出，日志如下：" >&2
  fi
  cat "$LOG" >&2
  exit 1
fi

URL="${URL_LINE##* }"
echo "==> DSH Web: $URL"

# 运行一次 agentforge-smoke。成功时把程序输出打到 stdout 并原样返回给调用方；
# 失败时打印完整输出和明确失败原因后退出。
run_smoke() {
  local desc="$1"
  shift
  echo "==> $desc ..." >&2
  local out
  if ! out="$(go run ./cmd/agentforge-smoke "$@" 2>&1)"; then
    echo "$out" >&2
    echo "验收失败：$desc 失败" >&2
    exit 1
  fi
  printf '%s\n' "$out"
}

cd "$ROOT"

FIRST_OUT="$(run_smoke "第一次运行 agentforge-smoke（创建会话）" \
  --url "$URL" --prompt "回复OK" --watch 5s)"
echo "$FIRST_OUT"

SESSION_ID="$(printf '%s\n' "$FIRST_OUT" | sed -n 's/^session\.id=//p' | tail -n 1)"
if [[ -z "$SESSION_ID" ]]; then
  echo "验收失败：未能从 agentforge-smoke 输出中解析 session.id" >&2
  exit 1
fi
echo "==> 首次运行得到 session.id: $SESSION_ID"

SECOND_OUT="$(run_smoke "第二次运行 agentforge-smoke（--resume 复用 ${SESSION_ID}）" \
  --url "$URL" --resume "$SESSION_ID" --prompt "再回复一次" --watch 5s)"
echo "$SECOND_OUT"

if ! grep -q "resume:.*$SESSION_ID" <<<"$SECOND_OUT"; then
  echo "验收失败：第二次运行未按预期复用会话 $SESSION_ID" >&2
  exit 1
fi
if ! grep -q "^history:" <<<"$SECOND_OUT"; then
  echo "验收失败：第二次运行未成功拉取 History" >&2
  exit 1
fi

echo "==> 全流程验收通过"
