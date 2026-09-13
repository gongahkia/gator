# Hot Cross Buns development checklist

This is the repository's single source of truth for planned work and release readiness. It covers a release-quality Linux and macOS product with a CLI/TUI and a windowed desktop app in **one repository**. Performance work follows measured user-facing bottlenecks. An unchecked item is work to plan or verify, not a claim that the current implementation is broken. New work, changes in scope, and completed acceptance criteria belong here rather than in a parallel issue roadmap.

The former native-wrapper issues [#423](https://github.com/gongahkia/hot-cross-buns/issues/423) and [#424](https://github.com/gongahkia/hot-cross-buns/issues/424) are superseded by the acceptance and release work below. Their rule that Linux work must wait for macOS acceptance does not apply to this Linux/macOS plan.

## Architecture direction

Keep the local Python domain, accounts, OAuth, SQLite storage and migrations, sync, and application services in a UI-independent shared core. The existing `hcb` CLI/TUI and a desktop app are separate frontends of that core; the CLI is a frontend, not the backend. A hosted service is not required for this local-first design.

Start with one cross-platform Linux/macOS desktop frontend and small platform-specific integrations for credentials, notifications, and packaging. A Windows frontend can use the same core later if platform support is prioritized and verified. Add separate OS-specific frontends only when a concrete product or technical need justifies them. Keep everything in this repository for now; consider a dedicated core repository only once its API, data compatibility policy, and release cadence are stable enough to version independently.

## First: establish a reliable baseline

- [x] Restore a billing-independent Fedora local baseline with `make local-ci`: format, lint, type check, tests, build, isolated wheel smoke, and terminal mouse smoke. Install the versioned pre-push hook with `make install-hooks` in each clone.
- [x] Document [bring-your-own OAuth setup](docs/oauth-setup.md) for a disposable Google Cloud desktop client and test account: consent screen, test users, API enablement, scopes, and troubleshooting without collecting credentials.
- [ ] Run and record the [live Google acceptance procedure](docs/testing/live-google-tui-smoke.md) with disposable accounts on both Linux and macOS. Cover initial and incremental pull; offline task and event writes; conflict-policy modes; task lists, notes projection, recurrence, search, and bulk mutations. Check the resulting data in Google Tasks and Calendar after each mutation class.
- [ ] Exercise expired tokens, revoked access, network loss, quota/retry, stale ETags, invalid Calendar sync tokens, and restart recovery. Preserve a redacted manual acceptance record; incomplete live acceptance blocks release promotion.
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

- [ ] Add independently run Linux and macOS CI and installed-artifact smoke coverage, including supported architectures and credential-free checks. The local pre-push gate does not publish a remote check status. Keep platform-specific manual acceptance documented.
- [ ] Provide desktop reminders and background scheduling on Linux; verify macOS notification and LaunchAgent behavior after the app exits. The current Linux notifier writes to stderr.
- [ ] Choose Linux distribution format(s) and validate sandbox access to the keyring, local data, OAuth loopback callback, notifications, and file import/export.
- [ ] Produce versioned, deterministic CLI and desktop artifacts for supported architectures, with checksums and installed-app smoke checks. Verify first launch, download/update, upgrade, downgrade/migration policy, and uninstall/cache behavior.
- [ ] Prepare macOS signing and notarization without embedded credentials; sign and notarize only with maintainer-provided Apple credentials after live-account acceptance. Verify Gatekeeper, Keychain credentials, and the localhost OAuth callback in the installed app.
- [ ] Decide licensing and security-reporting policy before wider distribution. The checkout has no `LICENSE` or `SECURITY.md`.
- [ ] Update installation, platform support, OAuth, limitations, and release documentation to describe the Python CLI/TUI and desktop app accurately. Release notes must state bring-your-own OAuth requirements, supported Google functionality, conflict behavior, and remaining limitations.

## Optimize performance after measuring it

- [ ] Set user-facing targets for cold start, view switching, search typing, calendar navigation and drag, large-account sync, and memory use on representative Linux and macOS machines.
- [ ] Extend the existing [benchmark](tools/benchmark_python.py) and performance tests with realistic account sizes and end-to-end UI timing; measure repeat runs and tail latency, not just pure calendar geometry.
- [ ] Profile the measured slow paths in SQLite queries, calendar layout and rendering, workspace updates, startup, and Google request handling. Preserve a trace or reproducible fixture for each significant finding.
- [ ] Apply targeted changes such as query/index improvements, incremental updates, viewport rendering, caching, or background work only where profiles justify them; compare results against the baseline.
- [ ] Keep stable performance regression checks in CI and run hardware-specific benchmarks separately before releases.

## Release gate

- [ ] Pass formatting, linting, type checking, tests, builds, installed-app smoke checks, and the relevant local performance budgets on the supported platforms.
- [ ] Complete redacted live-Google acceptance and installed-artifact checks before calling either product release-ready. Document any remaining limitations in release notes.
