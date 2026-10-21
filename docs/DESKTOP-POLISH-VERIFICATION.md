# Desktop polish: implementation and verification

**22 October 2026 follow-up:** the terminal frontend and its status-line
settings were subsequently retired when desktop became the primary interface.
This report records the preceding polish pass; see
[the current interface decision](DESKTOP-PRIMARY.md).


21 October 2026. Follow-up to the [baseline screen audit](DESKTOP-SCREEN-AUDIT.md).
Full review using `make-interfaces-feel-better`.

## Result

The twelve findings in the baseline audit have been addressed. The thirteen
screen families requested for this implementation now use the compact direction
in the exercised states: concise work summaries, direct native controls,
progressive disclosure, explicit review actions, and consistent semantic colors.

This is a presentation review of Gator's existing capabilities. Context capacity
still says **Not measured**; selecting a connection does not imply authorization;
file checks do not imply delivery. The agent loop, scheduling policy, approval
requirements, and canonical data services retain their existing behavior.

The implementation adds display fields and adapters around those services.
Inbox actions call the existing read/rating service. Native choices still pass
through Go validation and view identity checks. Folder selection fills a form;
it does not switch the conversation's workspace or create files. Restore and
relink retain their existing preview and final-review steps.

Verification also found a configuration compatibility defect: the existing
`editor` and `source` status-line items were missing from the settings validator.
They are now accepted, and save failures are visible while the editor remains
open. This does not expand the agent's permissions or tools.

## Scope and conventions

React 19, the existing Tailwind v4 theme tokens and semantic CSS, Lucide icons,
and additive Go panel projections. No additional font, styling system, or motion
library. The terminal continues to receive the complete facts and retains its
existing shortcuts. Desktop help explains desktop shortcuts; terminal-only
status-line settings are named explicitly.

The packaged macOS application was walked at 1440×960 and 800×600. The existing
end-to-end paths also check intermediate widths, fullscreen layout, focus
restoration, and modal drawers. Two fresh audit walks captured **109 states**:
57 in the main walk and 52 in the nested walk. This includes repeated navigation
checks and is not a count of 109 distinct screens. Layouts were inspected using
screenshots, contact sheets, DOM state, computed colors, and control dimensions.

All walks and tests use temporary configuration, state, Electron user data,
and workspaces. Notes and goals use the real local CLI; conversation, Inbox,
and paused-job examples use stored fixtures loaded by the real Go stores.
Conversation and queue tests use local fake providers. Rich outputs and approval
variants use display fixtures. Native file dialogs are stubbed in the interaction
test to verify path selection without displaying or accepting a real user dialog.

### Review coverage

| Category | Evidence inspected | Result |
| --- | --- | --- |
| Typography | All requested families, long paths and summaries, compact metadata, review facts, narrow forms, output labels, normal/contrast body colors | Addressed; content and supporting metadata have distinct reading order |
| Surfaces | Empty/populated lists, filters, forms, disclosures, destructive reviews, settings, dialogs, search focus, measured controls | Addressed; no measured button, select, or disclosure target below 40px in either walk |
| Animations | Existing packaged tests at 10% playback; sidebar reversal, menu/details dismissal, focus restoration, carousel interaction pause, reduced motion | Clear in exercised paths; no new motion dependency |
| Icons | Existing Lucide/currentColor rules, row chevrons, effort choice indicators, checkboxes, disclosure arrows, refresh actions, destructive actions | Addressed; affordances describe the action performed |
| Performance | Explicit CSS transition properties, bounded previews, local catalog filtering, original row identities, 300-message search and preserved reading position | Clear in exercised paths; no blanket transition or new `will-change` layer |

## Screen-by-screen result

“Aligned” applies to the states listed below, not every possible live integration
or operating-system failure. All gaps listed in the implementation request are
closed.

| Screen family | Current presentation | Exercised states | Result |
| --- | --- | --- | --- |
| Work details and output previews | Compact sections; result label and number stay together; checks, run outcome, and delivery remain distinct | Empty/rich results, earlier/failed results, Markdown, HTML source, binary unavailable, sidebar/drawer, selection/back/focus | Aligned |
| Conversations and task runs | Single destination with Chats/Task runs tabs; compact workspace/date/status metadata; explicit status filter | Empty/populated, query/no match, switching tabs, filtering, selecting a real run, continuing saved work | Aligned |
| Overview and ongoing goals | Work-first list hierarchy, actionable rows, setup guidance, supporting overview disclosure | Empty/populated, sections, blocker/detail, goal picker/search/selection, narrow view | Aligned |
| Inbox | Working result view; opening is read-only; separate read and usefulness actions | Empty, unread/urgent, open, mark read, rate useful, clear rating, back, narrow view | Aligned |
| Follow-up checks | Native grouped source/timing/permission/limit fields; health and next action lead details; complete agreement review | Empty, new form, exact changed values, folder selection, review/edit, paused check/detail, narrow form including limits | Aligned |
| Saved knowledge | Filter of loaded titles/text/scope with visible coverage; short previews; content before provenance | Empty/populated, matching/no-match filter, null reference data, detail, correction, forget review/cancel, narrow/reduced motion | Aligned |
| Memory and retrieval | Recall readiness first; distinct local/cloud choices; storage/retrieval/chat-processing disclosures | Basic recall, unavailable Ollama, local/cloud setup reviews, normal/contrast body text, narrow view | Aligned |
| Model selection | Searchable catalog, retained model identity, provider administration grouped away from selection | Setup chooser, 119-row catalog, filtered/no match, configure filtered provider, masked key/custom-provider forms, local unavailable, help | Aligned |
| Usage and limits | Shared compact settings layout, native numbers, one review summary, explicit Save/Pause/Resume actions | Uncapped/limited/activity, edit/review/save, paused/resumed, accounting details, normal/narrow | Aligned |
| Data and backups | Storage/retention summary; native path pickers and mapping pairs; specific preview/save/delete verbs | Root, export/verify/relink/restore/retention forms, mappings, file/destination pickers, reviews, deletion plan, no eligible cleanup, narrow forms | Aligned |
| Recovery | Background health/login configuration first; coverage caveat visible; diagnostics secondary | Offline, paused check recovery entry, start-at-login review/cancel, narrow review | Aligned |
| Saved guidance and status line | Native type/scope/status choices; content first; locked edit scope/type; explicit terminal visibility, toggles and order controls | Add/save/detail/edit, status filter/no match, terminal visibility, item selection/reordering/save/reopen, narrow guidance | Aligned |
| Workspace, search, help, and drafts | Short workspace labels with exact paths available; visible search focus; desktop help; contextual Back labels | Workspace picker, conversation/command search, help/authority results, draft recovery/manager/retention, focus and unsaved-view guard | Aligned |

## Findings closed

Severity records the baseline impact. Rows consolidate shared changes across
all affected destinations. Locations point to the implementation, not the old
line numbers in the baseline audit.

| Severity | Location | Before | After | Why |
| --- | --- | --- | --- | --- |
| HIGH · F1 | `internal/worktui/inbox.go:78`; `internal/worktui/bridge_panel.go:201`; `internal/worktui/update.go:455`; `desktop/src/Panel.tsx:122` | Inbox rows implied navigation but did nothing | Open retained content without acknowledgement; explicit read/rate/clear actions use the existing service and current view identity | Truthful affordances and deliberate state changes |
| MEDIUM · F2 | `desktop/src/styles.css:43`, `:447`; `desktop/src/Panel.tsx:115`; `desktop/src/App.tsx:291` | Search focus was suppressed or hard to identify | Shared visible focus treatment for collection, transcript, command and workspace search | Keyboard users can identify the active control |
| MEDIUM · F3 | `desktop/src/PanelField.tsx:6`; `desktop/src/StatusLinePanel.tsx:6`; `desktop/src/Panel.tsx:113`; `internal/worktui/bridge_fields.go:1`; `internal/worktui/followup.go:397`; `internal/worktui/bridge_panel.go:314`; `desktop/electron/main.mjs:114`; `desktop/electron/policy.mjs:54`; `internal/config/store.go:531` | Cyclic controls, comma-separated limits, terminal path mappings and indirect selection | Native selects, multi-choice sources, numbers, mapping pairs, path pickers, status filters, effort choice indicators, terminal settings controls; host accepts the bounded filter action; existing status-line choices persist | Direct manipulation with canonical validation and review retained |
| MEDIUM · F4 | `desktop/src/Panel.tsx:15` | Empty views offered unavailable selection/details or implied the whole library was empty after filtering | Useful empty/no-match guidance; unavailable actions removed; filtered guidance has its own empty state | Empty states explain the next available step |
| MEDIUM · F5 | `desktop/src/Panel.tsx:33`, `:80` | Long unsearchable model list and broad administration toolbar | Local provider/model filter with counts, original row indices, grouped provider administration and disabled selection on no match | Selection stays quick and refers to the model the user sees |
| MEDIUM · F6 | `internal/worktui/recall.go:114`; `desktop/src/PanelBlocks.tsx:9` | Diagnostics, provider labels and setup controls competed with readiness | Recall readiness first; distinct processing choices; supporting information disclosed progressively | Users can understand readiness and setup consequences |
| MEDIUM · F7 | `desktop/src/PanelBlocks.tsx:5`; `desktop/src/presentation.ts:1`; `desktop/src/DraftManager.tsx:8`; `internal/worktui/bridge_panel.go:245`, `:314`; `internal/worktui/followup.go:369`; `internal/worktui/context.go:153`; `cmd/gator/work_data.go:19`; `cmd/gator/work_recovery.go:51`; `cmd/gator/work_overview.go:53` | Long paths, IDs and equal-weight diagnostics dominated lists/details | Shared compact facts, readable row previews, shortened workspace/time metadata, health/content/next-action hierarchy and supporting disclosures; exact review paths stay visible | Information hierarchy makes work scannable without dropping facts |
| MEDIUM · F8 | `internal/worktui/controls.go:233`; `internal/worktui/followup.go:369`; `internal/worktui/context.go:153`; `internal/modelcatalog/panel_snapshot.go:91`; `cmd/gator/work_data.go:239`; `desktop/src/Panel.tsx:127` | Repeated descriptions, generic confirmations, duplicate destructive action, repeated old excerpt, meaningless empty review body | One review summary, labeled facts, specific verbs, single destructive affordance, complete agreements, content-only correction editor, no empty hint on complete reviews | Decisions remain explicit and consequences readable |
| MEDIUM · F9 | `desktop/src/Panel.tsx:35`, `:115`; `internal/worktui/context.go:153` | No quick knowledge filter and hidden collection boundary | Filter loaded items; visible count/up-to-100 boundary; separate explanation of full-search commands | Finding content does not overstate search coverage |
| MEDIUM · F10 | `desktop/src/DesktopHelp.tsx:18`; `desktop/src/App.tsx:71`; `internal/worktui/bridge_panel.go:43`; `desktop/src/Panel.tsx:10`, `:95`; `internal/worktui/recall.go:114` | Terminal guidance, inconsistent names, generic Back label and action casing | Desktop shortcut/help presentation; contextual Back; consistent destination names and sentence case; useful guidance-form subtitle; terminal settings named explicitly | Language describes the interface and action actually available |
| MEDIUM · F11 | `desktop/src/styles.css:1`, `:37`, `:337` | Secondary content bypassed contrast tokens | Secondary text, fills, focus, warning/danger and hover states use shared semantic tokens | Contrast preference applies to content-heavy screens |
| LOW · F12 | `desktop/src/WorkDetails.tsx:122`; `desktop/src/styles.css:65` | Result label and number could split in the inspector | Keep-together span for the label/number; surrounding status can wrap | Output provenance remains easy to scan |

### Considered but rejected

| Location | Candidate | Rejected because |
| --- | --- | --- |
| Work details / Context | Show a capacity percentage from daily token usage | Daily totals do not measure context-window occupancy; capacity remains unmeasured |
| Connections | Add an enablement switch styled like authorization | Source selection is not authentication or a new permission grant |
| Reviews and approvals | Collapse exact destinations and consequences | The user must see the complete action before deciding |
| Forms and lists | Introduce a new motion package or staged animation on each edit/filter | Existing transitions and reduced-motion support suffice; frequent actions should be immediate |
| Output rows | Treat successful file checks as delivered work | Run outcome, verification and delivery are different facts |

## Verification

Final application source: `26e95738a`. All checks below passed against the
completed implementation; the packaged build was allowed to finish before the
final browser tests and screenshot walks.

| Check | Observed result |
| --- | --- |
| `go test ./internal/config ./internal/worktui ./internal/modelcatalog ./internal/uisnapshot ./cmd/gator -count=1` | Pass; all packages with tests pass; `uisnapshot` has no test files |
| `go vet ./internal/config ./internal/worktui ./internal/modelcatalog ./internal/uisnapshot ./cmd/gator` | Pass |
| `npm test` in `desktop` | 10/10 pass |
| `npm run check` in `desktop` and `ui` | Both pass |
| `npm run pack` in `desktop` | Pass; macOS development package produced |
| `node --check desktop/electron/main.mjs` | Pass |
| Packaged `npm run test:e2e` | 14/14 pass; 35 existing and 18 added screenshot artifacts |
| Two final screenshot walks | 109 PNGs; zero renderer errors, skipped states, or page-overflow findings |
| Search focus | Conversation and command inputs have a computed 2px accent outline and match `:focus-visible` |
| Contrast preference | Actual Memory body text changes from `rgb(151,151,151)` to `rgb(184,184,184)` |
| Reduced motion | Final reduced-motion audit state has no active animations; interruption and 10% playback checks pass |
| Measured button/select/disclosure controls | None below 40×40px in either audit walk |
| Final diff whitespace | `git diff --check` passes |

The package emits existing Lucide module-directive and dependency-reference
warnings. Code signing is explicitly disabled for this local development build.
No signing or distribution claim is made by this verification.

### Reproducing the committed interaction checks

From the repository root:

```sh
go test ./internal/config ./internal/worktui ./internal/modelcatalog ./internal/uisnapshot ./cmd/gator -count=1
go vet ./internal/config ./internal/worktui ./internal/modelcatalog ./internal/uisnapshot ./cmd/gator
npm test --prefix desktop
npm run check --prefix desktop
npm run check --prefix ui
npm run pack --prefix desktop
```

Then, from `desktop`, after packaging finishes:

```sh
GATOR_DESKTOP_TEST_EXECUTABLE="$PWD/release/mac/Gator.app/Contents/MacOS/Gator" npm run test:e2e
```

The new committed `desktop/tests/e2e/polish.spec.ts` checks actual Inbox
acknowledgement/rating persistence, filter identity, direct form values, path
selection without side effects, restore mapping review, native guidance choices,
and saved terminal settings. Existing tests retain coverage of drafts, queues,
conversation search, first-run setup, approvals, motion and output previews.

Local ignored screenshot evidence is in
`desktop/playwright-report/polish-round/{walk,nested}` and `desktop/test-results`.
The audit JSON files record source commit, projected panels, DOM text, measured
controls, focus evidence, renderer errors and overflow results. The one-off
walks use their own temporary profiles and stop their own Electron children.

### Boundaries

Not verified with real services in this presentation pass: account authorization,
credential replacement against a real connector, a local-model installation or
download, background-service installation/start, a production backup restore,
external delivery, and macOS crash/startup dialog variants. The tests verify the
associated shared presentation and canonical review paths where listed above;
they do not certify those external operations. Windows/Linux runtime appearance
and very large collection/restore performance were not measured on this Mac.

## Rollback

Changes are split into atomic local commits, starting at `efb3d0e01` after the
baseline audit commit `43f461c30`. Each commit names one interaction, presentation
family, shared primitive, or verification correction. Inspect the sequence with
`git log --oneline 43f461c30..HEAD`. Revert dependent display-schema changes in
reverse order; individual screen refinements can be reverted independently when
their shared projection/renderer prerequisites remain present.

## Verdict

**Approve for the exercised desktop presentation scope.** No actionable
interface-polish findings remain from F1–F12 or the requested thirteen screen
families. The final screenshot pass confirmed the guidance subtitle, filtered
empty state, concise recovery review, mappings, and native settings after the
last fixes.

**Not verified:** the real-service, platform, startup/crash and workload variants
listed under Boundaries. This verdict does not certify those external operations.
