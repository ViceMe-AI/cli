GO ?= go
GOPATH ?= $(CURDIR)/.cache/go
GOCACHE ?= $(CURDIR)/.cache/go-build
GOMODCACHE ?= $(GOPATH)/pkg/mod
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X github.com/ViceMe-AI/cli/internal/buildinfo.Version=$(VERSION) -X github.com/ViceMe-AI/cli/internal/buildinfo.Commit=$(COMMIT)

.PHONY: build test test-race check skill-check clean update-check

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

# Reserved for the npm/Homebrew release shell. It intentionally performs no
# mutation until signed release manifests and checksums are implemented.
update-check:
	@echo "self-update distribution is not implemented in the MVP"

clean:
	rm -rf bin .cache
