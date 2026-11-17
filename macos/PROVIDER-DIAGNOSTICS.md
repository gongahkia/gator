# Provider failure diagnostics

OpenAI Responses and Gemini HTTP rejections now retain bounded metadata in the existing Go error and per-run trace. This applies to native macOS and other clients that use these Go adapters. It adds no model calls or retries.

For a failed Work run, open:

```text
~/.local/state/gator/traces/<run-id>/trace.json
```

`GATOR_STATE_DIR` or `XDG_STATE_HOME` replaces the `~/.local/state` root. The run's existing retained history also records its trace path. These are the existing private local trace files, created with mode0600 on macOS; the existing512-span limit still applies.

Read the failed attempt metadata with:

```sh
jq '.spans[] | select(.provider_diagnostic != null) | {step, attempt, provider_diagnostic}' \
  "$HOME/.local/state/gator/traces/RUN_ID/trace.json"
```

The `provider_diagnostic` object records:

- Provider adapter, model, HTTP method and responding endpoint.
- HTTP status, request elapsed milliseconds and streaming mode.
- Authentication method and credential source: `explicit`, `gator-store`, `environment`, or `unspecified` for adapters constructed directly without source metadata.
- A bounded request ID when the provider supplies an acceptable ID.
- Recognized provider error code/type/parameter. Gemini's recognized detailed reason takes precedence over its broad status.

Gator's existing credential priority remains explicit configuration → provider-specific Gator store → environment. The source records which branch supplied the value; it cannot tell whether an environment value originally came from `.env` or the shell. Metadata does not contain the key or a key fingerprint. An adapter-level authorization-header override or redirect is reflected in the effective authentication method; credential source still describes initial factory resolution.

The error text includes the same JSON as `provider_request=…`, so a retained failure or CLI error can carry the diagnostic even outside a Work trace. The runner preserves the metadata through wrapped errors and writes it to the matching model-attempt span. The new fields remain local; the current OTLP exporter does not export them.

Raw provider message text and response bodies are excluded because providers can echo credentials or input. Error codes/types and parameter names use explicit allowlists; unknown values are omitted. URL user information, queries and fragments are removed. Known key values and OpenAI key-shaped strings are redacted from bounded metadata, and request IDs reject key-shaped/control-character values. Status-based summaries preserve a readable explanation such as `authentication rejected` or `rate limit reached`. This deliberately trades unbounded provider prose for structured diagnostics. HTTP401 remains permanent; existing429/503 retry behavior is unchanged.

Scope: HTTP rejection responses from these two adapters. Missing-local-key errors, transport failures, interrupted streams after HTTP200, and other provider adapters retain their existing diagnostics. The change does not recover metadata discarded by historical probes.

## Verification — 2026-11-17

Started from clean HEAD `a7d26a1fa98e28bb308c3234fffe44a39f05b2dc`. No `.env`, credential or existing acceptance-evidence changes.

- Focused provider/agent/telemetry tests passed, then credential-source tests passed.
- `go build -o /tmp/gator-native-runtime ./cmd/gator` passed.
- `go test ./internal/model/... ./internal/agent ./internal/telemetry ./internal/workrun ./cmd/gator ./internal/workapp ./internal/attachment -count=1`:13 packages passed.
- `swift build --package-path macos --scratch-path /tmp/gator-native-build` passed.
- `swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel`:130 passed in47.861s. An additional real-Go bridge check follows the completed runtime build because the initial Swift suite began while that build was finishing.
- Tests cover streaming/nonstreaming401 → runner → private trace, one request/no retry on401, Gemini reason/request ID, original retry hints, credential priority, secret/prompt echoes, malformed HTML/JSON, redirect endpoint selection, and bounded unknown metadata. No assertion was weakened.

Logs are under `Evidence/Provider-Diagnostics/headless/`. All provider requests for this change used local fixtures; no live/paid call was made. GUI review was unnecessary for this backend-only change. No commits or README changes.

Updated local app: `/tmp/Gator-Native-Tranche08-Diagnostics.app`. Use this bundle to exercise the new Go diagnostics; the earlier acceptance bundle is preserved. Packaging verification and final checks are appended below.
