VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/chenhg5/myco/internal/cli.Version=$(VERSION)

.PHONY: build test test-integration smoke check vet cross clean web

build:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o bin/myco ./cmd/myco

test:
	go test -race ./...

# Hub + HTTP + client + CLI/runner wired together in one process (also part of `make test`).
test-integration:
	go test -race -count=1 ./internal/integration/

# Build the real binary and drive it end to end as separate processes (scripts/smoke.sh).
smoke: build
	MYCO_BIN=$(CURDIR)/bin/myco scripts/smoke.sh

# Everything CI runs for Go: formatting, vet, unit + integration tests, smoke test.
check: vet test smoke

vet:
	gofmt -l . | (! grep .) && go vet ./...

cross:
	@for os in linux darwin; do for arch in amd64 arm64; do \
		echo "building $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags '$(LDFLAGS)' -o bin/myco-$$os-$$arch ./cmd/myco || exit 1; \
	done; done

clean:
	rm -rf bin

# Rebuild the embedded dashboard (needs Node 18+); output is committed in
# internal/web/dist so Go builds never need Node.
web:
	cd web && npm ci && npm run build
