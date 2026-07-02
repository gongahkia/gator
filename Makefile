BINARY := paw
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X github.com/gongahkia/paw/cmd.version=$(VERSION) -X github.com/gongahkia/paw/cmd.gitCommit=$(COMMIT)

.PHONY: build build-linux test lint fmt clean

build:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

build-linux:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS) -s -w" -o bin/paw-linux-amd64 .

test:
	go test ./... -race

lint:
	golangci-lint run

fmt:
	gofmt -w .

clean:
	rm -rf bin coverage.out dist
