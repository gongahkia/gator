# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Documented macOS release signing and notarization setup.
- Pinned the verified Harbor SWE-bench Verified dataset id for benchmark runs.

### Changed

- None yet.

### Deprecated

- None yet.

### Removed

- None yet.

### Fixed

- None yet.

### Security

- None yet.

## [0.0.1-pre] - 2026-07-07

### Added

- Initial `paw run` pipeline with pipeable `gather`, `compress`, `plan`, `edit`, and `verify`
  stages.
- Deterministic context digest schemas, patch application checks, golden CLI tests, and real-stage
  integration coverage.
- Harbor benchmark adapter, `paw bench`, benchmark Make targets, and benchmark result templates.
- Local-first model defaults plus Ollama, OpenAI-compatible, Anthropic-compatible, and external CLI
  brain transports for Codex, Gemini, Claude, OpenCode, Aider, Goose, Qwen, and Cursor.
- Model diagnostics through `paw doctor models`, model listing, Ollama auto-pull, schema smoke
  checks, and CLI transport capability checks.
- User setup and configuration features including `paw init`, repo-local config discovery, env
  overrides, local OpenAI-compatible profiles, TLS options, and macOS local-model guidance.
- Runtime observability through progress output, color controls, verbose trace mirroring,
  structured logging, typed exit codes, trace resume, trace stats, and AI-marker watch mode.
- Release and maintenance scaffolding including GoReleaser, Homebrew tap publishing, curl
  installer, SBOM generation, cross-platform CI, govulncheck, Dependabot, and pre-commit hooks.

### Changed

- Defaulted model configuration to local-first Ollama profiles.
- Hardened model API behavior with fail-fast key checks, HTTP client timeouts, parser hardening,
  and local loopback no-key support.
- Documented model access tradeoffs, CLI smoke-test gates, benchmark wording, architecture, and
  contributing guidance.

### Fixed

- Fixed tokenizer fallback behavior.
- Fixed lint issues in benchmark and model-access code paths.
- Raised command test coverage around CLI behavior and golden output.

### Security

- Added TLS CA and insecure-skip-verify controls for HTTP LLM transports.
- Added govulncheck, Dependabot, and release SBOM generation.

[Unreleased]: https://github.com/gongahkia/paw-cli/compare/v0.0.1-pre...HEAD
[0.0.1-pre]: https://github.com/gongahkia/paw-cli/compare/4cfc5b4...v0.0.1-pre
