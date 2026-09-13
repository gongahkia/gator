# Local TUI parity matrix

This document records the acceptance scope for the completed repository pivot
from the former React PWA, self-hosted reliability stack, and C++/Qt desktop to
a **local, offline-first TUI plus a complete non-interactive CLI**. The current
roadmap and release gates live in the [development checklist](../../CHECKLIST.md).

It is implementation-neutral: it states product behavior, not libraries,
widgets, or package layout. The delivery is a locally installed Python program;
that does not change the rules below.

Legacy paths and documents named below are historical evidence available through
Git history, not current files or products. The destructive retirement was
authorized with the live Google gate explicitly waived and unexecuted; that
waiver does not count as a live acceptance pass.

## Status labels

| Label | Meaning for the TUI product |
| --- | --- |
| **Retain** | Ship this behavior. Tests must cover it. |
| **Fix-gap** | Intended or documented, but missing, ignored, or contradicted in current code. The TUI must implement the *intended* behavior, not the broken one. |
| **Defer** | Explicitly out of the pivot. Do not block parity. Record a follow-up; do not pretend it exists. |
| **Retire** | Stop shipping. Do not port. Git history is the archive. |

## Current products (what exists today)

| Surface | Role today | After parity |
| --- | --- | --- |
| `web/` React PWA | Direct browser OAuth to Google; IndexedDB cache and outbox; optional self-hosted Fastify/PostgreSQL “reliable” mode | **Retire** the product surface after the gate |
| `backend/` + `selfhost/` | User-operated refresh-token store, 5-minute poll, Web Push | **Retire**; local process replaces reliability |
| `native/` C++20/Qt | Desktop OAuth PKCE, Keychain, SQLite, tray, OS notifications | **Retire** as a shipped product after the gate; keep Git history |
| `docs/hcb-cli.md` | States that no CLI ships | **Retire** that status; CLI is a required surface |
| `khal/` and `gcalcli/` | Visual/interaction references only | Never dependencies; never storage/auth models |

Google Calendar and Google Tasks remain the **authoritative** synced services. Local storage is a replaceable cache plus a durable mutation journal. Notes remain undated Google Tasks, not a second remote schema.

---

## Doc-versus-code contradictions

These are not style nits. They decide what “parity” means. **Do not port the lie.**

### 1. Command palette is search, not commands

- **Docs:** [README2.md](../../README2.md) says the palette can switch surfaces, open cached items, **create** a task/event, **sync**, **find a time**, or **manage calendars**. [docs/specs/core-app.md](../specs/core-app.md) says the palette exposes create, navigation, search, sync, settings, and diagnostics.
- **Web code:** `CommandPalette` actions are `navigate`, `open-task`, `open-event`, `open-drive-file`. Empty query shows no commands.
- **Web tests:** `web/tests/CommandPalette.test.tsx` asserts the palette **does not** expose workspace action shortcuts or commands (querying “refresh all tasks” does nothing).
- **Web Settings copy:** palette “opens cached search.”
- **TUI decision:** **Fix-gap.** Palette must run the actions README2 claimed *and* keep title-first local search. Create/sync/find-time/calendar management are first-class commands, not documentation.

### 2. Notes projection modes are a type, not a product

- **Docs:** PRD and core-app: notes can be **disabled**, **notes-only**, or **mirrored**.
- **Web types/store:** `notesProjectionMode` exists (`disabled` \| `notes-only` \| `mirrored`), default `mirrored`, and `localStore.test.ts` persists it.
- **Web UI:** Settings never exposes the field. Nothing reads it. The Notes view always lists undated root tasks (`TaskPanel` `panel="notes"`).
- **Native:** `notesProjectionMode` is **notes-only or mirrored only** (`isValidNotesProjectionMode`). Disabled is not a valid native setting. Notes-only hides undated tasks from the Tasks model.
- **TUI decision:** **Fix-gap.** Honor all three PRD modes in both TUI and CLI. Do not ship a persisted-but-ignored preference.

### 3. Saved searches exist in native, were deleted on web

- **Docs:** PRD and core-app require saved, ranked, local search.
- **Native:** `SavedSearchStore`, QML save/rename/delete/apply.
- **Web:** IndexedDB upgrade **deletes** `savedSearches`. No UI.
- **TUI decision:** **Fix-gap** (promote native behavior). Saved named queries with the same DSL as live search. Not a web port.

### 4. “No global quick-capture” vs in-app capture

- **Docs:** PRD and native-parity **exclude a global OS quick-capture shortcut**. That exclusion stands.
- **Web:** In-app Quick capture (`QuickCaptureDialog`, `Meta+Shift+N` by default) is implemented and tested via parser logic.
- **TUI decision:** **Retain** in-app / CLI capture (`hcb capture` and a TUI capture command). **Retire** any claim of a global hotkey. Do not treat PRD’s exclusion as “no capture at all.”

### 5. Three “current products” in the docs

- [README.txt](../../README.txt) and [docs/README.md](../README.md): C++/Qt desktop is the product.
- [README2.md](../../README2.md) and [docs/architecture/web-pwa.md](../architecture/web-pwa.md): browser PWA is the product.
- [docs/product/prd.md](prd.md): “local desktop client **and** web client.”
- [docs/architecture/tech-stack.md](../architecture/tech-stack.md): Electron/React are **retired architecture**. The web tree still ships React.
- **TUI decision:** Treat this matrix as superseding those “current product” sentences after the retirement gate. Until then, use web for sync/UI behavior and native for credentials/SQLite/reminders.

### 6. Search DSL is not one language

- **Core-app:** `source:`, `status:`, `due:`, `start:`, `priority:`, `list:`, `calendar:`, `notes:`/`body:`.
- **Web palette:** also `type:`, `completed:`, `date:`, `in:"Calendar name"`, `task:`/`event:`/`drive:`. `source:local` matches **no** Google-backed tasks/events (filter always fails). Body search is opt-in (`notes:`/`body:` or a second step), matching README2’s Things 3 note — **retain** that ranking rule.
- **TUI decision:** **Fix-gap.** One documented DSL, validated with chips/errors, used by TUI palette, CLI `search`, and saved searches. `source:local` must mean local-only fields (priority, scheduled-block links, pending mutations), not “match nothing.”

### 7. Reminders: tab-bound vs OS-bound

- **PRD / native-parity:** Calendar popup reminders → local OS notifications; Snooze 10 minutes and Dismiss; email reminders never become alerts. Fedora uses `hcb-reminderd` after GUI exit.
- **Web architecture:** Desktop reminders are **native-only**. Direct PWA reminders run **only while an authorized tab is open**. Self-host Web Push is best-effort.
- **Web code:** `ForegroundReminders` plus optional managed Web Push; 90-second delivery window for due items.
- **TUI decision:** **Retain** native semantics (OS notifications while the TUI is closed). **Retire** tab-bound and Web Push as the reliability story. Task reminder markers (one exact local time per task) stay **retain** (web + PRD).

### 8. OAuth models are incompatible; pick desktop

- **Web:** user-owned **Web** OAuth client ID; GIS token model; **no refresh token**; reconnect on expiry; client secret forbidden.
- **Current desktop implementation:** user-owned **Desktop** OAuth client ID and
  optional secret in an owner-only local `.env`; PKCE loopback; refresh token
  encrypted in that file with its key in the OS credential store, never SQLite/QML/logs.
- **TUI decision:** **Retain native/desktop rules.** Do not port the browser token model. **Retire** Web client IDs and GIS as product requirements.

### 9. Sync incrementality is documented well and must be kept

Web README2 + `useWorkspace` and native google-sync agree on the hard parts:

- Calendar: full canonical collection, `nextSyncToken`, per-calendar tokens, page-at-a-time commit, resume records, **`410 Gone` rebuilds only the affected cache**, no `timeMin`/`timeMax`/`orderBy` on sync-token event lists.
- Tasks: **no** sync tokens; enumerate lists; `updatedMin` + overlap + `showHidden`/`showDeleted`; explicit **Refresh all Tasks**.
- Writes: optimistic local + durable outbox; Google wins by default; Prefer Local / Ask are user-selectable.

**Retain** this contract. Quota-exhaustion behavior that drops **only** regenerable occurrence cache (web) maps to dropping derived instance/layout cache, never canonical events or checkpoints.

### 10. Retired docs still look live

Electron CLI/MCP, local hoster/vault, browser extension, `.hcbvault`, historical roadmap phases, and “pnpm hcb” workflows are already marked retired in places (`docs/hcb-cli.md`, `docs/mcp.md`, `docs/specs/local-hoster.md`, `docs/browser-extension.md`) and still cited elsewhere (`docs/portable-export.md`, `docs/improvements/*`, release notes). **Retire** them as requirements. Do not implement MCP, vault, or Electron packaging in the pivot.

### 11. Calendar ACL

README2, PRD, native QML, and roadmap agree: **full sharing/ACL administration is deferred**. Calendar create, subscribe, remove-from-list, free/busy, Meet, Drive metadata, RSVP, status events stay in scope.

---

## Feature inventory

Legend in the **TUI** column uses the four labels. Evidence is abbreviated: **W** web, **N** native, **D** docs, **T** tests.

### Auth, config, identity

| Capability | Evidence | TUI |
| --- | --- | --- |
| User-supplied Google Cloud project; Tasks + Calendar APIs enabled | W onboarding, N README, D PRD | **Retain** |
| Desktop OAuth client ID + PKCE loopback; optional local client secret | google-sync spec | **Retain** (replaces web GIS) |
| Encrypted refresh token in owner-only account file; encryption key in OS credential store | google-sync spec | **Retain** |
| Tokens never in SQLite, `config.json`, logs, diagnostics, or URLs; encrypted only in the owner-only account credential file | google-sync spec + diagnostics tests | **Retain** |
| OpenID subject partitions local data | W `localStore` + tests | **Retain** |
| Connect / reconnect / disconnect; disconnect does not have to wipe cache without confirmation | W Settings, N | **Retain** |
| Destructive clear-local-data requires confirmation | W, local-data spec | **Retain** |
| First-run wizard: client ID, account, timezone, theme, reminder permission | W Onboarding e2e (web-shaped), N setup | **Fix-gap** (desktop-shaped onboarding) |
| Config: appearance, density, week start, 12/24h, display TZ, work hours, visible calendars, conflict policy, keybindings | W Settings, PRD | **Retain** (terminal-appropriate presentation) |
| Web OAuth client ID, GIS token model, no persisted refresh token | W README2, `tokenSession` | **Retire** |
| Self-hosted managed connection, `HttpOnly` cookie, operator-held refresh tokens | W `managedConnection`, selfhost | **Retire** |
| `VITE_HCB_SELF_HOSTED_RELIABILITY_ENABLED` public Pages vs selfhost split | W README2 | **Retire** |
| Browser font stylesheet URLs / Google Fonts CSS | W Settings | **Retire** (use terminal fonts/themes) |

**Acceptance — auth/config**

- Fresh install starts without network and explains that Google is required only for sync.
- Saving a Desktop client ID and completing PKCE stores tokens only in the OS credential API; a file dump of config + SQLite contains neither refresh nor access tokens.
- Disconnect leaves cache until an explicit confirmed reset.
- Invalid client ID, denied keychain, occupied loopback port, and revoked grant produce actionable errors, not stack traces.
- Account subject switch cannot mix another account’s outbox or cache.

### Sync

| Capability | Evidence | TUI |
| --- | --- | --- |
| Google is source of truth for synced fields | PRD, google-sync | **Retain** |
| Calendar `nextSyncToken`, page resume, cancel, progress | W README2, `useWorkspace`, SyncDialog tests | **Retain** |
| Per-calendar `410` full rebuild | W + N | **Retain** |
| Tasks `updatedMin` overlap; Refresh all Tasks | W Settings + README2 | **Retain** |
| Canonical events vs visible-range expansion; search does not expand every recurrence | W calendar search worker + tests | **Retain** |
| Conflict policy prefer-google / prefer-local / ask | W Settings, N | **Retain** |
| Bounded, cancellable Google work | core-app, QA plan | **Retain** |
| Manual vs launch/foreground vs polling sync modes | google-sync spec; native settings keys (partial) | **Fix-gap** (explicit modes; polling is a **local process**, not a hidden TUI loop) |
| Direct-mode “sync only while tab open” | W architecture | **Retire** |
| Server-side 5-minute worker + Calendar watch | selfhost | **Retire** (optional local scheduler is the replacement; see reminders) |
| IndexedDB quota dialogs | W | **Retire** as browser quota; **retain** “drop derived cache, keep canonical + checkpoints” |

**Acceptance — sync**

- Offline startup renders the last successful cache.
- Initial Calendar sync commits page-by-page; killing the process mid-page allows resume without duplicating canonical rows.
- Injected `410` on one calendar rebuilds only that calendar.
- Tasks incremental fetch uses watermarks; Refresh all Tasks rebuilds the task mirror and watermarks.
- A successful remote write replaces optimistic state; a 409/412/410 does not drop the local intent.
- Prefer Google auto-applies when a remote version exists; Prefer Local and Ask require an explicit user choice via TUI or CLI.
- No command that claims to “list today’s events” may hide a network fetch behind a local-looking path. Reads are SQLite; sync is named.

### Tasks

| Capability | Evidence | TUI |
| --- | --- | --- |
| Lists: create/rename/delete | W TaskPanel, N | **Retain** |
| Hierarchy, reorder, reparent | W drag/keyboard, N | **Retain** |
| Create/edit/complete/delete | W + N | **Retain** |
| Cross-list move = create destination + delete source; new remote ID communicated | PRD, google-sync | **Retain** |
| Batch actions with confirmation for destructive ops | W TaskPanel | **Retain** |
| Date-only due; timed work is a Calendar event | PRD | **Retain** |
| Local priority metadata | W `TaskMetadata` | **Retain** |
| HCB recurrence markers in task notes; successor duplicate reconciliation | W `taskRecurrence.ts` + tests, PRD | **Retain** |
| One exact portable task reminder time | W markers + PRD | **Retain** |
| Schedule-as-event link; one active block; orphan repair | google-sync, W `ScheduledTaskBlock` | **Retain** |
| Markdown task notes | W MarkdownEditor tests | **Retain** (plain + markdown; TUI editor may be external `$EDITOR`) |
| Drive metadata attach on tasks | W picker | **Retain** (link/metadata only) |

**Acceptance — tasks**

- CRUD, indent/outdent, reorder, and cross-list move work offline and flush through the outbox.
- Recurrence markers round-trip Google Task notes without corrupting user notes; malformed markers are diagnosed, not silently stripped.
- Completing a managed occurrence follows existing successor rules (web/native tests are the behavioral oracle).
- Scheduling a task creates or relinks exactly one Calendar event; deleting that event in Google marks the block orphaned and offers repair.

### Notes

| Capability | Evidence | TUI |
| --- | --- | --- |
| Notes = undated root Google Tasks | PRD, W TaskPanel filter | **Retain** |
| `disabled` / `notes-only` / `mirrored` | PRD vs W unused field vs N two modes | **Fix-gap** |
| Separate Notes surface when not disabled | W Notes view, N notes model | **Retain** |

**Acceptance — notes**

- `disabled`: no Notes surface; undated tasks appear only as tasks.
- `notes-only`: undated root tasks appear in Notes and are hidden from the Tasks tree.
- `mirrored`: same rows in both.
- Changing mode never creates a second Google resource type.

### Calendar

| Capability | Evidence | TUI |
| --- | --- | --- |
| Agenda / day / week / month | W CalendarPanel, PRD | **Retain** |
| Visible calendar filters and colors | W, PRD | **Retain** |
| Structured create/edit/delete/move; all-day date-stable; timed in event TZ | W CalendarPanel, PRD | **Retain** |
| Google recurrence lines unchanged; instances API authoritative | PRD, import docs | **Retain** |
| Split/exception flows for recurring events | W `splitRecurringEvent` | **Retain** |
| Invitations inbox + RSVP + comment | W CalendarPanel, core-app | **Retain** |
| Meet create; Drive metadata attachments | W, PRD | **Retain** |
| Free-busy / find-time | W CalendarTools + tests | **Retain** |
| Calendar create, subscribe, remove from list | W, N QML copy | **Retain** |
| Focus / OOO / working-location fields | W types + editors, PRD | **Retain** |
| Bulk event actions | W `bulkEvents` | **Retain** |
| Send-updates for guested events | W `SendUpdates` | **Retain** |
| Year view, maps, hover previews | improvements docs (legacy) | **Defer** |
| Full ACL / sharing administration | all current docs | **Defer** |
| Local RFC 5545 expansion as source of truth | PRD exclusion | **Retire** as a product rule (never do this) |
| ICS/vdir as live store (khal model) | khal reference | **Retire** as architecture; ICS is import/export only |

**Acceptance — calendar**

- Views render from cache; changing range may expand instances for **visible** calendars only.
- Creating a timed event stores wall time in the selected IANA zone; all-day dates do not shift across zones.
- Recurrence lines sent to Google are the user’s lines; cancelled/changed instances come from Google, not a local expander.
- Find-time uses free-busy and never writes until the user confirms a slot.
- RSVP updates go through the outbox and conflict path.

### Search and palette

| Capability | Evidence | TUI |
| --- | --- | --- |
| Local-first; no Google per keystroke | README2, core-app, palette tests | **Retain** |
| Title-first; body/notes as explicit second step | README2, CommandPalette tests | **Retain** |
| Historic canonical events searchable beyond visible range | W calendar search tests | **Retain** |
| Cached Drive names after user-authorized metadata search | README2, W | **Retain** |
| Unified DSL + validation errors | core-app vs web filters | **Fix-gap** |
| Saved searches | N yes, W deleted | **Fix-gap** |
| Palette actions: create, sync, find-time, calendars, settings, diagnostics | README2 vs tests | **Fix-gap** |
| Deep links to task/event | W `/task/` `/event/`; N `hotcrossbuns://` | **Retain** as `hcb:` / documented CLI open |

**Acceptance — search/palette**

- Keystroke search never calls Google.
- Ranking: title hits before body hits; body requires `notes:`/`body:` or an explicit “search notes” continuation.
- Filters compose; invalid tokens show an error and do not query.
- Saved searches persist per account and apply identically in TUI and `hcb search`.
- Palette with empty query still lists **commands**; with text it lists ranked cache hits then commands.

### Offline mutations

| Capability | Evidence | TUI |
| --- | --- | --- |
| Durable outbox for task/list/event/RSVP mutations | W `PendingMutation`, N | **Retain** |
| Optimistic UI + reconcile | google-sync, W | **Retain** |
| Undo/redo of local intents with retention limits | W Settings, N undo | **Retain** |
| Pending count in status | W badge, N pending sync count | **Retain** |
| Writes that cannot be represented in Google must be labeled local-only | google-sync | **Retain** (priority, UI expansion, reminder dismiss state) |

**Acceptance — offline**

- Create a task with the network down; restart; the mutation is still queued; coming online applies it once.
- Before network I/O, creates persist a `sending` delivery state. Calendar event
  creates use a deterministic client-assigned Google event ID and may retry safely.
  Because Google Tasks, task-list create, and calendar create expose no idempotency
  key, an interrupted `sending` row becomes an explicit uncertain-delivery conflict:
  it is never blindly replayed, and the user must confirm the remote ID or confirm
  absence before retry. Local intent is never dropped.
- Undo of an unpushed create removes the optimistic row and the queued mutation.

### Reminders and local process

| Capability | Evidence | TUI |
| --- | --- | --- |
| Calendar popup reminders → OS notification | PRD, native-parity | **Retain** |
| Snooze 10 minutes / Dismiss; local idempotent state | N + W reminder state | **Retain** |
| Email reminders never become notifications | native-parity | **Retain** |
| Task marker reminders | W ForegroundReminders | **Retain** |
| Delivery after TUI exit | Fedora `hcb-reminderd`, macOS native | **Retain** as an optional local scheduler |
| Foreground-only browser notifications | W architecture | **Retire** as the reliability model |
| Web Push from self-host | W reliablePush | **Retire** |
| App-icon badge, `web+hotcrossbuns`, PWA SW | W App.tsx | **Retire** |
| Tray / menu bar as a required GUI chrome | N | **Defer** (CLI daemon status is enough for v1) |
| Spotlight, App Intents, Share extensions | native-parity | **Defer** |

**Acceptance — reminders**

- With scheduler enabled, closing the TUI still delivers due popup reminders.
- Snooze writes local state; the same id is not re-fired until the snooze expires.
- Permission denial is visible in diagnostics and does not crash sync.

### Diagnostics

| Capability | Evidence | TUI |
| --- | --- | --- |
| Counts, sync phase, storage, capabilities; **no** tokens, titles, notes, emails | W `diagnostics.test.ts`, HealthPanel | **Retain** |
| User-actionable sync errors in status | google-sync | **Retain** |
| Doctor/health command | W Health panel | **Retain** as CLI `doctor` + TUI panel |
| Browser capability matrix (IndexedDB, SW, badges) | W | **Retire** |

**Acceptance — diagnostics**

- Export JSON contains no OAuth material, no task/event bodies, no email, no subject identifiers in plaintext if they are account secrets; use counts and enums.
- `doctor` exits non-zero on broken keychain, unreadable DB, or missing client ID, with a stable error code.

### CLI accessibility (non-TUI)

There is **no** shipping CLI today ([docs/hcb-cli.md](../hcb-cli.md)). That status is **retired**. The CLI is a **required** equal of the TUI, not a debug afterthought.

| Capability | TUI |
| --- | --- |
| Every retained mutation and query available as `hcb <resource> <verb>` | **Fix-gap** (new surface) |
| Human output default; `--json` and where useful `--tsv` | **Retain** as new requirement |
| Stable exit codes; `NO_COLOR`; `TERM=dumb`; no TUI in pipes | **Retain** |
| Stdin for import/capture; shell completion | **Retain** |
| Destructive remote deletes require `--yes` or an interactive confirm, never a silent default | **Retain** |

**Acceptance — CLI**

- `hcb tasks list`, `hcb events agenda`, `hcb search`, `hcb sync`, `hcb auth`, `hcb config`, `hcb capture`, `hcb import`, `hcb export`, `hcb conflicts`, `hcb doctor` cover the retained matrix without opening a full-screen UI.
- `--json` schemas are versioned; unknown fields are ignored by readers, not dropped from writers without a version bump.
- Screen-reader and SSH users can complete onboarding, sync, and CRUD using only the CLI.

### TUI interaction

Visual baseline is khal’s month+agenda efficiency and gcalcli’s readable grids and width wrapping — **configurable**, not a clone. Do not import those projects.

| Capability | TUI |
| --- | --- |
| Full-screen workspace: filters + agenda/calendar/tasks + inspector | **Retain** as new requirement |
| Discoverable keys; footer hints; never color-only status | **Retain** |
| Themes (including mono), density, Unicode/ASCII, week start, TZ, keymap | **Retain** |
| Narrow terminal: inspector collapses; no horizontal clip of titles without indication | **Retain** |
| Mouse optional | **Retain** |
| Modal editors: focus, Escape, confirm destructive | **Retain** (web ModalDialog tests are the a11y intent) |
| khal vdir/ICS live store; gcalcli pickle cache / live query per command | **Retire** as architecture |

**Acceptance — TUI**

- Opening `hcb` with a TTY shows cached data immediately.
- All palette actions reachable from visible buttons or a command list.
- `NO_COLOR` and a mono theme remain usable.
- Resize and East-Asian width do not corrupt selection or hide the selected item without a scroll indication.

### Import / export

| Capability | Evidence | TUI |
| --- | --- | --- |
| Delimited + CSV v1 import, preview, confirm, 5 MiB / 1000 row limits | `docs/importing-tasks-and-events.md`, W importParser tests | **Retain** |
| ICS as **file** import/export, not live store | PRD + this matrix | **Retain** import/export |
| `.hcbexport` / `.hcbvault` Electron portable packages | `docs/portable-export.md` | **Retire** those formats; replace with documented JSON/ICS export of **cache + settings**, never credentials |

**Acceptance — import**

- Preview lists accepted vs skipped rows; cancel writes nothing.
- Accepted rows land in one local transaction; Google apply is per-row retryable.

### Migration and retirement

| Item | TUI |
| --- | --- |
| Keep `web/` runnable as behavioral oracle until the gate | **Retain** until gate |
| After gate: remove `web/`, `backend/`, `selfhost/`, shipped native product/CI | **Retire** those trees in a dedicated change |
| Git history as archive | **Retain** |
| User migration: Desktop OAuth re-consent (web tokens cannot move); optional import of non-secret preferences if a documented exporter exists | **Fix-gap** (document; no silent IndexedDB read) |
| Fedora/Windows native GUI parity | **Defer** (not a TUI blocker) |
| MCP, local hoster, browser extension, Electron CLI | **Retire** |
| CalDAV, HCB cloud, CRDTs, multi-user collab | **Defer** / never in pivot |
| Drive file content read/upload | **Defer** (metadata/link only) |

**Acceptance — migration/retirement**

- Parity is declared only when every **Retain** and **Fix-gap** row has TUI and CLI coverage (where applicable), live Google smoke is redacted and documented, and no feature depends on the self-host worker.
- Retirement PR deletes product surfaces and rewrites root docs so they no longer describe PWA or Qt as current. Specs that still say “QML must not touch SQLite” are rewritten for the TUI/CLI process model.
- Users are told plainly: web sessions cannot export refresh tokens; they reconnect once.

---

## Test oracles to port (behavior, not files)

Use these as the **minimum** contract suite. Rewrite in the new tree; do not leave gaps that these already cover.

| Current tests | Behavior to keep |
| --- | --- |
| `web/tests/tokenSession.test.ts` | Access tokens are session-scoped; expired token is unusable |
| `web/tests/googleIdentity.test.ts`, `googleApiClient.test.ts` | Scope set, error mapping 401/403/404/409/410/429/5xx |
| `web/tests/localStore.test.ts` | Subject partition; preferences persist; no cross-account leak |
| `web/tests/taskRecurrence.test.ts` | Marker parse/serialize; reminder; malformed |
| `web/tests/paletteFilters.test.ts`, `calendarSearch.test.ts`, `CommandPalette.test.tsx` | DSL, title-first, historic events, **then add** command actions the old tests forbade |
| `web/tests/importParser.test.ts` | Delimited/CSV validation |
| `web/tests/diagnostics.test.ts` | Redaction |
| `web/tests/CalendarPanel.test.tsx`, `CalendarTools.test.tsx`, `TaskPanel.test.tsx`, `SyncDialog.test.tsx` | Views, find-time, editors, progress/cancel |
| `web/tests/e2e/onboarding.spec.ts` | First-run does not ask for a client secret (adapt to Desktop client ID) |
| Native C++/QML: mutations, conflicts, saved searches, notes projection, reminders, OAuth redaction | Durable SQLite + OS reminder + saved search oracles |
| `docs/testing/live-google-smoke.md` / web smoke | Manual live account; redact |

New tests required by **Fix-gap** rows: notes modes actually change lists; palette commands; unified DSL including real `source:local`; CLI JSON snapshots; scheduler-after-exit reminders; keychain denial; `410` and page-resume; crash recovery of the outbox, including deterministic Calendar event IDs and uncertain-delivery reconciliation for create APIs without idempotency keys.

---

## Parity gate (all must be true)

1. Every **Retain** and **Fix-gap** item above has an automated test or a named manual live-Google procedure.
2. Normal reads and queued writes work with the network disabled after a successful sync.
3. Credentials survive process restart via the OS store and never appear in diagnostics.
4. Calendar token incrementality, Tasks watermarks, conflicts, and recurrence markers match the documented Google contracts.
5. CLI can perform the same retained mutations as the TUI.
6. Reminders can fire with the TUI closed when the user enabled the local scheduler.
7. Root documentation describes only the local TUI/CLI product; web/native/self-host are historical.

Until then, do not delete `web/` or `native/` product code.
