# Gator terminal design system

## Product posture

Gator should feel like a quiet work surface, not an operations dashboard. The
first screen asks for an outcome and postpones navigation, history, jobs, inbox,
and the coding workflow until the user asks for them. Evidence and controls
remain available, but they appear at the point where they become relevant.

The interaction model is informed by the official [OpenCode TUI
documentation](https://opencode.ai/docs/tui/) and its current open-source TUI:
a centered first prompt, a bottom-docked session prompt, keyboard-first command
access, and resumable sessions. Gator does not copy OpenCode's branding or
screen treatment; it adopts the underlying progressive-disclosure pattern.

## Screen states

| State | Primary content | Composer | Secondary navigation |
| --- | --- | --- | --- |
| Home | Gator mark and one outcome question | Centered, at most 72 columns | Hidden behind `Ctrl+P` |
| Drafting | Selected folder and a quiet empty transcript | Docked after the first typed character | Hidden behind `Ctrl+P` |
| Conversation | Transcript and current run status | Docked below the transcript | Hidden behind `Ctrl+P` |
| Command palette | Sources, conversations, inbox, jobs, setup, and Code | Hidden | Centered, keyboard-selectable list |

Typing the first character docks the composer; deleting the draft recenters it.
Submitting the prompt starts a conversation. Merely opening Gator must not show
cards, counters, setup prose, or a navigation rail. First-run setup is one
palette entry and one subdued home hint.

## Visual language

- Use one green accent (`ANSI 42`) for identity, focus, and active work.
- Use neutral foreground (`ANSI 255`), muted metadata (`ANSI 242`), and a dark
  selection surface (`ANSI 236`).
- Use rounded one-cell borders only around an input or a selected surface.
- Keep the composer at 72 columns or the available width minus eight columns,
  whichever is smaller.
- Prefer whitespace over dividers. Do not repeat the product name, tagline, and
  folder name on every screen.
- Status belongs next to the conversation title. Controls belong in one short,
  muted footer.

## Interaction rules

1. Typing is always directed to the visible composer.
2. `Enter` submits; an empty submission does nothing.
3. `Ctrl+P` opens the palette from every state; `Esc` closes it.
4. Opening a folder returns to the clean home state. Opening a retained
   conversation goes directly to its session state.
5. Long-running work replaces input focus with a single active status; internal
   tool and specialist detail belongs in events and evidence, not permanent
   chrome.
6. Revision commands remain available, but are discoverable through help and
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
system. [Claude Code's feature overview](https://code.claude.com/docs/en/features-overview)
is a secondary reference for keeping advanced capabilities available without
putting them all on the starting screen. Any future redesign should preserve
Gator's own identity: proof-carrying artifacts, local snapshots, explicit
authority, and restrained green accents.
