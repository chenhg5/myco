VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/chenhg5/myco/internal/cli.Version=$(VERSION)

.PHONY: build test vet cross clean web

build:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o bin/myco ./cmd/myco

test:
	go test -race ./...

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
