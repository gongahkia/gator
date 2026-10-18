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
Gator requires Node.js 22 or newer; headless commands do not.

## Shared layout

- `🐊 Gator` labels the home and conversation. Green marks Gator, the prompt,
  focused rows, and active work; neutral text carries content and muted green
  carries secondary context.
- The transcript uses the kit's `MessageList`. The empty home uses its
  `WelcomeScreen`; no empty transcript viewport is reserved on home.
- The composer stays at the bottom of chat. A panel leaves a muted composer
  anchor at the bottom so its location stays predictable; Esc returns to chat.
- `/` suggestions appear immediately above the composer. Long lists keep the
  selected row in view. Detail content scrolls with PageUp and PageDown.
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

## Keyboard conventions

- Enter submits a prompt; Shift+Enter or Ctrl+J inserts a new line.
- Ctrl+C clears an idle draft or interrupts active work. Esc interrupts active
  work or closes a panel. Ctrl+D twice within two seconds exits.
- Ctrl+G opens the editor. Ctrl+F searches the transcript. Ctrl+P opens the
  command picker. Ctrl+X opens conversations, Ctrl+B opens inbox, and Ctrl+I
  chooses a workspace. Scheduled jobs are available through `/jobs` and the
  command picker.
- Up and Down navigate picker rows or recall prompts when the composer has no
  adjacent line. PageUp and PageDown scroll long content.

The kit hardcodes cyan in its `Select` and `ListItem` focus states, so Gator
renders those rows with green rather than inheriting a mismatched color. The
shared kit components used directly are `WelcomeScreen`, `MessageList`,
`Divider`, and `StatusLine`. Keep keyboard ownership in the React layer for
editing and in Go for workflow actions so the same key cannot trigger both.
