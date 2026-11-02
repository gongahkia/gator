# Independent chat branches

Implemented and verified against the real Go owner and Electron. All three branch/speech journeys passed after the operator resumed app testing, including the final whole-application run (81 passed / 2 opt-in paid tests skipped). Earlier checkpoints below are historical. Native audible speech remains separate manual acceptance. See [final acceptance](CHAT-UX-ACCEPTANCE.md).

## Durable boundary and publication

`worksession.ForkChatOnce` takes an exact parent conversation, saved revision, run and stable creation request ID. The selected boundary must have a completed retained answer. The desktop owner separately checks that this revision/run is actually represented by an eligible answer in the current conversation; visual list positions and display titles are not authority.

The fork uses the existing chat-creation receipt, profile guard, cross-process creation lock, staged directory and atomic publication. Matching retries return the same new conversation; conflicting reuse fails. If the published chat is later deleted, retry does not resurrect it. An incomplete staged publication is not exposed as a finished chat.

The new conversation has fresh conversation and revision identities. Its copied revision chain ends exactly at the selected boundary, excluding later parent turns. Parent metadata, movable head, execution lease, queued requests and current owner are untouched. A managed desktop navigation request opens the new chat using existing session ownership; it does not move the parent's worker into the child.

Branch creation is bounded to 256 turns and 32 MiB of canonical revision data (8 MiB per revision). Larger or unavailable boundaries fail explicitly rather than silently shortening the branch.

## Inherited context and authority

The branch inherits complete visible user/assistant turns and immutable source, artifact, candidate and uploaded-file references. Historical native replay and tool-call/result groups are omitted as whole groups, preserving valid provider history without copying consumed decisions, action identifiers or provider-specific replay state. Follow-up context uses the existing legacy replay path over the copied complete turns. Historical files are references; their bytes are not newly attached or executed just by creating the branch.

No pending approval, accepted-code permission, raw replay configuration, action receipt, running request, queue, worker, schedule, mutation grant or credential is copied. The new chat starts with an explicitly empty connector selection. Select connections again for subsequent work. Current profile model, voice and privacy preferences remain authoritative, and shared saved memories follow the existing profile settings. A branch is not a new user identity or a new privacy sandbox.

`branch.json` records the parent boundary and the original revision/run/digest for each inherited turn. Nested branches preserve the immediate parent link and resolve retained sources to their original provenance. This is conversation metadata in the existing store, not a session database or authorization registry.

Retained bundle manifests and source bytes still require their existing integrity checks. Original observed-read ranges are used only when the host-bound origin, history record, bundle path and original canonical revision digest agree. Missing, expired or changed original replay cannot invent a read range. Missing retained source bytes remain unavailable. Branching does not guarantee indefinite retention of shared evidence.

## Earlier verification checkpoint

- `go test -race ./internal/worksession -run 'TestBranch|TestCreateChatOnce'`: PASS, 7.370 s. Covers exact boundary, fresh identities, parent preservation while its execution lease is held, independent admission/leases, no inherited authority, nested provenance, stable duplicate creation, concurrent claims, conflicting IDs and no resurrection after deletion. Existing cross-process chat-creation tests exercise the reused publication mechanism.
- `go test -race ./internal/workapp ./internal/worksession`: PASS, 20.326 s / 28.288 s. Includes owner-side wrong-chat/revision/run and stale-background rejection.
- `node --test app-host/tests/policy.test.mjs desktop/tests/source-preview.test.mjs`: 27 PASS. Includes finite branch-frame validation and inherited-source origin/digest checks.
- `go test ./internal/workrun ./cmd/gator`: PASS, 118.406 s / 206.790 s. Includes existing execution and command behavior.
- `go vet ./internal/workapp ./internal/worksession ./cmd/gator`: PASS.
- Desktop TypeScript and build: PASS before the latest test-locator corrections; final checks remain required.
- Existing application-control journeys: 7 PASS, including voice dictation, spoken replies, source selection, frozen queued options and local voice setup. Model/voice services are isolated deterministic fixtures.
- New Electron branch/speech journeys: NOT YET PASS. Initial failures included a numeric queue-count assertion, an insufficient navigation wait and an incorrect citation accessibility label. These test issues were corrected, but the final rerun is deferred while app automation is disabled. The running-parent journey is added and awaits execution.

No paid API evaluation, real microphone/device acceptance or native platform acceptance is claimed.

## Resumed Electron acceptance

After the operator explicitly resumed app tests, all three branch/speech journeys passed against the real Go runtime and private scripted model fixture (10.9 s / 5.7 s / 5.6 s in the focused run; all three also passed in the broader run). This verifies the exact fork boundary, parent-byte preservation, inherited retained source/read coverage, independent drafts, ancestry across restart, selected-response speech/stop/unavailable behavior, and branching while the actual parent worker remains running. Speech API output is simulated; native audio acceptance is not claimed.

The resumed test found that **Open original chat** incorrectly used a host endpoint limited to activity rows. An idle parent is absent from that list. `Sessions.canOpen` now accepts only a projected activity target or the exact ancestry reference supplied by the current native owner, with finite identity validation. Arbitrary IDs, background ancestry and removed ancestry remain denied. The existing native saved-conversation catalogue remains the path for other idle chats; tests no longer misuse the activity endpoint to select them. No worker is transferred, restarted or created merely to inspect ancestry metadata. The existing session navigation still owns opening a selected retained chat.

`node --test app-host/tests/sessions.test.mjs`: 11 PASS after the related plan admission check, including exact ancestry validation, fresh current-owner scope, existing chat ownership/capacity and profile-operation restrictions. The ancestry fix and its initial tests were captured in the separately created commit `8e34376ad`; that commit was preserved without amendment.
