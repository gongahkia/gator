# Tranche 08 — final acceptance closure

Closure continuation, not Tranche09. Initial HEAD5897fe52b6e5280266b180b6f72fc44bc5eeac73. Existing modified/untracked files and the original final bundle are preserved. Original-report checksums are in `Evidence/Tranche08-Closure/original-report-sha256.json`. This report supplements the original evidence rather than replacing it.

## Reconciliation before targeted checks

| Original category | Exact reason / evidence already available | Missing requirement / classification | Headless sufficient? | Native/operator requirement | Closure action / severity |
| --- | --- | --- | --- | --- | --- |
| B Menu-bar | Open/shared drafts observed; broader dismissal/focus/Spaces incomplete | Verification gap, no reproduced defect | Ownership yes; presentation no | Targeted open/Escape/expand; physical multi-display/Spaces operator | Batch native journey; P2 evidence |
| C Full window | Toolbar/chat/plan screenshots; AX target/menu timeouts | Window reopen/resize/navigation evidence; cause unproven | Identity/owner isolation yes | Actual close/reopen, resize, review return | Native check; fix only reproduced defect; P2 investigation |
| F Streaming | Protocol/real Go snapshots green, no final native streaming capture | Visual stream + bounded handling measurement | State/performance yes | One held/streamed native reply screenshot | Measure real framing/state replacement; P2 evidence |
| G Composer | Actual paste/Tab; AppKit composition/lifetime tests | Full keyboard sequence, physical IME | Safety API yes; physical IME no | Keyboard batch; operator IME/FKA | Separate automated from physical; P1 if accidental Send reproduced |
| L Exact review | Real Go token/owner/correction safeguards green | Native exact-action screenshot and explicit keyboard decision | Authority yes; layout/focus no | Controlled pending action, long scroll, Return/Space decision | No real email; P1 if stale/Return authorization reproduced |
| P Multi-owner | Actual Go dialogue/capacity/drafts green; final AX navigation timed out | Native switch/return while held work, no focus steal | Execution ownership yes | Actual foreground switch and background finish | Batch native; P1 if cross-owner control reproduced |
| Q Appearance | Light/dark screenshots, contrast tests | Review/recovery and resized coverage; preference variants | Semantic contrast yes | Process-only light/dark captures; operator contrast/text variants | No OS setting writes; P2 evidence |
| R Accessibility | Labels/roles, Tab, marked-text APIs, Reduce Motion code | Spoken VO, physical IME, FKA/preferences | Cannot establish physical acceptance | Operator checklist required | `TRANCHE08-OPERATOR-ACCEPTANCE.md`; not automatic PASS |
| S Performance | Reproducible projection, owner startup/RSS/thumbnail samples | Streaming handling, clearly bounded startup/rendering method | Most measurements yes | Small native render/resize observation; true cold-cache cannot be guaranteed | Report medians + boundaries; P2 evidence |
| T Bundle/lifecycle | Four-file bundle validated, idle Quit + child cleanup; core guards | Actual close presentations while held, active Quit guard | Guards/cleanup yes | One held-work presentation-close/Quit journey | Distribution signing separately DEFERRED; P1 if work stops from close |

## Duplicate basename investigation

Native staging, native draft validation and `upload_submit.files` use distinct full opaque IDs. Go `internal/attachment/uploads.go` loads/validates full IDs then replaces provider input names with `filepath.Base`. `cmd/gator/attachments.go` subsequently calls shared `attachment.ValidateLoaded`, which rejects duplicate names. Retained originals also use basename identities. Thus an unchanged protocol does not safely support two equal provider/retention display names even though staged copies are distinct.

No Go authority or original filename was changed to bypass this. A targeted real Go regression verifies two independently selected `report.md` files retain separate immutable bytes/IDs, are rejected before model execution, preserve the complete draft, and can be removed independently. Supporting duplicates is explicitly deferred pending a coherent backend provider/retention identity contract; not a Swift path/overwrite defect.

## Provider diagnosis

Redacted read-only inspection: default gemini/gemini-3.5-flash; standard HTTPS `generativelanguage.googleapis.com/v1beta`; no URL credentials/query override; stored nonempty `api_key`; no GEMINI_API_KEY environment override, other stored provider or custom provider. Existing Go adapter sends `x-goog-api-key`. Previous HTTP401 explicitly reports invalid API key. No new calls or credential changes.

This is a provider credential/configuration blocker, not a native visual failure. Model compatibility cannot be inferred before authentication succeeds. Operator repair uses existing hidden-prompt flow: `gator provider login gemini --prompt` (or the bundled Go executable with the same arguments). Configuration: `~/.config/gator/config.json`; credentials: `~/.local/state/gator/auth.json`, subject to the existing GATOR/XDG overrides. Do not edit secrets into reports or command arguments. A valid replacement credential must be supplied by the operator; no alternative working configured provider is available for another smoke test.

## Closure evidence, changes and final matrix

In progress. Results will be appended after controlled native checks and the final headless regression pass. Original PARTIAL labels above are hypotheses/evidence requirements, not final statuses.
