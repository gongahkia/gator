.PHONY: build check fmt test vet

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(BUILD_DATE)

build:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/gator ./cmd/gator

check: fmt test vet build

fmt:
	gofmt -w cmd internal

test:
	go test ./...

vet:
	go vet ./...
