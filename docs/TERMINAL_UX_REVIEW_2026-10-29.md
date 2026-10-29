# Terminal chat UX review — 2026-10-29

Full review of the native terminal chat catalog, transcript, loading indicator,
header, and shared terminal colors. Framework: Solid / OpenTUI 0.6.8; existing
`useTheme` tokens, native text attributes and renderables. No new styling or
runtime dependency. Desktop and companion styling are outside this review.

## Coverage

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Catalog at 110×36 and 60×24; Markdown headings, emphasis, code, lists, links, tables, and quoted text; transcript search/scroll | Compact labels; hidden Markdown punctuation; retained content and identity preserved |
| Surfaces | Captured native color intents; dark/light previews; rail, composer, dialog and file text | Default terminal foreground/background; indexed accents; no branded canvas |
| Animations | Ten recorded spinner frames; timed native renderer test; approval, cancellation, reduced-motion and disposal paths | One-cell Dots preset at 80ms; static status when attention is needed |
| Icons | Alligator header; connected/starting/disconnected/error renderer states; search hover handlers | Emoji plus labeled status dot; color is not the only signal |
| Performance | Type check/build; transcript position/subscription checks; preset timer cleanup | No Rust subprocess, added package, theme polling or per-frame theme allocation |

## Findings addressed

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| Medium | `ui/src/gator/panel.tsx`, `display.ts`, `actions.tsx` | Raw fractional timestamps and full paths wrapped every chat; repeated shortcuts | Two-line rows, local short dates, folder names, dim metadata, conditional paging; F1 help and F3 full details | Text wrapping and hierarchy: more chats remain visible without discarding metadata |
| High | `ui/src/gator/transcript.tsx`, `markdown.tsx` | Markdown punctuation was literal text; missing dates appeared as year 0001 | Native styled Markdown; headings 1–6; compact dates; omit absent dates and empty Deliverables headings | Readable typography; preserve code literals, evidence, raw data and transcript indices |
| Medium | `ui/src/gator/working-status.tsx`, `component/spinner*.ts*` | Fading implementation jargon | Rattles Dots preset plus “Thinking…” and elapsed time; retain meaningful backend statuses | Motion restraint: fixed-width glyph, static approval/stopping labels, reduced motion |
| High | `ui/src/context/theme.tsx`, `index.tsx`, `gator/{files,panel,transcript}.tsx`, `component/prompt/index.tsx`, `ui/dialog.tsx` | Forced green canvas and white default labels on light backgrounds | Native default foreground/background intent and terminal palette accents; explicitly colored text/editor defaults | Surfaces and contrast follow the terminal, including terminals that cannot answer palette queries |
| Medium | `ui/src/component/session-tabs{,-rail}.tsx`, `ui/dialog.tsx` | Decorative half-block rail depended on fixed RGB fills; translucent black modal backdrop | Plain Search control with hover feedback, rail divider, terminal-default modal canvas and top rule | Surface consistency without opaque branded fills or decorative bars |
| Low | `ui/src/app.tsx`, `gator/connection-status.tsx` | Plain Gator title and unstyled connection word | 🐊 Gator; green/yellow/red palette dot with label; long title truncates ahead of status | Clear state without relying only on color |
| Medium | `ui/src/gator/hub.tsx`, `panel.tsx` | Appearance implied desktop presets controlled terminal colors | Explain that terminal colors come from terminal settings and presets apply to desktop | Accurate control descriptions |

Connection status describes the local application host connection, not account
sign-in or provider reachability. Transparency is controlled by the terminal;
Gator emits its default background instead of painting a green or white canvas.
`NO_COLOR` preserves labels with neutral colors. `GATOR_REDUCED_MOTION=1` keeps
working feedback static. Spinner notices ship in packaged distributions.

## Considered but rejected

| Location | Candidate | Rejected because |
| --- | --- | --- |
| Catalog | Remove folder/date metadata entirely | Similar titles need context; the compact row and full-details action retain it |
| Theme | Guess light/dark RGB colors or depend on OSC queries | Native color intent works without detection and keeps the terminal responsible for its own palette |
| Dialogs | Full box border | Added width/height altered wrapping and crowded existing compact dialogs; top rule preserves the existing content space |
| Spinner | Port the full Rust runtime and every preset | Only the Dots frame data and timing are needed; OpenTUI already owns terminal IO and lifecycle |

## Verification

Commands used from the repository root (Bun is locally installed):

```sh
.gator/native-tools/node_modules/.bin/bun run --cwd ui check
PATH="$PWD/.gator/native-tools/node_modules/.bin:$PATH" bun run --cwd ui build
.gator/native-tools/node_modules/.bin/bun test --cwd ui --preload ./scripts/preload.ts tests
env -u NO_COLOR .gator/native-tools/node_modules/.bin/bun test --cwd ui --preload ./scripts/preload.ts tests/terminal-colors.test.tsx tests/markdown.test.tsx tests/working-status.test.tsx tests/appearance.test.tsx tests/catalog.test.tsx
.gator/native-tools/node_modules/.bin/bun test --cwd ui --preload ./scripts/preload.ts tests/updates.test.tsx
```

- Type check and production build pass.
- Full-suite checkpoint: 61 passed, 3 skipped, 2 failed. The failures exposed
  dialog wrapping and an asynchronous file handoff test. After correcting the
  dialog decoration, Office keyboard editing/export passed independently.
- Update tests subsequently exposed a reply test that edited before its reply
  context was ready. It now waits for that observable context: both update tests
  pass, 52 assertions. An intermediate concurrent run also hit an OpenTUI
  `OutputBusy` capture; the final isolated run did not reproduce it.
- Color-enabled focused run: 6 passed, 97 assertions. Final Markdown heading
  coverage plus connection/date checks: 3 passed, 39 assertions. A separate
  focused run of panel, command, approval, hub and app controls passed 11 tests.
- Catalog checks include narrow rows, full detail access, paging, search,
  rename/pin/archive/restore, stale edit refusal and restart. Existing transcript
  reading-position and activity tests pass. No real account or user profile used.
- Captured native frames were previewed on light and dark canvases and inspected
  at 110 and 60 columns. They are visualizations of native renderer output,
  not screenshots of the user's terminal emulator. Local evidence is under
  `.gator/native-tools/terminal-ux-*`; frame capture source is retained under
  `.gator/native-tools/terminal-ux-evidence/`.
- The preview replays recorded spinner frames at 800ms (10% speed). Automated
  native frame checks verify motion and fixed text; continuous human playback at
  that speed is **not verified**. Mouse hover handlers were reviewed; keyboard
  focus/selection, loading, empty results and modal dismissal were exercised.

**Verdict: Approve within the native renderer review boundary.** No remaining
known actionable polish finding in this scope. Not verified: physical terminal
opacity/theme switching, native Windows appearance, continuous human 10% motion
review, and the three environment-dependent attachment, voice and browser tests.
The entire suite was not rerun after the focused fixes; the affected failures
were rerun individually.
