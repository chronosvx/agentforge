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

### Real-environment checks
- [x] Local MCP Server (temporary HTTP JSON-RPC server on 127.0.0.1:18000):
  ```bash
  MCP_URL=http://127.0.0.1:18000 ./scripts/acceptance-mcp.sh
  ```
- [x] Real Git Patch-to-PR (temporary private GitHub repo, PR created and repo deleted):
  ```bash
  AGENTFORGE_GIT_REMOTE=git@github.com:you/repo.git ./scripts/acceptance-git.sh
  ```
- [x] Real multi-agent execution:
  ```bash
  go run ./cmd/agentforge multiagent --tasks "run echo a,run echo b" --yes
  ```
- [ ] TUI manual test (interactive; requires human):
  ```bash
  agentforge --mock
  ```

### Deferred / skipped
- [ ] Linux libsecret native backend: not included. Linux Keychain uses `secret-tool` CLI fallback. Experimental CGo backend was removed because it did not compile reliably against libsecret.

## Preview: macOS GUI release

- [x] `make release-gui` builds `dist/agentforge_gui.app` and `dist/agentforge_gui-<version>.dmg`, then writes `dist/gui-checksums.txt`.
- [x] GitHub Actions workflow `gui-macos.yml` can be triggered manually (`workflow_dispatch`) to build and upload GUI artifacts.
- [ ] The macOS GUI is still a preview; it is not yet part of the normal tag release automation.
- [ ] Local GUI build requires **full Xcode** (not just Command Line Tools): `sudo xcode-select --switch /Applications/Xcode.app/Contents/Developer`. Without full Xcode, use the manual `gui-macos.yml` workflow on GitHub Actions.
