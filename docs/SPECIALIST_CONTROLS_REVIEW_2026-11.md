# Desktop specialist controls review — 3 November 2026

Scope: full review of the new live specialist cards, their guidance form and
individual Stop action. React 19, the existing plain CSS tokens and Lucide icons.
This review does not resume the paused competitive audit, Search setup work or
startup/screen-switching performance work.

## Coverage

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | `SpecialistTasks.tsx`, specialist CSS; dark desktop and light 820×740 screenshots | Existing 12/13/14 px hierarchy; wrapping text, explicit status labels and tabular count |
| Surfaces | Cards, assignment disclosure, guidance textarea and focus ring in both themes | Shared surface/radius/outline tokens; 40 px controls; no new overlay |
| Animation | Open/close guidance, focus return, status changes, reduced-motion desktop journey | No new animation; form scrolls instantly into view on intentional opening |
| Icons | Branch, guidance and stop icons alongside text | Existing outline set and 1.5 px regular-text stroke; status never relies on icon/color alone |
| Performance | Card projection, bounded text, acknowledgement handling and existing build output | Up to the existing assignment limit; no polling/model calls from cards; one control acknowledgement at a time; no new dependency |

## Findings resolved

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| MEDIUM | `desktop/src/SpecialistTasks.tsx`, `internal/workapp/bridge_specialists.go` | Specialists were visible through tool activity and slash commands | Named live cards with purpose, assignment, status, Guide and Stop | Make running work and individual controls discoverable |
| MEDIUM | `desktop/src/SpecialistTasks.tsx` | A transport acknowledgement could be mistaken for accepted guidance; snapshot batching could hide one of several acknowledgements | Match the native request ID, serialize control submissions, retain failed/unconfirmed drafts and never automatically resend | Report the actual acceptance boundary without losing user input |
| MEDIUM | `internal/workapp/update.go:253` | Deliberately stopping a worker added a failure warning | Show Stopped without the generic failure warning; retain warnings for unexpected failures | User-requested stopping should not imply a fault |
| MEDIUM | `desktop/src/SpecialistTasks.tsx`, `desktop/src/App.tsx` | Opening guidance could hide its submit action; live transcript updates could move the form away | Scroll the form into view on opening, pause auto-scroll while editing, restore focus on closing, use existing unsaved-edit protection | Preserve context and keyboard access while new activity arrives |
| MEDIUM | `desktop/src/SpecialistTasks.tsx` | Finishing a worker or parent could unmount an unsent guidance draft | Keep the text in the current view with Move to composer and Discard actions; never send it automatically | Task completion should not erase text the user is still editing |
| LOW | `desktop/src/styles.css` | Initial disclosure target was under 40 px; transient queued/stop notices survived terminal status | Use 40 px disclosures/buttons and hide transient notices once a worker ends | Keep target sizes usable and status concise |

The native action handler also checks the active conversation/run generation and
worker, bounds guidance to 8 KiB, rejects stale controls, deduplicates exact
requests, and retains the reviewed-plan steering restriction. Its renderer
projection excludes worker prompts, configuration and native/private capture
state. The supervisor remains the authority for actual liveness and cancellation.

## Considered but rejected

| Location | Candidate | Rejected because |
| --- | --- | --- |
| Specialist cards | Render full worker state, prompts and configuration | The user needs status and assignment; private execution state is unnecessary in the renderer |
| Specialist section | Separate agent-management screen | Chat already creates, saves, lists, reuses and deletes definitions; the remaining gap was live work visibility |
| Status labels | Animate every worker transition | Repeated activity should remain readable; explicit labels provide enough feedback |
| Guidance | Retry automatically after timeout | The original action may already have been accepted; retries risk duplicate instructions |

## Verification

- `go test -race ./internal/orchestrator ./internal/workapp ./internal/workrun`
  passed. After the cancellation-warning change, `go test -race
  ./internal/workapp` passed again.
- `go test ./cmd/gator -run 'Test(UIBridge|BridgeKey|.*[Aa]gent)' -count=1`
  passed; `go vet` passed for those four Go packages.
- `npm run check`, desktop runtime/UI builds and `npm test` passed. The desktop
  suite reported 31 passed/1 existing skip; application-host reported 81 passed/1
  existing platform skip. Existing CSS-highlight, Lucide directive and bundle-size
  build warnings remain.
- Real Electron journeys cover create/run/save, restart, new-chat reuse and
  deletion, plus two concurrent workers, guidance at the next model boundary,
  actual evidence reading, individual cancellation of an in-flight model call,
  parent completion, retained task records and rejection of a finished-run
  control. They also cover cancel/reopen draft retention, focus return, keyboard
  submission, draft recovery after worker/parent completion, light/dark layouts,
  820×740 layout and reduced motion. Reproduce:
  `GATOR_E2E_BACKGROUND=1 npx playwright test specialist-controls.spec.ts
  dynamic-agents.spec.ts --workers=1 --retries=0
  --output=node_modules/.cache/specialist-controls-release` from `desktop/`.
- Full `go test ./internal/workapp ./internal/orchestrator ./cmd/gator` has one
  unrelated failure in the paused Search work: `TestUsageDisplayUsesSharedAccountingAndKeepsReviewedPolicy`
  expects seven usage categories, while the working tree now exposes eight.
  Agent packages pass. That test and the paused Search changes are left untouched.
- An isolated checkout of exactly the staged specialist changes passed
  `go test ./cmd/gator ./internal/workapp ./internal/orchestrator
  ./internal/workrun`, including the full CLI suite (191.7 s), without the paused
  Search edits. This verifies that the feature commit stands on its own.
- No paid model or search calls were made for this follow-up. Local HTTP fixtures
  drive actual Electron and native Go execution; this verifies the interaction,
  not specialist quality or parity with another product.

Verdict: **Approve for the inspected specialist interface**. Not verified here:
Windows/Linux desktop rendering, a full assistive-technology audit, delivery of
steering by each optional external CLI adapter, and long-duration production
latency. The unrelated paused usage-test failure remains a whole-worktree check
failure, rather than evidence of a clean full test suite.
