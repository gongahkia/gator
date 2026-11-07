# CLI/TUI product port

7 November 2026. Product direction: CLI/TUI only, with chat as the default
surface. The prior OpenCode-derived Solid/OpenTUI renderer is restored from
`46be9ba8d^`. Current Go backend code is retained rather than rolled back.
The Electron product, renderer, packaging, release setup and dedicated tests
are removed. Its newer interactions have terminal entry points against the
current backend. The requested animated mascot follows that removal.

| Capability | Terminal destination | Authority |
| --- | --- | --- |
| Chat, queues, steering, approvals, independent sessions, recovery | Chat, `/queue`, `/chats` | Current Go journals, approval tokens and host sessions |
| Models, effort, providers, local models and settings | `/model`, `/models`, `/settings` | Existing Go forms and reviews |
| Connections, Gmail send and frozen originals | `/connections`, inline approvals, `/receipts` | Current connector/action code retained |
| Goals, scheduling, updates, memory and knowledge | Chat tools, `/following`, `/saved`, typed pages | Current Go services retained |
| Drafts, attachments, files, Office/PDF/images and export | `/drafts`, `/attach-file`, `/files` | Shared immutable files and exports |
| Usage, lifecycle, backups, retention and service repair | `/usage`, `/data`, `/recovery` | Current Go reviews |
| Source selection, retained sources and code evidence | `/sources`, `/evidence`, `/changes` | Current source IDs and verified manifests |
| Reviewed task-plan admission, edits and start | `/plan-first`, `/task-plans`, inline plan summaries | Current revision/digest-bound protocol |
| Branches and individual specialist controls | `/branch`, `/original-chat`, `/specialists`, inline status | Current Go lineage and live tokens |
| Local Whisper, dictation and system speech | `/voice`, `/read-aloud`, `/stop-reading` | Current speech configuration/downloads/usage |
| Link/message reuse, copy and read aloud | `/links`, `/prompt-history`, `/copy-message` | Private drafts; explicit send required |
| Optional companion and external tmux | Retained independent extension | Existing authentication and grants |
| Distribution and updates | Complete terminal packages | Explicit installer/switch/rollback; profiles retained |

## Product boundaries

Chat and the composer are the default surface. The navigation rail is hidden;
Ctrl+B can reveal it. Management and settings stay behind slash commands and
Ctrl+P. Current plans, specialist activity, files and approvals appear in context.
The current Go backend was not reverted. Existing profiles, credentials,
conversation IDs, queues, memory, files and schedules remain compatible.
No installed application, running service or user data was deleted by this port.
OS computer tools under `internal/desktop` and existing `desktop-*` protocol
names remain compatibility services, not Electron or a second GUI frontend.

Whole terminal packages include the frontend and native Go runtime. Releases
build on native macOS, Linux and Windows runners for amd64 and arm64. The added
ARM runner labels follow [GitHub's runner reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners).
Those remote jobs have not run in this session. The existing protected release
environment names are retained so OAuth client material and approval rules
continue to apply. Releases remain drafts for review.

## Verification

Observed locally on Intel macOS; no paid model calls were made.

- `go test ./...`: pass across the repository. `go vet ./...`: pass.
- Current application-guide and product-guide checks: pass after the help update.
- `npm test --prefix app-host`: 109 pass, one opt-in skip; includes verified
  source/code evidence, byte-range reads, voice cancellation and preferences.
- Terminal renderer and real-Go tests cover chat, exact reviews, branches,
  source selection, plan edits/stale approval rejection, specialist tokens,
  connections, memory, data, Office files, draft ownership and recovery.
  The final full run, including the mascot, passes 72 tests with three explicit platform/OS-helper
  skips (75 total). Seven focused navigation/form checks also pass.
- A colored terminal appearance run passes alongside NO_COLOR/native-color tests.
- 14 installer/uninstaller unit checks pass. The production package verifies its
  dependency closure, real Go host, PDF pages, native Office/text edits and exports.
- Installed launcher/help/version/CLI, profile-lock and nested-draft protection,
  launcher repair, owned uninstall and preservation of user data: pass.
- Actual PTY: chat connection, Unicode/bracketed paste, inline completion,
  grouped commands, draft retention, exit and terminal mode restoration: pass.
- Actual editor PTY: canonical input during handoff, returned text, temporary
  file cleanup and restored raw input: pass. File-preview terminal-control
  injection checks: pass.
- Installed installation switch and rollback preserve two drafts, file versions,
  edit drafts, selected chat/source, notes, preferences and configuration.
- Companion service/inspection/mutation/recovery checks: 21 pass, one opt-in skip.

Native microphone permissions, real account sign-in, browser sessions and new
remote release matrices were not exercised. Existing optional device/browser
integration tests retain their explicit opt-in flags. Old Electron/terminal audit
records retain their historical observations and are marked accordingly.

## Interface review

Full review scope: restored Solid/OpenTUI chat, composer, command menus, typed
forms, source/code details and the optional appearance presets. Existing terminal
tokens and components were reused; no CSS/animation framework was introduced.

| Category | Evidence | Result |
| --- | --- | --- |
| Typography | Narrow Markdown, file grids, command labels, source read ranges | Bounded terminal layout and Unicode checks pass |
| Surfaces | Chat-only default, draft/form guards, short menus and native PTY | Fixed stale dialogs on accepted navigation |
| Animations | Existing progress indicators and reduced-motion controls | No new routine entrance animation |
| Icons | Existing text/terminal glyphs and current connection indicators | No new icon library |
| Performance | Bounded evidence, dialog limits, production closure and subscriptions | Existing bounds retained |

Rejected changes: a persistent dashboard would crowd the chat surface; a second
rendering or styling framework would duplicate the existing terminal system.
Browser animation playback at 10% is inapplicable to this native terminal port;
real PTY and renderer tests provide the interface evidence instead. Physical
microphone and other-platform visual verification remain unverified.


No actionable interface-polish findings remain in the verified scope. Verdict:
Approve for the local terminal port; physical microphone and other-platform
visual checks remain unverified as listed above.

## Optional mascot

The ASCII Gator uses four fixed-width rows beside the composer. Idle blinking
wakes briefly every five seconds; work uses a 360ms frame interval. Review and
handoff states use a static attention expression. All timers stop when hidden,
when motion is reduced, or when the component closes. `/mascot` persists Animated,
Still or Hidden; widths under 70 columns automatically hide it. The existing
chat draft and keyboard focus remain the input authority.

Renderer checks cover observed work, attention, animation, still/hidden modes,
persistence, compact terminals and draft ownership. Reduced-motion checks pass.
The 100×24 character preview was inspected; the mascot shares the existing
terminal tokens and preserves the composer geometry while its frame changes.
Installed production PTY/editor/file-preview checks also pass with the mascot.
