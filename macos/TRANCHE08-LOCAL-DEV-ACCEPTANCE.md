# Tranche08 — final local-development acceptance

Date: 2026-11-17. Starting HEAD: `87339a6112d8c68f9d0a7b64e982b0d7dad6d424`; working tree was clean. Host: Apple M3 /16GiB, macOS26.5.2, Swift6.3.3, Go1.27.3, Node26.7.0 and Bun1.4.2. Earlier reports, bundles and evidence are preserved.

**Local-development acceptance can close for the verified native workflow and explicit working Chat route.** Pending-approval navigation is now PASS. Responses remains blocked by provider project geography. Physical accessibility acceptance remains unsigned; neither of those limitations is silently converted to PASS. No application defect requiring a production change was reproduced, so the existing bundle is retained.

## A. Pending-approval navigation — PASS

Used the actual `/tmp/Gator-Native-Tranche08-Enhanced-Final.app`, a disposable workspace/profile, a loopback custom provider and Go's existing `web_access` exact approval. The provider requests a review of access to `https://acceptance.example.invalid` and implements no fetch/search/send. The request was declined. No real account or credential was configured.

A transparent test-only runtime observer forwarded bridge bytes unchanged to/from the **actual bundled Go runtime**. It recorded owner IDs, interaction IDs, SHA256 hashes of tokens/review content and approval-frame metadata. It neither synthesized snapshots nor changed authorization. Raw tokens stay in the private disposable profile; committed evidence contains hashes only. This instrumentation is an evidence collector, not a product protocol or coordinator.

| Native step | Observed result |
| --- | --- |
| Start `Hold unrelated work`, then `Begin inert pending approval` | Go holds the original task and delegates the approval to its own child owner. |
| Type an unsent foreground draft | `Keep my foreground draft during exact review navigation` retained. |
| Close full window, open/dismiss menu-bar chat, reopen notch | Original task remains running; pending interaction/token/review unchanged. |
| Inspect notch attention | `1 chat to inspect`; row explicitly says `Begin inert pending approval, Needs review`. |
| Choose that row | Original child becomes foreground and its exact current review opens. |
| Close full window, reopen compact review, expand to full | Same owner, interaction1, token hash and complete review hash at every checkpoint. No presentation-created owner. |
| Tab to **Approve Exact Action**, verify focus, press Return | Approve remains focused; zero decision frames, pending review unchanged. |
| Shift-Tab to **Decline**, press Space | Exactly one `approved:false` frame with the original interaction/token, sent on the original owner's bridge. Pending approval clears. |
| Return to requesting chat | Exact unsent draft remains in the native composer; unrelated work still running. |
| Release the held fixture explicitly | Only then does that fixture complete; zero external-effect requests throughout. |

The native profile had one inactive precondition owner created before the operator left the desktop idle; it was excluded from the controlled task assertions. The same three-owner set existed before/after presentation changes. The new pristine headless regression uses exactly the two controlled owners.

An AX focus setter initially returned success while focus was on Inspect Current Task. That attempt is **not** evidence of Return on Approve. The accepted check used actual Tab traversal, verified Approve focus, then Return; see `native-keyboard-observations.json` and `verified_return_with_approve_focused` in `native-checkpoints.jsonl`.

Evidence under `Evidence/Local-Dev-Acceptance/`:

- `notch-pending-approval.png`: actual notch attention and owning row.
- `compact-exact-review.png`, `compact-exact-review.ax.txt`: actual compact exact review, scrolling content and decision controls.
- `full-exact-review-window.png`: actual expanded review; screenshot precedes the accepted Tab/Return check and does not claim Approve focus.
- `after-decline-parent.ax.txt`: returned foreground draft and unrelated work.
- `headless/native-checkpoints.jsonl`, `native-assertions.json`, `native-keyboard-observations.json`: exact identity/decision assertions.

The new headless real-Go test also checks owner-bound review reopen, parent and task-owner drafts, explicit decline, unchanged owner count and unrelated work completing only after fixture release.

## B. Responses401 — provider geography blocker identified

One permitted, justified live request used a minimal documented Responses body to obtain the error detail missing from earlier probes. It made **one request**, with32 output tokens maximum,25s timeout, no tools, no redirect follow, no retry, no fallback and `store:false`. It returned HTTP401, no generation/usage, with this redacted provider message:

> Attempted to access resource from outside project geography US

Evidence: `headless/responses-canonical-once.json`. The response also includes an `X-Openai-Authorization-Error` header name; its value, cookies and other header values were not retained. Body content type was `text/plain`, but the body carried a JSON error envelope, parsed only for bounded redacted error detail. The `.env` file was unchanged.

This identifies the **current reproduced rejection** as provider-side project geography authorization. It does not establish an invalid key, missing Responses implementation or malformed Swift request. Earlier error bodies were not retained, so this report does not invent their historical details.

| Compared property | Responses | Previously successful Chat |
| --- | --- | --- |
| Host/base | `https://api.openai.com/v1` | Same |
| Endpoint | `/responses` | `/chat/completions` |
| Identity | `openai` factory / Responses adapter | Chat Completions adapter; prior live probe labeled it `openai`, supported explicit compatible factory is `openai-compatible` |
| Model | `.env`: `gpt-5.6-terra` | Same configured model |
| Authentication | `Authorization: Bearer` | Same key source/scheme in earlier successful probe |
| Explicit header names | `Authorization`, `Content-Type` | Same |
| Adapter request format | `input` messages + `instructions`, `store:false` | `messages` including system instruction |
| Canonical discrimination request | String `input`, `model`, `max_output_tokens`, `store:false` | Not repeated; earlier200/output evidence preserved |
| Error detail | Project geography US rejection | Earlier real generation200 |

The offline capture used synthetic credentials and zero network requests. No base-URL/provider override or proxy variable was present; `.env` contains only the OpenAI key/model pair. Default transport headers are not claimed as explicitly captured adapter headers. See `headless/offline-wire-comparison.jsonl` and `provider-resolution.json`.

OpenAI documents both endpoints, the input-format difference and the Responses string-input form. [Official migration reference](https://developers.openai.com/api/docs/guides/migrate-to-responses). Therefore the evidence does **not** justify saying this provider has no Responses API. This project's current route is refused by geography authorization. The exact policy difference between Chat and Responses requires the provider/project administrator; no location-routing or authentication bypass was attempted.

No Gator adapter defect was found, so no protocol fix or automatic fallback was added. Preserve the explicit known-working Chat endpoint `https://api.openai.com/v1/chat/completions`. No key rotation, `.env` edit or provider-default change occurred. An authorized project administrator must resolve the provider geography policy before Responses-specific acceptance can close. Existing Gator paths, if needed for deliberate configuration repair, remain `~/.config/gator/config.json` and `~/.local/state/gator/auth.json`; no provider setup was implemented in this pass.

## C. Accessibility — automated PASS; physical checks NOT TESTED

Automated evidence: real native Tab/Shift-Tab, verified Return-safe approval, explicit keyboard decline, notch attention/owner navigation, draft restoration, existing AX semantics, marked-text API tests, contrast tests and exact-content projection tests. Normal-size compact/full text wrapping and reachable decisions are visible in the screenshots.

**NOT TESTED by a human:** spoken VoiceOver conversation/review/notch behavior; physical IME commit/Enter safety; Full Keyboard Access preference variants; Reduce Motion and Increase Contrast enabled by an operator; larger-text/display variants. AX metadata and synthetic events do not establish those results. No persistent OS accessibility setting was changed.

The concise eight-check operator table is appended to [TRANCHE08-OPERATOR-ACCEPTANCE.md](TRANCHE08-OPERATOR-ACCEPTANCE.md), with expected results and unsigned result fields. Prior detailed attachment/recovery/platform rows remain intact.

## D. Files changed

Application Swift/Go code: **none**. Tests and evidence only:

- `Tests/GatorCoreTests/Fixtures/dev-acceptance-provider.mjs`: deterministic inert advertised-tool proposal and independently held work.
- `Tests/GatorCoreTests/Fixtures/dev-acceptance-runtime.py`: transparent, test-only bridge observer.
- `Tests/GatorCoreTests/NativeReviewIntegrationTests.swift`: optional fixture name on the existing helper; original default unchanged.
- `Tests/GatorCoreTests/NativeInertApprovalIntegrationTests.swift`: new real-Go regression for owner/draft/approval isolation.
- This report, append-only supplements to the three reviewed reports and operator checklist, and `Evidence/Local-Dev-Acceptance/`.

The headless evidence directory also contains the probe/launch/observation source as `.txt` files for review/reproduction. The controlled launcher is run from the repository root after quitting other Gator instances; it launches only a disposable profile and loopback provider. Quit the fixture app and stop its terminal process afterward. Do not run the one-request live source again with a known rejected route without new justification.

## E. Exact checks and results

All commands ran from the repository root. Full logs are under `Evidence/Local-Dev-Acceptance/headless/`.

```sh
GATOR_INSPECT_MODEL=gpt-5.6-terra go run ./.gator/dev-wire-inspection
# Synthetic request capture; zero network requests. Source archived as offline-wire-comparison.go.txt.
python3 /tmp/gator-dev-headless-fixture.py
# Genuine pending approval established before desktop launch; source/log archived.
GATOR_TEST_BINARY=/tmp/Gator-Native-Tranche08-Enhanced-Final.app/Contents/Resources/gator-runtime \
  swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel \
  --filter realGoInertApprovalNavigation
# 1 PASS /0.883s.
go test ./internal/model/openai ./internal/model/chatcompletions ./internal/model/providerhttp -count=1
# 3 packages PASS.
go build -o /tmp/gator-native-runtime ./cmd/gator
# PASS.
go test ./internal/model/... ./internal/agent ./internal/telemetry ./internal/workrun \
  ./internal/cliagent ./internal/sandbox ./cmd/gator ./internal/workapp ./internal/attachment -count=1
# 15 packages PASS.
swift build --package-path macos --scratch-path /tmp/gator-native-build
# PASS; native executable identical to retained bundle.
GATOR_TEST_BINARY=/tmp/Gator-Native-Tranche08-Enhanced-Final.app/Contents/Resources/gator-runtime \
  swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel
# 133 PASS /48.335s, including real bundled-Go integrations.
fnm exec --using v26.7.0 node --test app-host/tests/drafts.test.mjs app-host/tests/uploads.test.mjs
# 16 PASS.
GATOR_TEST_BINARY=/tmp/Gator-Native-Tranche08-Enhanced-Final.app/Contents/Resources/gator-runtime \
  /Users/gongahkia/.npm/_npx/22f2fe8d8bc13000/node_modules/.bin/bun test app-host/tests/preparation-recovery.test.ts
# 9 PASS /208 assertions.
/Users/gongahkia/.npm/_npx/22f2fe8d8bc13000/node_modules/.bin/bun test --cwd ui --preload ./scripts/preload.ts \
  tests/approval-review.test.tsx tests/draft-synchronization.test.tsx tests/attachments.test.tsx
# 2 PASS /45 assertions; 1 existing platform skip in attachments.test.tsx.
git diff --check
# PASS.
```

The one canonical live probe was built from the archived Go source and invoked by a private `.env`-loading harness; output was checked for literal credentials before being written. No secret appeared in commands or evidence. No test assertion was weakened. No failed headless test/build occurred in this pass. The initial AX focus-setter mismatch is documented separately from the accepted native keyboard rerun.

## F. Final local app

Retained: **`/tmp/Gator-Native-Tranche08-Enhanced-Final.app`**.

```sh
open "/tmp/Gator-Native-Tranche08-Enhanced-Final.app" --args --open-window
```

No production fix was needed; fixture/test/report changes do not change the app. A new `/tmp/Gator-Native-Dev-Accepted.app` would contain the same application, so it was unnecessary. All six candidate bundle file hashes match the previous record; the native executable matches the fresh build. Actual arm64 native/Go metadata, literal credential scan and unchanged-candidate verification are in `headless/bundle-integrity.json`. The native desktop run and Swift integrations used its actual bundled Go runtime. No signing, notarization, installer, update or release-infrastructure work was performed.

## G. Remaining defects and limits

- **P0/P1:** none reproduced in this pass or in the relevant passing regressions.
- **Known P2:** independently selected documents with duplicate basenames remain safely rejected by the current shared validation contract. Existing files/bytes are not silently overwritten. This is a documented limitation, not a new defect or an authorization to change backend semantics here.
- **External blocker:** Responses project-geography authorization; no application workaround was added.
- **Operator-only evidence:** physical accessibility and preference variants listed above. Broader Spaces/multi-display and sustained performance profiling remain outside this focused pass; historical measurements are preserved.

## H. Acceptance decision

**YES — the practical local-development acceptance pass is closed for the tested native workflow and working explicit Chat path.** Exact pending approval navigation, ownership, Return safety, decline, drafts and unrelated-work preservation now have native and headless evidence. Operator accessibility sign-off and Responses geography authorization remain explicitly open as separate acceptance scopes. No blanket accessibility or all-provider PASS is claimed. No Tranche09, README modification, commit, feature development or backend-goal resumption occurred.
