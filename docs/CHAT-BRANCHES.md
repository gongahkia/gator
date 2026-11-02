# Independent chat branches

Backend implementation is in place. Electron acceptance of the new branch menu and per-message speech is still in progress; automated app-clicking journeys were stopped at the operator's request. This document does not claim those journeys passed.

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

## Verification so far

- `go test -race ./internal/worksession -run 'TestBranch|TestCreateChatOnce'`: PASS, 7.370 s. Covers exact boundary, fresh identities, parent preservation while its execution lease is held, independent admission/leases, no inherited authority, nested provenance, stable duplicate creation, concurrent claims, conflicting IDs and no resurrection after deletion. Existing cross-process chat-creation tests exercise the reused publication mechanism.
- `go test -race ./internal/workapp ./internal/worksession`: PASS, 20.326 s / 28.288 s. Includes owner-side wrong-chat/revision/run and stale-background rejection.
- `node --test app-host/tests/policy.test.mjs desktop/tests/source-preview.test.mjs`: 27 PASS. Includes finite branch-frame validation and inherited-source origin/digest checks.
- `go test ./internal/workrun ./cmd/gator`: PASS, 118.406 s / 206.790 s. Includes existing execution and command behavior.
- `go vet ./internal/workapp ./internal/worksession ./cmd/gator`: PASS.
- Desktop TypeScript and build: PASS before the latest test-locator corrections; final checks remain required.
- Existing application-control journeys: 7 PASS, including voice dictation, spoken replies, source selection, frozen queued options and local voice setup. Model/voice services are isolated deterministic fixtures.
- New Electron branch/speech journeys: NOT YET PASS. Initial failures included a numeric queue-count assertion, an insufficient navigation wait and an incorrect citation accessibility label. These test issues were corrected, but the final rerun is deferred while app automation is disabled. The running-parent journey is added and awaits execution.

No paid API evaluation, real microphone/device acceptance or native platform acceptance is claimed.
