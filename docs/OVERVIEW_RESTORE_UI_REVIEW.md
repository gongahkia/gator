# Overview and restore interface review

Mode: full. Scope: the everyday home, overview section/item pickers, restore and
source relink forms, confirmations, return navigation, and refresh behavior.
Framework: the existing Claude Code Kit React terminal renderer, with Go owning
state and actions. Existing Text, Box, Divider, picker, form, and keyboard-hint
patterns are reused. No additional styling system was introduced.

## Coverage

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | `ui/src/overview.tsx`, home and panel rendering at 80×24, 48×14, 20×12 | Four questions render together; compact labels and truncated details retain drilldown |
| Surfaces | Existing panel, form, confirmation, dividers, footer and composer anchor | Existing Kit surfaces retained; footer space is included in body geometry |
| Animations | Background refresh and explicit loading states | Quiet refresh adds no animation/loading flash; no new motion requiring a 10% speed review |
| Icons | Existing Gator logo and row selection marker | Existing icon and color conventions retained |
| Performance | Async read generations, selection identity, stable subtitle, cancellation | Refresh preserves selection and scroll-reset inputs; stale responses are ignored |

## Resolved findings

### Typography and surfaces

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| MEDIUM | `ui/src/overview.tsx:20`, `ui/src/main.tsx` | Home separated next-action hints from check health and omitted other everyday questions | Four summaries, then the existing section/item picker for exact actions | Gives each question a visible place while keeping details available in narrow terminals |
| MEDIUM | `cmd/gator/work_data.go` | Backup verification had no restore or moved-source workflow | Existing forms lead to exact destination/source reviews and separate final confirmation | Makes scope and the pending state clear before a change |
| MEDIUM | `ui/src/main.tsx` | Panel footer facts were hidden when keyboard hints existed | Footer facts get a reserved line alongside hints | Keeps recency and bounded-list information visible |

### Interaction and motion restraint

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| HIGH | `internal/worktui/update.go` | The global Ctrl+I shortcut intercepted Tab inside a form | Forms own Tab; the workspace shortcut applies in chat | Preserves the expected field-navigation interaction and entered values |
| MEDIUM | `internal/worktui/controls.go`, `internal/worktui/update.go`, `cmd/gator/work_overview.go` | Periodic reads could show loading, reset selection, or change scroll-reset inputs | Quiet reads preserve item identity; the subtitle remains stable and recency uses the footer | Avoids repeated attention cost while the user reads |
| MEDIUM | `internal/worktui/overview.go`, `internal/worktui/controls.go` | Closing a control could lose the everyday home context | Esc returns through section levels; the composer and current profile survive | Preserves orientation and ongoing work |

## Considered but rejected

| Location | Candidate | Rejected because |
| --- | --- | --- |
| Everyday overview | A separate dashboard/card system | Existing Kit text and pickers provide the required actions at all tested widths |
| Background refresh | Animated notices for each update | Repeated animation would add attention cost without helping the user choose an action |
| Restore success | Automatically switch the active session into the restored profile | The current conversation and composer should survive; an explicit launch command makes the destination clear |

## Verification

- `go test ./...`: passed. Subsequent keyboard, navigation, and refresh changes
  passed the complete Work TUI suite and affected command tests.
- Race checks for lifecycle, configuration, Work home, and Work TUI: passed.
- `go vet ./...`, followed by affected package checks: passed.
- TypeScript checking, frontend tests, and bundle build: passed.
- Offline Linux Go 1.25 container: restore, relink, overview, and controller tests passed.
- Compiled runtime with a local fake provider: exported, restored and continued
  a goal with fresh source capture; provider configuration stayed isolated and
  usage began paused. Only two local fake model calls were made.
- Real backend bridge: exact missing-source action, relink review, both restore
  confirmations, active-profile preservation, and composer preservation passed.
- Live 15-second refresh: selected item stayed selected, subtitle stayed stable,
  no loading flash appeared, and Esc returned to home with the draft intact.
- PTY walkthroughs at 80×24, 48×14, and 20×12: four home summaries, Ctrl+O,
  visible freshness, and restore/relink actions rendered and navigated.

Visual walkthroughs used macOS. Linux runtime logic was exercised in a
container; a Linux terminal visual walkthrough and participant usability study
were outside this review.

Verdict: **Approve**. No actionable interface findings remain in the inspected scope.
