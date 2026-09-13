# Hot Cross Buns development checklist

This checklist is for a release-quality Linux and macOS product with a CLI/TUI and a windowed desktop app in **one repository**, sharing the existing Python application, storage, and sync code. Performance work follows measured user-facing bottlenecks. An unchecked item is work to plan or verify, not a claim that the current implementation is broken.

The two open issues, [#423](https://github.com/gongahkia/hot-cross-buns/issues/423) and [#424](https://github.com/gongahkia/hot-cross-buns/issues/424), were written for the retired native wrapper. Their live-account acceptance and distribution requirements remain useful, but their scope needs updating for the Python product and the new Linux/macOS goal.

## First: establish a reliable baseline

- [ ] Re-scope or close and replace [#423](https://github.com/gongahkia/hot-cross-buns/issues/423) and [#424](https://github.com/gongahkia/hot-cross-buns/issues/424). In particular, reconsider #424's rule that Linux parity waits for macOS acceptance.
- [ ] Restore a green local and CI baseline. The month-grid rendering test currently fails reproducibly; the calendar mouse smoke reports a successful drag but times out while the child process exits.
- [ ] Run and record the [live Google acceptance procedure](docs/testing/live-google-tui-smoke.md) with disposable accounts on both Linux and macOS. Include initial and incremental sync, offline writes, recurrence, conflicts, OAuth expiry/revocation, retries, and restart recovery.
- [ ] Test the CLI/TUI, future desktop app, and optional reminder process running at the same time. Define process-level sync ownership and verify that active outbox deliveries are not mistaken for interrupted ones.
- [ ] Decide the OAuth distribution model: retain bring-your-own Google client credentials or pursue a project-owned client and its verification process. Confirm requested scopes, token storage, keyring behavior, and onboarding on both platforms.
- [ ] Define the first supported Linux distributions, macOS versions, architectures, and terminal environments; record what is outside the initial release target.

## Build the desktop app on the shared core

- [ ] Define a desktop MVP and a parity map against the existing CLI/TUI: tasks, calendars, search, capture, editing, sync/conflicts, settings, and reminders. Keep later features explicitly deferred.
- [ ] Establish a UI-independent application interface. Move direct storage and Google calls out of the TUI where necessary so the desktop UI can reuse the same operations without copying business logic.
- [ ] Choose and validate a Linux/macOS desktop toolkit with a small prototype covering a calendar view, a large task list, OAuth callback, and packaging.
- [ ] Add a separate desktop entry point while preserving the `hcb` CLI/TUI contract. Keep one account model, configuration format, data directory, credential store, and migration path across both entry points.
- [ ] Implement the MVP's keyboard and mouse workflows, accessible focus and labels, loading/error states, offline status, and conflict recovery.
- [ ] Keep long-running sync, search, and calendar layout work responsive in the desktop UI; test cancellation and shutdown during those operations.

## Make Linux and macOS releases dependable

- [ ] Add Linux and macOS CI and installed-artifact smoke coverage, including supported architectures and credential-free checks. Keep platform-specific manual acceptance documented.
- [ ] Provide desktop reminders and background scheduling on Linux; verify macOS notification and LaunchAgent behavior after the app exits. The current Linux notifier writes to stderr.
- [ ] Choose Linux distribution format(s) and validate sandbox access to the keyring, local data, OAuth loopback callback, notifications, and file import/export.
- [ ] Produce versioned, reproducible CLI and desktop artifacts with checksums, tested first launch, upgrade, downgrade/migration policy, and uninstall behavior.
- [ ] Sign and notarize macOS desktop artifacts after live-account acceptance, using maintainer-provided credentials; verify first launch and Gatekeeper behavior. Carry forward the relevant criteria from [#424](https://github.com/gongahkia/hot-cross-buns/issues/424).
- [ ] Decide licensing and security-reporting policy before wider distribution. The checkout has no `LICENSE` or `SECURITY.md`, although [ISSUES.md](ISSUES.md) points security reporters to the latter.
- [ ] Update installation, platform support, OAuth, limitations, and release documentation to describe the Python CLI/TUI and desktop app accurately.

## Optimize performance after measuring it

- [ ] Set user-facing targets for cold start, view switching, search typing, calendar navigation and drag, large-account sync, and memory use on representative Linux and macOS machines.
- [ ] Extend the existing [benchmark](tools/benchmark_python.py) and performance tests with realistic account sizes and end-to-end UI timing; measure repeat runs and tail latency, not just pure calendar geometry.
- [ ] Profile the measured slow paths in SQLite queries, calendar layout and rendering, workspace updates, startup, and Google request handling. Preserve a trace or reproducible fixture for each significant finding.
- [ ] Apply targeted changes such as query/index improvements, incremental updates, viewport rendering, caching, or background work only where profiles justify them; compare results against the baseline.
- [ ] Keep stable performance regression checks in CI and run hardware-specific benchmarks separately before releases.

## Release gate

- [ ] Pass formatting, linting, type checking, tests, builds, installed-app smoke checks, and the relevant local performance budgets on the supported platforms.
- [ ] Complete redacted live-Google acceptance and installed-artifact checks before calling either product release-ready. Document any remaining limitations in release notes.
