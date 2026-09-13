# Hot Cross Buns development checklist

This is the repository's single source of truth for planned work and release readiness. It covers a release-quality Linux and macOS product with a CLI/TUI and windowed desktop apps, with a Windows port to evaluate, in **one repository**. Performance work follows measured user-facing bottlenecks. An unchecked item is work to plan or verify, not a claim that the current implementation is broken. New work, changes in scope, and completed acceptance criteria belong here rather than in a parallel issue roadmap.

The former native-wrapper issues [#423](https://github.com/gongahkia/hot-cross-buns/issues/423) and [#424](https://github.com/gongahkia/hot-cross-buns/issues/424) are superseded by the acceptance and release work below. Their rule that Linux work must wait for macOS acceptance does not apply to this Linux/macOS plan.

## Architecture direction

Keep the local Python domain, accounts, OAuth, SQLite storage and migrations, sync, and application services in a UI-independent shared core. The existing `hcb` CLI/TUI and future desktop apps are separate frontends of that core; the CLI is a frontend, not the backend. A hosted service is not required for this local-first design.

Finish the shared-core and database work, then measure and optimize their slow paths before restoring historical GUI code. Audit the former Swift/Apple, Electron, and Qt frontends for reuse on macOS, Linux, and Windows; choose shared or platform-specific frontends based on working prototypes and maintenance cost. Preserve `hcb` throughout. Keep everything in this repository for now; consider a dedicated core repository only once its API, data compatibility policy, and release cadence are stable enough to version independently.

## First: establish a reliable baseline

- [x] Restore a billing-independent Fedora local baseline with `make local-ci`: format, lint, type check, tests, build, isolated wheel smoke, and terminal mouse smoke. Install the versioned pre-push hook with `make install-hooks` in each clone.
- [x] Document [bring-your-own OAuth setup](docs/oauth-setup.md) for a disposable Google Cloud desktop client and test account: consent screen, test users, API enablement, scopes, and troubleshooting without collecting credentials.
- [ ] Run and record the [live Google acceptance procedure](docs/testing/live-google-tui-smoke.md) with disposable accounts on both Linux and macOS. Cover initial and incremental pull; offline task and event writes; conflict-policy modes; task lists, notes projection, recurrence, search, and bulk mutations. Check the resulting data in Google Tasks and Calendar after each mutation class.
- [ ] Exercise expired tokens, revoked access, network loss, quota/retry, stale ETags, invalid Calendar sync tokens, and restart recovery. Preserve a redacted manual acceptance record; incomplete live acceptance blocks release promotion.
- [ ] Test the CLI/TUI, future desktop app, and optional reminder process running at the same time. Define process-level sync ownership and verify that active outbox deliveries are not mistaken for interrupted ones.
  - [x] Serialize CLI/TUI/reminder sync and explicit recurring-instance refresh per local database with a process-level lock. Verify a second sync leaves an active delivery untouched and a stopped owner's lock is released on Linux.
  - [x] Drain more than 100 pending writes in one explicit sync; `flush_outbox` fetches successive pages until the account's pending outbox is empty.
- [ ] Decide the OAuth distribution model: retain bring-your-own Google client credentials or pursue a project-owned client and its verification process. Confirm requested scopes, token storage, keyring behavior, and onboarding on both platforms.
- [ ] Define the first supported Linux distributions, macOS versions, architectures, and terminal environments; record what is outside the initial release target.
- [ ] Finish the shared-core, SQLite migration, outbox, and multi-process contracts needed by desktop frontends. Verify the CLI/TUI still works with the same accounts, credentials, and database after those changes.

## Optimize the shared core and database before desktop restoration

- [ ] Set user-facing targets for CLI cold start, search, large-account sync, database growth, and memory use on representative Linux and macOS machines.
- [ ] Extend the existing [benchmark](tools/benchmark_python.py) and performance tests with realistic account sizes and service-level timings; measure repeat runs and tail latency, not just pure calendar geometry.
- [ ] Profile the measured slow paths in SQLite queries, workspace updates, startup, and Google request handling. Preserve a trace or reproducible fixture for each significant finding.
- [ ] Apply targeted query/index improvements, incremental updates, caching, or background work only where profiles justify them; compare results against the baseline.
- [ ] Keep stable core performance regression checks in the local check workflow and any future CI; run hardware-specific benchmarks separately before releases.

## Restore and develop desktop frontends on the shared core

- [x] Define a [desktop MVP and CLI/TUI parity map](docs/product/desktop-mvp.md): tasks, calendars, search, capture, editing, sync/conflicts, settings, and reminders, with later features explicitly deferred.
- [x] Establish a UI-independent application interface. Move direct storage and Google calls out of the TUI where necessary so the desktop UI can reuse the same operations without copying business logic.
  - [x] Route pending counts, recurring-instance cache metadata, and conflict listing through `ApplicationService`; count the full outbox rather than the first fetched page.
  - [x] Route account, calendar, Drive, and event lookups through `ApplicationService`; route OAuth, free/busy, sync, and recurring-instance refresh through UI-independent `Runtime` operations with separate worker storage where needed.
- [ ] After shared-core/database work and measured core optimizations, audit Git history for the Swift/Apple project (`apps/apple/` before `e4f6318ae`), Electron app (`src/main/native/` and `src/renderer/` before `90f34603b`), and Qt app (`native/` before `35604ba0d`). Check whether earlier iOS code exists alongside the confirmed macOS Swift target. Record buildability, dependencies, reusable UI/workflows, and the changes each would need for Windows, macOS, and Linux.
- [ ] Restore promising historical frontend source from Git into separate subfolders under `frontends/legacy/` (`apple/`, `electron/`, and `qt/` as applicable) for review, preserving commit provenance and excluding old credentials, generated files, and bundled dependencies. Keep `src/hcb/` and the `hcb` CLI/TUI entry point intact.
- [ ] Prototype the restored candidates against the current Python core with a calendar view, large task list, OAuth callback, and packaging. Choose which frontend to advance for each OS based on compatibility, performance, accessibility, and maintenance cost; use a new toolkit only if the historical options do not fit.
- [ ] Add desktop entry points for the selected ports while preserving the `hcb` CLI/TUI contract. Keep one account model, configuration format, data directory, credential store, and migration path across the frontends.
- [ ] Implement the MVP's keyboard and mouse workflows, accessible focus and labels, loading/error states, offline status, and conflict recovery.
- [ ] Keep long-running sync, search, and calendar layout work responsive in the desktop UI; test cancellation and shutdown during those operations.
- [ ] Once a desktop port runs, measure view switching, search typing, calendar navigation and drag, rendering, and memory use; optimize measured UI bottlenecks and set regression budgets for each supported frontend.
- [ ] Validate the chosen macOS and Linux ports against the [desktop MVP](docs/product/desktop-mvp.md). Evaluate a Windows port with the same shared-core and CLI compatibility checks before setting its release target.

## Make Linux and macOS releases dependable

- [ ] Add independently run Linux and macOS CI and installed-artifact smoke coverage, including supported architectures and credential-free checks. The local pre-push gate does not publish a remote check status. Keep platform-specific manual acceptance documented.
- [ ] Provide desktop reminders and background scheduling on Linux; verify macOS notification and LaunchAgent behavior after the app exits. The current Linux notifier writes to stderr.
- [ ] Choose Linux distribution format(s) and validate sandbox access to the keyring, local data, OAuth loopback callback, notifications, and file import/export.
- [ ] Produce versioned, deterministic CLI and desktop artifacts for supported architectures, with checksums and installed-app smoke checks. Verify first launch, download/update, upgrade, downgrade/migration policy, and uninstall/cache behavior.
- [ ] Prepare macOS signing and notarization without embedded credentials; sign and notarize only with maintainer-provided Apple credentials after live-account acceptance. Verify Gatekeeper, Keychain credentials, and the localhost OAuth callback in the installed app.
- [ ] Decide licensing and security-reporting policy before wider distribution. The checkout has no `LICENSE` or `SECURITY.md`.
- [ ] Update installation, platform support, OAuth, limitations, and release documentation to describe the Python CLI/TUI and desktop app accurately. Release notes must state bring-your-own OAuth requirements, supported Google functionality, conflict behavior, and remaining limitations.

## Release gate

- [ ] Pass formatting, linting, type checking, tests, builds, installed-app smoke checks, and the relevant local performance budgets on the supported platforms.
- [ ] Complete redacted live-Google acceptance and installed-artifact checks before calling either product release-ready. Document any remaining limitations in release notes.
