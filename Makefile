.PHONY: build check fmt test vet

build:
	go build ./cmd/gator

check: fmt test vet build

fmt:
	gofmt -w cmd internal

test:
	go test ./...

vet:
	go vet ./...

