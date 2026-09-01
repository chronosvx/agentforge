#!/usr/bin/env bash
#
# Real Git Patch-to-PR acceptance: verifies agent can edit a repo and commit.
#
# Optional:
#   AGENTFORGE_GIT_REMOTE  if set, the script also pushes and creates a PR via gh.
#
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "==> Preparing temp git repo: $TMP_DIR"
cd "$TMP_DIR"
git init -q
git config user.name "acceptance"
git config user.email "acceptance@example.com"
printf '# Accept\n\nhello\n' > README.md
git add .
git commit -q -m "initial"

BASE_BRANCH="$(git rev-parse --abbrev-ref HEAD)"

# If a remote is provided, push the initial base branch first so PR creation
# has a distinct base branch even when the remote repository is empty.
if [[ -n "${AGENTFORGE_GIT_REMOTE:-}" ]]; then
  git remote add origin "$AGENTFORGE_GIT_REMOTE"
  git push -u origin "$BASE_BRANCH"
fi

echo "==> Running agent commit flow"
cd "$REPO_ROOT"
go run ./cmd/agentforge agent \
  --prompt "edit README.md add a line: agentforge acceptance" \
  --yes \
  --test "" \
  --workdir "$TMP_DIR" \
  --commit

echo ""
echo "==> Git log:"
git -C "$TMP_DIR" --no-pager log --oneline -3
echo ""
echo "==> README.md:"
cat "$TMP_DIR/README.md"

if [[ -z "${AGENTFORGE_GIT_REMOTE:-}" ]]; then
  echo ""
  echo "==> Skipping push/PR (AGENTFORGE_GIT_REMOTE is not set)."
  echo "    To also push and create a PR, run with:"
  echo "    AGENTFORGE_GIT_REMOTE=git@github.com:you/repo.git $0"
elif [[ "$AGENTFORGE_GIT_REMOTE" != *:* && "$AGENTFORGE_GIT_REMOTE" != https://* ]]; then
  echo "ERROR: AGENTFORGE_GIT_REMOTE does not look like a git remote: '$AGENTFORGE_GIT_REMOTE'" >&2
  exit 1
else
  echo "==> Pushing and creating PR"
  cd "$TMP_DIR"
  branch="$(git rev-parse --abbrev-ref HEAD)"
  git push -u origin "$branch"
  gh pr create --title "agentflow: acceptance" --body "Automated acceptance PR" --head "$branch" --base "$BASE_BRANCH"
fi

echo ""
echo "==> Acceptance passed: agent edited and committed successfully."
