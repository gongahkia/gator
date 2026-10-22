.PHONY: build runtime desktop package dist check fmt test vet

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
export VERSION COMMIT BUILD_DATE
LDFLAGS = -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(BUILD_DATE)

# The application is the product. Its Go executable is a private bundled runtime.
build:
	npm run build --prefix desktop

runtime:
	npm run build:runtime --prefix desktop

desktop:
	npm start --prefix desktop

package:
	npm run pack --prefix desktop

dist:
	npm run dist --prefix desktop

check: fmt test vet build
	npm run check --prefix desktop
	npm test --prefix desktop

fmt:
	gofmt -w cmd internal

test:
	go test ./...

vet:
	go vet ./...
