VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/chenhg5/myco/internal/cli.Version=$(VERSION)

.PHONY: build test test-integration smoke check vet cross clean web app

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
	@for os in linux darwin windows; do for arch in amd64 arm64; do \
		ext=; [ $$os = windows ] && ext=.exe; \
		echo "building $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags '$(LDFLAGS)' -o bin/myco-$$os-$$arch$$ext ./cmd/myco || exit 1; \
	done; done

clean:
	rm -rf bin

# Rebuild the embedded dashboard (needs Node 18+); output is committed in
# internal/web/dist so Go builds never need Node.
web:
	cd web && npm ci && npm run build

# The console app (macOS only: the window links the system WebKit through cgo).
# Produces bin/myco.app; signs it when MYCO_SIGN_IDENTITY names a Developer ID
# Application identity in the keychain, otherwise signs it ad hoc.
APP_VERSION := $(patsubst v%,%,$(firstword $(subst -, ,$(VERSION))))
app:
	@test "$$(uname)" = Darwin || { echo "make app needs macOS (the release workflow builds it on a macOS runner)"; exit 1; }
	CGO_ENABLED=1 CGO_CFLAGS=-mmacosx-version-min=12.0 CGO_LDFLAGS=-mmacosx-version-min=12.0 \
		go build -tags production -ldflags '-s -w -X main.version=$(VERSION)' -o bin/myco-app ./cmd/myco-app
	rm -rf bin/myco.app bin/myco.iconset
	mkdir -p bin/myco.app/Contents/MacOS bin/myco.app/Contents/Resources bin/myco.iconset
	cp bin/myco-app bin/myco.app/Contents/MacOS/myco-app
	# The app ships the myco CLI and installs it into ~/.local/bin on first run.
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o bin/myco.app/Contents/Resources/myco ./cmd/myco
	sed 's/@VERSION@/$(APP_VERSION)/g' cmd/myco-app/build/darwin/Info.plist > bin/myco.app/Contents/Info.plist
	for s in 16 32 128 256 512; do \
		sips -z $$s $$s cmd/myco-app/build/darwin/icon-1024.png --out bin/myco.iconset/icon_$${s}x$${s}.png >/dev/null; \
		d=$$((s*2)); sips -z $$d $$d cmd/myco-app/build/darwin/icon-1024.png --out bin/myco.iconset/icon_$${s}x$${s}@2x.png >/dev/null; \
	done
	iconutil -c icns bin/myco.iconset -o bin/myco.app/Contents/Resources/myco.icns
	rm -rf bin/myco.iconset
	if [ -n "$$MYCO_SIGN_IDENTITY" ]; then \
		codesign --force --options runtime --timestamp --sign "$$MYCO_SIGN_IDENTITY" bin/myco.app/Contents/Resources/myco && \
		codesign --force --options runtime --timestamp --sign "$$MYCO_SIGN_IDENTITY" bin/myco.app; \
	else \
		codesign --force --sign - bin/myco.app/Contents/Resources/myco && \
		codesign --force --sign - bin/myco.app; \
	fi
	codesign --verify --strict bin/myco.app && echo "built bin/myco.app ($(APP_VERSION))"
