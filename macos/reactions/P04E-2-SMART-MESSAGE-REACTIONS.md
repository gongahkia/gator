# P04E-2 — Smart message reactions

This is a Go-owned durable acknowledgement with native emoji presentation, not a native-only response suppressor. Final implementation/testing decisions are recorded below. Packaged-native desktop acceptance remains deferred; release readiness remains blocked by the separate P04E-1R2 conditions.

## Starting architecture and source audit

Starting HEAD: `60b5e74cab9785e12cade1c67190b955a38906e6`, clean worktree. [Baseline identities](evidence/p04e-2/baseline.json). HEAD advanced externally during work; this agent issued no commit, checkout or reset. Final file/binary hashes identify the delivered source independently of those external snapshots.

Inspected the current Work conversational router, dialogue owner/admission/bridge, strict dialogue store and its CAS/transition rules; actual native protocol/application/coordinator, transcript/message rendering, streaming/pulse/preferences, current tests and host/terminal protocol consumers. Read P04U-1 and P04E-1R2 acceptance reports. Additional inspection of `cmd/gator/work_dialogue.go` established the canonical processing-scope check that must precede a shortcut.

Existing boundaries retained:

- Go admits immutable text/options under a client request identity, assigns a durable turn ID/sequence and owns routing, permissions, task dispatch and revisions.
- `worksession.DialogueTurn` is version 1, has strict top-level JSON/state validation, requires nonempty ordinary answers and stores decision JSON opaquely. Terminal states cannot be replayed; decision bytes cannot be replaced after retention.
- Native transcript IDs already use conversation plus durable dialogue-turn ID and user/response suffix. Work message indexes used by exact reviews remain independent.
- Native and terminal hosts pass public owner/task projections; they do not grant model authorization. The new inspection flag only declines the acknowledgement shortcut.
- Swift renders a confirmed reaction. It never classifies the original message, changes Go response text or sends a reaction/approval command.

## Decision policy and cost boundary

The Go whole-message allowlist deliberately recognizes a small English vocabulary, including “Thanks!”, “Appreciate it!”, “Thanks Gator, that was helpful!”, “Hahaha”, “Haha that's hilarious”, “Nice, that's great!” and “See you!”. Case and trailing period/exclamation punctuation are normalized. Internal multiline/control input, extra clauses, questions, quoted text and unrecognized text stay on the normal router. Generic “okay”, “yes”, “great”, “nice”, “approved” and “sounds good” are deliberately absent because they may be consequential confirmation.

Eligibility requires a valid bounded conversation request, at most 160 input bytes, no attachments/update reply/plan-first/explicit source refresh, and no projected inspection requirement. Pending approvals/connections, plan preparation/revision, recovery/receipt uncertainty or errors, shutdown and dialogue errors decline the shortcut. Task context is accepted only in ordinary running/completed states with no review/edit/inspection capability; other and future unknown states use ordinary routing. A recent assistant question or confirmation/authorization prompt also declines the shortcut. These conservative checks can produce false negatives; ambiguity costs the normal route rather than silence.

The existing canonical routing callback still runs. It verifies the captured processing scope, checks cancellation, then applies the cheap classifier **before model initialization/inference**. The generic Work conversational router also validates reaction decisions. A model-supplied reaction must match the server-side eligibility, canonical kind/text and empty target/instruction exactly. No routing prompt expansion is needed and no resident classifier model is added.

Both Ollama and direct API providers retain their existing setup/admission gates. Provider selection, credentials, local-runtime installation and defaults are unchanged. A reaction is not an offline model-enrollment bypass. Tests exercise local/cloud callback paths without initializing a model, and an actual native-to-Go loopback fixture observes zero provider requests.

## Durable outcome and compatibility

There is **no new dialogue state or top-level stored field**. A reaction is retained as:

```json
{"state":"answered","response":"You're welcome.","decision":{"kind":"reply","text":"You're welcome.","reaction":"appreciation"}}
```

The existing version-1 envelope also retains its original conversation, turn/client identity, sequence, captured options and monotonically updated revision. Its strict reader is unchanged. Nonempty fallback text meets the ordinary-answer invariant; no empty answer is fabricated. Old Go readers can read the opaque decision metadata, and older native/terminal clients still render `response`. The old terminal component already renders that field without requiring knowledge of reactions. Unknown future reaction metadata degrades to text in the new native UI too.

The public Go dialogue projection adds `revision` and optional bounded reaction metadata: version, conversation ID, turn ID, exact revision, kind, emoji and presentation level. It emits this only for an answered turn with a canonical valid retained decision, matching response and safe captured inputs. Failed/routing/received/interrupted/unconfirmed states have no reaction. No private captured options, credential pins, tool arguments or approval tokens are projected.

The final routing callback additionally checks the captured routing revision, state and original text before committing. An external writer's newer terminal state is inspected rather than overwritten. Existing store locking/CAS, immutable admissions, retry recognition, queue separation and no-replay rules remain in control. Returning a nonce or receiving an admission acknowledgement is not enough to show a reaction; native requires the confirmed Go outcome.

Persistence/restart tests cover actual version-1 files, duplicate retries, competing outcomes, stale revisions, storage-write failure, conversation isolation, lost/incomplete routing and older dialogue pages. A 104-turn fixture reloads its older page without inference or new task dispatch. No new database, native reaction archive or native decision engine was added.

## Reaction vocabulary and preference

| Kind | Emoji | Native level | Truthful saved fallback |
|---|---|---|---|
| Appreciation | ❤️ | Subtle | You're welcome. |
| Laughter | 😂 | Subtle | Haha! |
| Praise | 🙌 | Subtle | Thanks for the kind words. |
| Farewell | 👋 | Subtle | Take care! |
| Greeting | 👋 | Expressive | Hello! |

No completion checkmark, generic approval-like thumbs-up or invented emotional/action outcome is used. Reactions acknowledge conversation only. Their meaning is not task success, authorization, a provider receipt or a sent external message.

Settings adds **Quick Message Reactions: Off / Subtle / Expressive**, default Subtle. This is a display policy over the same retained Go outcome. Off displays the ordinary saved acknowledgement; Subtle uses the four conservative acknowledgement categories; Expressive additionally uses greeting chips. It does not broaden task routing, permissions or inference policy. The preference uses its own native UserDefaults key and survives reopening; streaming motion, presence/GIF modes, Animate Reactions, desktop visibility and Tuck Away remain independent.

## Native binding, layout and search

Each immutable `BridgeMessage` validates and caches its optional reaction binding once when projected. Validation checks the conversation/turn/row identity, revision, answered state, nonempty canonical fallback, allowlisted kind/emoji/level, and absence of issue/task/input references. It never infers eligibility from message prose. The value cache has the message's own lifetime; no global owner cache or reaction task exists.

The original user bubble/content and stable ID remain intact. A small native Unicode emoji capsule sits against its lower trailing boundary; 12 pt reserved row spacing keeps insertion from changing the row height. It is passive, nonfocusable and click-through. Existing text selection remains on the original bubble, and review/approval surfaces are not involved.

A response row is hidden **only** when its exact dialogue response ID and user reference match a validated displayed reaction, its role is Gator and it is ordinary prose. Unrelated/structured/review messages remain visible even if a malformed fixture supplies a matching reference field. This was first reproduced by a failing negative test, then corrected; the failure is retained. Actual Go projections never emit that malformed case.

Find indexes the same presented message list. It still finds original user text; a hidden fallback cannot become an unrevealable search result. Turning reactions Off restores the normal acknowledgement row and searchability. If a reader's saved anchor refers to a now-hidden acknowledgement, the presentation maps it to that same original user turn without rewriting the stored reading-position intent. Cross-conversation prefixes remain distinct. A reaction update does not touch an unrelated streaming cursor or replay a completed response.

## Motion and accessibility

Chips are intentionally **static** for all modes in this increment. There is no entrance task, repeat animation, timer, frame cache, network image or GIF. Historical content therefore never replays motion, and Reduce Motion also has no reaction motion to suppress. The chip explicitly disables inherited animation transactions.

Accessible labels describe the reaction, with a concise hint that it is acknowledgement only, not approval or task completion. No live announcements are posted. Existing text/status/review labels remain authoritative; the presence emoji/GIF system is separate. Headless native layout/appearance tests cover light/dark and high-contrast color variants, full/compact-like widths and normal/disabled-animation transactions. Physical VoiceOver/IME, operator keyboard/focus and the system Reduce Motion preference variant remain NOT TESTED.

Headless renders use the actual production `MessageRow`, with an unstarted disposable coordinator:

- [Compact-like light](evidence/p04e-2/rows-380-light.png), [dark](evidence/p04e-2/rows-380-dark.png).
- [Full-width light](evidence/p04e-2/rows-780-light.png), [dark](evidence/p04e-2/rows-780-dark.png).

These are NSHostingView component renders, not packaged-app screenshots or recordings. No desktop input or ten-cycle native acceptance was attempted.

## Resource methodology and known limits

Fresh-process comparisons use the same debug native test executable, host, 1,000 user messages, viewport and private preference domain. Both modes retain the same fixture source dataset; the reference omits chip metadata from the displayed user rows, while the candidate decorates them. This isolates presentation overhead, not all durable metadata storage. The sampler uses the calibrated R2 Mach timebase and reports RSS and physical footprint separately; CPU is percent of one core.

Each process includes empty host, static visible rows, 12 preference toggles at 150 ms (explicit stress), settled, hidden-component and released-content phases. Local fixture variables and the host remain owned until test completion; released-content samples do not prove all framework caches were returned to the OS. There is no actual NSWindow/compositor or user typing/scroll measurement. GPU/energy and full-app performance alongside a local model remain NOT TESTED.

An early long-history test repeatedly decoded identical reaction metadata while filtering. The final implementation caches the validated value with the immutable message; initial projection pays that cost once and subsequent mode/elapsed-state updates reuse it. Intermediate measurements are preserved separately in `pre-cache/`, `pre-final-fallback/` and `pre-strict-binding/`, not mixed into final measurements. The final measured Swift source hash set is retained.

Final resource/classification results and complete verification follow below.

## Acceptance debt and limitations

The historical accepted approval input still has unknown provenance; its old negative native test remains INVALID. The historical approximately 1.4 GB SwiftUI stall/failed guarded Quit, roughly 23–25% pending-review CPU observation and incomplete native lifecycle acceptance remain unresolved. Headless Go negative controls here establish separate facts, not a retrospective explanation or native acceptance pass.

The classifier is deliberately small and English-only. It does not attempt general sentiment, sarcasm detection or multilingual social classification. Unknown/mixed input uses the ordinary router. Existing model-readiness/admission requirements remain. Older clients show text, and native history paging controls were not expanded; reactions survive Go's existing older-page projection. Static chips are a deliberate initial motion policy. Full packaged-native interaction and physical accessibility remain deferred.

## Final resource and inference measurements

Host: Apple M3, 16 GiB, macOS 26.5.2. SwiftPM debug build, macOS 15 minimum retained. Three fresh processes per mode with alternating order. [Raw trial summaries](evidence/p04e-2/resources.json), [comparison](evidence/p04e-2/resource-comparison.json), [reproduction harness](lab/Measure.py), [measured source](evidence/p04e-2/measured-source.json).

Values below are medians of trial medians, for 1,000 user rows. RSS and physical footprint are different metrics. The reference uses the same fixture data and renderer but no displayed chips:

| Metric | Reference | Chips |
|---|---:|---:|
| Static CPU, % one core | 0.000 | 0.000 |
| Preference-switch stress CPU, % one core | 1.414 | 4.949 |
| Settled CPU, % one core | 0.019 | 0.026 |
| Hidden-component CPU, % one core | 0.000 | 0.000 |
| Static RSS, MiB total | 56.805 | 57.664 |
| Static physical footprint, MiB total | 17.383 | 17.649 |
| Settled physical footprint, MiB total | 17.641 | 17.985 |
| Reload/projection, ms | 22.457 | 22.621 |
| Filtering 2,000 projected rows into 1,000 visible user rows, ms | 3.564 | 3.614 |
| Hosted insertion/layout, ms | 18.438 | 22.323 |

Incremental static physical footprint is approximately **0.27 MiB**, and static RSS approximately **0.86 MiB**, above the matched reference. Candidate settled physical footprint is approximately **0.34 MiB** higher. Largest sampled lifetime footprint is **18.05 MiB** candidate versus **17.81 MiB** reference, including the common fixture host/data. These are component-process totals, not whole-Gator memory or model residency. No continuing reaction timer/animation exists; static and hidden-component CPU settle near the sampler floor. A bounded trial does not prove unlimited lifetime behavior or resolve the historical full-app stall.

Final pure Go classification benchmark, three runs: **131.5–133.5 ns/op**, median **131.6 ns/op**, **32 B/op, 1 allocation/op**, for the supplied acknowledgement fixture. [Benchmark log](evidence/p04e-2/classifier-benchmark.log). Context validation still scales with the existing bounded public context; this is not a claim that every context costs exactly that much.

The actual packaged-runtime integration observes **zero HTTP provider requests**, one owner, a version-1 answered turn on disk, the same reaction after restart and an unchanged unsent draft. [Real-Go evidence](evidence/p04e-2/real-go-reaction.json). Its admission-to-confirmed-reaction latency is fixture-measured, not provider first-token latency or native paint latency. Ordinary conversation routing requires a model call; each eligible shortcut avoids that inference. The unit route counter remains zero for 104 acknowledgement turns, while explicit negative examples reach the ordinary scripted route. There is no estimated API price or fabricated monetary savings.

## Verification and failure ledger

All ordinary tests use inert scripts, private profiles and loopback fixtures. Live-test opt-in variables were absent; live provider, physical desktop and platform-specific tests remain skipped. [Opt-in check](evidence/p04e-2/no-live-opt-in.json).

Retained intermediate failures:

- An initial mixed streaming test run omitted the required `GATOR_TEST_BINARY`; it failed at fixture setup, then passed with the supplied isolated runtime.
- A test attempted to override the read-only Reduce Motion environment key. It was corrected to test static layout with normal/disabled-animation transactions; no physical system preference acceptance is claimed.
- The negative unrelated-message/reference test exposed an overly broad fallback filter. The final filter and reading-position mapper require the exact response identity, ordinary prose role and validated original user turn; its unchanged negative assertions now pass.
- The first strict compiler run rejected a redundant nonnil `#require` under an optional assignment context. The final test keeps that same nonnil assertion in a local binding, then assigns the result. No assertion or warning was disabled. Production source/bundle did not change for that test correction.
- Preliminary Go reaction revalidation test used a deliberately stale fixture issue; the second scenario now clears that issue before independently arranging the pending-review race. The production review exclusion was retained.

The first shortcut experiment bypassed the configured router callback. Inspection found that callback owns processing-scope validation; the final implementation always calls it, with the zero-inference branch after its scope/cancellation checks. New command-layer tests verify both local/cloud paths decline a changed scope without model initialization. Those intermediate configurations are not the delivered runtime.

### Complete final checks

| Check | Final result |
|---|---|
| Swift build, macOS 15 package minimum | PASS |
| Complete normal Swift suite | **305 PASS**, 174.149 s test time |
| Complete final strict Swift 6 suite, complete concurrency checking, warnings as errors | **305 PASS**, 144.418 s test time |
| Full host suite, installed Node 26.7.0 | **141 PASS, 0 FAIL, 1 Windows-only SKIP**, 142 total |
| Package Info.plist/Mach-O/resource checks | PASS; deployment minimum 15.0; prior reaction GIFs retained |
| Actual native desktop, physical macOS 15, VoiceOver/IME, external displays | NOT TESTED |
| GPU/energy | NOT TESTED |

Go results below include subtest pass events (top-level counts and every skip are separately listed in [go-counts.json](evidence/p04e-2/go-counts.json)):

| Package | Normal pass / skip / fail | Race pass / skip / fail |
|---|---:|---:|
| `cmd/gator` | 477 / 13 / 0 | Not repeated under race |
| `internal/workapp` | 358 / 0 / 0 | 358 / 0 / 0 |
| `internal/workrun` | 231 / 1 / 0 | 231 / 1 / 0 |
| `internal/worksession` | 100 / 0 / 0 | 100 / 0 / 0 |

The Go skips cover explicitly opted-in live/provider/desktop/platform/helper probes; none was silently treated as a pass. The permission-failure fixture ran unprivileged and passed on this host. The resource-trial hook returns without measuring in an ordinary Swift suite and was separately run six times with explicit before/after flags. No baseline Swift assertion was weakened. P04U streaming tests, exact review/recovery guards, archive/shutdown tests and the 100-spinner/zero-callback regression all pass. [Preserved authority/presentation sources](evidence/p04e-2/preservation.json).

Final commands/exits/times/runtime identity: [verification.json](evidence/p04e-2/verification.json). Logs: [normal Swift](evidence/p04e-2/swift-full.log), [final strict Swift](evidence/p04e-2/swift-full-strict-final.log), [Go packages](evidence/p04e-2/go-all.log), [Go race](evidence/p04e-2/go-race.log), [host](evidence/p04e-2/host-node26.log), [final packaging](evidence/p04e-2/package-delivery.log).

Reproduction uses the already installed supported Node version and the exact delivered Go runtime:

```sh
export PATH=/Users/gongahkia/.local/share/fnm/node-versions/v26.7.0/installation/bin:$PATH
export GATOR_TEST_BINARY=/tmp/Gator-Native-P04E2-Reactions.app/Contents/Resources/gator-runtime
swift build --package-path macos --scratch-path /tmp/gator-native-build
swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel
swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel -Xswiftc -strict-concurrency=complete -Xswiftc -warnings-as-errors
go test -count=1 ./internal/workrun ./internal/worksession ./internal/workapp ./cmd/gator
go test -race -count=1 ./internal/workrun ./internal/worksession ./internal/workapp
npm test --prefix app-host
```

No live-test opt-in variables should be supplied. [Verify.py](lab/Verify.py) records the exact invoked commands; [Measure.py](lab/Measure.py) reproduces hosted measurements. Building again requires a fresh destination because the existing development-app script refuses overwrite.

## Source, binary and changed-file inventory

Production changes:

- `internal/workrun/conversation.go` and new `conversation_acknowledgement.go`: finite whole-message policy, optional reply metadata, exact validation and inspection gating.
- `cmd/gator/work_dialogue.go`: zero-inference branch after the canonical scope/cancellation check.
- `internal/workapp/bridge_uploads.go`, `dialogue.go`, `dialogue_owner.go`: idle acknowledgement admission through the existing dialogue store, captured-revision finish guard, safe metadata projection and nonsecret inspection flag propagation.
- `macos/Sources/GatorCore/BridgeProtocol.swift` and new `NativeMessageReaction.swift`: stable turn binding and immutable validated metadata cache. Spinner comparison is byte-for-byte unchanged.
- `macos/Sources/GatorMac/ConversationView.swift`, `NativeMascotPreferences.swift`, new `NativeMessageReactionView.swift`: static chips, exact fallback filtering, consistent Find/reading presentation and an independent preference. `MessageRow` is module-visible for production-component tests.
- `app-host/contracts/protocol.ts`: optional additive dialogue metadata/inspection fields. Existing terminal response rendering remains unchanged.

Tests: new Go acknowledgement/router/store/scope tests, new Swift reaction/real-Go tests, and an additive host protocol compatibility test. The original version-1 dialogue store implementation, approvals, provider/credential flows, archive logic, coordinator ownership, native streaming engine, mascot/pet assets and app icons are unchanged. No README edits, signing/notarization, commits, model installs or paid inference were performed.

[Complete changed-path inventory](evidence/p04e-2/changed-files.txt), [final identity](evidence/p04e-2/final-identity.json), [bundle hashes/dependencies](evidence/p04e-2/packaging.json). Source snapshots advanced externally from the initial clean HEAD; no commit or reset was issued here. Intermediate candidates from this tranche were preserved with `Pre-Final` and `Pre-Binding` suffixes, as were all prior accepted/conditional bundles.

Delivered development app: **`/tmp/Gator-Native-P04E2-Reactions.app`**.

Native SHA256: `2f4d94c072526a9df6f8783066fdf06a32ad1f50334bcf4051375ed170ddcb2b`.

Go runtime SHA256: `30ebddf63efb4d5c154fe3a97bb62efc7789c8e0f7e6cf116951f4341ab138e1`.

## Verdict

- **IMPLEMENTATION: PASS**, for the bounded durable Go/native feature and final headless gates.
- **NATIVE ACCEPTANCE: NOT TESTED** for packaged-app interaction; hosted components and loopback Go integration are separately evidenced.
- **RELEASE READINESS: BLOCKED** by the deferred native stability/provenance/physical acceptance conditions. No overall production acceptance is claimed.

Stop after P04E-2. No local/provider management, API-key change, autonomous notification, CUA, P05–P11 or old mascot artwork integration was implemented.
