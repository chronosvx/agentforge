# agentforge Release Checklist

This document is the production-readiness checklist for agentforge.

## P0: Out-of-box experience

- [x] `agentforge doctor` checks environment
- [x] `agentforge init` writes `~/.agentforge/config.json`
- [x] `agentforge config` shows effective config with masked secrets
- [x] `agentforge --help` lists all subcommands
- [x] Friendly message when no backend is configured
- [x] Release `v0.3.0` published with multi-platform binaries
- [x] `scripts/install.sh` supports GitHub Release download + checksum
- [x] `scripts/install.ps1` supports Windows release download

## P1: Reliability

- [x] `agentforge backup` / `agentforge restore`
- [x] TUI status bar shows new version when available
- [x] CI runs tests with race detector on macOS / Linux / Windows
- [x] CI runs `go vet`, `govulncheck`, build, and Linux CLI smoke tests
- [x] Release workflow builds and publishes artifacts on tags

## P2: Real-world acceptance

### Tooling (done)
- [x] `agentforge mcp --url ...` CLI for MCP Server verification
- [x] `agentforge agent --json` / `--dry-run`
- [x] `agentforge multiagent --json` / `--dry-run`
- [x] `scripts/acceptance-llm.sh`
- [x] `scripts/acceptance-git.sh`
- [x] `scripts/acceptance-multiagent.sh`
- [x] `scripts/acceptance-mcp.sh`
- [x] `make acceptance`

### Remaining real-environment checks
- [ ] Real MCP Server:
  ```bash
  MCP_URL=http://127.0.0.1:8000/mcp ./scripts/acceptance-mcp.sh
  ```
- [ ] Real Git Patch-to-PR:
  ```bash
  AGENTFORGE_GIT_REMOTE=git@github.com:you/repo.git ./scripts/acceptance-git.sh
  ```
- [ ] Real multi-agent dependency run:
  ```bash
  agentforge multiagent --tasks "run echo a,run echo b" --yes
  ```
- [ ] TUI manual test:
  ```bash
  agentforge --mock
  ```
- [ ] Linux libsecret native backend:
  ```bash
  sudo apt install libsecret-1-dev libglib2.0-dev
  go test -tags libsecret ./internal/secret/...
  ```
