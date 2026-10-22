# Terminal UI audit — 18 October 2026

**Historical record.** The standalone CLI and terminal renderer were retired on 22 October 2026. See [the desktop decision](DESKTOP-PRIMARY.md) and [current development instructions](DEVELOPMENT.md). Original observations below are retained.

## Scope and reference

This audit covers every interactive screen projected by `worktui.BridgeState`,
`modelcatalog.UISnapshot`, and rendered by `ui/src/main.tsx`. The reference is
the imported [`@claude-code-kit` 0.4.0 package](https://github.com/Minnzen/claude-code-kit) and its conversational layout,
keyboard behavior, picker rows, and prompt placement. That package is an
independent Claude Code inspired kit, not Anthropic's frontend. Gator retains
its green color and its work product commands. `gator doctor` now uses a
one-shot Node-rendered report; `/doctor` uses the same report component in chat.

## Screen inventory and findings

| Screen or state | Audit finding | Resolution |
| --- | --- | --- |
| Empty home and first run | An empty `MessageList` still reserved transcript height and could push the composer down on short terminals. | Render the kit's `WelcomeScreen` without an empty transcript viewport; keep setup and next actions above the bottom composer. |
| Conversation, streaming work, and deliverable cards | Kit `MessageList` and `StatusLine` were already in use. Esc routed to Go cancellation. | Retained, with multiline prompt support and a stable bottom composer. |
| Slash command suggestions | Filtering and selection worked, but a fast exact command could select a stale highlighted suggestion. | Prefer the exact typed command on Enter; retain green focus instead of the kit's hardcoded cyan. |
| Transcript search | Match count and result navigation stay close to the composer. | Retained; Esc closes search and returns focus to chat. |
| Commands, conversations, goals, workspace, and copy pickers | Rows and search came from the Go snapshot, but opening a picker replaced the composer entirely. | Keep a muted composer anchor below the picker and constrain visible rows. |
| Cloud/local model choice and catalogs | The old Go renderer was still the fallback; the React catalog hid download progress and installer logs. | Remove the fallback; project progress and installer output into the React panel. Preserve readiness and credential status. |
| Model help, credential setup, custom provider, rename, local setup, model confirmations | Focused actions looked like ordinary lists. Wide kit dividers wrapped inside narrow panels. | Render model forms and confirmations in a constrained green bordered surface, show a cursor cue on the active field, and size dividers to the panel. |
| Status line editor | Ordered rows and live preview were already projected. Mixed rows and blocks could consume too much height. | Budget row and detail space together; keep a composer anchor and optional status line below. |
| Inbox and scheduled jobs | Read-only rows and contextual actions were projected consistently. | Keep the common panel layout and bottom composer anchor. `/jobs` replaces the conflicting Ctrl+J shortcut. |
| History list and detail | Retained runs, artifacts, feedback, and review actions were projected consistently. | Keep the common list/detail layout and scrolling. |
| Learnings list and detail | Provenance and explicit approval controls were projected consistently. The add/edit form was visually identical to a list. | Give the learning form the same bounded focus surface as model forms. |
| Save/apply transfer confirmation | Confirmation data and keyboard actions were projected, but the screen looked like a list. | Use the green bordered focus surface with its evidence summary and explicit confirmation footer. |
| External editor and connector handoff | React unmounts while an external terminal process runs, then resumes from Go state. | Retained; this is a handoff, not a Gator screen. |
| Doctor diagnostics | The standalone command had a Go spinner and formatter while `/doctor` appeared as plain chat text. | Node renders both from the same report component; Go retains only the diagnostic checks. The standalone command exits after showing results. |

## Shared interaction checks

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| High | `cmd/gator/work_interactive.go`, `internal/worktui/view.go` | Missing Node or `GATOR_NATIVE_TUI=1` launched a second Go interface. | One React renderer; clear Node 22+ startup error. | One interaction model is easier to learn and test. |
| High | `internal/modelcatalog/panel_snapshot.go` | Active download status and installer logs disappeared when using React. | Progress and installer output are included in the snapshot. | A user must see what is happening during a long installation. |
| Medium | `ui/src/main.tsx` home and panels | An empty transcript used space and panels hid the composer location. | Home omits the empty list; panels keep a muted composer anchor. | The next input stays discoverable. |
| Medium | `ui/src/main.tsx` forms | Forms and confirmations looked like ordinary lists; dividers wrapped inside bordered widths. | Bounded green focus surfaces, field cursor cue, width-aware dividers. | Editing and approval now read as distinct actions. |
| Medium | `ui/src/main.tsx` keyboard and suggestions | Ctrl+J conflicted with jobs, Ctrl+C exited when idle, and exact slash input could choose a stale suggestion. | Ctrl+J inserts a line, jobs live at `/jobs`, idle Ctrl+C clears, and exact commands win on Enter. | The composer follows the kit's editing conventions. |

### Detail checks

- **Keyboard:** Enter submits, Shift+Enter or Ctrl+J inserts a line, Ctrl+C
  clears an idle draft or interrupts active work, Esc interrupts or backs out,
  Ctrl+D exits, and slash suggestions stay above the prompt.
- **Focus and color:** One green focus indicator appears on a selected row.
  Text and form values use neutral text; secondary labels are muted. The kit's
  cyan `Select` and `ListItem` were not used because they cannot take Gator's
  green accent without a fork.
- **Typography and icons:** The kit's monospace text carries the UI. `🐊 Gator`
  marks identity and is not repeated inside every row. Other glyphs indicate
  state or a keyboard focus target.
- **Geometry:** Chat and panels reserve the composer at the bottom. Lists keep
  the selected row in view; detail blocks scroll with PageUp and PageDown.
  Bordered forms are narrower than the terminal and their dividers use their
  actual inner width.
- **Performance:** The transcript uses the kit's virtualized `MessageList`.
  Motion is limited to the running-work indicator. The terminal UI has no
  hover states, shadows, or pointer-only actions.

## Verification

- `npm run check` and `npm run build` in `ui/`.
- `go test ./...`, including bridge state, model setup, progress, cancellation,
  and screen projection tests.
- PTY smoke check of the first-run home, model choice, cloud catalog, and
  credential form. The form displayed a green border and the composer anchor;
  Ctrl+D exited cleanly from both chat and a panel.
- With Node removed from `PATH`, both interactive startup and `gator doctor`
  exit with the Node 22+ installation message. Headless commands still work.
- Static inspection of every `Panel.Kind`, launcher mode, model submode,
  history and learning detail, transfer confirmation, and React rendering path.

**Verdict:** Every interactive screen is now rendered by React and follows the
shared prompt, panel, focus, and green color conventions in
`docs/DESIGN_SYSTEM.md`. The audit checks Gator's own UI against the imported
kit's patterns. It does not establish pixel parity with the proprietary Claude
Code CLI.

## Result presentation follow-up

The command transcript now accepts `uisnapshot.Result` alongside its plain text.
React renders typed notices, fields, lists, previews, and approval requests with
one component. Memory, notes, skills, plans, activity, status, permissions, the
prompt queue, and verified deliverables have structured presenters. Agent prose
and typed Markdown documents use the kit's Markdown renderer. The `/effort`
command opens a picker. The standalone script commands keep their existing
plain output. Approval cards retain the complete JSON preview and display
their resolution; a stale request is marked as no longer pending.

## Palette, search, diagnostics, and control follow-up

| Surface | Kit reference | Gator implementation and reason |
| --- | --- | --- |
| Home and conversation | `WelcomeScreen`, `MessageList`, `Markdown`, `Spinner`, `Divider`, `StatusLine` | Uses these components directly. All accents now read the saved palette from bridge state. |
| Prompt and slash suggestions | `PromptInput` and its suggestions | Keeps the bottom prompt, history, multiline keys, and suggestions above it. A Gator adapter uses grapheme-aware cursor editing and palette-aware focus; the kit prompt uses JavaScript character offsets. Fast text plus Return is handled as one submission. |
| Transcript search | Kit search overlay near the prompt | Gator searches visible typed cards and deliverables as well as prose; Ctrl+N/Ctrl+P navigate matches. The Go result schema remains the source of truth. |
| Pickers, models, history, inbox, jobs, and learnings | `Select`, `ListItem`, `Byline`, `KeyboardShortcutHint` | Gator renders rows in the kit's single-focus style and uses the kit hint components. Go retains selection and action state. The kit's `Select` and `ListItem` hardcode cyan, so their row implementations are not used. |
| Forms and transfer confirmations | Kit `Dialog` | Gator keeps a centered, bordered focus surface with kit-style hints. Go retains Esc and Enter ownership for model, approval, and transfer state; the kit dialog's local Esc handler would duplicate those actions. |
| Doctor and command results | Kit transcript and Markdown primitives | Go emits semantic fields, items, sections, and tones. React renders one result body for `/doctor` and standalone `gator doctor`; no text-regex color inference remains. The standalone spinner starts in the saved theme. |

The kit's Markdown renderer reads its own dark theme for inline code and links.
Gator synchronizes that public theme object with its saved palette and remounts
Markdown when the theme changes. This adapter is tested against the pinned
kit version.

The screen inventory above was rechecked against each `Panel.Kind` and model
submode. Compact panels drop the subtitle and composer anchor to preserve the
active choice and Esc guidance at 20×12. Long detail lines wrap by terminal
cells. User text and untrusted previews stay literal; Markdown is limited to
agent prose and explicitly typed Markdown results. This remains a Claude Code
inspired interface built from an independent kit, not Anthropic's frontend.
