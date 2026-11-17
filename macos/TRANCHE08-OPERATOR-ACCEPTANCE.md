# Tranche 08 — operator acceptance

Acceptance candidate: `/tmp/Gator-Native-Tranche08-Acceptance.app` (arm64 build on macOS26.5.2). Automated checks and current screenshot evidence are in `TRANCHE08-ACCEPTANCE-CLOSURE.md`. None of the physical rows below has been signed off by an operator.

Record device/OS, app path, date and operator: ____________________
Use a disposable profile with the closure loopback fixtures, or harmless local data. Never approve a real external send for this checklist. Do not enter keys into screenshots. Automated AX labels/marked-text API tests are supporting evidence, not spoken VoiceOver or physical IME acceptance.

The operator chooses and restores any system accessibility settings or input source. This agent did not change them. Record PASS / FAIL / NOT TESTED and notes for each row.

| Check | Steps | Expected result | Result / notes |
| --- | --- | --- | --- |
| 1. VoiceOver conversation | With VoiceOver enabled by the operator, open compact chat; navigate headings, user/assistant messages and composer; read a longer conversation. | Roles and order are understandable; no repeated narration of the whole transcript on each streamed snapshot. Message text is selectable. | NOT TESTED — ______ |
| 2. VoiceOver task/action review | Open a controlled task and exact pending action. Navigate objective, account/destination, complete content, consequences, Back, Decline and Approve Exact Action. | Controls are distinct; long content scrolls; Return never approves; only an intentional Space/VoiceOver activation can decide the current exact proposal. Decline the controlled proposal. | NOT TESTED — ______ |
| 3. VoiceOver attachments | Stage a document and image, navigate each attachment and remove only one. | Name/type and Remove are understandable; correct item removed; draft text preserved; no Send. | NOT TESTED — ______ |
| 4. VoiceOver recovery | Open a controlled failed/interrupted task, expand retained evidence, inspect eligibility/receipt information, return to chat. | Failure, interruption and uncertainty are distinct; no automatic retry/check/write; focus returns sensibly. | NOT TESTED — ______ |
| 5. Physical IME | Choose an IME yourself. Start marked composition, press Return to commit; switch compact/full and conversations while composing; commit and then explicitly Send. Test Shift-Return. | Incomplete composition never submits; Return commits composition; Shift-Return inserts a newline; no text crosses conversation ownership. | NOT TESTED — ______ |
| 6. Full Keyboard Access | Enable it yourself if desired. Tab/Shift-Tab through composer, attachment buttons, plan-first, Send, task navigation and review controls. | No trap/unreachable critical action; focus visible; explicit Space decisions; unrelated drafts retained. | NOT TESTED — ______ |
| 7. Increase Contrast | Enable it yourself; inspect light/dark chat, error, approval, plan and attachment controls. | Critical text, separators, focus and disabled states remain perceivable; no color-only status. | NOT TESTED — ______ |
| 8. Reduce Motion | Enable it yourself; stream a reply, switch chats, open/dismiss inspectors and expand the window. | No distracting new transitions; state remains understandable through text/symbols. | NOT TESTED — ______ |
| 9. Larger text/wrapping | Use your preferred supported macOS larger-text/display setting. Resize full window to540×480. Read long addresses, exact action body, permissions and plan effects. | Material content wraps/scrolls and is selectable; no critical truncation; decisions remain reachable. | NOT TESTED — ______ |
| 10. Two-surface focus | Type an unsent draft, open task/review, expand, close/reopen full window, return compact, Escape, reopen, Cmd-L. Repeat while controlled work runs. | Same selected owner/draft/review; focus follows intentional navigation; no new owner from presentation changes; closing surfaces never stops work. | NOT TESTED — ______ |

Failures: record the exact steps, screenshot where safe, app version and whether a real external effect could have occurred. A failed authority/accidental-send check is a release blocker. Do not work around it with broad permissions.

Overall operator verdict: ____________________  Remaining checks: ____________________

Additional platform coverage: move the open compact/full views between Spaces and displays while a harmless task runs; dismiss/reopen and verify the selected owner, draft and keyboard focus. Expected: work continues and focus follows intentional navigation. PASS / FAIL / NOT TESTED: __________

## Conditional notch supplement

Use `/tmp/Gator-Native-Tranche08-Enhanced-Final.app` on a Mac that reports a camera notch. Do not replace the earlier VoiceOver/IME/preference checks with this supplement.

| Step | Expected result | PASS / FAIL / notes |
| --- | --- | --- |
| Hover the camera area while typing in another app, then leave | Gator's work glance opens/closes without keyboard focus leaving the other app. | ___ |
| In Gator, press ⌘⇧G, then Tab and Shift-Tab | Dismiss and chat controls receive visible keyboard focus in order; VoiceOver announces purpose and status. | ___ |
| Press Escape with an unsent composer draft | Glance closes; original window/composer draft remains available. | ___ |
| With a controlled pending action, choose its owning chat in the glance | Original owner and current exact review open; no approval or external effect occurs. | ___ |
| Repeat with VoiceOver, Full Keyboard Access, Increase Contrast and Reduce Motion | Labels remain understandable, controls reachable, contrast readable and motion suppressed when requested. | ___ |
| Move to a flat external display / rearrange displays | No invented notch on a flat display; the real notched screen's panel remains correctly placed. | ___ |

## Final local-development operator sign-off — 2026-11-17

Current candidate: `/tmp/Gator-Native-Tranche08-Enhanced-Final.app`. Automated pending-approval/notch/keyboard evidence is now PASS; these **human** rows remain NOT TESTED. Device/OS: ______ Date/operator: ______. Use a disposable profile and the inert local fixture; decline every proposal. The operator chooses and restores OS settings/input sources. Do not use simulated events or AX labels as spoken VoiceOver/physical IME sign-off.

| Check | Operator steps and expected result | PASS / FAIL / NOT TESTED; notes |
| --- | --- | --- |
| VoiceOver conversation | Navigate headings, messages and composer in compact/full views. Roles/order are understandable; streaming does not repeatedly narrate the whole transcript. | NOT TESTED — ______ |
| Approval review and decline | Begin the inert pending approval; read owner, destination/scope and full content. Return does not decide. Intentionally decline with VoiceOver/Space; no external action occurs. | NOT TESTED — ______ |
| Notch open/dismiss | Open from hover and ⌘⇧G. Status/rows are announced; hover leaves the other app's focus intact; Escape restores the original draft/focus. | NOT TESTED — ______ |
| Physical IME | Compose marked text and press Return to commit; repeat after compact/full changes. Composition commits without Send; Shift-Return inserts a newline. | NOT TESTED — ______ |
| Tab/Shift-Tab | Traverse composer, attachments, notch, task navigation and both decisions. Focus is visible, every critical control reachable, no trap. Repeat under the operator's Full Keyboard Access preference. | NOT TESTED — ______ |
| Reduce Motion | Enable it yourself and open/close notch, switch chats and inspect work. Expansion/opacity motion is suppressed; text/symbol status remains understandable. | NOT TESTED — ______ |
| Increase Contrast | Enable it yourself in light/dark views. Critical text, disabled controls, boundaries and focus remain perceivable without relying only on color. | NOT TESTED — ______ |
| Larger text/wrapping | Choose a supported larger-text/display preference; resize full view. Long exact content wraps/scrolls; owner, consequences and decision controls remain reachable. | NOT TESTED — ______ |

Overall physical accessibility verdict: ______. Outstanding rows: ______. Keep the earlier attachment-removal/recovery and Spaces/display rows for broader platform coverage.
