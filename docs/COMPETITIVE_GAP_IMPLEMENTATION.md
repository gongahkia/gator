# Competitive gap implementation

Objective: implement the code changes in the [22 October gap audit](COMPETITIVE_GAP_AUDIT_2026-10-22.md), with atomic commits and engineering, product and desktop verification. This ledger retains the entire scope across implementation sessions. An implementation entry is not a claim of successful live use.

## Requirements and evidence ledger

| Gap | Work and completion evidence required | Status |
| --- | --- | --- |
| G01 | Repeatable live/product evaluation, failure attribution, matched baseline and unseen cases; multi-day follow-up evidence | Pending |
| G02 | Independent task ownership; browse and converse during other work; original-task steering, approvals and results; bounded concurrency | Implemented and verified for local desktop sessions |
| G03 | Durable origin-linked updates in chat; delivery/read/resolution separation; reply routing; no duplicate delivery or execution | Implemented locally; live reply/clarification quality remains part of G01 |
| G04 | Reviewed conversational responsibilities with sources, timing, expiry and attention; actual save, pause and guarded edits | Implemented locally; live interpretation remains in G01 |
| G05 | Task-led onboarding, account setup/health/reconnect without Terminal, understandable model/access choices | Implemented locally; managed Google OAuth and observed first-use evidence remain external |
| G06 | Memory receipts/corrections and clear learning/history/privacy controls; cross-chat/restart/conflicting-history and multilingual evaluation | Controls and guarded edits implemented locally; live recall/intent quality remains in G01 |
| G07 | Ground-truth alert/miss evaluation, conversational notification conditions, failed-check visibility and quiet behavior | Local mechanisms implemented and verified; live alert quality and multi-day evidence remain in G01 |
| G08 | Complete selected connected jobs, reviewed delivery, account checks and receipts; uncertain send reconciliation | In progress: native receipts and Sent reconciliation verified with local fixtures; real-account jobs remain |
| G09 | Durable admission/queue, explicit interrupted states, recovery without replaying external effects; crash-transition tests | In progress: admission, queue and connector crash recovery verified locally; broader effect/checkpoint coverage remains |
| G10 | Representative Office/file fidelity, precise revisions/version comparison, larger bounded intake; native opening checks | In progress: saved-version comparison and larger intake verified locally; native fidelity remains |
| G11 | Integrated browser readiness/state/private sign-in/takeover, scoped browsing authority and representative workflow tests | Pending |
| G12 | Paginated/searchable chats, rename/archive/pin, scoped cross-chat files; retrieval over hundreds of records | In progress: indexed desktop chat browsing and organization verified locally; broader scoped file catalog remains |
| G13 | Release/update lifecycle, supervisor drain/restart and native platform gates; signed distribution requires external accounts | Pending |
| G14 | Authenticated remote execution option with explicit ownership/availability; client-off completion and local-resource boundaries | Direction chosen: self-hosted companion on an always-on machine |
| G15 | Authenticated remote conversation/update/decision surface, revocation, offline delivery and deduplication | Pending |
| G16 | Useful conversational voice/steering, interruption and transcript correction; inspectable personal style | Pending |
| G17 | Save/test/version/revert reusable procedures from successful tasks; skills remain separate from permissions | Pending |
| G18 | Latency/resources/cost metrics, long-history and low-resource checks, accessibility and multilingual product verification | Pending |

## Execution order

1. Persist message admission and independent task ownership; route results and recovery to their chat.
2. Add conversational responsibilities and account activation, then prove a complete follow-up workflow.
3. Improve memory, retrieval, files, browser and reusable workflows using those journeys.
4. Complete update/platform engineering and authenticated remote execution/client surfaces.
5. Verify voice, performance, accessibility and all acceptance scenarios; retain explicit external evidence gaps.

Use the current React/CSS/Lucide conventions, neutral text-field focus, keyboard command palette, compact chat and inline files. Do not revive the retired user CLI. Credentials, production signings, real-account operations and independent user judgments cannot be manufactured by code changes; record their status accurately while completing all independent work.

Remote direction confirmed by the user: a self-hosted companion on an always-on machine. That machine owns remote execution and its own accessible files/accounts; a laptop connection is a client. Do not imply that a remote task can access an offline laptop's files or that local tasks automatically migrate. Authentication, revocation, original-chat routing and retry receipts are required before remote control is usable.

## Completed changes and verification

- Admission storage: private, bounded per-chat message journals; atomic generation checks for edits/reorders/admission; independent execution leases released by process exit. `go test -race ./internal/worksession` passed, including concurrent writers, stale changes, corruption, retained options and cross-process ownership.
- Desktop admission/recovery: messages are acknowledged after durable admission; transport retries reuse their identity. Queued text, settings and upload references survive restart. Interrupted work is paused with an explicit uncertainty notice; retry creates a new linked attempt. Old files referenced by pending requests or saved drafts are protected from attachment cleanup. Corrupt/stale journals reject submission and preserve the draft. `go test ./...`, targeted race tests, TypeScript checks, Electron upload/recovery tests and a renderer build passed. Recovery was inspected at wide and minimum window sizes. These scripted local-provider tests verify interaction and retention, not live model quality or automatic execution checkpoint recovery.
- Independent local chats: the host manages separate Go runtimes with a maximum of three executing chats and four retained processes. New chat, recent-chat selection, conversation browsing and settings can leave work running in its original chat. Queues and approval tokens remain with their owner. Background completion cannot replace the last selected chat; model/provider choices are captured on admission. Profile maintenance retires idle cached chats and rejects other pending work; close and update gates cover every open chat. Startup failures restore the original selection. The host activity indicators are currently session-local; durable background delivery/read/resolution belongs to G03.

Verification for independent chats: targeted Go race and application-self-knowledge tests, command-package tests, 16 host lifecycle/update tests, TypeScript checks and renderer/runtime builds passed. Electron checks covered concurrent replies, original-task steering, queue ownership, browsing without empty-chat creation, shared memory, recent-chat drafts, reconnect/recovery, keyboard focus across themes and minimum-width layout. Queue editing/dragging/stopping/resuming also passed. These remain scripted-provider interaction tests; G01 live usefulness evidence and G14/G15 remote execution are still outstanding.

- Background update storage: immutable result identities deduplicate notification retries. Origin chat and execution chat are distinct. Display, explicit read and resolution have separate durable receipts; stale resolution changes and cross-chat acknowledgements are rejected. Concurrent receipt updates preserve ratings and other receipts, and legacy unlinked results remain in the inbox.
- Follow-up delivery: reviewed checks capture their requesting chat, including checks created in a separate view while that chat runs. Cards retain summary, next step and source references; unread indicators open the original chat even after restart. History uses pages of 50 with stable anchors as new results arrive. Reply identity travels through unsent drafts, admitted messages, queues and retained revisions. The execution adapter reloads the owned update as untrusted reference context and records its provenance without changing workspace, goal or connector grants. Unbound ambiguous replies are instructed to clarify; this instruction still needs live quality evaluation.

G03 verification: inbox/application/session race tests; full command and work-run package tests; host policy/draft/session tests; TypeScript and runtime/UI builds. Scripted-provider Electron checks exercised arrival during another chat, separate receipts, stale resolution rejection, reply ownership across chat switches and restart, retained revision linkage and transport deduplication. Existing request-recovery checks passed. These are interaction and durability evidence, not live model usefulness evidence.

Interface inspection for G03: compact dark chat at the minimum 760×560 window, including card, details, reply chip and existing composer, using React, CSS tokens and Lucide. Typography and wrapping were inspected in the screenshot; structural borders and spacing match existing cards; icons use the established outline weight. No new animation was introduced. Cards are bounded to 50 per page and acknowledgements use visibility observation outside React rendering. Other themes and broad resource/language/accessibility evaluation remain in G18.

- Follow-up expiry: optional end dates are included in agreement reviews and policy digests. The desktop uses a local date/time input and sends an explicit timestamp; ambiguous dates are rejected by the host. Due checks, catch-up, schedule claims, manual starts and new retries respect expiry. A check already executing may finish. Ended checks have no next-run claim and require a reviewed new date before resuming. Existing unlimited checks retain their previous digest. Jobs, follow-up, health, application form and targeted command tests plus TypeScript checks passed.

- Conversational responsibilities: foreground app tools inspect this chat's checks and prepare create/edit/pause/resume agreements. The host binds workspace and origin, requires exact existing IDs and current digests, rechecks selected connector configuration/account pins, and saves through the same adapter as the optional form. An explicit reviewed option starts this profile's local supervisor and discloses its effect on other enabled checks. Save/start partial failures report saved-but-offline state. Pausing works after source access disappears. The host supplies local time for date interpretation; ambiguous time or target selection still requires live model evaluation.

G04 verification: targeted command race tests cover no writes before review, duplicate prepared creation, source substitution, stale edits, cross-chat targets, expiry, permissions, pause/resume and supervisor startup failure. Application and work-run tests passed. A scripted-provider Electron journey declined then saved one agreement, edited only its notification condition, paused the same record, and exercised the native end-date input. Runtime/UI builds and TypeScript passed. The chat review uses existing result typography and icons with expandable limits/delivery details; screenshots were inspected. This proves local interaction and persistence, not live account access, real monitoring usefulness or remote availability.

- Native connections: Settings and the conversation information drawer open account management directly. Google email/calendar setup, private service-token setup, browser sign-in, exact-configuration reconnect, bounded account/read checks, disconnect and removal use native forms and reviews. Recovery routes here. OAuth uses PKCE and a five-minute loopback callback; private credentials never enter chat/display state. Pending sign-in retains its owning process across chat navigation and blocks profile deletion/update races. Credential compare-and-swap prevents stale refresh/sign-in/disconnect from overwriting a changed account. Verification checks each configured email/calendar/Drive metadata surface and labels local saved credentials separately from live access.

G05 connection verification: auth/connector and targeted command race tests, workapp/workrun tests, host session tests, TypeScript and desktop runtime/UI builds passed. One earlier full command-package run found a Recovery test still expecting the retired terminal/form route; the test was updated to exercise the native handoff and the targeted rerun passed. An isolated Electron journey added and reviewed a token connection, checked read access, disconnected, opened the browser sign-in action, left for another chat, returned and completed its callback. No real provider credentials were used. The final 980×780 desktop screenshot was inspected; existing typography, neutral notices, human service/access names and expandable technical details are retained. Managed Google OAuth ownership/verification and an observed first-use trial remain external evidence requirements; task-led onboarding remains next in G05.

- First use: decision, file and ongoing-follow-up starting points fill an editable draft. Cloud/local model choices explain processing, billing and download needs; optional account setup is available without making it a prerequisite. Text and attached files survive model setup and renderer reload. Desktop model-panel exits now reconcile readiness through the same path as the panel back action, fixing a stale first-run state that blocked attachment submission. Model selection uses an atomic settings update so concurrent connection setup is retained.

G05 first-use verification: full workapp and modelcatalog tests, the concurrent model/connection settings race test, TypeScript and runtime/UI builds passed. An isolated Electron journey selected a file, configured a fixture model, returned, reloaded and sent the first request; no chat request occurred before Send. At 760×560 the model setup action and draft composer are visible. The compact layout removes the decorative logo/intro spacing only during first use; existing CSS tokens, neutral focus, typography and Lucide icons are retained. No motion was added. This is scripted interaction evidence, not a novice usability trial or live model/account validation.

- Memory privacy: Settings now separates automatic learning from earlier-chat retrieval. New requests capture the retrieval preference, and disabling it removes both earlier-chat tools while retaining current conversation context and saved memories. The panel explains what is retained, what changes for already-running requests and where to manage deletion. The conversational settings tool uses the same reviewed, stale-checked preference.

G06 privacy verification: workrun/workapp/config race tests, targeted command tests, TypeScript and runtime/UI builds passed. The isolated Electron journey verified the settings route, actual provider tool list and settings across restart. The macOS Node 26 test runner timed out during process shutdown; the installed Node 24.19.0 runner passed with tracing enabled and normal application close. This was a runner limitation, not a reason to bypass Gator's normal shutdown. Broader memory quality and multilingual live trials remain in G01.

- Saved knowledge correction: edits and forgetting bind to the displayed version, with a locked comparison at mutation time. Note versions cover both metadata and body, including file edits outside the app. Conflicting changes preserve the editor draft. Successful edits update the detail/list version, including when inspecting an older run's retained excerpt.

G06 correction verification: full knowledge/memory/workapp race tests and command tests for displayed versions, retained excerpts, omission and memory setup passed. Tests cover concurrent reviewed edits, missing revisions, direct note-file edits and stale deletion. The notes store serializes application writers; ordinary external editors cannot participate in that lock, so their edits are detected when read before the guarded write, not promised transactional against an arbitrary simultaneous filesystem write.

- Conversational memory: explicit requests outside the recognized English patterns can use an exact host review instead of silently failing or treating the model's intent classification as permission. Forgetting requires the revision returned by the preceding knowledge inspection, checked again after review. Negative English forget requests are rejected, including when a tool quote crops away the negation. Earlier-chat matching accepts short names/acronyms, CJK text and combining marks within the existing scan/result bounds.

G06 language/tool verification: focused tools race tests passed for Chinese, Spanish and Malay reviewed requests, declined reviews, negative forget requests, stale and concurrently corrected memories, and short/multilingual history searches. These demonstrate authorization and retrieval mechanics; semantic memory selection, paraphrase recall and natural-language intent quality still require live G01 evaluation.

- Inline memory receipts: a collapsed receipt under the relevant answer opens the current saved fact, scope and original quote where still applicable. Edit, undo for an unchanged new save, and reviewed forgetting use chat ownership and exact revisions. Forgotten records are removed from the saved-memory projection rather than copied into permanent receipt text. An open draft survives transient refresh failure, external correction, deletion or rebinding; stale saves are disabled and navigation asks before discarding edits.

G06 receipt verification: memory/workapp race tests, host frame-policy tests, TypeScript and runtime/UI builds passed. Four isolated Electron journeys passed under Node 24.19.0: shared memory across independent chats, privacy across restart, inline edit/forget/undo across restart, and concurrent edits/removal while a draft is open. The expanded receipt at 900×680 and Memory & privacy settings were inspected: existing typography, CSS surfaces, outline icons and neutral inputs are retained; the disclosure uses the existing restrained arrow transition. No live model-quality success rate is inferred from these scripted interactions.

- Temporal follow-up evaluation: a bounded sequential dataset runs the production watcher gate, Work execution, retained attempts and inbox delivery in a disposable profile. Ten episodes exercise quiet and changed sources, unchanged skips, explicit source/provider/tool failures and recovery. Optional live execution requires an explicitly selected provider/model and request/time limits. Ground truth and future source versions stay outside the watched workspace.

G07 replay verification: focused evaluation and command race suites passed, including immutable attempt/report binding, failure attribution, shared request bounds, partial runs, unreviewed event windows, and uncertain first-notification/repetition scoring. Explicit review files bind actual outputs before usefulness, actionability, misses, repeats or delay receive denominators; health notices are separate. No live provider requests were made. Simulated episode offsets measure fixture timing; multi-day live usefulness and independent judgments remain in G01. See `examples/project-followup/TEMPORAL_REPLAY.md`.

- Follow-up failure visibility: origin-chat cards label failed and interrupted checks explicitly, retaining that outcome after Read or Done. A brief caution explains that an earlier result cannot establish that nothing changed. Existing reply, details and receipt controls remain available.

G07 status verification: TypeScript and the isolated Electron failure/interruption journey passed under Node 24. The 760×560 screenshot was inspected for readable labels, wrapping and actions without horizontal overflow. Existing warning/muted tokens and outline icons are used; no new motion was introduced.

- Quiet-check review: Following opens completed quiet decisions with their conclusions, rationale and retained source references. Users can record a missed change, mark a check correctly quiet, or clear feedback. Exact job/attempt/revision identities and locked version checks reject stale actions. Clearing retains a version tombstone. A conflicting save preserves the feedback draft and existing navigation protection.

G07 quiet-review verification: jobs/workapp/command quiet tests passed, including identical-looking targets, concurrent reviewers, legacy review loading and clear/review races. The Node 24 Electron journey passed agreement creation/edit/pause, exclusion of failed/unchanged attempts, missed/correct feedback, concurrent conflict, draft preservation, navigation guard and clear counts. The 900×720 review screenshot was inspected without clipping. Feedback does not change the monitoring agreement or send an alert; displayed review counts alone are not a recall estimate.

- Delivered-update feedback: an optional compact disclosure records Useful, Unnecessary or Incorrect and supports clearing. Chat ownership and the displayed feedback version are checked under the inbox lock. A shared pending action waits for the exact host acknowledgement, blocks navigation/update restart until confirmed, and reports uncertainty after connection loss or timeout. Ratings never imply Read or Done, and health notices are excluded from alert-quality totals. Native Inbox actions bind their screen to the same rating revision.

G07 delivered-feedback verification: targeted inbox/workapp/command race tests, host policy tests, TypeScript and runtime/UI builds passed. The Node 24 Electron journey held acknowledgements after the real disk write to verify no premature success, then exercised concurrent stale edits, clearing, failed-card exclusion and restart retention. The expanded 760×560 screenshot was inspected with readable choices and no overflow. Feedback is retained for review; it does not silently rewrite notification conditions.

- Follow-up health and quiet behavior: consecutive unsuccessful watch attempts share one durable incident. A change between request limits and execution failures starts a new notice; a later successful or unchanged check records recovery even when there is no material change to report. Every attempt remains immutable. OS notification handoffs use a durable at-most-once claim; a crash during that handoff can omit the OS toast while the in-app update remains saved. Recovery does not alter Read or Done and does not claim coverage of the unsuccessful interval.

- Chat health: one optional disclosure above the composer shows only checks whose reviewed agreement names this chat. It includes offline/limit/failure state, last successful check for current settings, unsuccessful counts, next action and observed supervisor gaps. Unrelated chats do not inherit health or health-refresh warnings. The composer DOM is bound to the selected conversation so its expanded controls and health content reset when chats change.

G07 health verification: jobs/inbox/workhome/workapp/evaluation race suites passed. Command watcher/replay/notification tests passed after updating the prior limit test to require a nonurgent, non-ratable health notice. Focused tests cover restart deduplication, notification claims, quiet recovery, unchanged user receipts, phase changes and origin ownership. TypeScript and runtime/UI builds passed. Electron verified scoped health/gap/offline reporting at 760×560, then four regression journeys passed delivered feedback, original-chat reply routing, pasted images, per-chat drafts, focus and compact layouts across themes. The expanded health screenshot was inspected for typography, wrapping, controls and overflow; existing surfaces/icons and restrained native disclosure behavior are retained. These are local mechanism tests; independently reviewed live monitoring remains G01.

- Connected request receipts: actual retained actions add one collapsed disclosure in chat, including failed, denied, proposed, uncertain and completed outcomes. Email acceptance, a matching Sent copy and recipient delivery have distinct meanings. Batch steps retain partial completion. An uncertain Gmail send offers a native read-only Sent lookup; it never retries a send. Reopening a receipt reads any saved lookup locally. Exact chat/run/action/correlation/manifest identities bind the request, completion acknowledgements and saved checks. Manifest replacement during lookup rejects saving the result. The host retains a busy lookup process and blocks reconnect, profile maintenance and update restart around it.

G08 receipt verification: full workapp/artifact race suites, focused native Sent/receipt/projection command race tests, 27 terminal-only host policy/session/update tests, TypeScript and runtime/UI builds passed. Coverage includes duplicate action IDs with distinct sends, cross-chat/stale rejection, ambiguous identity, failed reads, revoked access before lookup, restart retrieval, partial batch projections, exact owned bundles with duplicate run IDs elsewhere, and manifest replacement during lookup. No live account was accessed. Desktop interaction, focus and layout inspection for this new receipt remain pending: the user is on a call, so app-opening automation was deliberately not run. Selected connected jobs and real-account validation are still incomplete.


- External request journal: approved connector actions flush a private receipt before dispatch, retain the exact chat/run/proposal/correlation, and hold an operating-system lease until their outcome is saved. A crash leaves an uncertain receipt and never schedules a replay. Provider evidence is saved before the tool reports completion. A failed initial save prevents dispatch; a failed outcome save reports uncertainty. Drafts and denials do not create execution records. Reads, bounded scans, backups and explicit history deletion cover the journal; automatic age retention protects unconfirmed requests and their chats. Native Sent reconciliation records are now also included in history export/deletion.

G09 dispatch verification: action-log and connector race tests passed, including simultaneous admission, immutable results, approval ordering, dispatch blocked by a failed save, and provider success followed by an outcome-save failure. A production executor process was killed while its local fixture provider held a dispatched request: the owned uncertain receipt survived and repeated inspection caused no additional provider request. Retention/backup/deletion tests passed after excluding journal locks from backups. The action-log package also compiled for Windows; that is not a Windows runtime or installer claim. Broader execution checkpoints and non-connector effects remain incomplete.


- Chat recovery receipts: opening an affected chat can show up to 32 newest journal receipts that are absent from its verified task bundles. Identical-looking requests are counted as separate attempts; another chat's records are filtered again at projection. Existing bundle receipts are not repeated. Recovered records do not expose the full approved payload, cannot execute a retry, and describe owner liveness as the state observed when checked. Unsealed Gmail requests currently require manual provider inspection; native Check Sent remains limited to verified completed task bundles.

G09 recovery projection verification: the full workapp suite passed in a bounded terminal-only run and focused recovery race tests passed. An earlier concurrent full race run stalled during the file-backed inbox fixture and was interrupted; it is not counted as a passing full race run. TypeScript passed. Desktop interaction/layout inspection for recovery remains deferred during the user's call. This completes a connector receipt slice, not general execution checkpoint recovery or all remaining competitive gaps.

- Appearance: Settings → Appearance offers Dark, Light, High contrast and Monochrome. The saved profile preference refreshes cached chats on selection, restores before the renderer's first paint, and updates native controls/window backgrounds. A failed save retains the previous theme. Light mode uses soft white surfaces, deep green accents, neutral text-field focus, and theme-aware selection, search, overlays and shadows; document/slide paper colors retain their file meaning.

Light-mode verification: focused Go appearance/settings tests, TypeScript, native appearance/CSS contrast tests and runtime/UI builds passed. An isolated Node 24 Electron journey switched to Light, sent a scripted-provider reply, retained a draft while switching chats, inspected the command palette at 760×560, restarted, checked persistent/native appearance, and switched back to Dark. Chat, settings and palette screenshots were inspected. Small-text tokens meet 4.5:1 on the five tested surfaces; this is not a complete accessibility certification. No new motion or styling system was introduced. Stronger shadows and a new font were considered and rejected because the existing typography and surface hierarchy remain appropriate. Other screens inherit shared tokens; exhaustive visual inspection of every file format and platform remains G10/G13/G18.

- Standalone connected requests: app testing found that Act mode rejected a selected connected request unless an output file was also required. An explicitly selected connector with reviewed external actions can now produce its result in chat. Connector selection, permission rules, fresh approval and durable dispatch remain mandatory; required Code work still needs a file contract. Focused executor race tests passed for approval, denial, read-only boundaries, uncertain outcomes and existing artifact-backed actions. Desktop receipt verification continues below.

- Failed-task evidence: verified bundles now stay attached to failed and stopped turns, retaining partial files and native Sent lookup access. Previously only successful turns attached the bundle, causing uncertain sends to appear as unsealed recovery records. Focused application race tests passed for failed/stopped receipt ownership; the local-provider desktop Sent journey reached a persisted matching Sent check without replaying the POST.

- Interrupted-chat names: the first durably admitted request names an untouched chat before any revision finishes. Metadata updates share a lock with revision/head updates; subsequent queue changes cannot rename it. Naming failure cannot reject an already saved request. Titles are bounded in both Unicode characters and bytes. The full conversation-store race suite passed, and desktop crash/restart/chat-switch checks found the interrupted request by its retained title.

- Uncertain outcomes: a failed/stopped turn with an unknown external receipt now says “Request outcome unconfirmed” and asks users to inspect the receipt before retrying. It does not label an uncertain send as definitely failed. Focused application race tests and the desktop Sent journey passed.


- Compact composer: recovery receipts, check health and queued requests now share a bounded scrolling area above the composer. Expanding multiple retained controls no longer pushes the message field or Send button below the minimum window. The conversation-keyed composer remains in place, and slash suggestions remain outside the scrolling region.

G08/G09 desktop verification (after the user's call ended): two Node 24 Electron journeys passed against isolated local HTTP services. They exercised foreground approval, an accepted POST without a provider receipt, local-only reopening, an exact read-only Sent lookup, immutable original manifests, persisted check results, deduplication against bundle receipts and chat ownership across restart/switching. The crash journey killed only its isolated Go child while the approved POST was in flight: the uncertain journal survived, owner liveness became false, no unsealed Check Sent action appeared, and inspection/reopening sent no second POST. The Unix process-tree probe is skipped on Windows; native Windows crash evidence remains outstanding.

Four Electron journeys then passed the receipts, crash recovery, scoped check health and queue editing/dragging/resuming paths after the composer bounds change. Expanded receipt/recovery screenshots at 760×560 were inspected; the Send button and composer bounds are explicitly checked. Existing CSS tokens, outline icons, tabular timestamps, neutral inputs and native disclosures remain; no new animation was added. An additional recovery side panel and a separate visual system were rejected to keep this evidence in the current chat. TypeScript and runtime/UI builds passed. No real Google account or live model was used. These checks supersede the earlier call-related verification deferrals; selected real-world connected jobs and broader checkpoint recovery remain incomplete.

- File comparisons: a read-only host operation compares two exact saved versions of a file already available to this chat. It verifies both retained byte digests and leaves the current version, edit draft and original untouched. Bounded line comparisons distinguish inserted/deleted lines; Word/slide paragraph text and CSV/Excel cell values/formula expressions have positional comparisons. File-byte equality is distinct from supported-content equality. Layout, graphics and cached spreadsheet results are explicitly excluded. Comparisons over 4 MiB, large line-change windows, 200 changes or long fields disclose the partial/unavailable result. File/Office regression tests and five comparison tests passed, including repeated/blank lines, native formats, corrupt/stale version rejection and read-only behavior. Inline UI verification follows separately; native Office layout/opening and representative customer-file evidence remain incomplete.

- Inline version review: files with multiple saved versions offer Compare alongside Preview/Edit. From/To selectors and separate export actions bind the actual selected version; the ordinary export/restore controls cannot silently act on a different version while comparing. Unsaved edits must be saved/discarded first. Before/After labels, locations and optional tinted fills communicate changes without depending on color. Native selects, restrained surfaces, existing font sizing and outline icons are retained; a modal editor and a separate design system were rejected.

G10 comparison UI verification: TypeScript and renderer builds passed. Three Node 24 Electron journeys passed full-text draft/save/reload/send, Word/Excel/PowerPoint/CSV edit/export with unchanged originals, and exact version comparison/export. The comparison journey verified two changed sections, identical-version selection, reload retention, no model calls and an unchanged original. It was rerun after compact-control changes; dark/light comparison screenshots at 760×560 were inspected with the composer still visible and no horizontal overflow. No new motion was introduced. The Office tests verify retained native package content, not a native Office application opening or full page-layout fidelity.

- Larger bounded intake: up to 12 attachments, 8 MiB per document, 4 MiB per image and 16 MiB combined. The renderer, private import store, clipboard decoder, saved drafts, IPC and Go model-input boundary enforce those limits; a regression test compares JavaScript and Go constants. Copies allocate against the observed file size and reject growth rather than silently importing a truncated file. Original files remain untouched. Provider-specific context/media limits can still reject a request; this does not claim universal provider acceptance or automatic retrieval over arbitrarily large files.

G10 intake verification: 21 Node upload/clipboard/draft/policy tests, focused Go attachment/application/command race tests, TypeScript and runtime/UI builds passed. A Node 24 Electron journey selected 12 files including a document over 5 MiB, rejected a thirteenth dropped file while preserving the selection and message, restored all files after reload, and sent all 12 through the real Go loader to a local provider fixture. The minimum-window screenshot was inspected with the composer visible and no horizontal overflow. Original contents were unchanged; no live model or account was used.

- SQLite conversation index: a private, rebuildable `gator/cache/conversations-v1.sqlite` indexes display metadata and retained user/assistant text, using parameterized FTS5 substring queries. One- and two-character searches also work. Canonical conversation, revision and admission files remain authoritative. Queries reconcile file identities and remove changed/deleted excerpts before searching; private replay, provider configuration and admission options are omitted. Initial indexing has explicit record/byte/lineage limits and reports incomplete coverage. A corrupt cache is rebuilt; an unavailable/newer cache falls back to bounded canonical reads. The database uses the pinned pure Go SQLite driver so this does not introduce a C toolchain requirement.

SQLite verification: full conversation and lifecycle race suites passed. Focused checks cover literal/Unicode search, discarded admissions, canonical corruption/deletion, private file permissions, cache corruption repair, future-schema fallback, cache symlinks and concurrent queries/organization. History deletion and history retention remove derived indexes; backups omit them and retain canonical records. A local 1,000-chat benchmark (three iterations, warm filesystem) measured 294 ms / 28.3 MB allocated for a no-match file scan versus 126 ms / 4.2 MB for warm indexed search. This includes source stat reconciliation and is not a general workload or Windows runtime certification. Indexing is local and makes no provider calls. Desktop integration is the next slice.

- Desktop conversation catalog: asynchronous, cancellable searches query retained messages across pages, with literal highlighting and snippets. Active, Archived and All chats have stable next/previous controls. Rename, pin/unpin and archive/restore use the exact viewed record version; a refreshed title form keeps its draft but cannot edit a different row. Archiving retains messages, shared memory and ongoing work. Pinned chats stay first in recents; archived chats stay discoverable in the browser. A pending organization save retains its owning runtime across navigation and blocks profile maintenance/update/restart until acknowledged.

Desktop catalog verification: full workapp race tests, focused catalog/recents tests, 20 Node policy/session checks, TypeScript and runtime/UI builds passed. Two isolated Electron journeys passed dated chat search/keyboard navigation/chevron alignment and a 130-chat, three-page search/rename/pin/archive/restore/restart workflow. The latter found text in the oldest chat, checked exact retained revision bytes, rejected a stale title form without losing its draft, preserved the SQLite database across restart, and made no provider requests. Dark/light screenshots at 760×560 were inspected; compact header spacing leaves room for results, and menus stay within the viewport and follow their visible anchor on scroll. Existing CSS tokens, neutral textbox focus and Lucide icons are retained; no new motion was added. The SQLite conversation package also cross-compiled for Windows amd64 with CGO disabled; native Windows runtime testing remains separate.

- Quiet shared admission holds: regression testing found that the health-incident reducer introduced an inbox alert for a deliberately paused shared budget. Shared admission holds now stay in chat health/Usage and unsuccessful coverage counts, without creating inbox incidents or fabricated recovery notifications. Execution failures and check-specific limits retain incident handling. The original shared-pause test and focused job/command health race tests pass; model calls remain blocked while paused.

Final storage checks: the full command-package race suite passed after the shared-hold fix. SQLite file URIs preserve Unix punctuation/spaces and use the correct Windows drive form. Cache locks use the rooted file-identity checks; six concurrent cold queries initialize a usable cache. Focused URI and concurrent query/organization race tests passed three repetitions. Runtime Windows execution remains unverified.

## Current goal continuation — 29 October 2026

The requested scope remains G14/G15 remote monitoring and phone access, G05
simpler setup, terminal migration/native release gates, G08/G11 complete connected
and browser workflows, and G10 document quality/fidelity. None is closed by a
single implementation checkpoint.

- Google setup: distributor-configured runtime builds now supply desktop OAuth
  client material, remove client-ID/secret entry from the ordinary setup path,
  and preserve the manual option. Fixed endpoint/client binding prevents supplying
  that material to a modified connector. No production client is configured;
  provider registration/verification, actual account consent and novice usability
  observation remain outstanding.
- Remote companion/mobile: still requires authenticated persistent execution,
  revocation, disconnected-client completion, original-chat decisions, and receipt
  deduplication on an always-on host. No deployment has been made.
- Terminal: this macOS checkout has Go and Node but no Bun on PATH. Native
  terminal checks and the existing parity ledger remain outstanding.
- Connected/browser: real-account workflow validation and integrated sign-in,
  takeover and recovery remain outstanding.
- Documents: representative rendering/native opening, calculations and factual
  review remain outstanding beyond structural validators.

Verification for distribution Google setup: targeted Go setup/native-connection
suites pass normally and with `-race` on macOS; Node build-configuration tests
pass, including rejected partial configuration and linker-argument injection.
The setup test verifies unchanged read scopes, no credentials before consent,
manual fallback, the credential-free sign-in form and changed-endpoint refusal.
Existing loopback callback tests cover PKCE and credential lifecycle with a local
server. Real Google sign-in and rendered frontend acceptance remain unverified.

### Companion transport checkpoint

`companion/` now provides a loopback, bearer-authenticated API backed by the
existing Go UI bridge. Host processes outlive client connections. Message
admission uses the existing durable journal and exact request IDs; approval and
receipt frames keep their Go checks. Token rotation revokes subsequent and
queued requests without canceling already-admitted work. Origin, conversation,
body and concurrency bounds are enforced. Host setup and the job supervisor
remain separate; no remote deployment or login service was installed.

The real-runtime integration fixture verifies disconnected-client completion,
admission deduplication and token revocation. Mobile UI, per-device pairing,
remote file transfer, native deployment and full connected-action/recovery
journeys remain pending. This is progress on G14/G15, not their completion.
See `companion/README.md` for startup and protocol boundaries.

Verification: `node --test companion/server.test.mjs` passes both tests on macOS
with Go 1.27 and Node 24.19. The expanded real-runtime journey also restarts the
companion and confirms the saved answer reopens without additional model calls.
The forced socket-drop fixture reconnects an idempotent GET after an expected
stale pooled socket reset. Token-file permission, symlink and format tests pass;
Windows ACL acceptance is not inferred from them. Syntax and diff checks pass.

### Companion phone client checkpoint

The companion now serves a responsive, same-origin client for an existing chat.
It shows retained messages, updates, queued work and complete plain-text approval
previews. Approval requires an explicit second confirmation; a changed live
preview cancels it. Offline/hidden/refreshing clients cannot submit a decision.
Tokens remain in memory. Unconfirmed messages retain their exact payload and
admission ID in tab session storage, including across reload, and retries are
explicit. Revocation clears the displayed conversation. Fixed static assets use
a restrictive CSP; Host/Origin checks reject foreign browser origins and rebinding.

Verification on this macOS Intel host: the real-Go disconnect/restart/admission
test and token tests pass, as does the public-shell/authentication test. The
phone-width journey passes in installed Chrome and cached Playwright WebKit
2370, driven by the locally available Playwright 1.64.0 alpha. It exercises an
acknowledgement lost after admission, reload/retry with the identical request,
approval replacement/confirmation/denial, untrusted text rendering, offline
controls and revocation. Widths 320/390/760 have no horizontal overflow; dark
and light screenshots were inspected. The browser host is a deterministic
fixture; real Go admission is verified separately. Linux CI now runs the same
journey with the desktop's pinned Playwright browsers. No CI result is claimed.

Physical iOS/Android, remote HTTPS deployment, device enrollment/revocation,
conversation discovery/creation, native service installation, file transfer,
and live connected workflows remain open. A browser viewport fixture does not
close G14/G15 or the physical-device acceptance gap.

### DOCX styled-template regression

A representative body placeholder with paragraph properties reproduced malformed
output: prefix matching treated `<w:pPr>` as the enclosing `<w:p>`, leaving an
unclosed style element. Replacement now locates the actual Word paragraph using
XML token offsets and preserves surrounding bytes. A body placeholder outside a
Word paragraph rejects the whole render, even when a title marker was valid.
The regression checks XML parsing and preservation of surrounding paragraphs
and inserted table/prose content. `go test -race ./internal/document` and focused
document/artifact tool tests pass on macOS. This fixes one reproducible fidelity
defect; it does not establish visual layout or native Word acceptance.

### XLSX calculation integrity

A template regression reproduced a changed formula displaying the prior cell's
cached `999`, while its actual inputs totalled `200`. The renderer now clears
template formula caches (including dependent sheets), clears values before
writing replacement formulas, and requests automatic full recalculation on open.
The streaming writer also omits unverified supplied formula values. Preview
warnings distinguish formula creation from calculation verification.

`go test -race ./internal/workbook` and the workbook tool test pass. Tests reopen
the saved XLSX, reject stale caches, independently evaluate the expected `200`
total and `400` dependent result using Excelize, and check totals through both
ordinary and 10,001-row streaming paths. This is stronger calculation evidence
for those fixtures, not native Excel recalculation or general analysis accuracy.

### Native Mac terminal checkpoint

macOS Intel now has a passing full terminal suite (57 pass, 2 platform skips),
production dependency/service checks, isolated install/uninstall, and real PTY
editor/file/exit restoration evidence. Fixed macOS local-package symlinks that
broke installed dependencies; fixture synchronization and canonical temporary
paths were corrected separately. See `TERMINAL-MIGRATION-VERIFICATION.md` for
exact scope and remaining native-device/Windows/release gaps. Electron remains
available; this checkpoint does not close migration parity.

### Mac companion login service preparation

The local Mac is the first development target following the user's request for
the easiest available host. `companion/macos-service.mjs` generates a validated,
private, reviewable LaunchAgent plan from explicit installation/profile paths.
It does not activate a service or change sleep settings. The plan binds a fixed
loopback port, private log directory, token-file path and exact profile; launchd
restarts the listener with throttling. The README documents explicit activation
and removal, awake/logged-in requirements and the separate job supervisor.

The opt-in native macOS test passes: plist validation, isolated copied listener
startup, crash/restart, authentication after restart and completed service
removal. The test was removed from launchd and its temporary files cleaned up;
no permanent service was installed. This probe never opens a Go conversation;
the real-runtime durability test supplies that separate evidence. Production
profile selection, stable deployment paths, HTTPS and phone enrollment remain
open. No remote monitoring deployment is claimed.

### Browser private takeover foundation

The local browser controller now supports an explicit host-only pause/resume.
Once pause is acknowledged, all agent tab reads, snapshots, screenshots and
actions are blocked. The denial survives storage reopen and driver failure.
Resume invalidates old page references before re-enabling access; old references
cannot reappear after a new snapshot. Existing origin restrictions stay in force.
The agent-facing tool interface cannot change this permission.

Targeted controller/tool/command race tests pass. The pinned Playwright 1.56.1
real-Chromium integration passes on macOS, covering denied private captures,
resume and stale-reference rejection alongside existing scoped browsing checks.
It uses a local page and isolated state, not a real login. Current controls are
internal host commands; the integrated frontend lifecycle/task-grant journey
remains open. `docs/BROWSER.md` corrects a stale claim that the current terminal
already exposed that journey.

### Memory audit and requested next phase (29 October)

The user confirmed desktop notifications plus a retained CLI inbox, scheduled
work, and a ChatGPT-style About you summary after the current work. The deeper
[memory/scheduling audit](MEMORY_AND_SCHEDULING_AUDIT_2026-10-29.md) records existing
stores, reproduced defects, privacy/reliability gaps, and the ordered follow-up.
This adds scope; it does not close the current browser/live/native release gates.

Fixed two reproduced defects in separate commits: `48d948298` stops new chat
memories and corrected knowledge being permanently excluded from semantic
search; `feca5ef7f` rejects negative save requests even when evidence crops away
the negation. Memory/knowledge/retrieval/tools/workapp race suites and focused
command tests pass. Existing erroneous exclusions are repaired when a live
record is corrected; a migration for untouched records remains open.

The broader learning/jobs/followup/inbox/worksession race suites also pass.
The lifecycle suite exposed a macOS `/var` versus `/private/var` fixture mismatch;
canonicalizing the fixture preserves its exact source/no-revision assertions,
and the full lifecycle race suite passes after that test-only correction.

Further hardening is warranted: subject aliases can evade forgetting, original
history remains independently retrievable, and exact quotes alone do not filter
credentials or third-party statements. The personal summary should expose these
distinctions and evidence; it must not become another independent facts store.

### Integrated browser controls and next-task grants (29 October)

`/browser` now exposes reviewed runtime installation, a fresh headed window,
allowed sites, tab selection, screenshot permission, private pause/resume,
next-task selection, stop and unavailable-controller revocation. New application
windows start paused with screenshots off. Users sign in in the browser window.
The shared panel uses existing form/review components in terminal and desktop
command navigation; it can stay in the active chat during private takeover.

The reviewed browser grant is consumed by one submitted or queued task. Durable
queue recovery preserves it without automatically replaying interrupted work;
other tasks/chats and Code specialists do not inherit it. Delayed control results
cannot grant access to a different chat. Selected browser identity is visible in
the next-task details. The controller checks a policy digest on every granted
operation, so changed tab/site/capture/upload permissions cannot silently widen
the reviewed scope. Separate browser stores now serialize session updates using
a process-held file lock, protecting concurrent session creation/revocation.
Reviewed operations use distinct wire method names: an older daemon rejects
them instead of silently ignoring the policy field. Stop remains available to
revoke such a session without granting any new access.

Verification on macOS:

- Browser/workapp/workrun race suites and focused command tests pass.
- Tests cover exact review, changed-policy refusal, queued/restarted grants,
  one-task consumption, delayed results in another chat, private pause and
  unavailable-session revocation. Independent-store concurrency retains all
  sessions and the concurrent stop.
- TypeScript and terminal production build pass.
- Full terminal suite: 58 pass, 3 skip, 0 fail, 867 assertions across 44 files.
  Skips are native image paste, dictation, and the opt-in managed-browser test.
- Separately, both browser terminal journeys pass, including real pinned
  Chromium opening paused, site review, resume, one-task grant, no browser grant
  on the next task, private takeover and stop. The model is a local fixture.
  Setup cancellation creates no session or download. No leftover test daemon
  was found after cleanup.

This closes the missing local control surface, not the whole browser/connected
workflow gate. Real-account login/redirects, novice observation, native Windows
and desktop-specific interaction evidence remain. A hung sidecar's bounded
cancellation and forced shutdown/cleanup still need stronger recovery work;
unavailable-controller tests here cover missing transport, not every engine hang.
Attaching a CDP browser, registering uploads and exporting browser artifacts
remain advanced CLI operations. The phone companion does not yet expose this
complete panel. See `BROWSER.md` for current behavior.

### Browser interruption and native recovery follow-up

The driver now honors request cancellation, interrupts blocked pipe I/O, bounds
operations to 30 seconds, and closes instead of reusing an uncertain response
stream. Cancelling a queued request does not cancel the request already in
flight. The controller observes caller disconnects and engine exits; it revokes
the session/token, drains handlers and shuts down. Authenticated unconditional
Stop bypasses the operation lock. Startup/listen failures also clean up their
engine and access token. These paths do not automatically retry browser actions.

EOF lets Playwright close Chromium normally. After two seconds, an unresponsive
driver is killed and reaped. Gator now owns the temporary profile/download
directory and removes it after either exit path, including forced termination.
Cleanup errors reach the daemon log. Whole-daemon crashes/forced termination
remain a distinct recovery gap; this work does not establish crash-safe scratch
collection or protection against an OS-level unkillable process.

Verification covers real child processes, blocked reads, cancellation while
queued, stubborn-child termination, startup failure, caller disconnection and
Stop. Opt-in macOS Chromium tests hold an HTTP navigation open, separately
cancel and stop it, verify no observed child processes survive, verify token
revocation and no navigation replay, and launch a fresh blank browser. A further
test freezes the actual Node driver with SIGSTOP and checks forced termination,
native child exit and deletion of its actual Chromium profile directory.

The browser CI matrix now includes Windows alongside Linux/macOS. It installs
the pinned runtime and runs the managed-browser journey plus applicable recovery
tests. The frozen-process/process-tree assertions are Unix-specific. Local
Windows cross-compilation is evidence of compilation only; no new native Windows
run is claimed. Real-account sign-in remains deferred at the user's request.

One terminal browser run under simultaneous native test load hit a stale-view
selection rejection. The standalone rerun passed both journeys (16 assertions).
That intermittent selection-test timing issue remains recorded, rather than
being counted as evidence of consistently passing concurrent UI runs.

Final local checks: the full browser race suite passed with the native Chromium
integration enabled (17.1 seconds); focused browser-control command race tests
passed; the Windows amd64 browser test binary cross-compiled; and the Go runtime
rebuilt. Both terminal browser journeys passed against that final runtime
(16 assertions, 9.8 seconds). CI YAML parsed successfully with the Windows
browser entry. The CI change is committed locally, not remotely executed.

### Whole-controller crash recovery follow-up

The controller now holds a per-session process lease. Lease publication and
acquisition are serialized with probes to avoid declaring a starting controller
dead. A second controller cannot replace its socket or revoke its session.
When a client finds an existing lease released, it revokes the saved session,
removes the token and endpoint, and refuses the operation. Missing leases retain
legacy/startup behavior; a live owner or a response timeout is not crash evidence.
The browser panel reloads the recovered state and displays stopped/revoked access
instead of offering a stale revoke action.

The JS child also removes its assigned private scratch directory after browser
shutdown on stdin EOF, so killing the Go controller need not strand profiles.
An opt-in native test launches a separate real Go controller process and Chromium,
kills the Go process, observes its browser children exit and scratch directory
disappear, then verifies a client probe removes the dead session's token/socket.
The controller lease tests also cover a live owner, legacy/no-lease startup and
duplicate-controller refusal. These tests pass with the race detector on macOS;
focused command/browser-control tests pass and the Windows test package compiles.
The browser CI recovery step includes the new tests (Unix process inspection is
not a Windows runtime claim).

Host power loss or simultaneous controller/driver termination/freezing is still
outside this cleanup path. There is no startup collector for those leftover
scratch directories. Real-account sign-in remains deferred; no live accounts
were used or contacted by the recovery fixtures.

Final verification for this follow-up: browser recovery/ownership/service and
reviewed-policy race checks, including native Chromium journeys, passed in 22.6
seconds; command browser-control race checks passed. Both terminal browser
journeys passed against the rebuilt runtime (16 assertions, 10.3 seconds).
Windows amd64 test cross-compilation and CI YAML validation passed. No remote
CI execution or native Windows result is claimed.

### Presentation package, editing and native preview verification

Independent validation exposed defects that Gator's own PPTX parser accepted.
Microsoft's Open XML SDK 3.5.1 reported nine errors for a two-slide generated
deck: missing slide master, incorrect presentation child ordering, an invalid
notes-master element, empty theme style lists and incomplete font definitions.
The generator now emits slide masters/layouts and their relationships,
presentation properties, complete theme styles, and corrected notes markup.
Added slides reuse an existing layout; unsupported templates without a layout
are rejected instead of exporting another structurally incomplete slide.
This follows the [PresentationML package structure](https://learn.microsoft.com/en-us/office/open-xml/presentation/structure-of-a-presentationml-document).

The first usable Quick Look preview revealed a second issue: body paragraphs
were encoded as newlines inside one text run and displayed as continuous text.
New decks now emit separate DrawingML paragraphs and an explicit white master
background. A namespace-aware editor replaces one identified title/body/notes
shape completely, including text split across formatting runs and paragraphs.
It preserves the shape geometry and first paragraph/run styling, leaves
unrelated shapes untouched, and rejects ambiguous targets. Inspection uses an
identified title rather than an earlier decorative caption. The tool description
now distinguishes editable content from native layout verification.

A third defect appeared when the shared Node file editor exported a multiline
edit: it placed DrawingML breaks inside runs, where Word breaks belong but
PowerPoint breaks do not. The independent validator rejected the exported file.
The editor now places breaks at paragraph level, uses one text element per run,
keeps tabs as text, and preserves run formatting. Its styled preview includes
breaks and automatic fields; automatic fields cannot be silently replaced as
plain text. Word behavior is unchanged and remains covered by its own roundtrip
checks.

Reproducible checks are in `scripts/office-fixtures`,
`scripts/office-validation/roundtrip.mjs`, and the locked .NET validation project
in `scripts/office-validation`. The CI Office job generates all three themes,
notes, transitions, slide add/move/delete, whole-shape edits, alternate namespace
prefixes and split formatting runs; it then roundtrips a generated deck through
the shared editor and validates all nine resulting packages independently.
The .NET SDK is a verification dependency, not a Gator runtime dependency.

Local verification on macOS:

- The original sample produced nine SDK errors; all nine current fixture
  packages produce zero errors, including the formerly invalid multiline
  shared-editor export.
- Full presentation race tests, focused presentation tool/schema checks and
  the PPTX evaluation-grader regression pass. Eleven Node Office/comparison
  checks pass, including Word/Excel preservation, DrawingML line breaks,
  formatting and automatic-field refusal.
- Quick Look images were inspected for a short slide, a longer title/body,
  whole-body replacement, and the shared-editor multiline/tab/emoji export.
  Paragraph separation, white background and the expected replacement text are
  visible. An initial thumbnail attempt produced no output; subsequent fixed
  packages generated previews successfully. This does not prove that the
  initial timeout was caused by the package defects.
- The Go runtime was rebuilt. After the shared-editor changes, terminal
  keyboard Office/CSV editing/export passed with 15 assertions, and the
  terminal production build passed.

G10 remains incomplete: PowerPoint/Word/Excel themselves were not run, arbitrary
template dimensions, dense-slide overflow, notes-pane behavior and multilingual
font/shaping fidelity are not established by these synthetic fixtures. Quick
Look is useful independent rendering evidence, not native Office certification.
The CI job is committed locally and has not been remotely executed.

### Companion conversation discovery and switching — 29 October

The phone client no longer requires copying a conversation ID from the host.
After authentication it lists existing chats, searches titles/folder paths,
filters active/archived/all records, and pages through the retained catalog.
Known-ID opening remains available. The internal Go catalog transport uses the
existing bounded store and profile guard; it does not run a model, create a
conversation or run profile-retention maintenance. It may refresh the disposable
catalog index. Search does not expose private replay or search message contents.

Switching preserves each chat's draft and unconfirmed admission identity in the
tab's session storage. Selecting another chat never submits its draft. A failed
search can be retried with the same query, and partial/skipped reads are disclosed.
Tokens remain in memory, catalog responses require authentication, and token
rotation during a read prevents its private response from being returned.
Catalog errors do not expose subprocess commands/host paths. Existing messages
and action requests also recheck revocation before returning their response.

Opening a fifth chat retires only an idle process; running work, queued requests,
approvals, setup panels and local operations stay with their owning process.
Crashed workers still require host recovery. This is process retirement, not
deletion of retained chats. The catalog is explicitly refreshed or reloaded on
return to a visible page; open chats retain their polling behavior. Revocation
clears the client when its next request is rejected.

Verification on this Intel Mac:

- `go test -race ./cmd/gator -run 'TestCompanionCatalog|TestDesktopRuntime'
  -count=1` passes. A 53-chat fixture checks stable pages, exact title search,
  mismatched-cursor refusal and no newly created conversations.
- `node --test companion/server.test.mjs`: five pass, including the real Go
  catalog without a model call, client-disconnect durability, duplicate admission,
  restart without replay, query bounds, revocation during a read, and retirement
  of idle workers without stopping pending work.
- Chromium and WebKit: four browser journeys pass. The catalog journeys cover
  keyboard opening, failed-search retry, empty results, archived scope, paging,
  independent drafts, lost acknowledgement followed by switching and retrying
  the original admission identity, plain-text hostile titles and revocation.
  The existing exact-approval/reload journeys also pass. After the WebKit control
  fix, both catalog journeys pass again with actual minimum-height assertions.
- Screenshots at 390 pixels in dark/light themes were inspected; overflow checks
  pass at 320, 390 and 760 pixels. Evidence is under ignored
  `.gator/native-tools/companion-*-catalog*.png`.

Interface review scope: the new catalog and navigation controls, plain DOM and
the existing CSS tokens. The existing chat/approval layout was exercised by its
browser tests but not redesigned.

| Category | Evidence | Result |
| --- | --- | --- |
| Typography | Dark/light WebKit and Chromium screenshots; long hostile title | Text wraps without horizontal overflow. |
| Surfaces | Native controls and bounding boxes | WebKit initially ignored the select's minimum height; corrected native appearance and verified at least 44 pixels. |
| Animation | New catalog code | No motion added; no animation replay applicable. |
| Icons | Filter chevron in final WebKit screenshot | CSS chevron uses current text color; the semantic select retains keyboard selection. |
| Performance | Catalog bounds and browser tests | At most 50 rows per page; no periodic list rebuild while browsing. |

Rejected alternatives: adding a new component/styling framework would duplicate
the existing small client; animated list transitions would add movement to a
frequent navigation action. No actionable polish issue remains in the inspected
scope. Verdict: Approve for this scope; physical iOS/Android interaction, actual
Safari, screen-reader acceptance and deployed remote-host behavior are unverified.

G04 remains open: creating a new chat remotely, device enrollment/per-device
revocation, file transfer, mobile notifications and a permanently configured
always-on deployment are not established here. A Mac login service still needs
the Mac awake, online and logged in. No production service or real account was
activated, and native Windows execution remains unverified.

### Starting a chat from the companion — 29 October

The phone catalog now offers New chat in the configured host workspace. The
workspace is shown before creation; remote callers cannot supply a source path,
message or model options to the creation endpoint. Go creates the same kind of
empty retained conversation as the local UI, without a source snapshot, revision,
admitted request or model call. Opening the chat is a separate read. Send remains
the action that starts work.

Creation uses a durable request ID retained by the browser before dispatch. The
Go store serializes competing processes and saves a receipt before publishing a
complete staged chat directory. Retrying after a lost response, failed opening,
reload or companion restart returns the original chat. If a published chat was
subsequently deleted, an old request refuses to recreate it. Receipt metadata
contains the ID, creation time and a source fingerprint, without raw workspace
paths or chat text. Profile maintenance excludes concurrent creation through the
existing shared guard, and retained-history backups carry the receipts.

The browser keeps the creation retry until both creation and opening are
acknowledged. Dismiss retry only clears that browser retry; it neither deletes
an already-created chat nor sends a new request. Changing the advertised host
workspace disables the old retry. Existing drafts remain in their original chat.

Observed checks on this Mac:

- The full worksession race suite passes. Focused creation tests cover four
  concurrent processes, distinct requests, changed-source rejection, empty-chat
  state, persisted pre-publication/pre-rename recovery, deleted-chat refusal,
  invalid identities and linked receipt-directory rejection.
- The backup/restore race regression preserves the original creation identity
  and returns one chat on retry in the restored profile. Its initial assertion
  omitted the manifest's `gator/` prefix; correcting that fixture made the actual
  receipt and restored identity checks pass.
- Six HTTP tests pass, including real Go creation/opening with no model request,
  stable creation across retries, restart and token rotation, rejected source/
  message fields, size bounds and the existing disconnect/admission transport
  boundaries. Focused command race checks pass.
- All six Chromium/WebKit journeys pass. New-chat journeys lose the creation
  response, reload without automatic retry, fail the first open, then recover the
  same chat. No message is sent until Send. Both original and new drafts retain
  their ownership. Subsequent focused browser checks also verify that a changed
  workspace disables a pending retry and Dismiss makes no request or deletion.
- The phone control has a 44-pixel minimum height; 320/390-pixel overflow checks
  pass, and the WebKit new-chat screenshot was inspected. This extends the
  existing DOM/CSS system without animation or additional UI dependencies.
- Windows worksession tests cross-compile; this is not native Windows execution.
- The local UI runtime rebuild passes with the new internal creation command.

Limits: deleting all retained history also removes creation receipts, so old
browser retries must be dismissed after a history reset. Source relinking does
not rewrite a creation request's original workspace binding. Persisted-boundary
fixtures are not physical power-loss tests. Physical iOS/Android, permanent
remote deployment, device enrollment, mobile notifications and file transfer
remain open; this change does not make a sleeping Mac available.

### Word and Excel package verification — 29 October

The independent Open XML SDK check now covers DOCX and XLSX as well as PPTX.
Before these fixes, each new Word fixture failed on a missing table grid, and
each new Excel fixture produced three font/chart child-order errors. The Excel
failures also appeared with the Office 2019 validation target.

Word now emits a table grid, explicit borders instead of a reference to an
undefined table style, and native run-level breaks/tabs in paragraphs and cells.
The structure follows Microsoft's
[WordprocessingML table documentation](https://learn.microsoft.com/en-us/office/open-xml/word/working-with-wordprocessingml-tables).
Excel output normalizes font and supported chart-series child ordering after
Excelize serialization. It removes only unsupported false/default chart flags;
meaningful unsupported values reject rather than silently losing their meaning.
The normalizer retains namespace prefixes and child bytes, leaves unrelated ZIP
parts untouched, and preserves unfamiliar extensions/mixed content unchanged.
This is a targeted compatibility correction, not a general imported-file repair.

Observed checks on this Mac:

- All 24 generated/edited packages pass Open XML SDK 3.5.1 against Office 2007:
  nine PPTX, seven DOCX and eight XLSX. Fixtures cover three themes, filled Word
  templates, column/bar/line/pie charts, edited workbooks retaining charts, a
  10,001-row streaming workbook, and all three shared-editor round trips.
- Full document/workbook race suites pass. Regressions check escaped Unicode,
  CRLF/newline/tab text, table grids, unchanged unrelated workbook parts/chart
  references, stable repeated normalization and refusal of semantic loss.
- Shared-editor checks preserve the Word table, multiline/tab/emoji text and
  Excel formulas with blank caches for later recalculation. The rebuilt runtime
  is used for the terminal Office keyboard check recorded in the migration ledger.
- Quick Look thumbnails were generated and inspected. Word shows the paragraph
  break, tab spacing, table borders and multiline cell. Its thumbnail shows a
  box at the page-break position instead of establishing pagination. Excel's
  thumbnail is cropped to the first column; it establishes neither chart layout
  nor recalculated totals. These are preview limitations, not visual acceptance.
- CI now generates and independently validates all three formats. Workflow YAML
  parses and `git diff --check` passes; remote CI has not been run.

G10 remains open for native Word/Excel/PowerPoint acceptance, pagination, dense
layouts, arbitrary template fidelity and application recalculation. No actual
Office application was available for these checks. Schema validity alone does
not establish those outcomes. Local evidence remains under ignored
`.gator/native-tools/office-format-audit`, `office-formats-fixed` and
`office-format-previews`.

### Repair after moving an installation — 29 October

The installed login service previously had only a disable action in Recovery,
so repairing an obsolete executable required disabling and enabling it again.
Recovery now provides a single reviewed repair, showing the current executable
and profile paths before replacement/restart. Unix service commands recheck the
reviewed configuration digest, retain the resolved configuration directory and
refuse another profile's service or a linked configuration. Failed activation
reports that configuration was saved and allows a fresh repair attempt.

Local verification includes saved-check preservation, activation-failure retry,
stale/foreign-profile refusal, a 19-assertion terminal keyboard journey and actual
macOS launchd relocation between two copies of the bundled Go runtime, with a new
authenticated supervisor identity and verified service removal afterward. The
native test uses a unique temporary label and isolated profile. Full details and
the repeatable command are in the terminal migration verification ledger.

G13 remains open for complete update/drain/uninstall behavior, native Windows,
signed distribution and release-account work. The repair action is user-reviewed;
updating the application does not silently migrate or enable background services.

### Cloud setup failure recovery — 29 October

Cloud-model setup used a load/save pair outside the shared settings transaction.
It could overwrite an unrelated settings edit, and published the new model before
writing its API key. A failed credential write therefore left the model selection
changed even though setup reported failure.

The save now participates in the shared settings lock. It writes the credential
first and restores the prior credential if settings publication fails, using
compare-and-swap so recovery cannot overwrite a newer credential or resurrect a
disconnected one. A process-held setup lease prevents overlapping cloud setups
from restoring each other's temporary keys. Credential changes observed while
waiting for the settings lock refuse publication. These are coordinated normal
failure paths, not an atomic transaction across the two files: abrupt process or
power loss between publications still requires checking the saved configuration.

The terminal journey exposed two additional failures: the snapshot truncated an
error before its recovery instruction, and the frontend retained and resent the
key after Go cleared the failed form. Shared model notices now retain bounded
full text. Both frontends discard submitted secret fields; a failed cloud save
requires explicit key re-entry, while ordinary non-secret edits remain available.
Successful setup says it was saved locally and that provider access is untested.

Observed verification:

- Focused Go race checks cover credential-store failure with unchanged settings,
  settings-validation failure restoring an existing or absent key, concurrent
  credential replacement, overlapping setups, lock release/retry and preservation
  of another settings transaction's theme and model alias.
- Full modelcatalog/config race suites pass, including visible recovery text,
  cleared secret input, refusal of a blank retry and successful explicit retry.
- Four terminal journeys pass with 58 assertions: model/effort selection, first
  task draft and attachment across setup/restart, failed credential save/re-entry,
  and connection verification/browser-sign-in ownership using local fixtures.
- A Chromium component journey using the real retained desktop Panel passes:
  neither a failed save nor a transport failure silently resends the key; other
  form fields remain. This is a browser component check, not native Electron
  acceptance. The check is included in the existing Linux browser CI step.
- Runtime/terminal build, terminal/desktop TypeScript checks, Windows command-test
  cross-compilation and workflow YAML parsing pass. Remote CI was not run.

G05 remains open for observed novice use, actual provider access and distributor
Google OAuth registration/verification. No account or paid model was contacted;
real-account sign-in remains deferred at the user's request.

### Follow-up decisions from the phone — 29 October

The companion now exposes Mark read, Done and Reopen for retained follow-up
updates, with earlier/newer history pages, next steps and evidence. These use the
existing host conversation/identity/version checks. Marking done changes attention
state only; a failed check remains visibly failed. Opening or polling a chat does
not silently mark updates read or resolved. Replies with an update reference and
quality feedback remain host operations.

A lost response leaves an explicit unconfirmed notice. Refresh reads the retained
status without replaying the mutation. Stale resolution changes show the host's
error and refreshed version. Polling retains expanded details and keyboard focus;
the initial keyboard test caught focus loss when controls became disabled during
a request, which was corrected. Unavailable update storage is distinguished from
an empty conversation. Empty update sections stay out of ordinary chats.

Verification on this Mac:

- All six companion HTTP tests pass. The real-Go journey now seeds 55 retained
  updates, pages 50/5, rejects another conversation and stale resolution version,
  preserves read/done/version across companion restart and token rotation, and
  makes no model request for receipt or paging operations.
- All eight Chromium/WebKit browser journeys pass. New follow-up checks exercise
  explicit read, stale Done, successful Done/Reopen, an applied mutation whose
  response is lost, refresh without replay, older/newer pages, retained draft,
  unavailable storage, keyboard focus, offline controls and revoked access.
  Focused checks also verify that an empty section is hidden.
- Dark/light 390-pixel Chromium/WebKit screenshots were inspected. Overflow
  checks pass at 320, 390 and 760 pixels; update buttons are at least 44 pixels
  high. Hostile title/evidence markup stays plain text. Evidence is under ignored
  `.gator/native-tools/companion-*-updates-*.png`.
- The first WebKit attempt found no browser matching the freshly installed
  Playwright 1.64.1. Installing its WebKit 2370 in the isolated test directory
  allowed all four WebKit journeys to run and pass. No remote CI was run.

Interface review: **full** mode, limited to the companion follow-up cards, status
actions, paging, error/empty/offline states and focus behavior. Framework: plain
DOM; styling: the existing CSS tokens and native disclosure controls. No new
animation, icon library or styling framework was introduced.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Dark/light screenshots, hostile titles and failed-check copy | Readable wrapping; update count uses tabular numbers. |
| Surfaces | Existing cards/buttons, 320–760-pixel bounds, disabled/focus states | Shared styling and 44-pixel touch controls; keyboard focus retained. |
| Animations | New DOM/CSS paths | No animation; 10% speed motion inspection is not applicable. |
| Icons | Native details disclosure; text action buttons | Existing browser disclosure retained; no new icon treatment. |
| Performance | Poll/render code and expanded-detail checks | At most 50 visible updates; unchanged polling does not rebuild cards. |

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| HIGH | `companion/public/client.mjs` follow-up cards | Phone updates were read-only, with no history controls. | Explicit read/done/reopen and paging retain the original check result. | Makes the intended interaction available without implying successful execution. |
| MEDIUM | `companion/public/client.mjs` render/refresh | Polling rebuilt cards; disabling buttons lost keyboard focus. | Stable unchanged cards, retained expanded details and focus restoration. | Preserves interaction context during frequent refreshes. |
| MEDIUM | `companion/public/index.html`, `companion/public/styles.css` | No grouped follow-up status/history surface. | Labeled section, failed-check context, wrapping sources, shared touch buttons and hidden empty section. | Readable hierarchy and adequate hit areas without crowding ordinary chats. |

Considered but rejected: animated status/card transitions would add movement to
frequent polling; new action icons would introduce a visual vocabulary the small
client does not otherwise use. Text labels and the existing CSS were sufficient.
Verdict: **Approve for the inspected scope**. Physical iOS/Android, native Safari,
screen-reader acceptance and deployed remote-host behavior remain unverified.

G14/G15 remain open for permanent always-on deployment, device enrollment and
per-device revocation, mobile notifications, file transfer and broader remote
workflows. A sleeping/offline Mac is still unavailable. No production service or
real account was activated.


### Nested-profile uninstall protection — 29 October

A synthetic fixture demonstrated a data-loss bug: successful uninstall recursively
removed profile data stored under its owned installation. No real profile was
used. The leased Go helper now reports resolved state/configuration storage paths;
the uninstaller checks those plus default, terminal and desktop draft storage.
It resolves directory aliases and missing descendants, refuses overlapping paths
before removing the launcher, and rejects runtime metadata it cannot verify.
Installation roots and manifests must be ordinary directories/files. Successful
removal reports retention of checked storage, rather than claiming all profiles
were discovered.

Validation: 14 Node installer/uninstaller tests pass, including each storage
override, default drafts, existing/dangling directory aliases and a non-overlapping
sibling path. An independently packaged macOS artifact passes the Bun install,
launcher recovery, active-profile refusal, nested-draft refusal, removal and
retained-note lifecycle. Go guard race checks pass; Windows command tests compile.
The terminal OS matrix now includes these uninstall regression tests after the
runtime build. No remote CI or native Windows execution was performed.

The broader application-host run exposed an unrelated macOS test expectation:
export correctly returned the canonical `/private/var/...` location, while the
test expected its `/var/...` alias. The assertion now compares the canonical
parent while continuing to exercise writes through the original alias. The host
suite then passes 77 tests; its native Windows handoff test is skipped on macOS.

Limits: unselected profiles or other private files placed in the installation are
not automatically discovered. The existing exclusive profile guard is retained;
uninstalling an older installation while another process uses that profile still
requires work to stop. This does not close the broader release lifecycle gate or
claim atomic deletion on Windows. Real-account sign-in remains deferred.


### Linux companion service and native lifecycle — 29 October

The companion now has a reviewable Linux user-service generator and example
configuration. Preparation checks the runtime, entry, selected profile/workspace
and private token, then exclusively creates a mode-0600 unit. It never activates
a service or changes login/boot policy. The README describes explicit installation,
status, journal access, removal, optional administrator-managed lingering and the
separate Gator job supervisor required for recurring checks.

The generated unit preserves exact argv and environment values, escapes systemd
percent specifiers, disables command-line environment substitution and explicitly
sets the selected origin/profile. It keeps the existing loopback listener, uses
journal output, restarts the listener after exit and permits graceful child
shutdown before its stop deadline. The token contents are never embedded.

Validation used a disposable Debian Bookworm Linux container on this Mac's Docker
VM, Node 24 and systemd 252.39. It had no external network, published ports or host
filesystem mounts. Container-local privileges permitted its own systemd manager;
all repository code and the cross-built Linux Go runtime were copied in. The
native test ran as the ordinary `node` user, not root. All three Linux tests pass:
plan/preparation validation plus a real user-manager lifecycle. The lifecycle
verifies literal paths with spaces, quotes, backslashes, Unicode, percent and
variable-like text; authenticated creation of a real Go conversation; forced
listener crash and a new PID; retained history; opening a Go worker; worker exit
on service stop; retained data after stop; and removal of the temporary unit.
The container was removed afterward. No accounts, paid models or production
services were used.

The first container attempts exposed an executable-mount setting and a manager
startup race in the disposable harness. After correcting those, systemd's real
parser rejected quoted WorkingDirectory syntax. That product bug was fixed using
a raw path with escaped specifiers and a trailing directory separator; the full
native lifecycle then passed. Mac-local plan checks also pass; native opt-in tests
are skipped in that run. Workflow YAML parses and diff checks pass.

CI now includes plan tests and a Linux user-manager lifecycle step. No remote CI
was triggered. The initial service files were committed in an intervening user
commit; the path correction, lifecycle assertions and CI changes have separate
commits for rollback.

G14/G15 remain open: a permanent host, HTTPS/tunnel deployment, logout/reboot
acceptance, supervisor-plus-listener acceptance, physical-phone access, device
pairing/revocation and mobile notifications are not established by this container
run. The execution host must remain on; the new generator does not make the Mac
or its files available while it is asleep or offline.


### Offline companion monitoring and CLI chat routing — 29 October

A combined Linux service test now exercises the job supervisor and companion
against a real Go runtime and a loopback model fixture. Its first complete run
proved that a CLI-created check finished while the listener was stopped, but the
follow-up card was absent on reconnect. This exposed an integration gap: ordinary
CLI jobs have no originating chat, while chat update controls deliberately require
that exact owner. The global host inbox retained the result correctly.

`gator job add --conversation ID` now explicitly binds a new job to an existing
chat in the selected profile. The chat's folder becomes the default source instead
of shell cwd; an explicit source must refer to the same directory. Source edits
preserve this binding. Missing chats and mismatched sources fail before job
publication. The command reports the owning chat ID. Jobs without the option keep
their existing host inbox behavior. Execution conversations do not inherit the
originating chat's update authority; no inbox ownership checks were weakened.

Validation: focused command/edit/follow-up race tests and full inbox/jobs race
suites pass. Windows command tests cross-compile. A rebuilt Mac runtime passes a
real Bun-launcher smoke check from a different cwd, retaining the selected chat
and its source without starting a supervisor or model. Workflow YAML and diff
checks pass.

All four Linux service/monitoring tests pass in the disposable Debian systemd
container. The new journey creates its originating chat through the companion
API, adds the linked check through the runtime command path used by the terminal,
and starts the real supervisor as a unique transient user service. It waits for
the scheduled model request, disables future runs, stops the companion listener,
and then releases the admitted model response. It verifies a completed durable
attempt while the listener is inactive. After reconnecting, the originating chat
shows the retained, unread and unresolved card; the execution chat has no authority
for it. Supervisor restart and subsequent reads preserve the same update and make
no additional model requests. The temporary listener unit, supervisor and container
are stopped/removed. No real accounts, external network or production services
were used. CI includes this combined journey; remote CI was not run.

Limits: the test uses a transient supervisor service, not a permanent production
installation. Host reboot/logout behavior, permanent deployment, HTTPS and physical
phone access remain unverified. The companion still has no global inbox for
unbound CLI updates. Scheduled reminders and mobile/desktop push delivery are
separate work; this change routes scheduled-work results into an existing chat.


### Individually revocable companion device access — 29 October

The companion now supports host-managed device enrollment with separate bearer
tokens. `companion/devices.mjs OWNER_TOKEN_FILE add NAME NEW_TOKEN_FILE`, `list`
and `revoke DEVICE_ID` provide local management. Enrollment writes the secret to
an exclusive private export file and atomically publishes a complete private
registry record containing hashes, identity, label and creation time. Concurrent
enrollments use separate records and do not overwrite each other. Device secrets
are never printed or retained in plaintext in the registry. The browser's existing
token field accepts owner and device token formats.

The server revalidates the device and owner-token generation before queued work,
after worker startup and before returning private data. Revoked work that has not
been admitted receives HTTP 401; already admitted actions are not undone. Removing
one device leaves others connected. Owner-token rotation invalidates all enrolled
tokens without restarting the listener. Private directories, bounded regular-file
reads, symlink refusal and exact device identities protect registry access on
macOS/Linux. Existing owner-token access remains compatible.

Validation: ten authentication/HTTP tests pass, including concurrent CLI creation,
no secret output, overwrite refusal, individual revocation, global rotation,
malformed/public/linked storage refusal, revocation during worker startup and a
private response, and access across listener restart. Chromium and WebKit each
pass a two-device browser journey: both connect, one is revoked and cleared,
the other remains connected, and no token enters browser persistent/session
storage. Existing real-Go admission/deduplication coverage also runs with an
enrolled device token. Native Mac launchd startup/restart/removal passes (two
tests), and all four Linux service/monitoring tests pass with the new auth module
copied into the disposable service installations. Workflow YAML and diff checks
pass. Temporary services and the Linux container were removed; no real accounts,
production service or remote CI run was used.

Limits: enrollment is a host CLI operation, not interactive QR/device pairing.
All device tokens grant the same supported owner-profile access and can be copied;
labels do not bind credentials to hardware or create separate users/roles. The
browser clears private views on its next request after revocation, not while
fully offline, and cannot erase information already copied by a client. Exported
token copies require owner cleanup. Interrupted enrollment and power-loss limits
are documented; native Windows ACL acceptance remains open. Permanent hosting,
HTTPS deployment, physical-phone acceptance and mobile notifications are still
unverified. G14/G15 are not closed by these local tests.


### Independent spreadsheet calculation engine — 29 October

The Office fixture generator now produces explicit calculation expectations and
two additional workbooks covering analysis formulas and a stale manual-calculation
template with a dependent unedited sheet. A new Python checker invokes LibreOffice
in a temporary profile and reads its saved XLSX results using standard ZIP/XML
parsers. It does not use the renderer's formula evaluator. It requires coverage
for every generated workbook and formula, checks the original caches are empty,
refuses an existing output directory, verifies the input hashes remain unchanged,
and includes a wrong-total negative control. No renderer changes were necessary
for the exercised calculations.

Local evidence: LibreOffice Calc 24.2.7.2 recalculated all ten XLSX fixtures and
matched 10,024 expected formula values plus a formula-looking literal string.
Coverage includes all three themes, ordinary/streaming writers, template and
shared-editor changes, quoted cross-sheet references, negative/zero/fractional
inputs, conditional sums/counts, mean/rounding, conditions, lookup, booleans,
text results and an intentionally visible division error. The old template
values 999/1998 were absent before conversion; the independent results were
200/400. The two additional workbooks have zero Office2007 schema errors under
the independent Open XML SDK. A false expected total is rejected by the checker.

Calculation ran in a disposable Ubuntu container with no external network or host
filesystem mounts; only generated synthetic files and the verifier were copied
in. The container was removed. The result copies and JSON report are retained
under ignored `.gator/native-tools/calc-results/`; the report records engine version
and original file digests. Final coverage/reinspection, Python syntax, workflow
YAML and diff checks pass. CI now runs this calculation gate and uploads the
result copies/report, but no remote CI was triggered.

G10/document quality remains open for native Microsoft Excel, Word and PowerPoint
acceptance, representative customer files, visual layout and arbitrary template
fidelity. These fixtures demonstrate independent calculation compatibility; they
do not establish that an arbitrary model-generated analysis is correct.

### Independent Word/PowerPoint layout evidence — 29 October

Added a dense-slide fixture and an independent Writer/Impress → PDF check.
LibreOffice 24.2.7.2 rendered all seven DOCX and ten PPTX fixtures into 42 pages.
Checks cover expected text per page, Word page breaks, edited slide order,
page/slide text bounds, table column/row alignment and multiline cells. Four
negative controls reject missing text, reordered slides, off-page text and a
displaced table cell. Original hashes remain unchanged; output directories must
be fresh. The additional slide has zero independent Office2007 schema errors.

Image review found a missing crocodile glyph despite successful PDF text
extraction. Installing the emoji fallback font in the test environment fixed its
appearance in Word and PowerPoint, and the complete check passed again. CI now
installs the required rendering/font tools and retains PDFs, PNGs, extracted
text/bounds, font inventories and a report, including artifacts on failure.
Local evidence is in ignored `.gator/native-tools/layout-with-emoji/`; the
disposable rendering container was removed. No remote CI was triggered.

No production renderer changes were needed for these cases. G10 remains open
for native Office, representative customer documents, arbitrary template fidelity,
general typography/overflow and design quality. Automated text/bounds checks do
not prove glyph appearance or detect every overlap. The verification ledger
records the inspected examples, font-dependent limitation and repeat command.

### Unattended companion/supervisor startup — 29 October

The new `scripts/companion-boot` harness exercises the actual companion generator
and `job systemd install` in a disposable Debian systemd container. An enrolled
device creates the originating chat; the CLI adds a linked, initially unexecuted
scheduled check. Both services are enabled, and lingering is enabled only for
the container's test user. After two container restarts, fresh user-manager
invocation IDs, cleared runtime-directory markers and zero login sessions prove
that the test did not merely restart the listener or manually start a supervisor.

Local result: both services automatically start, the first boot completes the
check, and the second preserves its original unread/unresolved update and device
access. Unauthenticated catalog requests return 401. After disabling future
schedule slots, the retained attempt and local model request counts stay at one
through a 17-second observation after the second boot. This includes more than
the supervisor's 15-second poll interval; it does not establish an unbounded
exactly-once guarantee. No production service code change was needed.

The final run used systemd 252.39, Node 24.22.1 and Docker 29.7.2 on this Mac's
Linux Docker VM. Runtime hash, service/user-manager invocation IDs, counts and
diagnostics are retained under ignored `.gator/native-tools/companion-boot-verified/`.
The container was removed; no host mounts, published ports, real accounts or
external model requests were used. CI now runs the same harness and retains
evidence on failure; no remote CI was triggered. Syntax, workflow YAML and diff
checks pass. Physical-host/kernel reboot, remote network/proxy setup, permanent
deployment and physical phone acceptance remain open.
