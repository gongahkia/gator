# Gator terminal capability and native parity inventory

Date: 2026-11-17. **Selection inventory only: no feature or parity implementation is authorized or started.** Current source baseline is the same as [the engineering audit](SWIFT-ENGINEERING-AUDIT.md): initial HEAD `13134d5f2c81ae6f4ed8c4bea1948774d15307e6`, externally advanced to `137fb7ea923d14149badbb02e102ffdfb6e34bed` with no intervening changes to audited source/contracts/tests. Existing other-audit evidence under `macos/audits/evidence/uiux/` was left untouched.

## 1. Coverage summary and interpretation

This inventories user outcomes against **actual terminal entry points**, rather than counting slash aliases or every backend DTO as a feature. Related aliases share an ID. Manual editing, requesting a model revision, selecting sources, enrolling accounts, and granting permissions are different workflows and intentionally separate selections. Read/copy/reuse through ordinary native text controls counts as access even without a matching terminal-style menu.

- **FULL** means a reachable native workflow exists for the stated outcome, including a verified conversational tool route where it completes the operation. It is an availability classification, not a defect-free guarantee; engineering findings still apply.
- **PARTIAL** means a real part exists but an essential step/scope is missing. A conversational proposal that only opens a terminal review does not establish completion. An inaccessible private Core setter does not count as native UI parity.
- **MISSING** means no complete reachable native workflow was found. Backend capability alone does not count.
- **INTENTIONALLY DIFFERENT** means the native presentation/host model deliberately differs and should not copy terminal mechanics automatically. These are not folded into the missing count.
- **DEPRECATED** is available as a status, but no retained current terminal outcome below is classified deprecated merely because its older documentation discusses Electron.

Conversational **Yes** is source-verified tool reachability with the same configured model/tool/permission prerequisites as the terminal. It is not a claim that a remote model was called or will choose the right tool reliably. Source/usage/privacy setting tools were inspected in `internal/workrun/app_control.go`, `cmd/gator/work_app_{control,settings,management,followup}.go`. Knowledge/local-plan/suggestion definitions and specialist profile tools were inspected only as needed to establish their boundaries. No new network/service/account action was performed. Features requiring provider repair, an already-paused quota, host file services or unimplemented final review panels are not marked full simply because a model-facing schema exists.

**Counts (one meaningful capability per ID):**

| Total | FULL | PARTIAL | MISSING | INTENTIONALLY DIFFERENT | DEPRECATED |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 105 | 37 | 21 | 40 | 7 | 0 |

Category counts are included below and computed from the same rows. Do not interpret these numbers as percentages of product value: a single file-version service is much larger than a pin button.

| User outcome group | FULL | PARTIAL | MISSING | DIFFERENT |
| --- | ---: | ---: | ---: | ---: |
| Conversations, history and drafts | 6 | 3 | 8 | 0 |
| Queued messages and task control | 3 | 0 | 5 | 0 |
| Models and request configuration | 3 | 1 | 3 | 0 |
| Accounts, sources and workspace context | 1 | 2 | 4 | 0 |
| Following, goals, schedules and updates | 2 | 4 | 2 | 0 |
| Memory, saved knowledge and reusable guidance | 2 | 4 | 1 | 0 |
| Attachments, retained files, editing and export | 2 | 1 | 9 | 0 |
| Usage, budgets and pause controls | 2 | 1 | 0 | 0 |
| Appearance, navigation and keyboard preferences | 0 | 0 | 0 | 4 |
| Voice and speech | 0 | 1 | 2 | 0 |
| Web, browser and computer workflows | 2 | 1 | 1 | 0 |
| Specialists and subagents | 1 | 1 | 0 | 0 |
| Plans and exact action reviews | 5 | 0 | 2 | 0 |
| Failure recovery and uncertain outcomes | 3 | 0 | 1 | 0 |
| Private data, backups and service lifecycle | 2 | 1 | 2 | 0 |
| Application tools and presentation differences | 1 | 1 | 0 | 3 |
| Background checking service lifecycle | 2 | 0 | 0 | 0 |

## 2. Verified entry points, authority and source keys

All paths below are repository-relative. Keys make the per-feature matrices readable while retaining an exact source trail. Commands beginning `/` are terminal actions; names like `catalog-actions` are command-palette/contextual actions, not invented slash aliases.

| Key | Source and entry-point evidence |
| --- | --- |
| T1 | `ui/src/gator/actions.tsx` (`useActions`, drafts, queue, models, catalog, chats, search, prompt-history, task/earlier-message actions); `ui/src/gator/client.ts` (submit, stage, admission/navigation), `ui/src/component/prompt/index.tsx`, command palette/keymap. |
| T2 | `ui/src/gator/product-actions.tsx` (`useProductActions`, planAction/editPlan/reviewPlan, specialists, branch, goalCommitment/commitment); `following.tsx`, `hub.tsx`, `command-actions.tsx`. |
| T3 | Go-advertised commands in `internal/workapp/commands.go:193`, command routing at `:413`; terminal `command-actions.tsx`, `panel.tsx`, `hub.tsx` actually expose them through the current bridge. Includes advanced controls, rather than requiring an identical bespoke frontend widget. |
| T4 | `ui/src/gator/native.tsx` (`useNative`, voice, links, external-editor/continue-external, releases); `app-host/src/voice-session.mjs`, `native.mjs`, `terminal.mjs`; launcher/package/install scripts for terminal distribution. |
| T5 | `ui/src/gator/files.tsx` (`useFiles`, FileEditor open/save/discard/editContent/versions/selectContent); `file-selection.ts`; `app-host/contracts/fileProtocol.ts`. |
| T6 | `ui/src/gator/result-actions.ts`, `receipt-actions.ts`, `outputs.ts`; exact manifest/source/candidate/file/receipt actions, not arbitrary renderer file reads. |
| G1 | `cmd/gator/ui_bridge.go`; `internal/workapp/{bridge,bridge_sessions,bridge_queue,bridge_specialists,bridge_uploads,dialogue,dialogue_owner,dialogue_task_control,run,requests}.go`; Go owns canonical conversation/execution/control state. |
| G2 | `internal/workapp/{conversation_catalog,bridge_conversations,bridge_panel_action}.go`; catalog list identities, organization and indexed scopes. |
| G3 | `internal/workapp/{branches,task_plans,task_plan_revision,request_recovery,request_access,receipts,approval_result}.go`; exact current revision/token/result contracts. |
| G4 | `internal/workapp/{composer_settings,models,controls,preferences,terminal_preferences}.go`; `cmd/gator/work_app_{control,settings,limits,preferences,management}.go`; `internal/workrun/app_control.go:64`. |
| G5 | `internal/workapp/{composer_sources,controls,bridge_panel_action}.go`; existing Go connection/browser/permission control screens exposed by workapp. Selection app mutation in `cmd/gator/work_app_management.go:31` does not enroll/disconnect accounts. |
| G6 | `internal/workapp/{bridge_checks,bridge_goals,chat_updates,chat_checks,followup,overview}.go`; `cmd/gator/work_app_followup.go`; `internal/tools/local_plans.go`, `suggestion.go`. |
| G7 | `internal/workapp/{chat_memory,recall,learnings,commands}.go`; `internal/tools/{knowledge,chat_memory,suggestion}.go`; saved knowledge is not model-generated authority. |
| G8 | `internal/attachment/`; `cmd/gator/{attachments,work_app_management}.go`; existing Go sealed artifact/delivery and manifest checks called by those controllers. |
| H1 | `app-host/src/{application,sessions,backend,drafts,uploads}.mjs`; JS owns terminal-local process/session/draft/upload services. Swift has its own typed host implementation. |
| H2 | `ui/src/gator/` rendering/actions and `app-host/src/{reading-state,native,terminal-preferences,preferences}.mjs`; text selection/clipboard/navigation and terminal presentation state. |
| H3 | `app-host/src/{files,file-content,office,pdf,file-comparison,file-sources,export,source-preview,code-preview}.mjs`; bounded authorized file services, immutable working versions, exact evidence and safe destination writes. |
| H4 | `app-host/src/{voice-session,voice,native}.mjs` plus current Go speech configuration/usage, invoked through the host; no native Swift audio pipeline yet. |
| N1 | `macos/Sources/GatorMac/{GatorApp,AppDelegate,ConversationView,ComposerView,NativeComposer,NotchQuickAccess}.swift`; Core `ManagedSessionCoordinator`, `GatorApplicationState`, `BridgeProtocol`. |
| N2 | Core `NativeDraftStore`, `NativeDraftAttachments`, `NativeUploadStore`; Mac `NativeAttachmentActions`, `NativeDropInput`, `AttachmentSelectionView`. |
| N3 | Core `NativeTaskReview`, `NativePlanExecution`, `NativeRecovery`, `NativeRecoveryPreparation`; Mac exact-review, task-inspector, plan/recovery content and decision-button files. |

All eight `app-host/contracts/` files were inspected: protocol, frontend, files, voice, source preview, code preview, inspection, external. The optional companion/external-tmux contract is not evidence of an ordinary terminal UI entry point; see excluded items below.

### Dependency keys

| Key | Existing dependency / native work boundary |
| --- | --- |
| D1 | Existing bridge, managed owner identity/capacity, canonical Go result and durable client operation handling. Engineering E01–E05 are relevant prerequisites for more mutations. |
| D3 | Existing Go configuration/auth/readiness/usage/settings authority. Native may need secure host UI; credentials must never transit chat. |
| D4 | Existing Go goals/jobs/supervisor, canonical agreements/digests/timezones and explicit local-service lifecycle. Opening a view cannot start checks. |
| D5 | OS helpers/permissions/external-process lifecycle. Actual audio, browser, computer and notification behavior remains operator-dependent. |
| D6 | Existing Go conversation catalog, paging/search/list identities and organization API. |
| D7 | Existing retained manifests/originals/versions and host file/export/evidence authority. Swift currently has uploads, not a native replacement for the JS FileStore. References must account for the distinct native-upload namespace. |
| D8 | Native presentation, AppKit editing/selection, focus/scroll state, shortcuts and accessibility. No new backend service required for pure presentation. |

## 3. Categorized capability matrices

Each category has an availability matrix followed by delivery considerations keyed to the **same IDs**. Together they give all 14 requested attributes. `Native host/UI gap` is “optional” for a full workflow where only convenience UI is absent. Effort estimates apply to the described additional native work, not rebuilding existing Go features: SMALL ≈ localized adapter/view; MEDIUM ≈ several states/reviews/tests; LARGE ≈ new host lifecycle or several connected views/formats. They are relative estimates, not schedules. Expected value is qualitative. The final column records approach and a reason to avoid or limit a port.

### Conversations, history and drafts

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| CH01 | Send messages, stream replies, keep talking during work | T1 chat composer | G1 admission/dialogue | N1 composer/transcript | FULL | Yes; ordinary conversation |
| CH02 | New independent chats and switching through recents/activity | T1 /new, /chats | G1 sessions/catalog + H1 | N1 shared coordinator/Recents | FULL | Navigation is host controlled |
| CH03 | Search all retained conversations by indexed content and scope | T1 /conversations, Ctrl-F in catalog | G2 catalog queries | Recents only | MISSING | No equivalent indexed catalog tool |
| CH04 | Rename a retained conversation | T1 catalog-actions | G2 panel rename | None | MISSING | No verified chat mutation |
| CH05 | Pin/unpin a conversation | T1 catalog-actions | G2 panel pin/unpin | Shows Go-ranked recents; cannot change pins | MISSING | No verified pin tool |
| CH06 | Archive/restore a conversation | T1 catalog-actions, scope picker | G2 panel archive/restore | None | MISSING | No verified archive tool |
| CH07 | Read all older conversation/dialogue history | T1 /earlier-messages | G1 dialogue_page | N1 current projected history; earlier-dialogue notice | PARTIAL | Model context is not an exact history browser |
| CH08 | Search/reveal text inside the current transcript | T1 /search local action | H2 transcript search | None | MISSING | Asking a model is not exact literal search |
| CH09 | Inspect and copy full retained message/result text | T1 Ctrl-E, /actions, /copy-message | H2 result actions | N1 selectable text and complete result disclosures | FULL | Yes, but not necessary |
| CH10 | Reuse earlier message text in an editable draft | T1 /prompt-history | H2 draft staging | N1 select/copy/paste into native editor | FULL | Can quote a prior message; manual path available |
| CH11 | Resume reading position and catch up from first unseen item | T1 /catch-up, /latest | H2 ReadingStore/transcript reader | N1 follow/jump latest; no durable anchors/unseen model | PARTIAL | No |
| CH12 | Branch an independent chat from an exact saved answer | T2 /branch, /original-chat | G3 BranchConversation + H1 | None | MISSING | No branch tool verified |
| CH13 | Browse previous/next retained conversation revision | T3 /revision-back, /revision-forward | G1 retained revision navigation | None | MISSING | No revision-navigation tool verified |
| CH14 | Automatically retain per-conversation unsent drafts/files | T1 composer and restart | H1 DraftStore; native N2 | N2 private archive, revision and uploads | FULL | No need |
| CH15 | List, recover, delete drafts and configure draft retention | T1 /drafts | H1 DraftStore requests | Automatic restoration only | MISSING | No native-draft management tool |
| CH16 | Open safe answer links and reuse links in drafts | T4 /links, link actions | H2 native URL/clipboard helpers | N1 http/https links; standard copy/edit/paste | FULL | Yes; ordinary editable text |
| CH17 | Browse all retained task runs, statuses and individual results | T3 /history | G1 retained Work history | N1 current-owner results; app outputs lists output-bearing runs only | PARTIAL | Can inspect some outputs; not full run-history catalog |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| CH01 | None required | D1 configured model | Captured owner and request ID; no implicit queue | SMALL | Core daily workflow | native GUI; Already present; retain |
| CH02 | None required | D1,D6 | Original owner keeps run, tokens and draft | SMALL | High; multitasking | native GUI; Already present |
| CH03 | Paged catalog state, query/scope and selected identity UI | D6 | Stale list identity and private content scope | MEDIUM | High for growing history | native GUI; Do not substitute model retrieval for exact catalog search |
| CH04 | Exact catalog item/title request and result | D6 | Current panel/list identity; no arbitrary path | SMALL | Medium; organization | native GUI; Optional convenience |
| CH05 | Exact pin mutation and refreshed recents | D6 | Revalidate selected catalog item | SMALL | Medium | native GUI; Optional convenience |
| CH06 | Scope-aware listing and explicit mutation review | D6 | Archive is not deletion; retain work ownership | MEDIUM | High for long-lived profiles | native GUI; Avoid hidden cancellation or data deletion |
| CH07 | Older-page loading while preserving raw approval indexes | D1 | Read-only paging must not replay work | MEDIUM | High for long chats | native GUI; Backend history alone does not establish reachability |
| CH08 | Search index/query/highlights and stable reveal target | D8 | Keep search scoped to original owner | MEDIUM | High | native GUI; Independent of global catalog search |
| CH09 | None required; contextual copy is optional | D8 | Literal structured data; safe links | SMALL | High | native GUI; No identical Copy button is required for parity |
| CH10 | Optional searchable reuse picker only | D8 | Explicit Send; keep current attachments/new draft | SMALL | Medium | native GUI; Workflow exists without a prompt-history button |
| CH11 | Per-owner durable reading anchor and seen identity | D8 | Do not mistake unseen UI state for approval | MEDIUM | Medium for long running work | native GUI; Latest/follow behavior already exists |
| CH12 | Exact revision/run selection, result and owner creation | D1,D6,D7 | No copied approvals, queues, grants or replay | MEDIUM | High for alternatives | hybrid; Do not implement branching by copying visible text alone |
| CH13 | Revision selection/projection with idle-owner rules | D1 | Keep immutable history and current task identity | MEDIUM | Medium | native GUI; Distinct from creating a branch |
| CH14 | None for availability; see engineering E01/E02 | D1 | Native namespace/profile; no automatic pending replay | SMALL | High | native GUI; Do not merge terminal/native archives silently |
| CH15 | Native archive management, retained-ID protections and explicit clearing review | D1 | Do not discard uncertain submissions or selected bytes | MEDIUM | Medium | native GUI; Terminal draft tool does not manage native namespace |
| CH16 | Optional link picker only | D8 | HTTP(S) only, no URL user/password; explicit opening | SMALL | Medium | native GUI; Do not port terminal link menu solely for identical UI |
| CH17 | Paged run-history browser, status filter and exact result navigation | D6,D7 | Read-only run/revision identity; do not resume work | MEDIUM | High for auditing past work | native GUI; Conversation catalog and run history are distinct |

### Queued messages and task control

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| QT01 | Explicitly enqueue a message after current work | T1 /queue-message, queue keybinding | G1 upload_submit.queue_only | None; normal native input remains dialogue | MISSING | Ordinary dialogue is not explicit enqueue |
| QT02 | Edit a queued message | T1 /queue edit | G1 ApplyBridgeQueue | Queue evidence read-only | MISSING | No queue-edit tool verified |
| QT03 | Reorder queued messages | T1 /queue earlier/later; drag move | G1 queue move | None | MISSING | No |
| QT04 | Remove queued work without running it | T1 /queue remove, dequeue/clear aliases | G1 queue removal | None | MISSING | No verified queue-removal tool |
| QT05 | Run/resume a retained interrupted or queued request | T1 /queue run | G1 queue run/execution lease | N3 read-only queued/interrupted evidence | MISSING | No general queue-resume workflow verified |
| QT06 | Guide active work while keeping a conversation | T1 ordinary message, /steer | G1 dialogue route/control | N1 original-owner dialogue dispatch | FULL | Yes, routed by Go |
| QT07 | Stop the selected owner’s current work | T1 cancel key, /cancel | G1 canonical cancel | N1 Stop button/owner method | FULL | Yes when Go routes a stop |
| QT08 | Inspect independent/background task and return to requester | T1 /tasks, item original-task actions | G1 task/peer projections + H1 | N3 task inspector, activity, parent link/notch | FULL | Yes, but retained UI is direct |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| QT01 | Queue-only typed intent and capacity/result handling | D1 | No automatic execution on restoration | MEDIUM | High for sequential requests | native GUI; Keep ordinary conversation semantics |
| QT02 | Exact queue ID editor and matching result | D1 | Retain uploads/reply identity and admission history | SMALL | Medium | native GUI; Depends on visible queue selection, not QT01 implementation |
| QT03 | Ordered queue controls with exact IDs | D1 | Owner-scoped queue; stale target rejection | SMALL | Medium | native GUI; Avoid aliases as separate features |
| QT04 | Exact item removal/review; distinguish discard from Stop | D1 | Keep durable evidence/attachment ownership | SMALL | High when queue is blocked | native GUI; Do not delete retained upload bytes automatically |
| QT05 | Explicit identity-bound resume and request-specific result | D1 | Fresh authority/lease; inspection must not start it | MEDIUM | High for interrupted work | hybrid; Do not implement automatic resume |
| QT06 | Optional deterministic guidance affordance | D1 | Exact parent/turn/run; no implicit approval | SMALL | High | hybrid; Preserve Go routing rather than add another agent loop |
| QT07 | None required; bounded outcome feedback in R04 | D1 | Original owner only; ack is not cancellation receipt | SMALL | High | native GUI; Do not target the newly selected chat after await |
| QT08 | None required | D1 | Inspection has no control/approval authority | SMALL | High | native GUI; Already present; no separate task dashboard needed |

### Models and request configuration

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| MD01 | Choose an already configured provider/model | T1 /models choose model | G4 ComposerAction | N1 configured-model menu | FULL | Yes via gator_app_update |
| MD02 | Choose low/standard/high effort | T1 /models effort | G4 ComposerAction | N1 Effort menu | FULL | Yes via work_effort |
| MD03 | Enroll/repair provider credentials and sign-in | T1 /model provider screens; continue-external | G4 provider forms/external jobs | N1 readiness/terminal setup instructions only | MISSING | Credentials deliberately unavailable to chat tools |
| MD04 | Configure custom providers/endpoints/models | T1 /model custom provider review | G4 custom provider forms | Can select existing configured custom model only | MISSING | Can select existing model; cannot create endpoint |
| MD05 | Install/manage local model runtime and downloads | T1 /model Local model | G4 local setup/external jobs | Existing local model selection only | MISSING | App tools explicitly exclude new downloads |
| MD06 | Inspect full request sources, processing and work limits before send | T1 /request-context, /draft-context | G4 readiness/budget + H2 requestContext | N1 provider description/model/effort; incomplete combined context | PARTIAL | App inspect covers settings, not full private host endpoint view |
| MD07 | Choose work authority mode for this chat | T3 /mode | G1 work options | N1 Work Mode menu | FULL | Mode-related discussion; direct menu available |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| MD01 | None required | D3 | Captured request models stay frozen; actual result | SMALL | High | hybrid; Do not claim configured means authenticated |
| MD02 | None required | D3 | Changes future foreground messages only | SMALL | High | hybrid; Keep task budget distinct from shared quota |
| MD03 | Secure forms/host sign-in handoff/result lifecycle | D3 | Never route secrets through conversation or logs | LARGE | High; first-run usability | native GUI; Needs deliberate host handoff; no raw executable UI API |
| MD04 | Reviewed endpoint/model form and sensitive-data handling | D3 | Endpoint/data-destination review; hidden credentials | LARGE | Medium for advanced users | native GUI; Port only if users need native administration |
| MD05 | Reviewed download/process progress/cancel and readiness | D3 | Download provenance, disk/network use, no auto execution | LARGE | High for private/local workflows | native GUI; Separate from selecting an installed model |
| MD06 | Unified current scope/attachment/budget read-only view | D1,D3 | Do not expose exact endpoints/secrets to model | SMALL | High for informed choices | native GUI; No new authority; render existing projection |
| MD07 | None required | D1 | Inspect/draft/act never bypass exact approval | SMALL | High | native GUI; Existing finite modes should remain finite |

### Accounts, sources and workspace context

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| AS01 | Connect, verify and repair an account’s sign-in | T3 /connections | G5 connection controls/external jobs | N3 recovery reads configured choices; no enrollment | MISSING | Inspect only; no credential/sign-in tool |
| AS02 | Disconnect/remove an existing account | T3 /connections controls | G5 account mutations | None | MISSING | connections app change selects IDs, does not disconnect |
| AS03 | Inspect and change per-operation account permissions | T3 /connections, /permissions | G5 connector PermissionSet | N3 displays recovery operation/access; no general permission editor | PARTIAL | App inspect lists access; cannot grant rights generally |
| AS04 | Select existing connected sources for future messages | T2 /sources | G5 SourceAction; app connections mutation | Reachable via N1 chat app tools; no source picker | FULL | Yes: inspect connections then update exact IDs |
| AS05 | Choose/change current workspace folder | T3 /source; directory picker | G1 workspace resolver/session request | Launch-time native workspace override only | MISSING | No arbitrary workspace selection app tool |
| AS06 | Attach/detach workspace-relative files and refresh snapshot | T3 /attach, /detach, /source-refresh | G1 composer source options | N2 external immutable file uploads; no workspace selection/refresh controls | PARTIAL | Model can read allowed source files; not persistent selected-input configuration |
| AS07 | Ignore/restore specific project instruction files | T3 /source ignore/unignore/ignored | G1 source options | None | MISSING | No verified setting tool |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| AS01 | Native account UI, reviewed OAuth/helper handoff and completion | D3 | Review account/scopes; secrets stay host-side | LARGE | High | native GUI; Do not treat recovery account selection as enrollment |
| AS02 | Explicit account identity/review and refresh cached owners | D3 | Do not alter in-flight captured scope silently | MEDIUM | High for privacy | native GUI; Independent from enrolling a new account |
| AS03 | Permission listing/review and canonical mutation result | D3 | Exact operation/account; account selection is not permission | MEDIUM | High | native GUI; Never infer approval from an enabled control |
| AS04 | Optional deterministic source picker, no missing host authority | D1,D3 | Future requests only; cannot authenticate or expand current run | SMALL | High | hybrid; Do not call missing solely for lacking a picker |
| AS05 | NSOpenPanel directory choice, canonical resolver, new owner/draft scope | D1 | Scoped read-only snapshots; validate paths in Go | MEDIUM | High for project work | native GUI; Environment override is not an in-app workflow |
| AS06 | Workspace-relative chooser, refresh intent and retained selection view | D1,D7 | Read-only snapshot; traversal checks; distinguish uploads | MEDIUM | Medium | hybrid; Do not count upload picker as complete workspace parity |
| AS07 | Exact workspace-relative instruction selection/listing | D1 | Instructions remain untrusted; no authority expansion | SMALL | Medium for advanced project use | native GUI; Optional advanced control; do not add raw slash execution |

### Following, goals, schedules and updates

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| FL01 | Inspect all ongoing commitments, check health and offline gaps | T2 /following, /overview, /check-health | G6 check/goal/overview projections | N1 update-chat attention; app inspect current-chat followups | PARTIAL | Yes for current chat; not full Following surface |
| FL02 | Create/edit a supported recurring follow-up check | T2 /followup new/edit | G6 app followup mutation + controls | N1 chat can review supported app followup change | FULL | Yes; create/edit with current digest and 5/15/30/60-minute cadence |
| FL03 | Pause/resume one exact recurring check | T2 check commitment actions | G6 digest-bound check/app mutation | N1 conversational inspected followup controls | FULL | Yes; current-chat exact ID/digest |
| FL04 | Manage advanced job schedules, run/history/remove | T3 /jobs; /suggestions for proposed schedules | G6 jobs/goal schedule controls | No job controls; proposals may be queued conversationally | PARTIAL | Can propose schedule; acceptance/advanced management unavailable |
| FL05 | Open/resume a goal’s owning chat and pause/resume its responsibility | T2 Following goal actions, /goals | G6 goal requests + H1 goal owner | Coordinator rejects goal session requests | MISSING | local_plan can inspect; does not open/activate goal owner |
| FL06 | Inspect/edit goal plan, dates, dependencies and agenda | T3 /plan, /agenda | G6 local_plan/suggest_change/plan commands | N1 conversation can inspect; edits remain proposals | PARTIAL | Yes for inspection/proposals; final suggestion acceptance missing |
| FL07 | Inspect/read/resolve/reopen/rate an update and stage a reply | T2 /updates and contextual update actions | G6 BridgeUpdateAction | N1 new-update chat attention; no update body/actions selector | PARTIAL | Followup app inspection is not exact update action API |
| FL08 | Configure desktop alerts, urgent-only and quiet hours | T3 /notifications | G6 goal notification controls | None | MISSING | Followup alert conditions differ from notification preferences |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| FL01 | Global commitment/health view and refresh/result handling | D4 | Next due is estimate; offline gaps stay explicit | MEDIUM | High | hybrid; No need for a permanent dashboard by default |
| FL02 | Optional form; no missing host work for conversational subset | D3,D4 | Explicit ends_at/sources/alert condition; may start local service only by review | MEDIUM | High | hybrid; Advanced arbitrary schedule belongs to FL04 |
| FL03 | Optional direct check controls | D4 | Pause affects future checks; already running check may finish | SMALL | High | hybrid; Do not count menu absence as missing workflow |
| FL04 | Canonical job review/forms, run history, explicit run/remove | D4 | Timezone/cadence/resource budgets and unattended restrictions | LARGE | Medium/High | hybrid; No automatic acceptance of model schedule proposals |
| FL05 | Goal-mode owner creation/navigation and digest-bound controls | D1,D4 | Goal chat/foreground task/check lifecycle stay distinct | LARGE | High for ongoing goals | hybrid; Do not pretend normal independent chat is the goal owner |
| FL06 | Goal selection, typed plan editing/review and agenda presentation | D4 | Expected revision; user-reported progress; timezone/recurrence | LARGE | High | hybrid; Do not infer completion from elapsed time |
| FL07 | Exact update ID/version list, feedback and reply-context selection | D1,D4 | Preserve original chat/version; reply never auto-sends | MEDIUM | High | hybrid; Internal setReplyToUpdateID alone is not reachable UI parity |
| FL08 | Reviewed goal notification UI and OS presentation integration | D4,D5 | Explicit permission/quiet timezone; avoid unwanted interruptions | MEDIUM | Medium | native GUI; Keep alert content decisions separate from OS notification preferences |

### Memory, saved knowledge and reusable guidance

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| KN01 | Search/read saved notes and personal memories | T3 /knowledge, /memory search/show | G7 knowledge_search/read | Reachable through N1 conversation | FULL | Yes; global/current-goal scope only |
| KN02 | Inspect/correct/forget/undo saved memory receipts | T1 /memories | G7 memory actions/tools | N1 remember/correct/forget tools; no receipt/undo UI | PARTIAL | Yes for guarded memory save/forget/correction; original-save undo differs |
| KN03 | Create/edit personal notes with reviewed changes | T3 /note, /knowledge | G7 notes + suggest_change | N1 read/propose only | PARTIAL | Can propose new/revised note; proposal does not activate itself |
| KN04 | Create/edit/enable/disable reusable text skills | T3 /skill | G7 skills/suggestions | N1 enabled skill list/read and proposal tools | PARTIAL | List/read/propose supported; explicit management incomplete |
| KN05 | Review/accept/reject learnings, durable suggestions and feedback | T3 /suggestions, /learnings, /feedback | G7 retained proposal/revision controls | N1 can produce/read some proposals; no acceptance browser | PARTIAL | Proposal/feedback possible; full review/accept workflow absent |
| KN06 | Control chat learning, earlier-chat recall and retrieval destination | T3 /memory privacy/setup | G4 app settings | N1 conversational app inspection/update | FULL | Yes: chat_learning, chat_history, retrieval |
| KN07 | Inspect exact memory/context supplied to one retained run and omit/correct it | T3 /context RUN_ID; result supplied-context action | G7 per-run context revision controls | N1 can search knowledge; no exact supplied-context UI | MISSING | Knowledge search differs from retained run-start/tool excerpts |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| KN01 | Optional browse view; no missing conversational host service | D1 | Saved text is reference, never instructions/authority | MEDIUM | High | hybrid; Do not duplicate retrieval/storage just for a native browser |
| KN02 | Provenance receipt list and exact undo/correction intents | D1 | Expected updated/quote/scope; no silent relearning | MEDIUM | High for trust | hybrid; Do not call model paraphrase a provenance receipt |
| KN03 | Suggestion acceptance and note editor/read view | D1 | Exact prior digest/source references; do not duplicate corrections | MEDIUM | High | hybrid; Do not silently auto-accept conversational suggestions |
| KN04 | Skill library and canonical accepted-edit/enable controls | D1 | Skill text grants no tools, permissions or recursion | MEDIUM | Medium | hybrid; Use existing library; avoid another skill engine |
| KN05 | Typed retained proposal list, exact decision/revision result | D1 | A suggestion is not approval or truth resolution | MEDIUM | High for user control | hybrid; Do not use free text acceptance without exact identity |
| KN06 | Optional privacy settings UI | D3 | Cloud embeddings upload eligible text/queries; explicit review | MEDIUM | High for privacy | hybrid; Native API keys/local downloads remain MD03/MD05 |
| KN07 | Run-bound context viewer and canonical correction/omission actions | D1,D7 | Preserve provenance, scope and expected revision; no inferred prompt context | MEDIUM | High for debugging answers/privacy | hybrid; Do not present retrieved current memory as proof it was supplied earlier |

### Attachments, retained files, editing and export

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| FI01 | Choose/paste/drop immutable message attachments | T1 attach-file/paste-image/attachments | H1 uploads + G8 admission | N2 NSOpenPanel/paste/drop/thumbnails/remove | FULL | Files processed after explicit Send; selection is host-owned |
| FI02 | Open complete retained inputs/outputs, images and PDF pages | T5 /files; result File actions | H3 FileStore, PDF renderer + G8 originals | N1/N3 raw bundle previews; selected-input thumbnails only | PARTIAL | Model can analyze selected inputs; not an exact full-file viewer |
| FI03 | Edit text/Markdown/table files and save a working version | T5 FileEditor Ctrl-S | H3 versioned files | None | MISSING | Model may generate replacement output; no same host version transaction |
| FI04 | Edit Office paragraphs/cells without damaging packages | T5 Ctrl-E in Office/table file | H3 office/file-content | None | MISSING | Can discuss/generate documents; not exact package-preserving edits |
| FI05 | Compare/restore/browse immutable file versions | T5 Ctrl-O saved versions | H3 file-comparison/FileStore | None | MISSING | No file-version host tool available through chat |
| FI06 | Export exact selected file-version bytes with replacement review | T5 version export | H3 ExportStore | No exact-version export UI | MISSING | Bundle save differs from selected working version |
| FI07 | Ask Gator about a saved version or selected paragraph/cell | T5 Ask Gator / Ctrl-L | H3 file attach/version refs + H2 staging | N2 can attach local originals; not saved-version selection | MISSING | Analysis works once selected bytes are attached; selection workflow absent |
| FI08 | Inspect exact retained source evidence/read ranges | T6 /evidence; result Sources | H3 sourceCatalog/sourcePreview + G8 manifests | N3 some raw source labels; no verified evidence reader | MISSING | Model prose/citations do not expose exact host-retained byte ranges |
| FI09 | Inspect proposed code patch and verification evidence | T6 /changes; result Change | H3 codePreview + G8 candidates | N1 raw candidate metadata only | MISSING | Can ask about proposal; exact retained patch bytes unavailable in UI |
| FI10 | Save verified completed deliverables to a folder | T3 /save; app output_action | G8 delivery + app controller | N1 chat can inspect output run then review save | FULL | Yes, save supported sealed output run to absolute folder |
| FI11 | Apply/retry delivery of a verified code/output proposal | T3 /apply, /retry | G8 bundle action/review/delivery | None for apply/retry; FI10 covers ordinary save | MISSING | App output_action only allows save |
| FI12 | Set expected deliverable paths for future work | T3 /artifact add/remove | G1 RunOptions.Artifacts | Request text can describe outputs; no persistent expected-artifact control | MISSING | Can ask for a file; cannot set this host option directly |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| FI01 | None required; duplicate basenames E07 remains | D1,D7 | Opaque IDs/hash, limits, symlinks, scoped access, owner capture | SMALL | High | native GUI; Keep native strengths; do not switch to mutable source paths |
| FI02 | Authorized file references, bounded reads/viewer and PDF paging | D7 | Immutable original/working version distinction; no arbitrary path read | LARGE | High | native GUI; Preview text is not complete-file parity |
| FI03 | Native edit buffer/save/discard with expected revision | D7 | Optimistic concurrency; originals remain immutable | LARGE | High | native GUI; Do not write over original retained output |
| FI04 | Format-aware adapter and native bounded editing UI | D7 | Preserve formulas/unsupported content/protection; explicit notices | LARGE | Medium/High | hybrid; Do not promise a full Office replacement; user may prefer external apps |
| FI05 | Native version metadata/read/compare/restore adapter | D7 | Expected hash/revision; restore creates new version | LARGE | High for editing workflows | native GUI; Depends on versioned file service, not new database |
| FI06 | Save panel, destination review and safe atomic export | D7 | Destination identity/overwrite review; path containment | MEDIUM | High | native GUI; Do not equate saving a bundle with exact edited-version export |
| FI07 | Version-specific immutable staged copy plus editable context | D1,D7 | Original chat/version binding; no automatic Send | MEDIUM | High | hybrid; Never attach current mutable bytes under an old version label |
| FI08 | Manifest-authorized source viewer with hash/range validation | D7 | No arbitrary path/URL fetch; explicit original link open | MEDIUM | High for trustworthy answers | native GUI; Reuse host authority checks conceptually; no need to embed JS UI |
| FI09 | Verified frozen-candidate diff/command-result viewer | D7 | Inspection does not apply changes; exact run/candidate/hash | MEDIUM | High for coding | native GUI; Do not reconstruct patch from model text |
| FI10 | Optional folder picker/direct save; no missing conversation service | D1,D7 | Exact sealed manifest and destination review; report delivery result | MEDIUM | High | hybrid; Keep distinct from FI06 per-version export |
| FI11 | Explicit canonical proposal/retry review and result UI | D7 | Exact target/preconditions; never replay uncertain external effects | LARGE | High for coding; advanced | hybrid; Do not conflate retry delivery with retrying task admission |
| FI12 | Validated relative deliverable selection and future-request options | D1,D7 | No arbitrary output path authority; scope and captured options stay Go-owned | SMALL | Medium for repeatable work | hybrid; Optional advanced preference; ordinary file requests already work |

### Usage, budgets and pause controls

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| US01 | Inspect shared requests/tokens/categories/cost and reset time | T3 /usage | G4 app inspect usage/control | N1 conversational app inspection | FULL | Yes |
| US02 | Change daily request/token/concurrent limits | T3 /usage limit forms | G4 app usage_limits | N1 conversational exact review/update | FULL | Yes |
| US03 | Pause/resume new shared provider work | T3 /usage pause | G4 app usage_pause | N1 conversational app mutation | PARTIAL | Mutation tool exists; availability depends on current quota/pause admission |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| US01 | Optional deterministic read-only usage view | D3 | Shared across work; known cost may be incomplete | SMALL | High | hybrid; No native usage chart is required for conversational reachability |
| US02 | Optional typed numeric controls | D3 | Go validation/review; task budgets stay captured | MEDIUM | High for spend control | hybrid; Do not let frontend edit counters or reset active usage |
| US03 | Model-independent pause/resume control; paused admission cannot reliably invoke chat repair | D3 | When admission is already paused, conversational repair may be blocked | SMALL | High | hybrid; Reachability while paused must be verified; see dependency note |

### Appearance, navigation and keyboard preferences

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| PR01 | Choose terminal color presets/contrast theme | T2 /appearance, /terminal-settings | G4 Go terminal preferences + H2 renderer | Native follows system light/dark; terminal theme can be changed conversationally | INTENTIONALLY DIFFERENT | Yes for terminal setting; native appearance follows OS |
| PR02 | Customize terminal keyboard shortcuts | T3 /appearance shortcut forms | G4 desktop-named Go key preferences | Native standard Cmd/AppKit editing and fixed safety decisions | INTENTIONALLY DIFFERENT | Chat can edit terminal shortcuts, not native binding map |
| PR03 | Show/resize terminal rail and group chats by folder | T2 Ctrl-B, terminal-settings | G4/H2 terminal layout | Native compact/full menus and notch; no terminal rail | INTENTIONALLY DIFFERENT | Terminal preference tools exist |
| PR04 | Customize terminal status-line fields | T3 /statusline, settings statusline | G4 status-line editor/projection | Native semantic status/model/attention views | INTENTIONALLY DIFFERENT | No verified native status-layout setting |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| PR01 | No terminal palette port required; optional native preferences | D8 | Respect system accessibility settings | SMALL | Low for identical terminal styling | native GUI; Do not copy terminal colors over native semantic controls |
| PR02 | If requested, separate native supported actions/reserved keys | D8 | Return must remain non-authorizing; no conflicting shortcuts | MEDIUM | Medium for power users | native GUI; Terminal Ctrl bindings should not replace native Cmd conventions |
| PR03 | Optional native sidebar/grouping only if selected | D8 | Presentation state cannot change owner authority | MEDIUM | Medium with many projects | native GUI; Do not port a terminal rail mechanically |
| PR04 | Optional native status customization only if requested | D8 | Presentation customization cannot hide required exact review context | SMALL | Low/Medium | native GUI; Do not embed terminal status strings as a native control system |

### Voice and speech

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| VO01 | Record dictation, cancel/transcribe and review in draft | T4 /voice, toggle-dictation | H4 VoiceSession/native capture + Go speech | None | MISSING | No microphone capture through conversation |
| VO02 | Read a reply aloud, stop playback and optionally speak new replies | T4 /read-aloud, stop-reading, /voice | H4 system speech/playback | No playback lifecycle | MISSING | Chat may change preferences, cannot run native playback |
| VO03 | Configure dictation route/voice/language/rate and local Whisper download | T4 /voice settings | Go voice config/download + H4 | N1 chat can configure voice preferences; no native download/voice list | PARTIAL | Preferences yes; model download/capture unavailable |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| VO01 | Permission/review, bounded audio capture, cancel and owner-bound transcript staging | D3,D5 | Route/session consent, audio transient, never auto-send | LARGE | High for hands-free input | native GUI; Do not request microphone access until user invokes recording |
| VO02 | Owned speech queue/cancel, new-reply filtering and OS voice adapter | D5 | Avoid reading private history unexpectedly or replaying old replies | MEDIUM | Medium/High accessibility | native GUI; Enable automatic speech explicitly only |
| VO03 | Native settings, installed voice list, reviewed download/progress | D3,D5 | Changed processing destination requires fresh recording consent | LARGE | Medium | hybrid; Do not silently substitute a cloud route |

### Web, browser and computer workflows

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| BR01 | Perform reviewed task-only public-web research | T3 chat web_access review, web-origin | Go web_access/tool grants | N1 conversation + N3 exact approval | FULL | Yes; search configured separately |
| BR02 | Manage private browser session/sign-in and next-task browser grant | T3 /browser | G5 browser controls/host helpers | No native session/grant management; configured tool use may work | PARTIAL | Tools only with pre-existing grant/configuration |
| BR03 | Ask for supported OS computer/browser tool actions during a task | T3 ordinary tool conversation | Go tool capability/OS authority | N1 same Go runtime and exact action review | FULL | Yes when existing host prerequisites grant tools |
| BR04 | Set persistent allowed web origins for future requests | T3 /web-origin | G1 captured RunOptions.WebOrigins | N1 task-only web_access review; no future-origin editor | MISSING | Can request/revoke task-only scope, not the persistent option |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| BR01 | Optional read-only scope display | D1,D3 | Credential-free public scope, budgets, exact review; no private networks | SMALL | High | conversational; No separate browser UI needed for ordinary public research |
| BR02 | Reviewed browser handoff/session selection/grant lifetime UI | D3,D5 | Explicit sign-in/next-task scope; no inherited blanket permission | LARGE | High for browser tasks | hybrid; Do not infer access to all tabs or reuse approval across tasks |
| BR03 | No new host authority; optional task tool-status view | D1,D5 | Actual OS permission and advertised tool availability; fresh approval | SMALL | High for automation | conversational; Not a new native remote-control engine; live OS behavior not tested |
| BR04 | Origin list validation, future-scope controls and canonical refresh | D1,D3 | No wildcard/private-network bypass; current task grants stay separate | MEDIUM | Medium for repeated research | hybrid; Do not inherit a prior task grant as permanent consent |

### Specialists and subagents

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| SP01 | Create/assign/list/save/revise/delete reusable specialists | T3 ordinary chat; /agents descriptions | Go orchestrator list/create/profile tools | N1 conversational orchestration | FULL | Yes; exact saved definition digests and user quote |
| SP02 | Inspect/guide/stop one chosen live specialist | T2 /specialists, task controls | G1 SpecialistAction tokens + Go supervisor tools | N3 public task tool activity; no individual specialist list/control | PARTIAL | Manager may route instructions through supervisory tools; no direct token-bound native chooser |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| SP01 | Optional library browser; no new orchestration layer | D1 | Inherited tools/budget; no recursive grant expansion | MEDIUM | High for repeated work | conversational; Use existing Go definitions; do not introduce Swift agents |
| SP02 | Current specialist projection, exact target/token control/result | D1 | Live worker token; stopping one must not stop another | MEDIUM | High for parallel tasks | hybrid; Do not equate whole-owner Stop with individual control |

### Plans and exact action reviews

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| PL01 | Request a plan first, inspect it, approve/start its exact revision | T2 /plan-first, /task-plans | G3 TaskPlanAction | N2 plan-first + N3 full exact review/authorization | FULL | Yes for preparation; execution needs exact review |
| PL02 | Manually edit plan outcome/steps/effects/sources | T2 editPlan menu | G3 task_plan edit | Can request model revision; no manual exact-content editor | MISSING | Narrative revision differs from editing content yourself |
| PL03 | Request a narrative plan revision and inspect its replacement | T2 conversation revision/correction | G3 dialogue revise_plan | N3 Request Plan Revision + correction draft | FULL | Yes |
| PL04 | Remove selected retained inputs from a proposed task | T2 task-plan selected inputs | G3 task_plan remove_inputs | No remove-inputs workflow for retained plan; draft removal is separate | MISSING | No exact removal tool verified |
| PL05 | Cancel a retained unstarted plan/request | T2 task-plans cancel; conversation stop control | G3 plan cancel/dialogue stop | N1 dialogue dispatch can route cancellation; no direct plan Cancel | FULL | Yes when Go projects exact stoppable retained task |
| PL06 | Inspect/approve/decline the exact pending action | T2 approval-review; approval shortcuts | G1 RespondBridgeApproval | N3 literal exact review + Return-resistant decisions | FULL | Conversation may request action; consent requires exact controls |
| PL07 | Request correction of a proposed action without approving | T2 conversation action correction | G1 revise_action control | N3 Request Correction and preserved ordinary draft | FULL | Yes via correction text |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| PL01 | None required; preserve current identity guards | D1 | Revision/digest/scope/focus epoch; plan approval not tool approval | SMALL | High | hybrid; Already implemented and real-Go tested |
| PL02 | Local edit buffer, explicit save revision and canonical reread | D1 | Expected digest; no execution from saving edits | MEDIUM | High for precise control | native GUI; Do not silently convert manual edits to a model rewrite |
| PL03 | None required | D1 | Original task/run; replacement needs new exact review | SMALL | High | hybrid; Keep correction distinct from approval |
| PL04 | Typed plan-input references, explicit withdrawal and fresh review | D1,D7 | Original retained; earlier processing not undone | MEDIUM | High for sensitive-input correction | native GUI; Do not implement by deleting retained files |
| PL05 | Optional deterministic cancel button and explicit result | D1 | Exact retained task/control ID; cancel is not decline of another review | SMALL | High | hybrid; Verify supported plan state; no raw cancel command escape hatch |
| PL06 | None required | D1 | Original 64-character token/interaction/result; no auto replay | SMALL | Critical | native GUI; Do not reduce to a generic confirmation sheet |
| PL07 | None required | D1 | Retire original proposal; replacement needs fresh exact approval | SMALL | High | hybrid; No correction submission may become approval |

### Failure recovery and uncertain outcomes

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| RC01 | Reconnect original owner and inspect retained work without replay | T1 /recover, /reconnect | H1 original session + G1 journals | N1 explicit Reconnect and Inspect | FULL | No implicit reconnect request needed |
| RC02 | Review configured account access and prepare a safe recovery plan | T2 request-recovery; Go recovery actions | G3 RequestRecoveryAction/access token | N3 positional account review + explicit preparation | FULL | Provider repair remains terminal; preparation is typed native |
| RC03 | Inspect retained action receipts and explicitly check uncertain Gmail Sent | T6 /receipts | G3 ReceiptAction + G8 sealed manifest | N3 original receipt projection/inspect/read-only check | FULL | Model narrative is not receipt confirmation |
| RC04 | Discard a retained failed-request retry | T2 request-recovery discard | G3 recovery discard | Read/prepare only | MISSING | No exact discard tool verified |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| RC01 | None for availability; engineering shutdown work applies | D1 | Preserve request IDs, generation and no automatic resend | SMALL | Critical | native GUI; Do not reconnect by opening a competing owner |
| RC02 | None within current supported preparation contract | D1,D3 | Original recovery token, positional selections, separate plan approval | SMALL | High | hybrid; Preparation is not execution or account enrollment |
| RC03 | None for availability; E04 reconciliation is a correctness defect | D1,D3,D7 | Original account/run/action/correlation/manifest; no resend | SMALL | High for uncertain external effects | native GUI; Found in Sent does not establish delivery |
| RC04 | Explicit retained retry/token discard and truthful result | D1 | Keep original outputs/receipts; do not delete history | SMALL | Medium/High | native GUI; Separate from retry, stop or archive |

### Private data, backups and service lifecycle

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| DR01 | Inspect/change history/knowledge/snapshot retention | T3 /data retention | G4 app retention/control | N1 conversational inspected retention changes | FULL | Yes |
| DR02 | Export and verify a private backup archive | T3 /data backup/verify | G4 data_action backup/verify | N1 conversational exact-review path | FULL | Yes; explicit absolute archive path |
| DR03 | Restore a reviewed backup into the profile | T3 /data restore | G4 canonical restore review | No native data panel/actions | MISSING | App tool opens a review; cannot finish it natively |
| DR04 | Preview/confirm cleanup or scoped private-data deletion | T3 /data cleanup/delete | G4 lifecycle plan/review | N1 can inspect/prepare review; cannot confirm native panel | PARTIAL | Can open canonical review only; no completed native workflow |
| DR05 | Relink a moved workspace/profile source | T3 /data relink | G4 canonical relink review | No native relink controls | MISSING | App tool opens form/review only |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| DR01 | Optional typed settings view | D3 | Reviewed future cleanup policy; no implicit deletion | MEDIUM | High for private long-term use | hybrid; Native draft/upload retention is a separate namespace/gap |
| DR02 | Optional destination picker and model-independent verify | D3 | Private unencrypted archive; excludes credentials/original projects; no secret logging | MEDIUM | High | hybrid; Verify native-only draft coverage before advertising whole-app backup |
| DR03 | Restore-plan review, global-owner guard and matching result | D1,D3 | Stop/retire cached owners, compare exact source/profile plan; irreversible effects | LARGE | High but infrequent | native GUI; Do not auto-apply a chat-requested restore |
| DR04 | Native exact review/confirmation and all-owner coordination | D1,D3 | Profile lease, destructive scope, keep originals/credentials excluded as declared | LARGE | High for privacy | native GUI; Needs global lifecycle readiness; do not prioritize before E01–E03 |
| DR05 | Folder selection, canonical relink plan and global cache refresh | D1,D3 | Exact old/new source identity; no arbitrary model path authority | MEDIUM | Medium | native GUI; Do not rewrite native archive keys ad hoc |

### Application tools and presentation differences

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| AP01 | Inspect setup/dependencies/version and find update guidance | T3 /doctor, /about | G4 control screens + H1 version | N1 connection/model details only | PARTIAL | App inspect covers settings; not full doctor/version UI |
| AP02 | Edit idle composer text in an external terminal editor | T4 external-editor | Go reviewed editor job + H1 PTY | N1 native NSTextView editing/undo | INTENTIONALLY DIFFERENT | No external editor launch via ordinary chat |
| AP03 | Install/switch/rollback terminal packages and inspect releases | T4 about/releases; launcher scripts | Terminal installer/package ownership | Native development bundle script; no updater | INTENTIONALLY DIFFERENT | No |
| AP04 | Quit with work/review/unsaved-data guard and child cleanup | T1 exit/quit keybindings | H1 application close + Go owners | N1 AppDelegate/coordinator guard | FULL | No external action |
| AP05 | Discover actions through slash commands, palette and terminal rail | T1 slash/Ctrl-P/Ctrl-B | H2 action dispatch + G1 command policy | Native menus/toolbar/keyboard/notch; slash input rejected | INTENTIONALLY DIFFERENT | Chat accesses supported Go tools only |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| AP01 | Read-only diagnostics/version/dependency screen | D3 | Redact secrets; configuration is not access proof | SMALL | High for support | native GUI; No need for an updater to show version/diagnostics |
| AP02 | None required; optional explicit export/editor handoff | D5,D8 | Review executable/cwd; preserve changed drafts on return | MEDIUM | Low/Medium for editor enthusiasts | native GUI; Native editor already serves the user outcome; PTY dependence is optional |
| AP03 | Separate native distribution design only if requested later | D5 | Artifact provenance, profile continuity; signing outside this audit | LARGE | Medium outside current scope | native GUI; Do not port terminal installer into native app or start signing work |
| AP04 | Correctness work E01–E03 precedes expansion | D1 | Do not terminate before durable draft and confirmed owner exit | SMALL | Critical | native GUI; Available workflow has documented defects; parity is not a safety certificate |
| AP05 | Optional native command palette with finite typed actions | D8 | No raw host command execution; reserved approval keys | MEDIUM | Medium for discoverability | native GUI; Do not add blanket slash forwarding merely to claim parity |

### Background checking service lifecycle

| ID | User-facing behavior | Terminal entry point | Existing authority | Current native implementation | Native status | Already conversational? |
| --- | --- | --- | --- | --- | --- | --- |
| BG01 | Inspect/start/stop the local checking supervisor | T3 /recovery | G4 app recovery and local supervisor | N1 conversational inspected/reviewed recovery change | FULL | Yes: recovery start/stop after current run |
| BG02 | Enable/disable/repair start-at-login for the current profile | T3 /recovery reviewed service actions | G4 recoveryServiceReview/digest + app install/remove | N1 conversational reviewed enable_login/disable_login | FULL | Yes; existing install action revalidates service digest |

| ID | Missing native host/UI responsibilities | Dependencies | Security / permission boundary | Effort | Expected user value | Approach; reason not to port / limitation |
| --- | --- | --- | --- | --- | --- | --- |
| BG01 | Optional model-independent service status/control UI | D3,D4,D5 | Explicit service effect; saved checks may run; respect profile ownership | MEDIUM | High for scheduled checks | hybrid; Do not start service merely by opening Following |
| BG02 | Optional explicit native service review/status UI | D3,D4,D5 | Exact service ownership/digest/executable; no automatic background installation | MEDIUM | Medium/High | hybrid; Separate from foreground task lifecycle and signing |

## 4. Backend versus frontend ownership and material workflow gaps

Go must retain admission/deduplication, task leases, model/permission scope, exact approvals, plan revision/digest, recovery/access tokens, external action receipts and all destructive lifecycle reviews. Swift should add finite typed intents plus request-specific results to the existing bridge, then render those results. It should not add a second scheduler, memory engine, specialist runtime, configuration authority or agent loop.

The terminal host supplies more than rendering. `Application` coordinates sessions, draft retention, immutable files, version edits, safe export, source/code evidence, voice and external handoffs. Native upload staging implements only the upload portion. Porting the file viewer therefore needs a native authorized file/version/evidence adapter with containment, expected revision and destination review; directly opening a `bundle.Path` from raw JSON would bypass important host responsibilities. Importantly, native uploads use `gator/native-uploads`, while terminal uploads use `gator/uploads`; do not assume terminal FileStore references automatically resolve native attachments.

The highest-value absent **deterministic native** workflows are full catalog/transcript search; queue controls; full retained-file viewing/versions/export; account setup and source/workspace controls; and update/Following management. These are independently selectable, not a proposed combined tranche. Smaller candidates such as rename, pin or draft listing can be chosen separately.

Several workflows are already conversational: existing model/effort changes, selecting configured connections, supported check agreements, shared usage inspection/limits, memory/privacy configuration, saving verified output bundles, public web tasks and specialist definitions. Optional GUI controls could improve precision or model-independent access, but absence of the terminal button does not make these workflows missing. Conversely, advanced schedules/notes/skills may be **proposed** through `suggest_change`, but that tool explicitly does not activate its suggestions. Native lacks the terminal acceptance/rejection browser. Data restore/delete/relink similarly use `appScreenResult` after the model reply (`cmd/gator/work_interactive.go:871`); an opened canonical panel is not a completed native review workflow.

Usage pause is marked PARTIAL despite the update tool: once admission is paused or its budget exhausted, asking a model to resume can itself be unavailable. A model-independent native pause/resume/limits path is needed to claim unconditional recovery parity. Native provider readiness and recovery copy also intentionally route account/provider setup to terminal controls today.

The public native `setReplyToUpdateID` method is not used by a visible update selector; therefore it does not establish update reply parity. Similarly, raw artifact names/previews, task capability flags or backend specialist support do not establish reachable full-file/specialist-management workflows. The matrices reflect those gaps.

### Important contract-specific limits

- Duplicate document basenames remain a known P2 limitation: distinct Swift immutable IDs are safely rejected by Go's provider/original basename validation. This is a shared contract issue, not an excuse to bypass validation or silently rename originals during parity work. Engineering E07 contains current integration evidence.
- Native exact action/plan review is already implemented. Future panels must preserve original owner/generation, exact tokens/revisions/scope and post-persistence revalidation; disabled UI is not sufficient authorization.
- Native uncertainty recovery has availability but current correctness limits (engineering E04/E05). A FULL parity cell is not a claim that these defects are resolved.
- Terminal backup/export acts on the existing Go lifecycle store. This review did not verify inclusion of native-only archives or staged uploads. Do not advertise “complete native app backup” until its explicit coverage is established.
- Browser/computer tools share the existing Go runtime and its prerequisites; a reachable conversation does not establish an actual OS grant on this machine. Private session/grant configuration has a separate gap from using already-available tools.

## 5. Documentation reconciliation and excluded capabilities

`docs/CLI-TUI-PORT.md` is dated 7 November and declares a CLI/TUI-only product after Electron removal. That is historical product direction relative to this worktree's new `macos/` implementation. It is useful evidence for terminal entry points, not proof that native does not exist. No README/document outside these audit reports was modified.

`docs/TERMINAL.md` and `docs/DEVELOPMENT.md` were reconciled against source. Slash/palette aliases share capabilities rather than inflating counts. `/search` is intercepted by the terminal local transcript action; the Go-advertised web-search-service configuration remains a backend settings/command concern, not the same literal-search workflow. Source/connection selection is different from signing in or granting an operation. Goal plans are different from exact executable task plans. Retained delivery retry differs from admission retry. System narration is different from provider/model voice preferences.

**Terminal source/documentation contradiction:** Go advertises a web-search-service screen at `/search` (`internal/workapp/commands.go:211`; `cmd/gator/work_search.go:356`). The terminal defines `controls.actions.search` as transcript search, and `GatorClient.navigate` intercepts that local name. The Go command-palette wrapper also calls navigate, so the ordinary named route does not open service setup. No alternate reachable terminal UI route was verified; source presence alone is insufficient. This backend-only setup screen is excluded from parity counts rather than invented as an existing terminal workflow. No fix was made. A future user may choose native service configuration separately from porting actual current terminal capabilities.

Optional companion/owned-work/external-tmux contracts (`contracts/inspection.ts`, `external.ts`) describe a separate extension. No corresponding ordinary `ui/src/` terminal frontend for enrollment/remote control was established in this review. They are **excluded from the terminal capability counts**, not automatically called missing native features. Calendar/Google Tasks are explicitly unavailable in the current product guide; local agenda/plan dates are included, but cloud calendar/task features are not invented as parity gaps. Old Electron widgets, browser capture not actually exposed, generic upstream OpenCode behavior, future native voice permissions and unrun remote release matrices are also excluded.

No frontend dependency/framework is recommended for installation by this review. macOS signing/notarization, an updater and service deployment remain outside the authorized audit. Terminal package-install controls are documented as intentionally different rather than a mandate for native distribution work.

## 6. User-selectable candidates — no automatic implementation decision

Each row below is one selectable outcome. Related dependencies are disclosed; selecting a row does not select every adjacent feature. FULL rows with optional GUI improvements are identified as such. Estimates retain the detailed matrix's scope; selecting exact file versions necessarily requires version-service groundwork, but does not automatically select Office editing, voice or other workflows.

| Choose ID | User-facing outcome | Current status | Effort | Why it may be useful / dependency |
| --- | --- | --- | --- | --- |
| CH03 | Search all retained conversations | MISSING | MEDIUM | Find older work through canonical indexed catalog. |
| CH04 | Rename conversations | MISSING | SMALL | Clearer recents; needs exact catalog selection. |
| CH05 | Pin/unpin conversations | MISSING | SMALL | Keep frequent chats near the top. |
| CH06 | Archive/restore conversations | MISSING | MEDIUM | Organize history without deleting it. |
| CH07 | Load earlier dialogue | PARTIAL | MEDIUM | Finish the native reading workflow for long chats. |
| CH08 | Search within current transcript | MISSING | MEDIUM | Exact literal finding, independent of model access. |
| CH11 | Restore reading position/catch up | PARTIAL | MEDIUM | Follow long-running work without losing reading place. |
| CH12 | Branch from an exact saved answer | MISSING | MEDIUM | Explore another direction with correct lineage. |
| CH15 | Manage native saved drafts | MISSING | MEDIUM | Find/remove retained drafts with safety guards. |
| QT01 | Explicitly enqueue after current work | MISSING | MEDIUM | Distinguish sequential work from conversation/guidance. |
| QT02 | Edit queued text | MISSING | SMALL | Repair a retained request without executing it. |
| QT03 | Reorder queue | MISSING | SMALL | Choose sequence explicitly. |
| QT04 | Remove queued work | MISSING | SMALL | Clear blocked unwanted requests safely. |
| QT05 | Explicitly resume queued/interrupted request | MISSING | MEDIUM | Restore a stopped workflow under fresh authority. |
| MD03 | Enroll/repair provider credentials | MISSING | LARGE | Native first-run/setup independence. |
| MD05 | Set up local models | MISSING | LARGE | Private/offline processing; download lifecycle required. |
| MD06 | Inspect full request context/limits | PARTIAL | SMALL | Know source/model/budget before sending. |
| AS01 | Connect/repair an account | MISSING | LARGE | Native sign-in rather than terminal handoff. |
| AS02 | Disconnect an account | MISSING | MEDIUM | Direct privacy control. |
| AS03 | Review/change operation permissions | PARTIAL | MEDIUM | Explicit account authority independent of source selection. |
| AS04 | Add a configured-source picker | FULL conversationally | SMALL | Optional precise/model-independent selection. |
| AS05 | Choose workspace folder | MISSING | MEDIUM | Work with projects without launch environment edits. |
| FL01 | Browse Following/check health | PARTIAL | MEDIUM | See responsibilities/offline gaps across chats. |
| FL02 | Add a follow-up agreement form | FULL for supported conversational cadence | MEDIUM | Optional exact source/timing controls. |
| FL05 | Open/control a goal owner | MISSING | LARGE | Native ongoing-goal lifecycle. |
| FL07 | Inspect/manage updates and replies | PARTIAL | MEDIUM | Complete attention-to-action workflow. |
| KN02 | Review memory provenance and undo | PARTIAL | MEDIUM | User control over saved learning. |
| KN03 | Create/edit accepted notes | PARTIAL | MEDIUM | Requires explicit suggestion acceptance/editor. |
| KN05 | Accept/reject durable suggestions | PARTIAL | MEDIUM | Completes conversational proposals without auto-acceptance. |
| FI02 | Open full retained files/PDFs/images | PARTIAL | LARGE | Authorized native file-read service needed first. |
| FI03 | Edit text files as new versions | MISSING | LARGE | Preserve originals while improving outputs. |
| FI04 | Edit Office content | MISSING | LARGE | Separate optional format-preserving project. |
| FI05 | Compare/restore file versions | MISSING | LARGE | Depends on immutable working-version service. |
| FI06 | Export exact file-version bytes | MISSING | MEDIUM | Native destination/replacement review. |
| FI08 | Inspect retained source evidence | MISSING | MEDIUM | Hash/range-verified answer provenance. |
| FI09 | Inspect verified code changes | MISSING | MEDIUM | Frozen candidate diff, distinct from Apply. |
| US01 | Add a direct usage view | FULL conversationally | SMALL | Optional model-independent accounting. |
| US03 | Add model-independent pause/resume | PARTIAL | SMALL | Recover when model admission itself is paused. |
| VO01 | Dictate into editable draft | MISSING | LARGE | Explicit recording consent and cancellation. |
| VO02 | Speak replies/stop playback | MISSING | MEDIUM | Accessibility and hands-free consumption. |
| SP02 | Select/guide/stop a live specialist | PARTIAL | MEDIUM | Exact individual worker controls. |
| PL02 | Edit proposed plan content manually | MISSING | MEDIUM | Precise user edits without a model rewrite. |
| PL04 | Withdraw selected plan inputs | MISSING | MEDIUM | Sensitive-input correction with fresh review. |
| RC04 | Discard a failed-request retry | MISSING | SMALL | Resolve retained retry state without running it. |
| DR02 | Add native backup destination/verify UI | FULL conversationally for Go store | MEDIUM | Native-only archive coverage must be established. |
| DR03 | Restore backup | MISSING | LARGE | Requires whole-profile owner/cache coordination. |
| KN07 | Inspect supplied context for one run | MISSING | MEDIUM | Exact retained excerpts and omission/correction. |
| BG01 | Add direct local supervisor controls | FULL conversationally | MEDIUM | Optional model-independent start/stop/status. |
| BG02 | Add login-service review UI | FULL conversationally | MEDIUM | Exact owned service/executable review; never automatic. |
| AP01 | Show native diagnostics/version | PARTIAL | SMALL | Read-only support aid; redact secrets. |

Do not begin implementation from this list. The user can select IDs independently, or choose to keep an outcome conversational/terminal-only. The next engineering phase should first address the durable-state and lifecycle risks in the engineering report; that recommendation does not choose any parity feature.
