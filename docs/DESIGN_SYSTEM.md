# Gator terminal design system

## Product posture

Gator should feel like a quiet work surface, not an operations dashboard. The
first screen asks for an outcome and postpones history, jobs, inbox, and
specialist controls until the user asks for them. Commands and destinations are
separate concepts: `Ctrl+P` searches actions, while direct control-key bindings
open conversations, inbox, or jobs.

The interaction model is informed by the official [OpenCode TUI
documentation](https://opencode.ai/docs/tui/) and
[keybinding model](https://opencode.ai/docs/keybinds): a centered first prompt,
a bottom-docked session prompt, a searchable `Ctrl+P` command list,
keyboard-first navigation, and resumable sessions. Gator does not copy OpenCode's branding or
screen treatment; it adopts the underlying progressive-disclosure pattern.

## Screen states

| State | Primary content | Composer | Secondary navigation |
| --- | --- | --- | --- |
| Home | Centered `🐊 Gator` wordmark and one outcome question | Centered, at most 72 columns | Direct `Ctrl+X`, `Ctrl+I`, and `Ctrl+J` hints |
| Drafting | Top-left `🐊 Gator` wordmark, selected folder, and a quiet empty transcript | Docked after the first typed character | Available through direct shortcuts |
| Conversation | Top-left `🐊 Gator` wordmark, transcript, and current run status | Docked below the transcript | Available through direct shortcuts |
| Command palette | Searchable slash actions only | Hidden | `Ctrl+P`; type to filter, arrows to choose |
| Conversations | Retained Work conversations only | Hidden | `Ctrl+X` |
| Inbox | Recent scheduled-work results in a read-only view | Hidden | `Ctrl+I` (also reported as Tab by terminals) |
| Jobs | Configured schedules in a read-only view | Hidden | `Ctrl+J` |

Typing the first character docks the composer; deleting the draft recenters it.
Submitting the prompt starts a conversation. Merely opening Gator must not show
cards, counters, setup prose, or a navigation rail. First-run setup begins only
after the first submitted prompt, or explicitly through `/model`.

When a first task requires setup, `openai`, `anthropic`, and `gemini` open a
hidden API-key prompt if their normal environment variable is absent. A
successful setup persists both the selected provider and its stable default
model before resuming the retained task. Vendor-CLI logins are not presented as
native Work providers because Gator does not import another CLI's credentials.

## Visual language

- Use one green accent (`ANSI 42`) for identity, focus, and active work.
- Use the `🐊 Gator` wordmark consistently. Center it only on the empty
  home state; keep it at the top-left everywhere after composition begins.
- Use neutral foreground (`ANSI 255`), muted metadata (`ANSI 242`), and a dark
  selection surface (`ANSI 236`).
- Use rounded one-cell borders only around an input or a selected surface.
- Keep the bordered composer at no more than 72 columns and leave at least two
  columns of margin on each side.
- Prefer whitespace over dividers. Do not repeat the product name, tagline, and
  folder name on every screen.
- Status belongs next to the conversation title. Controls belong in one short,
  muted footer.

## Interaction rules

1. Typing is always directed to the visible composer.
2. `Enter` submits; an empty submission does nothing.
3. `Ctrl+P` opens the searchable command palette from every state; it never
   mixes folders, conversations, inbox entries, or jobs into the action list.
4. `Ctrl+X` opens conversations, `Ctrl+I` opens inbox, and `Ctrl+J` opens jobs.
   `Esc` returns from a destination or closes a picker. Because terminal input
   encodes `Ctrl+I` as horizontal tab, Tab opens inbox too.
5. Opening a retained conversation goes directly to its session state. `/new`
   returns to the clean home state for the current folder.
6. Long-running work replaces input focus with a single active status; internal
   tool and specialist detail belongs in events and evidence, not permanent
   chrome.
7. Revision commands remain available, but are discoverable through help and
   the palette instead of occupying the default footer.

## Accessibility and terminal behavior

Color is never the only indication of selection or activity: the palette uses
`›`, and active work uses `●` plus text. Layouts retain usable fallbacks when no
terminal size has arrived. Labels remain plain text so screen readers and copied
terminal output keep their meaning. Future animation must respect reduced
motion and must not be required to understand run state.

## Reference boundary

OpenCode is the primary interaction reference because its official docs cover
prompting, sessions, command navigation, and customization as one terminal
system. Claude Code's [command discovery](https://code.claude.com/docs/en/commands)
and [contextual keybindings](https://code.claude.com/docs/en/keybindings) are
secondary references for separating searchable actions from navigation and
focused views. Any future redesign should preserve Gator's own identity:
proof-carrying artifacts, local snapshots, explicit authority, and restrained
green accents.
