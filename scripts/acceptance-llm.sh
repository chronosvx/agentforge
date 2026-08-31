#!/usr/bin/env bash
#
# Real LLM acceptance for agentforge agent main flow.
#
# Required environment:
#   AGENTFORGE_PROVIDER       e.g. deepseek, openai, moonshot
#   AGENTFORGE_PROVIDER_KEY   API key
#   AGENTFORGE_MODEL          optional; falls back to provider default
#
# The script creates a temporary Go module + git repo, then asks the LLM to add
# a --version flag to main.go. It verifies the agent run completes and reports
# the resulting diff/status. It does NOT commit or push.
#
set -euo pipefail

if [[ -z "${AGENTFORGE_PROVIDER:-}" || -z "${AGENTFORGE_PROVIDER_KEY:-}" ]]; then
  echo "Missing LLM credentials." >&2
  echo "Set AGENTFORGE_PROVIDER and AGENTFORGE_PROVIDER_KEY, optionally AGENTFORGE_MODEL." >&2
  exit 1
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "==> Preparing temp repo: $TMP_DIR"
cd "$TMP_DIR"
go mod init accept >/dev/null 2>&1
cat > main.go <<'EOF'
package main

func main() {
	println("hello agentforge")
}
EOF
git init -q
git config user.name "acceptance"
git config user.email "acceptance@example.com"
git add .
git commit -q -m "initial"

MODEL_ARGS=()
if [[ -n "${AGENTFORGE_MODEL:-}" ]]; then
  MODEL_ARGS+=(--model "$AGENTFORGE_MODEL")
fi

echo "==> Running agent LLM acceptance..."
cd "$REPO_ROOT"
go run ./cmd/agentforge agent \
  --planner llm \
  --provider "$AGENTFORGE_PROVIDER" \
  --provider-key "$AGENTFORGE_PROVIDER_KEY" \
  "${MODEL_ARGS[@]}" \
  --repo \
  --yes \
  --workdir "$TMP_DIR" \
  --prompt "add a --version flag to main.go and keep the program compilable"

echo ""
echo "==> Resulting diff:"
cd "$TMP_DIR"
git --no-pager diff --stat
git --no-pager diff | sed -n '1,160p'

if git diff --quiet; then
  echo ""
  echo "==> Acceptance FAILED: no code changes were produced." >&2
  exit 1
fi

echo ""
echo "==> Acceptance passed: agent completed and produced code changes."
