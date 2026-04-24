.DEFAULT_GOAL := help

VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/oksasatya/chasago/internal/version

LDFLAGS := -s -w \
  -X $(PKG).Version=$(VERSION) \
  -X $(PKG).Commit=$(COMMIT) \
  -X $(PKG).Date=$(DATE)

## help: show targets
help:
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

## build: compile binary into ./bin/chasago with VCS info stamped
build:
	@mkdir -p bin
	@go build -ldflags "$(LDFLAGS)" -o bin/chasago ./cmd/chasago
	@echo "✓ ./bin/chasago $(VERSION) ($(COMMIT), $(DATE))"

## install: go install with ldflags (local checkout)
install:
	@go install -ldflags "$(LDFLAGS)" ./cmd/chasago
	@echo "✓ installed $(VERSION) to $$(go env GOBIN || echo $$(go env GOPATH)/bin)"

## test: run tests
test:
	@go test ./... -race -cover

## tidy: go mod tidy
tidy:
	@go mod tidy

## release: tag + push tag (usage: make release v=v0.1.3)
release:
	@[ -n "$(v)" ] || (echo "usage: make release v=v0.1.3" && exit 1)
	@git tag $(v)
	@git push origin $(v)
	@echo "✓ tagged $(v) and pushed"

.PHONY: help build install test tidy release
