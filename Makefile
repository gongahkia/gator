.PHONY: build check fmt test vet

build:
	mkdir -p bin
	go build -o bin/gator ./cmd/gator

check: fmt test vet build

fmt:
	gofmt -w cmd internal

test:
	go test ./...

vet:
	go vet ./...
