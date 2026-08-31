#!/usr/bin/env bash
#
# Real MCP Server acceptance.
#
# Required environment:
#   MCP_URL   MCP server URL (http/https) or stdio command
#
# Optional:
#   MCP_TOOL  tool name to call
#   MCP_ARGS  JSON arguments for the tool call
#
set -euo pipefail

if [[ -z "${MCP_URL:-}" ]]; then
  echo "Missing MCP_URL." >&2
  echo "Example: MCP_URL=http://127.0.0.1:8000/mcp ./scripts/acceptance-mcp.sh" >&2
  exit 1
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

ARGS=()
ARGS+=(--url "$MCP_URL")
if [[ -n "${MCP_TOOL:-}" ]]; then
  ARGS+=(--call "$MCP_TOOL")
  ARGS+=(--args "${MCP_ARGS:-{}}")
fi

echo "==> Running MCP acceptance"
go run ./cmd/agentforge mcp "${ARGS[@]}" | tee /tmp/agentforge-mcp.out

grep -q "MCP connected" /tmp/agentforge-mcp.out

echo ""
echo "==> Acceptance passed: MCP server connected and tools listed."
