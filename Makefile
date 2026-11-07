.PHONY: build setup runtime run terminal terminal-setup terminal-build terminal-check package dist install check fmt test vet help

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PACKAGE_DIR ?= $(CURDIR)/ui/release/gator-terminal
PREFIX ?= $(HOME)/.local
export VERSION COMMIT BUILD_DATE

# One terminal product: OpenTUI chat with the current Go agent runtime.
build: setup runtime
	cd ui && bun run build

setup: app-host/node_modules/.package-lock.json ui/node_modules/.gator-ready

app-host/node_modules/.package-lock.json: app-host/package.json app-host/package-lock.json
	npm ci --prefix app-host
	@touch $@

ui/node_modules/.gator-ready: ui/package.json ui/bun.lock app-host/package.json
	cd ui && bun install --frozen-lockfile
	@touch $@

runtime:
	node ui/scripts/build-runtime.mjs

run: build
	bun ui/launcher/gator.mjs

terminal: run
terminal-setup: setup
terminal-build: build
terminal-check: check

package: build
	node ui/scripts/package.mjs "$(PACKAGE_DIR)"

dist: package

install: package
	node ui/scripts/install.mjs "$(PACKAGE_DIR)" "$(PREFIX)"

check: build test vet
	cd ui && bun run check
	npm test --prefix app-host
	cd ui && bun run test
	node --test ui/scripts/install.test.mjs ui/scripts/uninstall.test.mjs scripts/google-oauth-build.test.mjs

fmt:
	gofmt -w cmd internal

test:
	go test ./...

vet:
	go vet ./...

help:
	@printf '%s\n' \
	  'make run      Build and open the chat TUI' \
	  'make build    Build the Go runtime and OpenTUI frontend' \
	  'make setup    Install pinned host and terminal dependencies' \
	  'make package  Create a fresh terminal package (PACKAGE_DIR=...)' \
	  'make install  Install a fresh package into PREFIX (default ~/.local)' \
	  'make check    Build, typecheck, test and vet the terminal product'
