.PHONY: build setup runtime run desktop package dist check fmt test vet help terminal terminal-setup terminal-build terminal-check

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
export VERSION COMMIT BUILD_DATE

# The application is the product. Its Go executable is a private bundled runtime.
build: setup
	npm run build --prefix desktop

# Reinstall pinned dependencies after a fresh checkout or a manifest change.
setup: app-host/node_modules/.package-lock.json desktop/node_modules/.package-lock.json

app-host/node_modules/.package-lock.json: app-host/package.json app-host/package-lock.json
	npm ci --prefix app-host
	@touch $@

desktop/node_modules/.package-lock.json: desktop/package.json desktop/package-lock.json app-host/node_modules/.package-lock.json
	npm ci --prefix desktop
	@touch $@

runtime:
	npm run build:runtime --prefix desktop

run: desktop

desktop: setup
	npm start --prefix desktop

# Migration entry points; Electron remains the reference until the parity gate.
terminal-setup:
	npm ci --prefix app-host
	bun install --cwd ui --frozen-lockfile

terminal-build: terminal-setup
	node ui/scripts/build-runtime.mjs
	bun run --cwd ui build

terminal: terminal-build
	bun run --cwd ui start

terminal-check: terminal-build
	npm test --prefix app-host
	bun run --cwd ui check
	GATOR_TEST_BINARY="$(CURDIR)/ui/runtime/gator-runtime" bun run --cwd ui test

package: setup
	npm run pack --prefix desktop

dist: setup
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

help:
	@printf '%s\n' \
	  'make run      Build and launch Gator (same as make desktop)' \
	  'make build    Build the private Go runtime and frontend' \
	  'make setup    Install pinned desktop dependencies if needed' \
	  'make package  Build an unpacked app in desktop/release/' \
	  'make dist     Build installers in desktop/release/' \
	  'make check    Format Go code, run tests, vet, typecheck and build'
