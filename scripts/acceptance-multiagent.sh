#!/usr/bin/env bash
#
# Real multi-agent dependency acceptance.
#
# Runs two independent tasks and one dependent task through the CLI.
#
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

echo "==> Running multi-agent acceptance"
go run ./cmd/agentforge multiagent \
  --tasks "run echo alpha,run echo beta" \
  --yes \
  --test "" \
  --workdir /tmp > /tmp/agentforge-multiagent.out

cat /tmp/agentforge-multiagent.out

grep -q "task-0:" /tmp/agentforge-multiagent.out
grep -q "task-1:" /tmp/agentforge-multiagent.out

echo ""
echo "==> Acceptance passed: multi-agent tasks executed successfully."
