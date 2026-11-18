# P04 — MD03 native provider credentials

## Security design before implementation

Baseline HEAD: `d0de3963f56216d78cb17f613162abfc3252e08c`, clean worktree. Accepted P03 bundle is preserved. This increment is MD03 only.

Boundary: native secure entry → finite local NDJSON save request → canonical Go cloud validation → `saveTUICloudModelConfiguration` → existing `auth.Store` and `config.Store.Update` → request-specific, non-secret completion and status reread. No second authentication store, Keychain record, OAuth implementation, engine or live request is added.

A read-only review request carries configuration fields and a replace/retain intent, never the entered key. Go retains one bounded, short-lived opaque review identity with its settings/credential expectations. The exact provider/default/model/destination and add/replace/retain/removal intent are shown before explicit Save. Only Save carries a new key. Review identities are random, not credential fingerprints.

The existing callback selects the future default provider/model as well as saving its endpoint/options; the review must state this. Reviewed native changes use the existing cloud setup lock, credential comparison and settings Update lock. Failed settings publication retains compare-before-rollback behavior. Removal uses the same Go credential authority with reviewed comparison; it leaves settings, environment and unrelated providers intact. There is no invented cross-file crash journal.

Secrets remain transient in secure UI input and the required local save operation. They never enter drafts, chat/history, prompts, task traces, argv, environment mutations, status/review/result DTOs, logs, tooltips, screenshots or clipboard output. UI input is cleared on dispatch, cancellation, selection/navigation away and failure. Clearing Swift String is not a physical-memory-erasure guarantee.

The auxiliary native window uses a pinned existing managed Go owner and finite typed commands, not the terminal panel or a new owner. It does not replace the current conversation or interrupt Work. Existing captured RunOptions remain authoritative; confirmed settings refresh applies to future submissions. Quit closes admission synchronously, awaits an already accepted setup action, then uses the existing durability/shutdown transaction.

Status means configured and/or locally ready, never live verified. No network probe, discovery, vendor CLI credential import, OAuth handoff or paid inference occurs. Unsafe endpoint userinfo/query data is not projected, and destination changes require explicit override/default selection and review. Built-in provider/options allowlisting and canonical URL/adapter validation remain Go-owned.

Implementation, verification, precise provider matrix and remaining limits will be recorded below.

## 1. Existing authority and actual supported authentication

The current provider set is 38 built-in direct adapters (`internal/model/provider_sets.go`, `provider.go`). Each supports the existing Gator API-key setup. The terminal `/model` configuration form (`internal/modelcatalog/cloud_model_setup.go`) and `cmd/gator/cloud_model_config.go` already implement provider validation, API-key storage and future default selection. `cmd/gator/command_login.go` also exposes API-key enrollment; native never invokes its secret-bearing CLI argument mode.

| Provider IDs | Native authentication | Additional supported setup fields |
|---|---|---|
| `azure-openai`, `azure-openai-responses` | Gator API key | Deployment/model, resource/full endpoint, Azure API version; canonical Go URL expansion |
| `cloudflare-workers-ai` | Gator API key | Model, endpoint mode, account ID |
| `cloudflare-ai-gateway` | Gator API key | Model, endpoint mode, account ID, gateway ID, existing protocol enum (`openai-responses`, `anthropic-messages`, `workers-ai-chat-completions`) |
| `radius` | Gator API key | Model, endpoint mode, existing gateway URL |
| `openai`, `anthropic`, `gemini`, `mistral`, `xai`, `groq`, `openrouter`, `together`, `fireworks`, `deepseek`, `cerebras`, `nvidia`, `huggingface`, `moonshotai`, `zai`, `zai-coding-cn`, `minimax`, `minimax-cn`, `baseten`, `vercel-ai-gateway`, `ant-ling`, `xiaomi`, `moonshotai-cn`, `qwen-token-plan`, `qwen-token-plan-cn`, `qwen-token-plan-individual`, `xiaomi-token-plan-cn`, `xiaomi-token-plan-ams`, `xiaomi-token-plan-sgp`, `openai-compatible`, `kimi-coding`, `opencode`, `opencode-go` | Gator API key | Model and explicit endpoint mode |

“Built-in OpenAI-compatible” uses the existing adapter; adding a custom provider definition is excluded. Historical inventory text mentions external sign-in jobs. The **current model provider allowlist** has direct API-key adapters; this increment does not invent vendor CLI/OAuth enrollment. Existing bearer/OAuth credential metadata can be shown and an API-key replacement/removal can be explicitly reviewed, but those authentication methods cannot be enrolled/refreshed here. Installed Pi/OpenCode/vendor tools do not grant credential-sharing authority. Connector/account OAuth, MD04 custom definitions and MD05 local runtimes remain excluded.

Credential precedence remains `model`'s explicit request configuration → Gator credential store → environment. Native neither changes the environment nor reads another application's store. Ambient-source status reflects the Go owner's captured process environment; changing an environment outside that process does not rewrite it.

## 2. Native architecture and lifecycle

`ProviderSetupView` is a singleton auxiliary native window, with an ordinary provider List and scrollable details. The full-window toolbar, Conversation menu and compact More menu expose **Models and Providers…**; the notch is unchanged. Return to Chat restores the existing conversation window. The existing ordinary model/effort controls remain available.

`NativeProviderSetup` owns bounded **non-secret** presentation data, non-secret unsaved forms, one selected provider, a review, and one operation Task. The view owns the transient secure entry. Changing provider/editor generation, cancelling, dispatching a save, dismissing or failing clears the editor. Non-secret form input is retained for correction. Failure requires a fresh status refresh and explicit review, never an automatic save retry.

`ManagedSessionCoordinator.showProviders` captures an existing owner's reference/generation and pins it while presented, busy or awaiting reconciliation. No management owner is spawned, no terminal panel is opened and no active conversation is replaced. Navigating to another chat is allowed; the initiating owner remains the local IPC endpoint. All command/result identities remain checked against that endpoint. A late result refreshes shared metadata without changing the selected provider or foreground chat; confirmations name the provider actually changed.

The existing synchronous all-owner closing barrier remains first in shutdown. Quit waits for the setup Task before the existing draft durability/process shutdown sequence. The Go restart-readiness gate also refuses restart while a provider callback is pending. Dismissing the window after submission does not imply cancellation of an admitted write.

## 3. Finite bridge contract

New host-only `native_provider` command, with UUID request identity and one of:

| Action | Contents | Authority/completion |
|---|---|---|
| `status` | Request ID | Non-secret Go provider rows; invalidates old review |
| `review` | Exact allowlisted provider, non-secret form, add/replace-vs-retain intent | Existing canonical Go form validation; opaque review ID |
| `review_remove` | Exact provider | Review of currently stored credential; no mutation |
| `save` | Exact provider/review ID, transient new key or empty retain | Canonical reviewed cloud callback; exact result with `confirmed` |
| `remove` | Exact provider/review ID | Reviewed existing auth-store deletion; exact result |
| `cancel` | Request ID | Discards the Go review; no mutation |

`NativeProviderRequest` is not exposed to native views and has a redacted debug description. Its encoder includes `api_key` only for Save. It never emits a chat/submission/slash command. A transport `action_applied` only acknowledges receipt; Swift waits for the exact request/action/provider result and requires `confirmed` for mutation success. Native waits up to 20 seconds, reports uncertainty on timeout/disconnection and does not resend. Go accepts one pending operation per owner and one review slot per backend. Reviews are random (not secret-derived), single-use and valid for five minutes.

`NormalizeNativeCloudSetup` calls the existing terminal form's `configuration()` validator; no Swift URL/parser/auth implementation duplicates it. Native additionally rejects credential-bearing URL userinfo, fragments and query parameters (except Azure `api-version`), unsupported options, control characters/oversized inputs. Endpoint modes are explicit **keep current**, **provider default**, or **override**. Retaining an endpoint does not project its potentially unsafe raw URL. An unsafe retained endpoint must be explicitly replaced; it is not silently sanitized or changed to a regional endpoint.

## 4. Credential security and redaction

The only durable secret is in the existing private `state/gator/auth.json` store. No Keychain record, new database, credential preference or draft field is added. The only new-key transfer is the necessary trusted local NDJSON Save. Keys never become process arguments/environment variables, chat messages, history, model context, runtime prompts, public status/review DTOs or logger calls. `NSSecureTextField` supplies native masking/secure accessibility and is cleared/detached on dismantle. There is no reveal/copy-key affordance. The app's existing Cut/Copy actions remain the native responder-chain operations; secure entry uses the system secure editor.

Go returns only credential presence/kind/expiry, an ambient-source boolean, safe local readiness and processing hostname. It does not return a key fragment, fingerprint, authorization header or raw credential. Known credentials across the Gator store and ambient provider keys are redacted if malformed configuration contains them in public fields. Review validation rejects known credential material in configuration. The native editor also rejects putting the same newly entered key into non-secret form fields before requesting review. As with any configuration editor, an arbitrary unknown string pasted exclusively into a model/URL field cannot universally be identified as a secret; those fields are explicitly labelled non-secret configuration.

Native errors are safe fixed messages. Raw callback/filesystem/configuration errors are not projected. The original credential comparison and settings compensation remain Go-only. Clearing Swift String or Go references does not guarantee physical memory erasure, and review expectations of an already stored credential remain host-local memory until invalidated/released.

## 5. Review, atomicity and concurrency

Save review shows provider, canonical model/deployment, safe processing destination, explicit endpoint mode, provider options/API version and add/replace/retain intent. It states that Save selects the provider/model as the **future default**, in addition to credential/settings storage. No live-verification claim is made.

The native backend snapshots settings and the exact stored credential at review. Save delegates to `saveTUICloudModelConfiguration` with that reviewed expectation. Existing stable `.cloud-setup.lock` serialization and `auth.ReplaceReviewed` protect credential changes across chats/processes. The existing `config.Store.Update` lock encloses a comparison of current settings against the reviewed snapshot and publication. A concurrent settings change fails safely, including unrelated configuration edits (conservative conflict). Empty key means retain; the expectation is still checked.

If settings publication fails after this callback writes a credential, the existing comparison-before-rollback restores only its own write. A newer credential is never blindly restored over. Reviewed removal uses the same cloud setup lock and `ReplaceReviewed(expected, nil)`, leaving defaults, endpoints, ambient sources and unrelated credentials alone. Removal is not “sign out of all accounts.” A fresh explicit review is required after conflict; attempted non-secret input remains available, while the key is cleared and must be re-entered for replacement.

No cross-file crash transaction was invented. Existing credential/settings compensation cannot guarantee atomicity if the process/OS fails between the two separate durable file writes. This inherited limitation is retained and must be reconciled by status/explicit review; no automatic retry is added.

## 6. Configuration/readiness semantics

Provider rows distinguish **not configured**, **configured / needs attention**, and **locally ready**. Gator credential presence, ambient availability, expiry, model and safe processing destination are separate facts. The current future-default provider is marked. For other providers, Go's built-in suggested model is labelled a suggestion, because the existing configuration stores a single default provider/model rather than a per-provider selected-model database.

“Live verified” is never inferred from storing a key or constructing a local adapter. There is no automatic probe, model listing, inference, key test or paid request on open, edit, review, save, cancel, restart or removal. Endpoint overrides are explicitly selected/reviewed; no 401/geography diagnostic changes routing.

## 7. Safety and regression boundaries

Real-Go tests keep the original Work running while storing an inert future default, then release the local fixture and observe completion through its original captured model. Cached owners refresh only future configuration. Drafts, transcript IDs, source identity, approval tokens, run/plan/recovery state and admission are not edited by setup. The existing four-owner/three-work limits, archive transactions, safe Quit and inherited-pipe shutdown remain in place.

P01/P02/P03 source and regression paths remain: transcript Find, exact-message reveal/reading anchors, catalog pagination/search/rename/version checks, workspace picker/canonical Unicode identity and independent conversation sources. No other selected parity feature is implemented.

## 8. Verification ledger

Final headless and desktop results are appended below after testing. All fixture credentials are distinctive inert strings; they are never real credentials. Only controlled fixture output/artifacts are scanned. Private credential files are inspected for actual persistence, not inferred from a masked snapshot.

## 9. Final headless results

| Verification | Final result / evidence |
|---|---|
| Swift build, Swift 6 language mode, macOS 15 minimum | PASS; existing development packaging script invokes the same SwiftPM build. [Build/package log](evidence/p04/build-package.log). No new dependency or language-mode relaxation. |
| Complete sequential Swift suite against the **delivery bundle's Go runtime** | **227 PASS**, 140.623 seconds. [Full log](evidence/p04/swift-full.log). P03 baseline 215 + 12 new P04 tests; no previous tests removed. The existing managed real-Go integration test was also expanded. |
| Strict complete concurrency + warnings-as-errors | **14 selected PASS**, 9.430 seconds, with all production/test sources compiled. [Strict log](evidence/p04/swift-strict.log). Selection is all 12 new tests plus existing managed real-Go and workspace Unicode integration tests; the earlier reported 17-test P03 selection used a different filter. |
| Go regression suites | **7 packages PASS**: `cmd/gator`, `internal/workapp`, `internal/worksession`, `internal/workspace`, `internal/modelcatalog`, `internal/auth`, `internal/config`. [Log](evidence/p04/go-full.log). |
| Go race detector | **3 focused packages PASS**: native bridge/backend, canonical cloud setup, provider normalization. [Log](evidence/p04/go-race.log). |
| Host regression suites | **41 PASS**, zero failures/skips/cancellations. [Log](evidence/p04/host.log). No host production change. |
| Formatting/integrity | `gofmt` applied to changed Go files; `git diff --check` PASS. Protected safety/source files and every accepted P03 bundle file retain their initial hashes. [Integrity and log scan](evidence/p04/verification.json). |
| Fixture leak checks | Nine distinct fixture markers scanned in controlled test logs: zero matches. Real Swift/Go integration scans controlled artifacts outside canonical auth storage; native desktop scan also found zero key occurrences outside that file. No credential fingerprint is published. [Desktop checks](evidence/p04/desktop-final.json). |

Reproducible commands (existing locally installed tools only):

```sh
swift build --package-path macos --scratch-path /tmp/gator-native-build
GATOR_TEST_BINARY=/tmp/Gator-Native-Parity-P04.app/Contents/Resources/gator-runtime \
  swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel
GATOR_TEST_BINARY=/tmp/Gator-Native-Parity-P04.app/Contents/Resources/gator-runtime \
  swift test --package-path macos --scratch-path /tmp/gator-native-build --no-parallel \
  -Xswiftc -strict-concurrency=complete -Xswiftc -warnings-as-errors \
  --filter 'provider|Provider|actualGoManagedDialogue|workspaceActualGoUnicode'
go test ./cmd/gator ./internal/workapp ./internal/worksession ./internal/workspace \
  ./internal/modelcatalog ./internal/auth ./internal/config
go test -race ./cmd/gator ./internal/workapp ./internal/modelcatalog \
  -run 'TestNative|TestUIBridgeNativeProvider|TestCloudSetup|TestSaveTUICloud'
/Users/gongahkia/.local/share/fnm/node-versions/v26.7.0/installation/bin/node --test \
  app-host/tests/native.test.mjs app-host/tests/sessions.test.mjs \
  app-host/tests/drafts.test.mjs app-host/tests/reading-state.test.mjs \
  app-host/tests/backend.test.mjs
GATOR_NATIVE_APP_PATH=/tmp/Gator-Native-Parity-P04.app \
  bash macos/scripts/build-dev-app.sh
git diff --check
```

The package script refuses an existing destination. During acceptance, only this increment's intermediate P04 candidates were preserved under additional `/tmp/Gator-Native-Parity-P04-Pre*.app` names before rebuilding the delivery path. Earlier P01–P03 bundles were not replaced.

Coverage added:

- **Seven Core protocol/presentation/lifecycle tests:** finite encoding and secret-on-Save only; redacted request description; review/cancel/save/removal; failures requiring refresh; stale completion/provider change/dismissal; acknowledgement without confirmed storage; in-flight operation ownership; captured-owner navigation with pending approval; Quit waiting behind the all-owner closing barrier; disconnection, generation replacement and explicit reconciliation; late result identifying its actual provider.
- **Two real-Go integrations:** actual auth/settings persistence, `0600`, retain (including whitespace-only input, matching canonical Go trimming), replacement, removal, profile relaunch, safe destination/status; actual child argv/environment absent the entered key; private draft/history/snapshot artifacts absent the key; two independent native instances rejecting a competing reviewed save. The artifact scan excludes the canonical auth file and compiled seed helper; `/var` and `/private/var` aliases are resolved before comparison.
- **Three AppKit tests:** native secure cell/masking/dismantle clearing; hidden-window Escape equivalent after review replaces secure entry; secure text-field delegate clearing its transient binding before cancellation. These run without ordering a window on screen.
- **Expanded existing real-Go managed-work integration:** save a future provider/model while an original Work request remains held by a loopback fixture; preserve owner count, source and draft, release the fixture and observe original Work completion. No request uses the newly stored inert credential.
- **Go backend/bridge tests:** all canonical direct providers allowlisted; unsupported providers/modes/fields/URLs rejected; Azure and Cloudflare normalization; actual persistence, empty/whitespace retain, replacement/removal and unrelated credential preservation; review expiry/consumption/provider/intent checks; storage failure; known stored/ambient secret redaction; restart blocked while pending; ack distinct from completion; no live HTTP request.
- **Cross-process tests:** an independent Go test-helper process receives fixture data only on stdin, modifies the canonical credential or settings store after review, and the native backend rejects stale save/removal. Two independent native bridge processes also compete from the same reviewed state. Final stored credentials/settings are checked, not only result flags. Existing cloud setup tests cover failed settings publication, concurrent settings updates and compare-before-rollback preserving a concurrent credential replacement.

Development failures were not ignored: initial new test-code compilation errors were corrected; an artifact assertion initially misidentified the canonical auth file through `/private/var` aliasing and was corrected while retaining the scan of every other controlled artifact. A new reconnect test initially attempted New Conversation through a disconnected owner; it was corrected to use explicit reconnect and verify the new generation. The two genuine Escape issues below received focused fixes. No existing assertion was weakened or timeout increased.

## 10. Native acceptance and evidence provenance

All GUI tests used a disposable profile under `/tmp`, seeded conversations, a loopback-only endpoint and an inert API key. No production workspace/credential/account, paid provider request or external action was used. The fixture copy has bundle ID `dev.gator.native.p04.disposable`; executable and Go runtime hashes match the corresponding packaged candidate. Only the fixture Info.plist identity and its own menu-bar/notch preferences differ. No system accessibility/privacy/focus preference was changed.

Final application: `/tmp/Gator-Native-Parity-P04.app`. [Candidate/fixture build hashes](evidence/p04/desktop-builds.json), [storage confirmation](evidence/p04/desktop-storage.json), [final artifact/draft checks](evidence/p04/desktop-final.json).

| Interaction | Observed result |
|---|---|
| Full-window toolbar and Conversation menu | PASS; both open the native auxiliary setup window without changing chat identity. |
| Provider keyboard navigation | PASS; actual Down-arrow events select `openai` in the native outline and scroll the selected row into view. Offscreen AX row-parent lookup was an automation limitation; the controller was corrected, not production List code. |
| Secure entry and keyboard focus | PASS; Tab from endpoint reaches `AXSecureTextField`; the screenshot shows bullets. No reveal affordance or key-containing accessibility label/tooltip is added. |
| Invalid endpoint | PASS; Go rejects it, visible error identifies the configuration problem, Refresh allows repair and non-secret input remains. No auth file was created by rejected review. |
| Review | PASS; provider, canonical future model, processing host, endpoint mode and credential intent are visible; key absent. No storage before explicit Save. |
| Native Save | PASS; visible confirmed status; actual existing auth store matches the fixture key, `0600`; existing config stores the chosen future default/model and loopback endpoint. This is not an inference/key-validity claim. |
| Cancel / Escape | PASS on the affected P04 UI revisions after focused corrections: review Escape returns to editing; active secure-editor Escape clears entry, closes setup and restores composer focus. Explicit Cancel/Return to Chat also pass. Actual stored credential remains unchanged. Both paths have final headless regressions. |
| Compact entry | PASS; menu-bar chat More → Models and Providers opens the same full setup window. Compact composer retains its original unsent draft. No notch dashboard added. |
| Minimum window | PASS at 560×500; provider list/details scroll independently; scrolling details reaches Cancel, Remove, Review and the complete explanatory footer. Return to Chat remains available. |
| Light/dark | PASS visual inspection; native surfaces, typography, secure field, semantic status and existing Gator accent. |
| Return/relaunch | PASS; original conversation selection and exact unsent draft preserved; stored provider status/model/destination restored; no inherited task/approval or inference. |
| Delivery bundle smoke | PASS launch, ready provider projection, review/cancel controls and guarded Quit. A final repeated CG keyboard sequence was refused after external focus changed; it is **not** counted as another PASS. The same cancellation code had passed the earlier controlled packaged UI checks, and both exact final headless regressions pass. Final refinements concerned whitespace retention and the nonvisual reconciliation Quit guard. |

Screenshots were inspected before inclusion. [Final delivery light](evidence/p04/final-light.png) shows the exact delivery build. The other captures record earlier P04 acceptance revisions, with identical relevant layout/masking: [dark](evidence/p04/dark.png), [masked secure input](evidence/p04/secure-light.png), [review](evidence/p04/review-light.png), [error](evidence/p04/error-light.png), [confirmed save](evidence/p04/saved-light.png), [minimum scrolled details](evidence/p04/minimum-bottom-dark.png), [compact draft continuity](evidence/p04/compact-dark.png). They are not presented as screenshots of accepted P03 or as proof that final nonvisual refinements were desktop-retested.

### Reproduced native defects corrected within P04

1. After review removed the secure editor, the window could become first responder and root `onExitCommand` did not handle Escape. Added native Cancel keyboard equivalents to Review Cancel and Return to Chat. The hidden-window regression **fails on an isolated pre-fix copy** and passes after correction: [before](evidence/p04/escape-before.log), [after](evidence/p04/escape-after.log). Production worktree was not reverted to obtain the before result. Packaged-app Escape also passed.
2. Active secure text editing could consume Escape before window cancellation. The `NSTextFieldDelegate` now handles `cancelOperation`, clears both field/binding, then calls the view's cancellation action. [Delegate regression](evidence/p04/secure-cancel.log); actual app subsequently dismissed setup and focused the original composer with credentials/draft unchanged.

Desktop focus changed to Ghostty and Java/ForkedBooter during some attempts. The controller verified target bundle/window/focused element and refused keyboard delivery on mismatch. This is evidence of shared-desktop interference, not evidence of an application focus defect. The controlled window-first-responder Escape reproduction above occurred while Gator remained frontmost and was also reproduced headlessly. No blind repeated input, system preference change or production focus workaround was used.

Physical VoiceOver, IME, password-manager preference variants and exhaustive Full Keyboard Access settings are **NOT TESTED**. Secure accessibility metadata and ordinary keyboard observations are not physical VoiceOver acceptance. No real-provider live verification was requested or performed.

## 11. Changed-file inventory

| Files | Purpose |
|---|---|
| `internal/modelcatalog/native_provider.go` (new), `native_provider_test.go` (new) | Finite non-secret DTOs/transient input, provider field allowlist and canonical terminal form normalization tests. |
| `cmd/gator/native_provider_management.go` (new), `native_provider_management_test.go` (new) | Host-local reviewed setup/removal, status/redaction, configuration/credential CAS, actual-store/cross-process tests. |
| `cmd/gator/cloud_model_config.go`, `model_management.go`, `work_model_status.go` | Optional reviewed credential expectation in the existing callback; reviewed auth-store removal; reuse existing local status for sampled settings. Legacy callers remain compatible. |
| `cmd/gator/work_interactive.go`, `ui_bridge.go`, `ui_bridge_test.go` | Wire existing owner to the typed backend; finite NDJSON dispatch; ack-vs-result/redaction regression. |
| `internal/workapp/bridge_native_provider.go` (new), `app.go`, `bridge.go`, `update.go`, `restart.go` | One pending provider command, exact non-secret completion, future-only preference refresh and restart guard. |
| `macos/Sources/GatorCore/NativeProviderProtocol.swift` (new), `NativeProviderSetup.swift` (new) | Typed requests/results, redacted request description, non-secret state and owned operation lifecycle. |
| `macos/Sources/GatorCore/ManagedSessionProtocol.swift`, `ManagedSessionCoordinator.swift` | Finite host command, captured/pinned owner, future-only cached preference refresh, pending/unreconciled Quit warnings and operation wait behind the existing closing barrier. |
| `macos/Sources/GatorMac/ProviderSetupView.swift` (new), `NativeProviderSecretEntry.swift` (new), `GatorApp.swift`, `ConversationView.swift` | Native window, provider-specific secure form/review, scoped cancellation, scene/menu/toolbar/compact entry. |
| `macos/Tests/GatorCoreTests/NativeProviderTests.swift` (new), `NativeProviderIntegrationTests.swift` (new), `ManagedSessionTests.swift`, `ManagedGoIntegrationTests.swift` | Protocol/state/lifecycle and real Go/store/owner regressions; fixture extensions. |
| `macos/Tests/GatorMacTests/NativeProviderSecretEntryTests.swift` (new) | Headless native masking, keyboard-equivalent and secure-delegate cancellation regressions. |
| This report and `macos/parity/evidence/p04/` | Security design recorded before controls, reproducible fixture/controller, logs, screenshots and integrity evidence. |

No `internal/auth`/`internal/config` storage implementation, JavaScript production code, attachment storage, native archive locking, process transport/shutdown, AppDelegate, P01 reading/search implementation or P03 Unicode resolver code was changed. Those safeguards remain covered by the full suite. No README was edited, dependency installed or git commit invoked. HEAD advanced externally from the initial `d0de3963…` to `12ee4a43…` during work; external captures were preserved rather than reset. Initial/final identities and protected hashes are in [verification.json](evidence/p04/verification.json).

## 12. Remaining limitations and risk classification

- **No known new P0/P1 regression** in the exercised paths. This is a bounded test/source assessment, not proof that all possible defects are absent.
- **Inherited P2:** credential/configuration durability is two canonical files with guarded compensation, not a cross-file crash transaction. Power loss/process death between publications can require inspection/repair. No new database/journal was introduced.
- **Inherited P2:** previously documented conservative owner retirement/recovery pinning remains. Provider presentation/reconciliation can conservatively pin one existing owner, within the existing four-owner limit; it adds no process. No capacity rule is weakened.
- Provider readiness is a local, point-in-time configuration check; ambient environment is captured by each existing owner. A later external change can make displayed metadata stale; explicit Refresh/review handles it. Conservative configuration CAS can require reviewing again after an unrelated settings edit.
- Native endpoints deliberately disallow userinfo, fragments and arbitrary query parameters; only Azure API version query data is supported. A legacy unsafe endpoint requires explicit repair. No automatic regional/default routing change occurs.
- Vendor CLI/OAuth sign-in, custom definitions, local model installation and connector/account permissions remain unavailable here and are clearly labelled. No currently allowlisted direct provider requires a native OAuth handoff for this API-key workflow.
- GUI removal's actual storage mutation is verified headlessly through real Go; desktop Save/review/cancel/status were exercised. Physical accessibility/OS preference variants remain unverified as above. Full live-provider authentication is deliberately **NOT TESTED**, and no key is described as live verified.
- No new P3 cleanup work or unrelated architecture refactor is queued by this increment.

## 13. Delivery and readiness

**MD03 is complete and accepted for the current built-in API-key configuration/enrollment/repair/removal scope.** The explicit unsupported authentication paths and physical operator-only checks above are not claimed as complete. Native Save is Go-confirmed, reviewed changes are conflict-checked, key retention/removal is explicit, and prior chat/task/approval/workspace safety remains covered.

Delivery candidate: `/tmp/Gator-Native-Parity-P04.app`. The accepted `/tmp/Gator-Native-Parity-P03-Accepted.app` is byte-for-byte preserved. Disposable native/Go processes were closed after testing; the fixture state contains only inert credentials. The final repeated keyboard attempt blocked by external focus is recorded as refused, while already passed affected interactions and final headless regressions retain their stated provenance.

The codebase is ready for a separately selected P05 within the documented inherited P2 limits. No P05, other parity capability, signing, notarization or distribution work was started.
