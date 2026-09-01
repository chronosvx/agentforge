.PHONY: run build test tidy install install-local smoke check-release release package publish-plugin fuzz security-scan sbom acceptance

run:
	go run ./cmd/agentforge

build:
	go build -o bin/agentforge ./cmd/agentforge

install:
	go install ./cmd/agentforge

install-local:
	./scripts/install.sh

test:
	go test ./...

tidy:
	go mod tidy

smoke:
	./scripts/smoke-real.sh

acceptance:
	./scripts/acceptance-multiagent.sh
	./scripts/acceptance-git.sh
	@if [ -n "$$AGENTFORGE_PROVIDER" ]; then ./scripts/acceptance-llm.sh; fi
	@if [ -n "$$MCP_URL" ]; then ./scripts/acceptance-mcp.sh; fi

check-release:
	./scripts/check-release.sh

release:
	./scripts/build-release.sh

package:
	./scripts/package-release.sh

publish-plugin:
	./scripts/publish-plugin.sh "$(NAME)" "$(VERSION)" "$(URL)" "$(KEY)" "$(INDEX)"

fuzz:
	for pkg in ./internal/provider ./internal/search ./internal/session; do \
		go test -fuzz=Fuzz -fuzztime=5s "$$pkg" || exit 1; \
	done

security-scan:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

sbom:
	mkdir -p dist
	go list -m -json all > dist/sbom.json
