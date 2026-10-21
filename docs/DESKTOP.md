# Desktop companion

## Decision

Add desktop alongside the terminal first. Compare their usefulness in real
work before deciding whether to retire the terminal. A desktop release does
not change the CLI or the current React terminal renderer.

The sibling `Referene` directory contains OpenHands Agent Canvas 1.29.0, a
React DOM / Tailwind web app with an Electron desktop shell. Gator adapts its
neutral palette, sidebar, spacious conversation area, command menu, and
optional right detail panel. Product navigation follows Gator's ongoing
goals, inbox, reviewed follow-ups, saved knowledge, and deliverables.

## Ownership

```text
React terminal (Claude Code Kit) ─┐
                                ├─ Go UI bridge ─ Go Work application
React desktop ─ Electron host ───┘                ├─ conversations and goals
                                                 ├─ tools and approvals
                                                 ├─ checks and inbox
                                                 └─ shared usage and data
```

The desktop host launches the same `gator ui-backend` transport used by the
terminal. It consumes JSON-line display snapshots and sends typed actions.
`ui/src/protocol.ts` is the shared frontend contract. The desktop has no
agent loop, provider credentials, second copy of goal state, or HTTP listener.
Normal state/config overrides still select the profile. The two interfaces
can use the same saved profile, but each window/process owns its own live
foreground run; opening the desktop does not attach to a terminal's live run.
Shared usage and storage locks remain owned by Go.

Desktop view navigation and form edits are small adapters on the existing Go
workflow. A panel carries a content identity; a stale click cannot activate a
newly reordered row or confirm a different preview. All native field values
are checked before any are applied. Saving follows the existing Go validation
and review path. Secrets are replacement inputs and stay absent from display
snapshots and the renderer's persistent storage. Approval buttons carry the
current live interaction ID; retained cards have no approval authority.

## Product surfaces

| Surface | Behavior |
| --- | --- |
| Home | Four live overview summaries and an automatically scrolling strip of workspace actions |
| Navigation | Goals, conversations, inbox, follow-ups, knowledge, history, usage, backups, recovery, models |
| Conversation | Markdown agent text, literal user/output text, typed result cards, streaming progress, stop/steering |
| Reviews | Exact structured previews with request-bound approve/deny buttons; separate confirmation for workflow writes |
| Native forms | Text/password/multiline fields; existing Go review and save actions |
| Work details | Deliverables and recurring checks; no Context tab or profile footer |
| Usage & limits | Daily request/token meters, shared limit controls, activity breakdown, and expandable accounting details |
| Commands | Cmd/Ctrl+K, searchable commands, arrow navigation, input-prefilling commands |
| Transcript search | Cmd/Ctrl+F, searches prose plus typed result/deliverable data |
| Drafts | Kept in memory while opening views; unsaved forms prompt before leaving |
| Terminal handoff | Reviewed connector sign-in and other interactive CLI steps open the system terminal and return their status |

Enter sends and Shift+Enter inserts a line. Cmd/Ctrl+N starts a conversation
in the selected workspace. Escape closes a view/menu/search or stops active
work. Modal focus stays inside the dialog and returns to its trigger.

The home dashboard advances every six seconds and pauses on hover, keyboard
focus, or manual interaction. Previous/next and pause/resume controls remain
available. Reduced-motion preferences disable automatic movement while keeping
manual navigation. Profile tiles, promotional headings, refresh timestamps,
composer footnotes, and the idle send shortcut have been removed.

Usage is structured display data from Go's shared accounting store. Uncapped
limits stay explicitly uncapped; the desktop does not invent billing estimates
or provider subscription quotas. Limit edits and pause/resume actions retain
the existing separate review and confirmation, including stale-policy checks.

The desktop starts with a private empty folder rather than capturing the
user's home directory. Choose a workspace with the native folder picker, set
`GATOR_DESKTOP_WORKSPACE`, or resume existing work. Nothing in the sibling
reference repository is required at build or runtime.

## Host boundary

The window loads bundled local assets through `gator://app`. The renderer is
sandboxed, has context isolation enabled, and has no Node or arbitrary IPC
access. The preload exports narrow methods; main checks the sender frame and
validates actions. New windows and page navigation are blocked. Browser links
open explicitly in the system browser and only accept HTTP(S). Markdown raw
HTML is skipped; remote images are not fetched merely by viewing a message.
The content security policy denies renderer network requests. These choices
follow [Electron's security guidance](https://www.electronjs.org/docs/latest/tutorial/security).

Only Go-produced external actions can launch a terminal handoff. The renderer
cannot supply executables, arguments, environment, or shell code. Arguments
are shell-quoted, scripts are private, and terminal return status resumes the
same Go workflow. macOS uses Terminal; Linux needs `x-terminal-emulator`.
An interactive handoff must finish or be cancelled in that terminal before
the desktop closes. Closing other active foreground work asks to stop it,
then shuts down its Go child. Scheduled checks retain their existing service
lifecycle; the desktop does not silently enable an OS service.

## Rollout

Keep both interfaces while checking:

- Whether a user can resume a goal, understand changes, review an action, and
  find the result without knowing slash commands.
- Whether model setup, source recovery, shared limits, and restore reviews
  remain understandable in the desktop.
- Whether startup, idle memory, packaging size, and everyday responsiveness
  justify the extra Electron runtime.
- Whether real terminal usage still provides a useful workflow.

Remove a frontend only after there is evidence to do so. The Go runtime and
headless CLI remain reusable regardless of that later product decision.

## Development verification

The desktop's Electron integration test uses a real built Go backend and an
isolated profile with a local fake inference endpoint. It exercises home,
overview navigation, draft preservation, detail tabs, native follow-up edits
and review, conversation continuation, and multiple window sizes. It also
checks empty and populated Saved knowledge, dashboard motion controls and
reduced motion, real usage counters, and reviewed usage changes. It does not
call paid providers or enable background services. Go tests cover stale view
actions, atomic field validation, secret redaction, reviewed mutations, live
approval IDs, and active-run navigation guards. Host tests cover action and
resource policy, process communication, and child shutdown.

Native packaging is available through `desktop/package.json`. Signed and
notarized release distribution, automatic updates, and Linux desktop runtime
verification remain release work; they are not prerequisites for trying the
desktop alongside the terminal locally.

Referene attribution is in `desktop/licenses/OPENHANDS.txt`.

The app icon is the same standalone 🐊 symbol used in the interface, on a
transparent background. `desktop/assets/icon.icns` supplies the macOS bundle
icon; `icon.png` supplies the running Dock/window icon and Linux packaging.
Both assets are checked in. To regenerate them on macOS, run
`swift desktop/scripts/generate-icons.swift`; ordinary builds do not need Swift.

### Local verification, 21 October 2026

- `go test ./...` and `go vet ./...` passed; subsequent model-status and bridge
  changes passed the affected command/package tests and vet checks.
- `go test -race ./internal/worktui ./internal/modelcatalog -count=1` passed.
- Terminal TypeScript checks, all five UI tests, and bundle build passed. The
  regenerated terminal bundle includes the empty-block rendering fix.
- Desktop TypeScript checks, all five host tests, and frontend/runtime build
  passed.
- All three Electron integration tests passed in development and against the
  packaged `Gator.app`. The packaged test exercised its bundled assets and Go
  executable, with two local inference turns and an isolated saved profile.
- Screens were checked at 1440×960, 1024×768, and 800×600, including narrow
  detail-panel access, command keyboard navigation, dialog focus trapping and
  restoration, and the grid with its titlebar hidden.
- `npm run pack --prefix desktop` produced an unsigned macOS x64 application
  at `desktop/release/mac/Gator.app` (about 362 MB). Test screenshots are under
  `desktop/test-results/` and are intentionally untracked.

Interactive OAuth/terminal handoffs have script quoting and return-status
tests; live sign-in with an external account was not exercised. Linux runtime,
native OS fullscreen transitions, signed installers, and notarization were
not verified in this local pass.

### Interface review: requested desktop cleanup

Full review of the requested home, right panel, Saved knowledge, and Usage &
limits changes. React DOM, Tailwind v4, and the existing semantic CSS remain
the styling system. Other workflow pages retain their existing layouts.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Home, usage totals, reset date, narrow windows | Tabular numbers and existing font/wrapping conventions retained |
| Surfaces | Home cards, usage meters/settings, native forms, details pane | Requested clutter removed; controls retain 40px hit areas |
| Animations | Automatic advance, hover/focus pause, manual controls, reduced motion | Browser state transitions checked at 10% playback; native scroll timing belongs to Chromium |
| Icons | Dashboard controls, usage actions, existing navigation | Existing Lucide/currentColor/stroke conventions retained |
| Performance | Scroll container, six-second interval, hidden document guard | No animation dependency or continuous React render loop |

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| HIGH | `desktop/src/Panel.tsx:48`, `ui/src/main.tsx:281`, `ui/src/protocol.ts:43` | Empty source references caused an array dereference on null | Nullable wire type and empty-array rendering fallback in both frontends | Empty knowledge must remain usable |
| MEDIUM | `desktop/src/App.tsx:185`, `desktop/src/styles.css:97` | Promotional headings, refresh copy, profile tile, composer footnote and idle send hint | Requested elements and obsolete styles removed | Reduce visual competition with the task |
| MEDIUM | `desktop/src/App.tsx:79` | Context occupied the first right-panel tab and repeated workspace metadata | Deliverables opens first; Checks remains available | Keep the right panel focused on outputs and follow-ups |
| MEDIUM | `desktop/src/StarterDashboard.tsx:13` | Static starter grid | Scrollable action dashboard with pause, manual navigation and reduced-motion support | User-requested motion remains controllable |
| MEDIUM | `desktop/src/UsagePanel.tsx:20`, `internal/uisnapshot/usage.go:5`, `cmd/gator/work_controls.go:82` | Terminal action rows and a long prose dump | Typed quota cards, grouped settings, activity counts and collapsed details | Improve scanning without changing accounting or review ownership |

| Location | Candidate | Rejected because |
| --- | --- | --- |
| Dashboard | Seamless duplicated cards and a continuous animation loop | Duplicate focus targets and constant motion add cost; native scrolling keeps normal keyboard and pointer behavior |
| Usage | Copy Codex's subscription periods and credit balances | Gator owns daily UTC admission limits; provider billing/subscription data is not available here |
| Desktop styling | Add a new animation/component framework | Existing CSS and browser scrolling cover the requested behavior |

Verification uses the commands listed above, plus the real Go accounting
projection test and Electron assertions that pending usage edits have not
changed persisted policy before confirmation. The Saved knowledge regression
failed against the previous build and passed after the fix. Screenshots cover
empty, populated, uncapped, capped, paused, and narrow states. Verdict: Approve
for the requested scope. Platform and external sign-in limitations above
remain unverified.
