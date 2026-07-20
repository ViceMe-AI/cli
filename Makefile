GO ?= go
GOPATH ?= $(CURDIR)/.cache/go
GOCACHE ?= $(CURDIR)/.cache/go-build
GOMODCACHE ?= $(GOPATH)/pkg/mod
VERSION ?= dev
NPM_VERSION = $(shell node -p "require('./package.json').version")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X github.com/ViceMe-AI/cli/internal/buildinfo.Version=$(VERSION) -X github.com/ViceMe-AI/cli/internal/buildinfo.Commit=$(COMMIT)

.PHONY: build test test-race check skill-check quality-check npm-test npm-package-check command-manifest release-manifest release-prepare clean update-check

build:
	mkdir -p bin
	GOPATH=$(GOPATH) GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/viceme ./cmd/viceme

test:
	GOPATH=$(GOPATH) GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) test ./...

test-race:
	GOPATH=$(GOPATH) GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) test -race ./...

skill-check:
	GOPATH=$(GOPATH) GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) test ./internal/skillcontent ./internal/command -run 'Skill|CommandExamples'

check: test
	GOPATH=$(GOPATH) GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) vet ./...
	mkdir -p bin
	GOPATH=$(GOPATH) GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/viceme ./cmd/viceme

quality-check: test npm-package-check

npm-test:
	NPM_CONFIG_CACHE=$(CURDIR)/.cache/npm npm test

npm-package-check: build
	mkdir -p .cache/npm-pack
	GOPATH=$(GOPATH) GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) build -trimpath -ldflags "-X github.com/ViceMe-AI/cli/internal/buildinfo.Version=$(NPM_VERSION) -X github.com/ViceMe-AI/cli/internal/buildinfo.Commit=$(COMMIT)" -o bin/viceme-release-smoke ./cmd/viceme
	NPM_CONFIG_CACHE=$(CURDIR)/.cache/npm npm pack --pack-destination .cache/npm-pack
	VICEME_INSTALL_METHOD=npm VICEME_NPM_PACKAGE_VERSION=$(NPM_VERSION) ./bin/viceme-release-smoke --version
	NPM_CONFIG_CACHE=$(CURDIR)/.cache/npm VICEME_TEST_BINARY=$(CURDIR)/bin/viceme-release-smoke VICEME_TEST_PACKAGE_TARBALL=$(CURDIR)/.cache/npm-pack/viceme-ai-cli-$(NPM_VERSION).tgz npm test
	NPM_CONFIG_CACHE=$(CURDIR)/.cache/npm npm pack --dry-run

command-manifest:
	GOPATH=$(GOPATH) GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) run ./cmd/command-manifest --output skills/viceme/references/command-manifest.json

release-manifest:
	GOPATH=$(GOPATH) GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) run ./cmd/release-manifest --output quality/release-manifest.json

release-prepare:
	node npm/scripts/prepare-release.mjs --fallback-ref origin/main
	NPM_CONFIG_CACHE=$(CURDIR)/.cache/npm npm install --package-lock-only --ignore-scripts --no-audit --no-fund
	gofmt -w internal/buildinfo/buildinfo.go
	$(MAKE) command-manifest
	$(MAKE) release-manifest

update-check: build
	./bin/viceme update --check

clean:
	rm -rf bin .cache
