# Diagnostics and bug reports

Gator is local-first. It does not persist provider credentials. Gator-owned chat transcripts and reviewed handoff artifacts are stored locally under `.gator/` after redaction; terminal output is never scraped. Do not enable telemetry to investigate a problem; telemetry is disabled by default and is not needed for a bug report.

## What Gator records

Gator has no general persistent activity log for normal provider runs. Use the following local, redacted artifacts instead:

| Command | Path | Purpose |
| --- | --- | --- |
| `:GatorExportDiagnostics` | `stdpath('state') .. '/gator/diagnostics/current.json'` | One rolling snapshot of compatibility, workspace state, and safe configuration. It sets `network_telemetry` to `false`. |
| `:GatorBetaReadiness` | `stdpath('state') .. '/gator/beta-readiness/report.json'` | Local readiness checks. When readiness is not `ready`, `failure.json` is also written in that directory. |

Each command reports the generated path in a Neovim notification. Reports remain local until you explicitly share them. Review every artifact before attaching it: redaction reduces accidental disclosure but cannot assess information you intentionally add elsewhere.

## Collect a useful report

1. Reproduce the problem once, then run `:GatorHealth` from the affected project.
2. Run `:GatorExportDiagnostics` and, for startup/readiness failures, `:GatorBetaReadiness`.
3. Open `:messages` and copy only the relevant Gator notification or error text.
4. Record your Neovim version, Gator version/commit, operating system, and affected provider CLI/version.
5. Open a GitHub issue using the bug-report template. Include minimal reproduction steps, expected behavior, actual behavior, and the reviewed diagnostic artifact when it helps.

Do not attach credentials, API keys, tokens, private prompts, provider transcripts, unredacted logs, or private repository paths. Provider account, billing, and login problems must be resolved in the provider's own CLI.

## Neovim logs

Neovim may have its own log configuration. If `:echo $NVIM_LOG_FILE` prints a path, treat that file as separate from Gator diagnostics and review/redact it before sharing. Gator does not configure, rotate, or upload Neovim's log.
