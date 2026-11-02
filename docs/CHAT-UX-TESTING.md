# Chat UX application verification

The implementation uses real Electron and Go owners with disposable profiles, scripted models and private loopback services. An authored journey is not a passing journey. Physical speech/microphone, real external accounts, phones and signed installers require separate acceptance.

## Isolated local runs

`GATOR_E2E_BACKGROUND=1` keeps test windows unfocusable on macOS/Windows while Playwright drives their renderer. The helper covers both existing windows and later `browser-window-created` events: Electron launch can return before the asynchronous Go backend is ready and the first window exists. `chat-search.spec.ts` asserts that actual fixture windows are unfocusable in this mode. Linux retains its existing launch behavior. This mode is for deterministic local regression; it does not claim ordinary OS keyboard/focus acceptance.

The initial broad resumed run reported 70 passes, 5 failures and 8 unexecuted cases after its configured failure limit. Draft text, inline recovery, follow-up scrolling and a citation focus assertion needed investigation. The three draft/inline cases passed together after the late-window isolation correction (13.7 s), without a production draft-storage or editor change. The follow-up journeys also passed on the subsequent focused run. These observations do not prove a particular physical keystroke caused an earlier failure.

Resize intentionally dismisses an excerpt while keeping its citation focused. The citation race test now leaves and re-enters the chip before expecting a new focus excerpt; calling focus on an already-focused node does not create a focus event. Its delayed A/B IPC fixture still proves late A cannot overwrite B.

The running-parent branch test waits for the exact parent owner to report idle after the released answer appears. A rendered/streamed answer is not proof that sealing and cleanup have completed. No execution lifetime was shortened to satisfy that assertion.

Branch and plan narrow-window tests set the actual saved Gator light theme through the native command and assert `data-theme="light"`. Browser color-scheme emulation alone does not select Gator's theme. Dark plan editing and light narrow branch/plan screenshots were inspected; bounded modal scrolling, text wrapping, retained identity and the explicit review controls remain visible/reachable. The existing motion journey also inspects transitions at 10% playback and verifies reversal, focus and reduced motion.

## Commands

```sh
npm run build --prefix desktop
GATOR_E2E_BACKGROUND=1 npm run test:e2e --prefix desktop -- branches.spec.ts task-plans.spec.ts
GATOR_E2E_BACKGROUND=1 npm run test:e2e --prefix desktop
npm run check --prefix desktop
npm test --prefix desktop
```

Ordinary UI tests do not read a real API key for evaluation. Paid live-model/coding journeys require their separate explicit environment and budgeted launcher; do not enable them as an incidental regression step. Existing native clipboard fixtures retain the prior clipboard in the private test main process and restore it; no contents are logged. OS-native installation/service/tmux tests retain their explicit opt-in conventions.

Final acceptance results belong in the linked stage records and the consolidated acceptance report. Earlier suspension records remain historical: the operator explicitly resumed app tests. No additional paid evaluation or toolchain installation was performed for these checks.
