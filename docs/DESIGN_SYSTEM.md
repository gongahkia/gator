# Gator terminal interface

## Reference and ownership

The interactive UI is the React renderer in `ui/src/main.tsx`, built with
`@claude-code-kit/ink-renderer` and `@claude-code-kit/ui` 0.4.0. The imported
kit is an independent project inspired by Claude Code; it is not Anthropic's
frontend. Gator uses its conversational layout and keyboard conventions while
keeping its own green identity, work commands, and review flows.

Go owns workflow state, actions, and approvals. It publishes display data
through `BridgeState` and `UISnapshot`. Go `View()` methods are empty Bubble Tea
interface adapters. There is no Go screen renderer or fallback. Interactive
Gator and `gator doctor` require Node.js 22 or newer; headless
commands do not. The Go doctor checks feed a React report in either the
standalone command or `/doctor` conversation result.

Command results travel as typed `uisnapshot.Result` values through
`BridgeMessage.result`. React owns their presentation: notices, status fields,
lists, previews, and approvals use one result component with explicit semantic
tones. Go retains readable text for transcript search and headless commands.
The renderer applies Markdown only to agent prose and explicitly typed Markdown
documents; user text, command output, and approval previews stay literal.

## Shared layout

- `🐊 Gator` labels the home and conversation. Green marks Gator, the prompt,
  focused rows, and active work; neutral text carries content and muted green
  carries secondary context. `/theme` applies the saved `gator`, `contrast`, or
  `mono` palette to the React interface immediately, including panels, result
  cards, status, and doctor diagnostics.
- Yellow marks pending review; red marks errors. Colors follow typed state from
  Go, never words inferred from arbitrary output text. Approvals show the exact
  structured preview and keep `/approve` and `/deny` visible until resolved.
- The transcript uses the kit's `MessageList`. The empty home uses its
  `WelcomeScreen`; no empty transcript viewport is reserved on home.
- The composer stays at the bottom of chat. A panel leaves a muted composer
  anchor at the bottom so its location stays predictable; Esc returns to chat.
- `/` suggestions appear immediately above the composer. Long lists keep the
  selected row in view. Detail content scrolls with PageUp and PageDown.
- Transcript search covers the content rendered in result cards, approvals,
  deliverables, and agent turns. Repeated matches are counted and Ctrl+N/Ctrl+P
  move among them.
- Forms and confirmations use a constrained green bordered surface. Lists and
  detail pages remain open surfaces with one visible selection and a footer.
- The optional kit `StatusLine` sits below the composer. Its content is chosen
  through `/settings statusline` and can be hidden.

## Screen inventory

| Surface | Content | Input and exit |
| --- | --- | --- |
| Home | Welcome, model setup or next actions | Bottom composer; `/` commands |
| Conversation | User and Gator turns, deliverables, run status | Bottom composer; Esc interrupts active work |
| Slash suggestions | Available commands filtered by typed prefix | Above composer; arrows, Tab or Enter select; Esc dismisses |
| Search | Transcript match count and result navigation | Ctrl+F opens; Esc closes |
| Pickers | Commands, conversations, goals, workspace, copy target | Search above rows; arrows and Enter; Esc closes |
| Model choice and catalogs | Cloud or local options, readiness, model details | Arrows and Enter; contextual footer; F1 help |
| Model forms and confirmations | Credential, provider, rename, install, download, removal | Green bordered focus surface; Tab or arrows; Enter confirms; Esc cancels |
| Status line settings | Ordered items and preview | Space toggles, arrows reorder, Enter saves, Esc cancels |
| Inbox and jobs | Results and configured schedules | Read-only rows with action guidance; Esc returns |
| History | Retained runs, artifact and feedback details | Selection, detail, review and retry controls; Esc returns |
| Learnings | Memory list, provenance details, edit form | Selection and explicit approval controls; Esc returns |
| Transfer confirmation | Verified save or apply summary | Green bordered focus surface; Enter confirms, Esc cancels |
| Command results | Typed status, notices, lists, and verified previews | Read in chat; next action appears in the footer |
| Approval request | Structured action preview and request ID | `/approve` or `/deny` while work is running |

## Keyboard conventions

- Enter submits a prompt; Shift+Enter or Ctrl+J inserts a new line.
- Ctrl+C clears an idle draft or interrupts active work. Esc interrupts active
  work or closes a panel. Ctrl+D exits.
- Ctrl+G opens the editor. Ctrl+F searches the transcript. Ctrl+P opens the
  command picker. Ctrl+X opens conversations, Ctrl+B opens inbox, and Ctrl+I
  chooses a workspace. Scheduled jobs are available through `/jobs` and the
  command picker.
- Up and Down navigate picker rows or recall prompts when the composer has no
  adjacent line. PageUp and PageDown scroll long content.
- Left, Right, Backspace, and Delete in the composer and search treat an emoji
  or combining sequence as one character. Small terminals compress panel
  chrome and wrap long detail lines; the picker fits at 20×12.

The kit hardcodes cyan in its `Select` and `ListItem` focus states, so Gator
renders those rows with green rather than inheriting a mismatched color. The
shared kit components used directly are `WelcomeScreen`, `MessageList`,
`Markdown`, `Divider`, `StatusLine`, `Spinner`, `Byline`, and
`KeyboardShortcutHint`. Gator's prompt, search, row picker, and focused form
surfaces are adapters that follow the kit's placement, focus, and key
conventions while using the saved palette and Go workflow state. The kit's
`PromptInput` uses JavaScript character offsets, which split some emoji, and
its `Dialog` owns Esc locally; Gator needs Go to own Esc for approval and model
forms. Keep keyboard ownership in React for text editing and in Go for workflow
actions so one key cannot trigger both.
