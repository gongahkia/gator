.PHONY: temp test fixture-test conformance-test live-aider-test live-aider-e2e live-amp-test live-amp-e2e live-cline-test live-cline-e2e live-cursor-test live-cursor-e2e live-codex-test live-codex-e2e live-gemini-test live-gemini-e2e live-goose-test live-goose-e2e live-kimi-test live-kimi-e2e live-vibe-test live-vibe-e2e live-copilot-test live-copilot-e2e live-pi-test live-pi-e2e live-handoff-e2e indexer-test sidecar benchmark fmt format-check lint check issues

NVIM ?= nvim
NVIM_TEST = tests/gator-test.sh
NVIM_LINT = tests/gator-test.sh lint
INDEXER_TEST = tests/gator-test.sh indexer

temp:
	$(NVIM) . --cmd 'set rtp+=$(CURDIR)' --cmd 'lua require("gator").setup()'

test:
	$(NVIM_TEST)

fixture-test:
	GATOR_TEST_GLOB='adapters_fixtures_spec.lua' $(NVIM_TEST)

conformance-test:
	GATOR_TEST_GLOB='adapter_conformance_spec.lua' $(NVIM_TEST)

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

live-cursor-test:
	GATOR_LIVE_CURSOR=1 GATOR_TEST_GLOB='cursor_live_spec.lua' $(NVIM_TEST)

live-cursor-e2e:
	GATOR_LIVE_CURSOR=1 GATOR_LIVE_CURSOR_AUTH=1 GATOR_TEST_GLOB='cursor_live_spec.lua' $(NVIM_TEST)

live-codex-test:
	GATOR_LIVE_CODEX=1 GATOR_TEST_GLOB='codex_live_spec.lua' $(NVIM_TEST)

live-codex-e2e:
	GATOR_LIVE_CODEX=1 GATOR_LIVE_CODEX_AUTH=1 GATOR_TEST_GLOB='codex_live_spec.lua' $(NVIM_TEST)

live-gemini-test:
	GATOR_LIVE_GEMINI=1 GATOR_TEST_GLOB='gemini_live_spec.lua' $(NVIM_TEST)

live-gemini-e2e:
	GATOR_LIVE_GEMINI=1 GATOR_LIVE_GEMINI_AUTH=1 GATOR_TEST_GLOB='gemini_live_spec.lua' $(NVIM_TEST)

live-goose-test:
	GATOR_LIVE_GOOSE=1 GATOR_TEST_GLOB='goose_live_spec.lua' $(NVIM_TEST)

live-goose-e2e:
	GATOR_LIVE_GOOSE=1 GATOR_LIVE_GOOSE_AUTH=1 GATOR_TEST_GLOB='goose_live_spec.lua' $(NVIM_TEST)

live-kimi-test:
	GATOR_LIVE_KIMI=1 GATOR_TEST_GLOB='kimi_live_spec.lua' $(NVIM_TEST)

live-kimi-e2e:
	GATOR_LIVE_KIMI=1 GATOR_LIVE_KIMI_AUTH=1 GATOR_TEST_GLOB='kimi_live_spec.lua' $(NVIM_TEST)

live-vibe-test:
	GATOR_LIVE_VIBE=1 GATOR_TEST_GLOB='vibe_live_spec.lua' $(NVIM_TEST)

live-vibe-e2e:
	GATOR_LIVE_VIBE=1 GATOR_LIVE_VIBE_AUTH=1 GATOR_TEST_GLOB='vibe_live_spec.lua' $(NVIM_TEST)

live-copilot-test:
	GATOR_LIVE_COPILOT=1 GATOR_TEST_GLOB='copilot_live_spec.lua' $(NVIM_TEST)

live-copilot-e2e:
	GATOR_LIVE_COPILOT=1 GATOR_LIVE_COPILOT_AUTH=1 GATOR_TEST_GLOB='copilot_live_spec.lua' $(NVIM_TEST)

live-pi-test:
	GATOR_LIVE_PI=1 GATOR_TEST_GLOB='pi_live_spec.lua' $(NVIM_TEST)

live-pi-e2e:
	GATOR_LIVE_PI=1 GATOR_LIVE_PI_AUTH=1 GATOR_TEST_GLOB='pi_live_spec.lua' $(NVIM_TEST)

live-handoff-e2e:
	$(MAKE) live-aider-e2e
	$(MAKE) live-amp-e2e
	$(MAKE) live-cline-e2e
	$(MAKE) live-cursor-e2e
	$(MAKE) live-codex-e2e
	$(MAKE) live-gemini-e2e
	$(MAKE) live-goose-e2e
	$(MAKE) live-kimi-e2e
	$(MAKE) live-vibe-e2e
	$(MAKE) live-copilot-e2e
	$(MAKE) live-pi-e2e
	GATOR_TEST_GLOB='handoff_cross_provider_spec.lua' $(NVIM_TEST)

indexer-test:
	$(INDEXER_TEST)

benchmark:
	GATOR_TEST_GLOB='performance*_spec.lua' $(NVIM_TEST)

sidecar:
	cargo run --manifest-path crates/gator-index/Cargo.toml --quiet

fmt:
	stylua lua plugin tests
	cargo fmt --manifest-path crates/gator-index/Cargo.toml

format-check:
	stylua --check lua plugin tests
	cargo fmt --manifest-path crates/gator-index/Cargo.toml --check

lint:
	$(NVIM_LINT)

check: test indexer-test format-check lint

issues:
	node scripts/seed_issues.mjs
