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
