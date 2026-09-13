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
  - [x] Complete the [Fedora read-only preflight](docs/testing/live-google-tui-smoke.md#read-only-preflight) with the connected account. Keep all Google writes, revocation, and reset out of scope for this account.
  - [x] Complete the [Fedora disposable-account basic write smoke](docs/testing/live-google-tui-smoke.md#disposable-account-fedora-partial-smoke): verify account identity and isolated credentials, task-list/calendar create-delete and task/event create-edit-delete through Google readback, offline task delivery after restart, repeat sync, and cleanup. The full live procedure remains open.
  - [x] Complete the [Fedora disposable-account conflict and crash follow-up](docs/testing/live-google-tui-smoke.md#disposable-account-fedora-follow-up-partial): exercise Ask, Prefer Google, and Prefer Local with synthetic stale Calendar ETags; verify both uncertain-create outcomes and later queued edits against Google; remove fixtures. CLI/TUI interaction and the full live procedure remain open.
  - [x] Extend disposable-account Fedora CLI/API acceptance to managed Task recurrence, all three Notes projections, bulk completion, rich timed/all-day Calendar fields, changed/cancelled instances, explicit instance refresh, and free/busy. Verify Google readback and synthetic fixture cleanup; see the [redacted follow-up](docs/testing/live-google-tui-smoke.md#disposable-account-fedora-follow-up-partial). Interactive TUI, external invitation/RSVP, Meet/Drive attachment, scheduler notification, reset, and macOS checks remain open.
  - [x] Check Task parent/sibling insert, reparent, reorder, cross-list move, and list rename on two disposable lists. Send Google `parent`/`previous` insert parameters, translate pulled parent IDs back to local IDs after all pages, and ignore source-list tombstones for a task now in the destination. Verify one remote task ID and fixture cleanup.
  - [x] Move an attendee-free default event between two disposable calendars, verify Google destination/source readback and a stable local record after repeat sync, and remove both fixtures.
  - [x] Create an attendee-free Meet event on a disposable calendar, verify Google's successful conference request and video entry point, repeat sync, and fixture cleanup.
  - [x] Inject loss of a Calendar event-create response after Google accepts the client-assigned ID, then verify restart reconciliation leaves one active Google event, an empty outbox, and no conflict; remove the fixture.
  - [x] Mount the TUI headlessly on the isolated disposable cache after live sync and verify cached rows render with no pending write. Physical terminal interaction and simultaneous process checks remain open.
- [ ] Exercise expired tokens, revoked access, network loss, quota/retry, stale ETags, invalid Calendar sync tokens, and restart recovery. Preserve a redacted manual acceptance record; incomplete live acceptance blocks release promotion.
  - [x] Keep the original sync token on every page of paginated incremental Calendar list and event pulls; cover the request arguments and final cursor with fake-gateway tests.
  - [x] Distinguish malformed Calendar token `400` from expired-token `410`; test a per-calendar `410` with real disposable-account reads and an interrupted fake-gateway multi-page resume. Keep surviving events' local IDs so task-event links and reminders remain valid, and prune missing events only after final-page commit.
  - [x] Make stale-ETag conflict policy configurable as Ask, Prefer Google, or Prefer Local, and pause later same-entity writes until an Ask conflict is resolved. Verify Google readback for each mode and preserve retry order for later edits.
  - [x] Keep notes entered in Disabled projection local-only across Google pulls and a later mode switch, using a migrated private-note record; clear it after a subsequent Google write includes the note. Verify mixed-mode disposable-account readback and local persistence.
- [ ] Test the CLI/TUI, future desktop app, and optional reminder process running at the same time. Define process-level sync ownership and verify that active outbox deliveries are not mistaken for interrupted ones.
  - [x] Serialize CLI/TUI/reminder sync and explicit recurring-instance refresh per local database with a process-level lock. Verify a second sync leaves an active delivery untouched and a stopped owner's lock is released on Linux.
  - [x] Drain more than 100 pending writes in one explicit sync; `flush_outbox` fetches successive pages until the account's pending outbox is empty.
  - [x] Make schema migrations atomic across concurrent database opens, including rollback after a failed migration and bounded WAL startup contention.
  - [x] Pause account sends after an uncertain create, preserve later queued edits and their dirty state, and restore the original queue position on verified retry. Exercise abrupt process exit with a fake gateway.
- [ ] Decide the OAuth distribution model: retain bring-your-own Google client credentials or pursue a project-owned client and its verification process. Confirm requested scopes, token storage, keyring behavior, and onboarding on both platforms.
- [ ] Define the first supported Linux distributions, macOS versions, architectures, and terminal environments; record what is outside the initial release target.
- [ ] Finish the shared-core, SQLite migration, outbox, and multi-process contracts needed by desktop frontends. Verify the CLI/TUI still works with the same accounts, credentials, and database after those changes.

## Optimize the shared core and database before desktop restoration

- [ ] Set user-facing targets for CLI cold start, search, large-account sync, database growth, and memory use on representative Linux and macOS machines.
- [x] Extend the existing [benchmark](tools/benchmark_python.py) and performance tests with realistic account sizes and service-level timings; measure repeat runs and tail latency, not just pure calendar geometry. Preserve the [initial local baseline](docs/testing/performance-baseline.md).
- [ ] Profile the measured slow paths in SQLite queries, workspace updates, startup, and Google request handling. Preserve a trace or reproducible fixture for each significant finding.
  - [x] Profile large-account workspace loading; record the SQLite-versus-hydration split and before/after results in the [workspace profile](docs/testing/workspace-profile.md).
  - [x] Profile 250, 1,000, and 5,000 queued writes; record the repeated outbox scan and before/after samples in the [outbox profile](docs/testing/outbox-profile.md).
  - [x] Profile synthetic TUI startup and first task frame at 1,000 and 10,000 tasks; retain repeated samples and memory measurements in the [startup profile](docs/testing/tui-startup-profile.md).
  - [x] Profile synthetic paginated Tasks and Calendar pulls at 1,000/200 and 10,000/2,000 rows; record repeat timings, request counts, page-application time, memory, and restart checks in the [pull profile](docs/testing/pull-profile.md). Live Google request latency remains open.
  - [x] Measure read-only live Tasks and Calendar page latency in an isolated database, with private numeric reports and a redacted [live pull profile](docs/testing/live-pull-profile.md). No remote writes or Drive account-data requests were made.
- [ ] Apply targeted query/index improvements, incremental updates, caching, or background work only where profiles justify them; compare results against the baseline.
  - [x] Skip JSON decoding for canonical empty event arrays during workspace hydration; preserve decoding of nonempty event fields and compare repeat timings.
  - [x] Fetch each sending outbox mutation by indexed ID and account instead of rescanning pending rows; verify delivery semantics and compare queue-size scaling.
  - [x] Skip URL copying and scanning for ordinary task rows without web links; preserve linked titles and Notes previews, and compare row-building and startup samples.
  - [x] Skip unchanged task-list upserts during pull so an empty incremental sync does not rebuild every child task search row; preserve changed list metadata and compare repeated timings.
  - [x] Request 100 Tasks and 1,000 Calendar events per page, reducing live initial pull requests and comparing stage timings on the same read-only account.
  - [x] Index non-null Task parent references for post-pull local-ID normalization; a five-run 10,000-task fake-gateway benchmark returned empty incremental Tasks pull median from about 13 ms to 1.1 ms.
  - [x] Bound independent first-page Tasks and Calendar reads to four outstanding requests, use a separate HTTP transport per worker, serialize SQLite page application, and verify retry, cancellation, `410`, and restart behavior. Compare one-, two-, and four-worker read-only live samples in the [live pull profile](docs/testing/live-pull-profile.md).
- [ ] Keep stable core performance regression checks in the local check workflow and any future CI; run hardware-specific benchmarks separately before releases.
  - [x] Re-run medium/large shared-core, synthetic paginated-pull, and TUI startup benchmarks on Fedora; retain [follow-up samples](docs/testing/performance-baseline.md#fedora-follow-up-after-conflict-and-sync-hardening). Cold-start budgets and macOS measurements remain open.

## Restore and develop desktop frontends on the shared core

- [x] Define a [desktop MVP and CLI/TUI parity map](docs/product/desktop-mvp.md): tasks, calendars, search, capture, editing, sync/conflicts, settings, and reminders, with later features explicitly deferred.
- [x] Establish a UI-independent application interface. Move direct storage and Google calls out of the TUI where necessary so the desktop UI can reuse the same operations without copying business logic.
  - [x] Route pending counts, recurring-instance cache metadata, and conflict listing through `ApplicationService`; count the full outbox rather than the first fetched page.
  - [x] Route account, calendar, Drive, and event lookups through `ApplicationService`; route OAuth, free/busy, sync, and recurring-instance refresh through UI-independent `Runtime` operations with separate worker storage where needed.
- [x] Audit Git history for the Swift/Apple project (`apps/apple/` before `e4f6318ae`), Electron app (`src/main/native/` and `src/renderer/` before `90f34603b`), and Qt app (`native/` before `35604ba0d`). The [audit](frontends/legacy/README.md) records targets, dependencies, buildability, reusable workflows, and integration work; the confirmed Swift project has macOS targets but no iOS target.
- [x] Restore historical Apple, Electron, and Qt review snapshots under [`frontends/legacy/`](frontends/legacy/README.md), retaining commit provenance and excluding the historical non-placeholder Apple OAuth config, generated outputs, and bundled dependencies. Keep `src/hcb/` and the `hcb` CLI/TUI entry point intact.
- [ ] Prototype the restored candidates against the current Python core with a calendar view, large task list, OAuth callback, and packaging. Choose which frontend to advance for each OS based on compatibility, performance, accessibility, and maintenance cost; use a new toolkit only if the historical options do not fit.
  - [ ] Start with a Qt read-only task/calendar adapter to `ApplicationService.workspace()` and a bounded local bridge; keep Qt off the Python SQLite and credential files. The historical Qt build passed on Fedora, but one of seven focused UI/model test targets fails on multi-day timed range normalization. Its offscreen exit-after-load smoke timed out twice without diagnostics. Diagnose startup and fix/retest the timeline defect in the active prototype.
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
