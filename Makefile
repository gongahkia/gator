.PHONY: test fixture-test live-codex-test indexer-test sidecar fmt format-check lint check issues

NVIM_TEST = nvim --headless --noplugin -i NONE -u tests/minimal_init.lua -c "lua dofile('tests/run.lua')"

test:
	$(NVIM_TEST)

fixture-test:
	GATOR_TEST_GLOB='adapters_fixtures_spec.lua' $(NVIM_TEST)

live-codex-test:
	GATOR_LIVE_CODEX=1 GATOR_TEST_GLOB='codex_live_spec.lua' $(NVIM_TEST)

indexer-test:
	cargo test --manifest-path crates/gator-index/Cargo.toml

sidecar:
	cargo run --manifest-path crates/gator-index/Cargo.toml --quiet

fmt:
	stylua lua plugin tests
	cargo fmt --manifest-path crates/gator-index/Cargo.toml

format-check:
	stylua --check lua plugin tests
	cargo fmt --manifest-path crates/gator-index/Cargo.toml --check

lint:
	nvim --headless --noplugin -i NONE -u NONE -c "lua dofile('tests/lint.lua')"

check: test indexer-test format-check lint

issues:
	node scripts/seed_issues.mjs
