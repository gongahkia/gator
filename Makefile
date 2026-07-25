.PHONY: test test-e2e

test:
	go test -race ./...
	go vet ./...

test-e2e:
	./scripts/test-e2e.sh
