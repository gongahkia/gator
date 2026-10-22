# Conversation concept

Open `index.html` in a browser. There are no dependencies, network requests,
provider calls, file reads, persistent storage or real approvals. All figures,
files, messages, schedules and availability states are fictional examples.

The top selector demonstrates returning, first-visit, working and asleep states.
The appearance toggle demonstrates light and dark palettes. Drafts stay in
memory as topics change and are discarded when the page closes or reloads.

See [the surface plan](../../CONVERSATIONAL-SURFACE-PLAN.md) for product decisions
and what needs real application support before shipping.

## Interface review

Full review of this static concept, using plain HTML/CSS/JavaScript. Production
Electron screens and backend behavior were not changed or validated by this
prototype. This is not a new production styling system.

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | Light/dark returning and first-visit screenshots; narrow layout | Reading column, wrapping and hierarchy inspected; small supporting text needs user testing |
| Surfaces | Composer, file card, review dialog, details drawer | Distinct structure, contained review, subdued elevation |
| Animations | Drawer entrance and reduced-motion CSS | Restrained 180 ms entrance; no message/keystroke animation; reduced motion disables movement |
| Icons | Shared inline outline SVGs, alligator identity | One stroke convention and currentColor; no external asset dependency |
| Performance | Local static assets; Chrome page errors during interactions | No page errors; no network/model work or continuous animation |

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| MEDIUM | Memory/check examples in `app.js` | Corrections and pause state could reset when a drawer reopened | Keep the demonstration state for the lifetime of the page | An acknowledged change must remain reflected in the interface |

| Candidate | Decision |
| --- | --- |
| Animated incoming-message entrances | Rejected: routine messages should be easy to read without repeated movement |
| Persistent context meter/environment panel | Rejected: the ordinary conversation does not need diagnostic accounting |
| Voice and upload controls that pretend to work | Rejected: voice is absent; file intake explains its prototype boundary |
| One merged history for every topic | Rejected: a familiar agent identity does not justify crossing conversation/source boundaries |

Verification: exercised with headless Chrome through Playwright. Checked draft
continuity across topics, exact email review, file preview, theme switching,
queued-message editing, unavailable-check copy, first-request preservation
through setup, reduced motion and 390 px overflow. Inspected screenshots at
1440×1000 and 800×600. No production or competitor app runtime was inspected.

Verdict: **Needs changes before production**. Native platform behavior, real
account/file access, durable read/resolved state, full accessibility and contrast
auditing, real-user usefulness and the production journeys remain unverified.
