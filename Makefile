.PHONY: test fixture-test live-aider-test live-aider-e2e live-amp-test live-amp-e2e live-cline-test live-cline-e2e live-codex-test live-claude-test live-gemini-test live-gemini-e2e live-copilot-test live-copilot-e2e live-opencode-test live-opencode-e2e live-pi-test live-pi-e2e indexer-test sidecar benchmark fmt format-check lint check issues

NVIM_TEST = nvim --headless --noplugin -i NONE -u tests/minimal_init.lua -c "lua dofile('tests/run.lua')"

test:
	$(NVIM_TEST)

fixture-test:
	GATOR_TEST_GLOB='adapters_fixtures_spec.lua' $(NVIM_TEST)

live-aider-test:
	GATOR_LIVE_AIDER=1 GATOR_TEST_GLOB='aider_live_spec.lua' $(NVIM_TEST)

live-aider-e2e:
	GATOR_LIVE_AIDER=1 GATOR_LIVE_AIDER_AUTH=1 GATOR_TEST_GLOB='aider_live_spec.lua' $(NVIM_TEST)

live-amp-test:
	GATOR_LIVE_AMP=1 GATOR_TEST_GLOB='amp_live_spec.lua' $(NVIM_TEST)

live-amp-e2e:
	GATOR_LIVE_AMP=1 GATOR_LIVE_AMP_AUTH=1 GATOR_TEST_GLOB='amp_live_spec.lua' $(NVIM_TEST)

live-cline-test:
	GATOR_LIVE_CLINE=1 GATOR_TEST_GLOB='cline_live_spec.lua' $(NVIM_TEST)

live-cline-e2e:
	GATOR_LIVE_CLINE=1 GATOR_LIVE_CLINE_AUTH=1 GATOR_TEST_GLOB='cline_live_spec.lua' $(NVIM_TEST)

live-codex-test:
	GATOR_LIVE_CODEX=1 GATOR_TEST_GLOB='codex_live_spec.lua' $(NVIM_TEST)

live-claude-test:
	GATOR_LIVE_CLAUDE=1 GATOR_TEST_GLOB='claude_live_spec.lua' $(NVIM_TEST)

live-gemini-test:
	GATOR_LIVE_GEMINI=1 GATOR_TEST_GLOB='gemini_live_spec.lua' $(NVIM_TEST)

live-gemini-e2e:
	GATOR_LIVE_GEMINI=1 GATOR_LIVE_GEMINI_AUTH=1 GATOR_TEST_GLOB='gemini_live_spec.lua' $(NVIM_TEST)

live-copilot-test:
	GATOR_LIVE_COPILOT=1 GATOR_TEST_GLOB='copilot_live_spec.lua' $(NVIM_TEST)

live-copilot-e2e:
	GATOR_LIVE_COPILOT=1 GATOR_LIVE_COPILOT_AUTH=1 GATOR_TEST_GLOB='copilot_live_spec.lua' $(NVIM_TEST)

live-opencode-test:
	GATOR_LIVE_OPENCODE=1 GATOR_TEST_GLOB='opencode_live_spec.lua' $(NVIM_TEST)

live-opencode-e2e:
	GATOR_LIVE_OPENCODE=1 GATOR_LIVE_OPENCODE_AUTH=1 GATOR_TEST_GLOB='opencode_live_spec.lua' $(NVIM_TEST)

live-pi-test:
	GATOR_LIVE_PI=1 GATOR_TEST_GLOB='pi_live_spec.lua' $(NVIM_TEST)

live-pi-e2e:
	GATOR_LIVE_PI=1 GATOR_LIVE_PI_AUTH=1 GATOR_TEST_GLOB='pi_live_spec.lua' $(NVIM_TEST)

indexer-test:
	cargo test --manifest-path crates/gator-index/Cargo.toml

benchmark:
	GATOR_TEST_GLOB='performance_suite_spec.lua' $(NVIM_TEST)

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
