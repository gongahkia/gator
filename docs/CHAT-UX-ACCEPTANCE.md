# Chat UX final automated acceptance

Automated integration: **PASS** for the five patterns in [the implementation plan](CHAT-UX-PATTERNS-PLAN.md). The real Electron application and Go backend were exercised with disposable profiles, scripted model replies and private HTTP services. This is not physical-device, native audio or real-account acceptance. The operator explicitly resumed app-opening tests after the earlier temporary hold.

## Shipped functionality

| Pattern | Connected behavior | Boundary |
| --- | --- | --- |
| Sources | Composer picker, selected chips, per-chat selection, account/authentication/access state, refresh and existing Connections setup | Only backend-supported integrations; saved credentials are not proof of checked access. Queued/running requests retain their frozen selections. Google sign-in depends on host OAuth configuration. |
| Citations | Number/name chips, bounded hover/focus excerpts and exact retained-source sidebar; local files, observed web previews and connected evidence | Run/manifest/digest checks remain authoritative. Cited passages differ from actual tool reads. Ambiguous, expired, tampered and unavailable references fail honestly. PDF page jumps are unsupported without trustworthy extraction mapping. |
| Activity | Ordered public narration and observed tool metadata, domain chips, changed paths/diff links and recorded check results | No private model reasoning, raw argv, unrestricted terminal output or inferred successful steps. Failed/partial/cancelled execution stays distinct from verified results. |
| Branches and speech | Exact durable new-chat forks, ancestry navigation, independent drafts, selected-message read aloud and stop/unavailable feedback | No automatic model call, replay, worker transfer or copied mutation authority. Historical replay/tool groups are omitted as complete groups. Speech API is simulated in automated tests. |
| Optional Plan first | Tool-free bounded preparation, editable review, revision/digest-bound approval, durable admission, scope enforcement, restart/cancel handling and actual Work-result links | Ordinary chat stays ungated. Proposed steps are intentions, not verified completion. Approval does not replace exact command/service approvals, artifact verification, configured usage limits or terminal grants. |

The prior coding and local Whisper changes were verified and committed separately. No new UI framework, daemon, authentication system, database or general workflow engine was introduced. Electron remains the product surface. Native coding-agent semantic controls remain separate from raw external-terminal interaction. No Ghostty integration was added.

## Backend and security decisions

- Source selection uses existing native account descriptors, private fingerprints, bounded access-check freshness and conversation compare-and-swap. Unknown acknowledgements block submission until refreshed; they do not silently authorize broader sources.
- Citations reuse sealed Work evidence and retained snapshot bytes. Metadata/excerpt reads revalidate owner and exact identity, use bounded safe text and cannot start model execution. Late responses cannot overwrite another selected reference. Retrieved HTML-looking text is never interpreted as HTML.
- Observable activity exposes allowlisted display metadata from actual events. Tool/candidate evidence determines which inspector may open; narration does not manufacture a changed file or a passed check.
- Branch publication reuses the existing cross-process creation lock, stable creation receipt, staging and atomic publication. Parent head, active lease and requests are untouched. The current owner's exact ancestry can be opened; arbitrary browser-supplied conversation identities remain denied.
- Plans reuse the `worksession` admission journal and existing execution lease. Exact conversation/task/revision/digest, frozen prompt/options/sources/context and current owner are checked. Editing invalidates approval. The durable dispatch claim precedes implementation; interruption never enters automatic replay.
- Plan scope gates run before tool effects and preserve the existing command, connected-service, artifact and verification protections. Plan-bound new steering is denied; a changed task requires a new reviewed request. Stop and supported exact interaction decisions keep their existing semantics.
- Existing T0–T4 device grants, independent Work/tmux authority, durable receipts and conservative unknown outcomes remain intact. Terminal contents do not automatically enter model context or persistent memory through this UI.

## Final verification

The full suite was run at `e065bc071`, including the prior retained implementation commits. The only subsequent production change restores scoped decimal list markers and an accessible list name. Its final TypeScript/build and four plan journeys passed afterward. Broad suites were not repeated for this isolated visual change.

| Command / scope | Actual result |
| --- | --- |
| `env -u GATOR_LIVE_UI_ENDPOINT -u GATOR_LIVE_UI_TOKEN -u GATOR_LIVE_UI_SUITE GATOR_E2E_BACKGROUND=1 npm run test:e2e --prefix desktop -- --max-failures=3` | **81 passed, 2 skipped, 9.9 min**; real Electron/Go, no paid requests |
| Same environment, `npm run test:e2e --prefix desktop -- task-plans.spec.ts --max-failures=1` after final list polish | **4 passed, 18.2 s**; includes visible-numbered-list assertions, exact edit/approval, restart/cancel, scope denial and later pending exact command review |
| `go test -p 2 ./...` | **72 packages passed, 11 without tests**, no failures; `cmd/gator` 204.708 s, `workrun` 112.977 s, `workapp` 25.174 s |
| `go vet ./...` | **PASS**, exit 0 |
| `go test -race ./internal/workapp ./internal/worksession ./internal/workrun ./internal/workactivity ./internal/executiontarget/...` | **7 packages passed**; final confirmation workapp 23.153 s, remaining packages cached after passing runs |
| `go test -race ./internal/workrun -run 'TestTaskPlan\|TestServiceApproval\|TestExecutorConnectedRequestNeedsNoInventedOutputFile\|TestExecutorRetainsUnknownExternalOutcomeWithoutDerivingRetryLearning'` | **PASS**, 8.134 s; exact command/service preview, deny/allow/unknown, consumed review and no receipt-read replay |
| `go test -race ./internal/workapp ./internal/worksession ./cmd/gator -run 'TestTaskPlan\|TestDesktopTaskPlan'` | **PASS**, 6.018 / 7.496 / 2.882 s |
| `npm test --prefix desktop` (includes `npm test --prefix app-host`) | Desktop **31 passed / 1 skipped**; application host **80 passed / 1 skipped** |
| `npm run check --prefix desktop` | **PASS** on final UI/test changes |
| `npm run build --prefix desktop` | **PASS**, private Go runtime and UI; `npm run build:ui --prefix desktop` passed again after final polish |
| `GATOR_TEST_BINARY="$PWD/desktop/runtime/gator-runtime" node --test companion/*.test.mjs` | **37 passed / 5 skipped / 0 cancelled**, 45.650 s; includes headless Chromium auth, recovery, Work inspection/semantic controls and external-target journeys |
| Formatting and `git diff --check` | **PASS**; changed frontend uses existing project conventions; no additional Go changes after the passing Go/vet checks |

The resumed Electron suite covers all three branches/speech journeys, all four plans, all six citation/link journeys, both activity journeys, all seven deterministic coding journeys, sources, voice setup, attachments, memory, follow-ups, scrolling, restart, existing conversation behavior and startup without a standalone Gator/Node on PATH. The coding journeys perform real edits/checks against disposable files; scripted model replies are not evidence of commercial-model coding quality.

Final light narrow plan/branch and dark plan editor screenshots were inspected. Plan numbering was corrected after visual review. Keyboard Escape/focus return, bounded modal scrolling, stale selection handling and actual theme selection are exercised. Existing motion checks inspect 10% playback, reversal and reduced motion. See [application test isolation](CHAT-UX-TESTING.md) and the [plan review](CHAT-TASK-PLANS.md).

### Skips and retained failed attempts

- Electron skips: the two explicitly enabled live-model/live-coding API evaluations. No further paid API use occurred; the prior cumulative budget ledger was not reset or modified.
- Node skips: the opt-in panel-secret Electron journey and native Windows argv handoff.
- Companion skips: two opt-in isolated real-tmux fixtures, native Linux supervisor/user-manager fixtures, and native macOS launchd fixture. No ordinary user terminal was inspected or controlled.
- Bun is unavailable and was not installed. No Bun-dependent runtime checks were run. The retired OpenTUI is not reintroduced or claimed as a second product surface. Existing non-fatal bundler warnings (`::highlight` syntax and dependency module directives), npm user-config warnings and the test color-environment warning remain; the builds and checks exit successfully.
- The first broad resumed UI attempt reported **70 passed / 5 failed / 8 unexecuted** at its failure limit. Draft/inline/follow-up/focus cases were investigated; the final complete run passed all of them. The fixture driver now covers windows created after asynchronous backend startup so background tests remain unfocusable. A citation test now leaves/re-enters focus after resize, and the branch test waits for actual owner cleanup rather than assuming rendered final text means idle. No production execution timeout or lifetime was weakened.
- Earlier concurrent-load checks hit one companion health timeout and an existing Work Git-command deadline. Isolated reruns and the complete final companion/race confirmations passed. These earlier failures remain recorded in the stage documents; they were not counted as successful first attempts.

## Commit record

These commits are retained in order. Some externally created commits captured the shared implementation while work continued; none was amended or rewritten. The final acceptance-document commit can be identified by its subject in `git log`.

| Commit | Purpose |
| --- | --- |
| `2e7c59646` | Preserve coding project context and verification evidence |
| `243802793` | Local Whisper dictation and explicit model downloads |
| `b878c7456` | Scoped composer source selection |
| `1e1249585` | Match source selection to native connector limits |
| `7b553abf8` | Retained-source citation implementation (external commit) |
| `5eb0f07d3` | Citation interactions and verification records |
| `ed7f9faa4` | Keep indexed conversations visible during rename races |
| `a72953345` | Bounded observed sources, paths and check results |
| `6a938ee59` | Durable independent branches from saved turns |
| `187ef7497` | Branch bridge/UI/speech and initial task-plan store (external commit) |
| `1a20516d8` | Durable reviewed task-plan admission and scope gates |
| `42c8a5897` | Plan owner, typed bridge and initial card wiring (external commit) |
| `2d7bec033` | Frozen plan limits and queued/restart recovery |
| `9ae1bc75b` | Exact review UI and retained Work activity linking |
| `524746148` | Exact-action approval regression after plan admission |
| `8e34376ad` | Exact host-bound ancestry navigation and tests (external commit) |
| `0326758bb` | Outcome accessible label, plan admission capacity and resumed evidence (external commit) |
| `e065bc071` | Late-window fixture isolation and actual owner-completion waits |
| `8aec3d526` | Visible numbered plan steps, accessible list names, final four-journey acceptance and plan review record |

The consolidated acceptance records are committed separately after these entries. No generated screenshots, binaries, test state, credentials, `.env` data or paid evaluation results are staged. The final handoff reports the exact final commit hashes and worktree status.

## Manual acceptance and limitations

No important automated integration criterion remains blocked. The following checks are separate release/manual acceptance and are **NOT VERIFIED** by these results:

1. Physical microphone recording, native OS speech output and a real downloaded Whisper model on each supported OS. Automated voice tests use controlled local services/speech APIs.
2. Real external-account OAuth/consent and live connector permissions. Backend support is not a claim that every brand in the reference screenshots is a turnkey integration.
3. Physical iPhone connectivity, browser background/reconnect behavior and end-to-end device revocation. Headless Chromium is not a physical phone.
4. Signed/installed application acceptance and native Linux/WSL/Windows runtime behavior. macOS fixture success is not those platforms' acceptance.
5. Native screen-reader and ordinary OS keyboard/focus behavior. Background regression windows deliberately cannot receive physical keystrokes; renderer automation still exercises the actual UI.
6. Opt-in real tmux fixtures were not rerun for this chat-UX goal. Existing reviewed-backend alias, preflight/native-dispatch race, local-keyboard and unverifiable foreground-program outcome limitations remain documented in the execution-target records.

PDF page mapping, branch merging, a universal workflow builder, reliable total dollar estimates and per-step verification without execution evidence are explicitly unsupported rather than simulated. Dynamic integration setup and desktop control are unsupported in plan-first mode. No new live-model quality claim is made. There is no automatic next feature tranche, push or paid evaluation.
