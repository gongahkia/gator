# Provider failure diagnostics

OpenAI Responses and Gemini HTTP rejections now retain bounded metadata in the existing Go error and per-run trace. This applies to native macOS and other clients that use these Go adapters. It adds no model calls or retries.

For a failed Work run, open:

```text
~/.local/state/gator/traces/<run-id>/trace.json
```

`GATOR_STATE_DIR` or `XDG_STATE_HOME` replaces the `~/.local/state` root. The run's existing retained history also records its trace path. These are the existing private local trace files, created with mode `0600` on macOS; the existing 512-span limit still applies.

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

Raw provider message text and response bodies are excluded because providers can echo credentials or input. Error codes/types and parameter names use explicit allowlists; unknown values are omitted. URL user information, queries and fragments are removed. Known key values and OpenAI key-shaped strings are redacted from bounded metadata, and request IDs reject key-shaped/control-character values. Status-based summaries preserve a readable explanation such as `authentication rejected` or `rate limit reached`. This deliberately trades unbounded provider prose for structured diagnostics. HTTP 401 remains permanent; existing 429/503 retry behavior is unchanged.

Scope: HTTP rejection responses from these two adapters. Missing-local-key errors, transport failures, interrupted streams after HTTP 200, and other provider adapters retain their existing diagnostics. The change does not recover metadata discarded by historical probes.

For the earlier investigation, the subsequent key-only `GET /v1/models` check succeeded (HTTP 200 and configured model listed). The historical generation 401 therefore does not establish that the current key is invalid; its exact cause remains unresolved. No additional real-provider call was made while implementing these diagnostics.

## Verification — 2026-11-17

Started from clean HEAD `a7d26a1fa98e28bb308c3234fffe44a39f05b2dc`. No `.env`, credential or existing acceptance-evidence changes.

- Focused provider/agent/telemetry tests passed, then credential-source tests passed.
- `go build -o /tmp/gator-native-runtime ./cmd/gator` passed.
- `go test ./internal/model/... ./internal/agent ./internal/telemetry ./internal/workrun ./cmd/gator ./internal/workapp ./internal/attachment -count=1`: 13 packages passed.
- `swift build --package-path macos --scratch-path /tmp/gator-native-build` passed.
- `swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel`: 130 passed in 47.861s. An additional real-Go bridge check follows the completed runtime build because the initial Swift suite began while that build was finishing.
- Tests cover streaming/nonstreaming 401 → runner → private trace, one request/no retry on 401, Gemini reason/request ID, original retry hints, credential priority, secret/prompt echoes, malformed HTML/JSON, redirect endpoint selection, and bounded unknown metadata. No assertion was weakened.

Logs are under `Evidence/Provider-Diagnostics/headless/`. All provider requests for this change used local fixtures; no live/paid call was made. GUI review was unnecessary for this backend-only change. No commits or README changes were made by this agent. HEAD advanced externally during the task to `48319e8af526ddf15836640dbca0766c6a4ae513`; that commit and its work were preserved.

Updated local app: `/tmp/Gator-Native-Tranche08-Diagnostics.app`. Use this bundle to exercise the new Go diagnostics; the earlier acceptance bundle is preserved. Packaging verification and final checks are appended below.

During final review, a new adversarial URL case reproduced partial redaction of a percent-encoded key-shaped path (`encoded-path-before.log`). Redacting the decoded path and clearing its raw encoded representation resolved that case (`encoded-path-after.log`). Final regression and packaging were repeated after this fix; earlier passing logs and the pre-hardening bundle are preserved separately.

Final post-hardening verification: all 13 Go packages passed; Swift build passed; the complete 130-test Swift suite passed in 40.615s after the rebuilt Go runtime was ready. The focused decoded-path regression also passed. `git diff --check`, shell syntax and plist validation passed. The packaged native binary matches the Swift build; actual Go module metadata and hashes of all six bundle files are in `bundle-verification.json`/`runtime-metadata.txt`. The repo `.env` secret values were absent from the bundle and new evidence in a literal-value scan. The earlier acceptance bundle was preserved; no signing/notarization claim is made.

## All-provider continuation — 2026-11-17

The sections above describe the original two-adapter change and remain historical. The current implementation extends safe HTTP rejection metadata to **all38 direct factory provider IDs**, including OpenCode Zen/Go, all compatible providers, Azure, Cloudflare and the Radius gateway. All five wire adapters participate: Responses, Anthropic Messages, Gemini, Chat Completions and Radius pi-messages. OpenCode's four catalog-selected protocols and Cloudflare Gateway's three routes retain the configured provider ID, rather than being mislabeled as OpenAI or Anthropic. Custom and managed-local endpoints also retain their configured identity through metering; keyless local requests report credential source `none`.

Credential precedence, capability interfaces, authorization headers and Go retry decisions are preserved. The Radius configuration GET retains its original permanent-error behavior; the model POST retains its existing status classification. Shared sanitization also covers Anthropic `x-api-key`, Cloudflare gateway authorization and decoded URL paths. A dynamic compatible credential resolver reports `dynamic` when no stronger source was supplied.

Gator's Codex, Claude and Pi CLI adapters now attach safe host-known diagnostics to existing failure events and private traces. They record CLI kind, model, selected endpoint when configured, elapsed time, operation (`exec`/`rpc`), phase, cancellation/timeout category and actual process exit code when available. CLI failures do **not** invent a provider HTTP status or request ID. Missing selected credentials report `none`. Error wrapping preserves cancellation identity. Raw CLI stderr, prompts, assistant text and provider error prose are excluded from this metadata. This means a third-party CLI may expose a detailed reason in its own terminal output that cannot be recovered from the structured host-only trace.

These additions do not cover every transport, local preflight or interrupted-stream error for the HTTP adapters; those retain their original diagnostics. No automatic fallback, provider setup, credential-store discovery or new backend protocol was added. OpenCode Zen/Go remain direct HTTP adapters: installing OpenCode CLI does not turn them into a CLI adapter or supply an OpenCode account key. Radius pi-messages and Pi CLI are also distinct paths.

### Installed tools and live output

Installed under the existing Node26.7.0 fnm prefix:

```sh
fnm exec --using v26.7.0 pi --version       # 1.5.2
fnm exec --using v26.7.0 opencode --version # 2.3.0
```

Packages: `@earendil-works/pi-coding-agent@1.5.2` and `@opencode/cli@2.3.0`. The default Node version was not changed. Pi used the official recommended `--ignore-scripts` installation. OpenCode's installer warned about a disallowed postinstall hook; the installed native executable nevertheless passed its version command and actual generation check. Installation records are in `Evidence/All-Providers/headless/installation-metadata.json`. Sources: [Pi quickstart](https://docs.pi.dev/latest/quickstart), [OpenCode](https://opencode.ai/docs/), [OpenCode provider runtimes](https://opencode.ai/docs/providers/).

The repo `.env` was read privately. Only its existing OpenAI key/model pair was used, in process-local environments and disposable profiles; no key or saved provider default was changed. User/project extensions, tools and ambient instructions were disabled for the CLI smoke checks. OpenCode ran standalone in an empty disposable directory, with tool permissions denied and no shared persistent service.

| Check | Result | Evidence under `Evidence/All-Providers/headless/` |
| --- | --- | --- |
| OpenAI Responses, configured model `gpt-5.6-terra` | HTTP401; no retry, no output | `openai-live-generation.json`; actual request ID retained. One request,512-output-token transport cap,45s timeout. |
| Same key/model, Chat Completions through Gator's existing compatible adapter | **HTTP200**, “Gator model integration received this message.” | `openai-live-chat-generation.json`;46 input +11 output tokens; one request,512-token transport cap. |
| Pi1.5.2 standalone, disposable Chat Completions model | **PASS**, “Gator Pi integration received this message.” | `pi-direct-live-generation.json`;74 input +11 output;512-token configured limit. The fetch probe did not observe its SDK transport, so the empty request list is not proof of zero requests. |
| Pi through Gator's actual sandboxed `cliagent.Run` | **PASS**, same phrase, one assistant turn, zero patch bytes | `pi-gator-live-success.json`;162 input +11 output; disposable wrapper selects the working text-only protocol and configured512-token limit. |
| OpenCode2.3.0 CLI, disposable OpenAI Chat runtime | **PASS**, “Gator OpenCode integration received this message.” | `opencode-live-generation.json`; exit0 /1.893s; configured512-token limit, one agentic step, tools denied. Usage was not captured. This is CLI evidence, not Zen/Go gateway evidence. |
| Other direct providers, including Gemini and OpenCode Zen/Go | NOT TESTED live | No matching credentials configured. Their diagnostics are fixture-tested; another provider's credential was not sent to them. |

**The key/model pair works for real generation.** The Responses401 is specific to that route in this environment; the available non-secret metadata does not establish its precise provider-side cause. It does not establish an invalid key or a Swift defect. Gator's standard `openai` backend still uses Responses; no automatic switch was introduced. To deliberately select the working API shape, use the existing `openai-compatible` provider or a configured custom Chat Completions endpoint via terminal setup. The endpoint is `https://api.openai.com/v1/chat/completions`. Saved Gator settings are `/Users/gongahkia/.config/gator/config.json`; Gator-managed credentials belong in `/Users/gongahkia/.local/state/gator/auth.json`. Those files were absent before this task and were not created by these probes. The repository `.env` remains at `/Users/gongahkia/Desktop/coding/projects/gator/.env`; launching a Finder app does not itself source that file.

No exact USD spend is claimed. Known reported successful usage is315 tokens across the direct Chat and two Pi checks, plus the OpenCode call whose usage was not captured. Short prompts,512-output-token limits and bounded process timeouts were used; provider billing was not queried. No further live calls are needed for this pass.

### Original failures and verification

Preserved separately from final successes:

- The first CLI compile attempt failed on an unused `fmt` import; removing that unused import restored the build.
- New CLI fixtures initially referenced the original environment-variable name after Codex/Claude intentionally remapped it, causing exit1 instead of the intended fixture exit7. The fixture now emits only a literal synthetic credential. Another assertion expected an unescaped redaction path; URL serialization correctly percent-encodes its brackets. Final fixture rerun passed; these were test setup/expectation errors, not relaxed production validation.
- The first two OpenCode smoke configurations did not load the disposable agent (`Agent not found`). Explicitly supplying the disposable config resolved setup. The next attempt returned400 because function tools with the configured model's reasoning mode are unsupported on Chat Completions. A text-only fixture with `reasoning_effort: none` and an empty tools list succeeded. See `opencode-first-attempt.json`, `opencode-second-attempt.json`, `opencode-third-attempt.json`, then `opencode-live-generation.json`.
- Initial sandboxed Pi attempts returned failed terminal messages. A diagnostic probe established the same400 tools/reasoning constraint. The original smoke wrapper's `--no-tools` did not override Gator's separate `--tools` allowlist; its metadata describes intended isolation, not proof that tools were disabled. The successful wrapper removes that allowlist and explicitly sets the model's reasoning sampling parameter to `none`. No Gator production tool envelope was widened. Failed JSON results remain alongside the final success.
- A temporary OpenCode harness had a Python syntax error before process execution. An unauthenticated schema download returned403; official docs remained readable through the documentation tool. Neither is a model integration result.

Final relevant complete regression:

```sh
go build -o /tmp/gator-native-runtime ./cmd/gator
go test ./internal/model/... ./internal/agent ./internal/telemetry ./internal/workrun \
  ./internal/cliagent ./internal/sandbox ./cmd/gator ./internal/workapp ./internal/attachment -count=1
swift build --package-path macos --scratch-path /tmp/gator-native-build
swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel
```

All15 Go packages PASS. Final Swift132 PASS /37.262s after the native focus change. A further custom/local rejection and private CLI-trace persistence check is recorded separately. Factory rejection fixtures exercise38 provider IDs plus five additional routing cases; HTTP401 remains nontransient. CLI failure fixtures cover Codex/Claude/Pi exit7, cancellation identity, metadata redaction, absence of invented HTTP status and missing-credential metadata. Custom/local coverage includes authenticated, keyless and managed-local configurations through existing metering.

Host16 PASS; recovery integration9 PASS /208 assertions; selected TUI2 PASS /45 assertions with1 existing platform skip. Logs are retained under `Evidence/All-Providers/headless/` and `Evidence/Notch-Quick-Access/`; the initial failures were not overwritten. Physical VoiceOver/IME acceptance and signing/notarization remain outside these automated results.

Current final bundle: **`/tmp/Gator-Native-Tranche08-Enhanced-Final.app`**. See [native interaction verification](MENUBAR-NOTCH-VERIFICATION.md) for the added presentation, screenshots and actual bundle inspection. Earlier app bundles and closure evidence are preserved. HEAD advanced externally during the task; those commits and work were preserved. This agent made no commits or README changes.

## Final local-development diagnosis — Responses geography rejection

The earlier unresolved Responses401 assessment is superseded for the current reproduced rejection. One explicitly permitted canonical Responses request returned HTTP401 with the redacted detail **“Attempted to access resource from outside project geography US.”** It used the existing `.env` key/model privately, string input, `store:false`,32 output tokens maximum and25s timeout; no retry, fallback, redirect follow, credentials/default change or geography bypass. Error content type was text/plain with a JSON error envelope. Only bounded redacted error detail, request ID and non-secret header names were retained.

Both adapters resolve the same official host and configured model and use Bearer authorization plus Content-Type. The offline synthetic capture confirms the expected `input`/`instructions` versus `messages` formats; no environment base/provider override or proxy variable was present. The official [Responses migration reference](https://developers.openai.com/api/docs/guides/migrate-to-responses) documents both API shapes. Therefore the evidence supports a provider-side **project geography authorization blocker**, not a missing API or a concrete Gator adapter defect. The exact difference in that project's Chat and Responses geography policy requires provider/project administration. Earlier error bodies remain unavailable; their specific historical reason is not invented.

The working explicit Chat path is preserved. No adapter change, automatic fallback, credential rotation or `.env` edit was made. Evidence: `Evidence/Local-Dev-Acceptance/headless/provider-resolution.json`, `offline-wire-comparison.jsonl` and `responses-canonical-once.json`. Full local-development decision and exact regression commands: [TRANCHE08-LOCAL-DEV-ACCEPTANCE.md](TRANCHE08-LOCAL-DEV-ACCEPTANCE.md). Final15 Go packages and133 Swift tests passed; candidate bundle remains unchanged. Responses-specific acceptance stays blocked until the project's authorized geography configuration is resolved.
