.PHONY: test indexer-test fmt check issues

test:
	nvim --headless --noplugin -i NONE -u tests/minimal_init.lua -c "lua dofile('tests/run.lua')"

indexer-test:
	cargo test --manifest-path crates/gator-index/Cargo.toml

fmt:
	stylua lua plugin tests
	cargo fmt --manifest-path crates/gator-index/Cargo.toml --check

check: test indexer-test

issues:
	node scripts/seed_issues.mjs
