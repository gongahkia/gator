# Changelog

## Unreleased

- Replaced Droid one-shot managed execution with persistent JSON-RPC sessions, strict Gator-owned approvals, questions, cancellation, and resume.
- Added explicit managed-session detach/stop lifecycle, Droid `close_session` cleanup, and `VimLeavePre` child shutdown.
- Added capability-gated Copilot ACP reattach with an interactive CLI-resume fallback and explicit readiness states.
- Added strict runtime Droid JSON-RPC validation plus local authenticated Droid and Copilot bridge E2E coverage.
- Added first-class adapters for Amp, Aider, Cline, Cursor Agent, Goose, Kimi Code CLI, Mistral Vibe, and Droid.
- Added public-release documentation, provider capability matrix, and release hardening.

## Versioning

Gator follows semantic versioning. Adapter capability changes and Neovim support changes are documented in this file.
