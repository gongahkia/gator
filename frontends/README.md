# Desktop frontend candidates

These source trees were restored from Git history and are **not active entry
points**. The current `hcb` CLI/TUI and Python core remain under `src/hcb/`.
Do not run a restored frontend against the current HCB database or credential
file: all three snapshots contain their own storage, OAuth, and sync assumptions
and have not been adapted to the Python application interface.

The target layout keeps the Swift frontend in `macos/`, the Qt candidate in
`qt/` for Linux and a later Windows port, and the incomplete Electron candidate
in `electron/` as a fallback. The Python core remains the sole owner of account
state, Google access, and SQLite. A small local bridge must connect each desktop
frontend to it before shared-account use. Work on macOS and Linux first; defer
Windows validation and release decisions until those ports work.
The current Python wheel builds only `src/hcb/`, and its explicit source
distribution list excludes `frontends/`; relocating these candidates does not
change the installed CLI package.

| Snapshot | Source commit | Restored material | Assessment on Fedora 43 |
| --- | --- | --- | --- |
| [`macos/`](macos/) | `cc1e7f01abdf0e8bf0d42e3bcbc0d8fce51df5eb` | `apps/apple/` Xcode project, Swift source, tests, and source assets | The project defines a macOS app, macOS tests, and a share extension with macOS 14 deployment target. No iOS/iPadOS target was found in this commit. Xcode build and runtime behavior cannot be verified on Fedora. |
| [`electron/`](electron/) | `6a98dc8dadba0775f9e09056d6595661b7f06a50` | React renderer, platform adapters, package/build configuration | The original package declared Electron 33, React 18, Node 20+, and Linux/macOS/Windows adapters. This review snapshot omits the old main-process backend, preload, dependency lockfile, and release scripts, so it is not a standalone build. |
| [`qt/`](qt/) | `24dde3d9bb6b6c956a03634c6de3c0356e3d83d9` | CMake project, native C++/QML source, tests, benchmarks, and source brand assets | CMake configures and the app builds on Fedora with Qt 6.10.3 and SQLite 3.50.2 in system-dependency mode. The repaired timeline test and full-font offscreen smoke pass. A read-only Python-bridge client is tested but not wired into the historical controller; installed-package and current-core integration checks remain open. |

The source commits are the parents of the changes that removed each frontend:
`e4f6318ae` (Apple), `90f34603b` (Electron), and `35604ba0d` (Qt).
The historical `apps/apple/Configuration/GoogleOAuth.xcconfig` was deliberately
excluded because it contained non-placeholder values. A new blank
`macos/Configuration/GoogleOAuth.xcconfig` restores the Xcode project reference
and optionally includes an ignored local override. One client-ID-shaped literal
in an Apple release-gate test was
replaced with an explicitly synthetic value. No generated build outputs, dependency directories, private keys,
or token files were restored. A filename and content scan of these snapshots
found no Google client secret or private key patterns; test fixtures may contain
synthetic token strings.

## Reuse assessment

The Swift app has polished macOS-specific calendar, search, quick capture,
settings, and accessibility code, but its `AppModel` and services are tied to
their own Google and persistence stack. Reusing views needs an explicit adapter
to the current Python application interface and a macOS build/test pass. The
project file does not support an iOS port as restored.
`HotCrossBunsApp` constructs `AppModel.bootstrap()`, which builds its own
`GoogleAuthService`, Tasks/Calendar clients, `SyncScheduler`, and `LocalCacheStore`.
The `CalendarStore` and `TaskStore` snapshot surfaces are a possible adapter
seam, but the model's mutation methods also call the old services directly.

The Electron renderer offers a cross-platform React UI with platform-specific
native adapters and a virtualized task list. Its TypeScript view-model and IPC
types describe the former backend. A prototype must define a narrow local IPC
boundary to Python, replace the former Google/storage calls, and measure startup,
memory, and calendar rendering on packaged builds.
`App.tsx` mounts `CoreDataProvider`, whose view-model source imports
`@shared/ipc/contracts`. That contract and the main-process/preload backend are
absent from this review snapshot, so renderer reuse starts by rebuilding a
minimal read-only provider and IPC contract rather than trying to run the old
package scripts unchanged.

The Qt tree has the broadest self-contained native snapshot: C++ services, QML
views, packaging, and benchmark sources. It also has a separate C++ SQLite and
OAuth implementation. Its old backend must be replaced or isolated behind a
bridge to the current Python core before it can share accounts or data with
the CLI. Reuse the QML calendar and task flows first in a small adapter prototype.
The entry point gives QML an `AppController`; its constructor owns the old
SQLite, Google, OAuth, sync, and mutation services. A Python-core prototype
should provide account-scoped task/calendar snapshots and mutations behind that
controller while letting QML models render data. It must launch a bounded local
bridge or embedded core process without opening the Python SQLite database from
Qt. `ApplicationService.workspace()` supplies a local snapshot and `Runtime`
owns sync and OAuth, but neither is yet a versioned cross-process protocol.

A Fedora 43 system-dependency test build passed for the calendar layout, month
grid, timeline, task model, task tree view, local search, and QML test targets.
With `QT_QPA_PLATFORM=offscreen`, six of those seven CTest targets passed. The
timeline target failed `normalizesMultiDayTimedRanges`: it returned
`2026-08-04T16:00:00.000Z` for an expected `2026-08-05T04:00:00.000Z` end.
[Inference] `TimelineModel::timedRangeInput` supplies
`lastMinute % kMinutesPerDay` to `QTime`'s minute argument, where a value
modulo 60 is needed. Validate and fix this in the active Qt adapter prototype;
the historical source remains unchanged.

Earlier offscreen launches of the built app with `HCB_BENCHMARK_EXIT_AFTER_LOAD=1`
timed out after 20 and 35 seconds. The QML test target passed. A later isolated
run traced the slow startup to font enumeration; see the verification below.

After relocation, all 932 files tracked in the three source snapshots and
audit were present in their new paths. Content was unchanged for 925 files;
the seven edited files are this audit and macOS source/test documentation paths.
The 57 Swift test source paths checked under `frontends/macos/` exist. The Qt
native target and all seven selected test executables built from `frontends/qt/`;
the same six tests passed and the same timeline assertion failed. The moved
offscreen app still timed out after 15 seconds. `make local-ci` passed the
Python CLI/TUI gate with 373 tests, wheel smoke, and terminal mouse smoke.
There is no Xcode toolchain on this Fedora host, so the macOS build and tests
remain unverified. The Electron snapshot still lacks its main, preload, and
shared contract entry points and was not built.

## Fedora verification, 2026-09-14

Verification used a clean CMake/Ninja build under `/tmp`, empty private XDG
config/data/cache directories, and `env -i` so the historical Qt app could not
inherit account credentials or open the active HCB database. No live Google
account was used.

- **Qt:** `hcb_native` and eight selected test executables built. With the
  system font inventory, six of eight CTest targets passed, the timeline target
  reproduced the same `normalizesMultiDayTimedRanges` failure, and both QML
  shell smoke cases exceeded their 15-second process timeout. GDB interrupts
  sampled the main thread inside `AppController::availableFontFamilies` and
  `QFontMetrics::inFont` while QML loaded. This host has about 1,178 font-family
  entries. With a temporary Fontconfig file exposing one font, seven of eight
  selected targets passed; the shell smoke passed in 0.74 seconds, and a normal
  initialization launch exited after its one-second idle-RSS timer with status
  zero in 1.35 seconds. It created only an isolated SQLite file. [Inference]
  Filtering every installed font synchronously during QML creation is the
  startup bottleneck on this host. The timeline failure remains a separate code
  defect. These checks do not establish a usable desktop app or current-core
  integration.
- **Electron:** Node 22 and pnpm are present, but this snapshot has no
  `node_modules` or lockfile. `pnpm run typecheck` stopped because `tsc` was not
  installed. The configured `src/main/index.ts` and `src/preload/index.ts`
  entrypoints are absent, as is `src/shared/ipc/contracts.ts`, which the native
  adapter and renderer tests import. Build, tests, and runtime remain unverified;
  the restored package cannot be built as configured without source restoration
  or replacement and dependency installation.
- **macOS Swift:** The Xcode project and scheme are present, and seven JSON,
  xcstrings, or Package.resolved files, two plists, and the scheme parsed
  successfully. The committed OAuth xcconfig is blank. `HotCrossBunsApp` still
  bootstraps its old Google, sync, and cache services. This Fedora host has no `xcodebuild`,
  `xcrun`, `swift`, or `swiftc`, so compilation, unit tests, launch, and packaging
  were not run. A macOS host is required for those checks.

Fedora source-build check (without modifying the current Python install):

```sh
cmake -S frontends/qt -B /tmp/hcb-qt-build \
  -DHCB_NATIVE_DEPENDENCY_MODE=system -DBUILD_TESTING=OFF
cmake --build /tmp/hcb-qt-build --target hcb_native -j4
```

Focused historical test command (currently six of seven targets pass):

```sh
cmake -S frontends/qt -B /tmp/hcb-qt-tests \
  -DHCB_NATIVE_DEPENDENCY_MODE=system -DBUILD_TESTING=ON
cmake --build /tmp/hcb-qt-tests --target \
  hcb_calendar_layout_engine_tests hcb_month_grid_model_tests \
  hcb_timeline_model_tests hcb_task_model_tests hcb_task_tree_view_tests \
  hcb_local_search_service_tests hcb_qml_tests -j4
QT_QPA_PLATFORM=offscreen ctest --test-dir /tmp/hcb-qt-tests \
  --output-on-failure \
  -R '^hcb_(calendar_layout_engine|month_grid_model|timeline_model|task_model|task_tree_view|local_search_service|qml)_tests$'
```

No port has passed a current-core integration, OAuth callback, accessibility,
large-account, or installed-package test. Choose a production frontend only
after those prototypes and platform measurements.

## Fedora bridge and repair verification, 2026-09-14

The initial Qt audit above accurately records the restored historical source
before adaptation. The following follow-up used the same credential-free,
full-host-font environment and a clean CMake build at `/tmp/hcb-qt-tests`.

- **Timeline and launch:** `TimelineModel::timedRangeInput` now supplies the
  `QTime` minute field modulo 60. All ten timeline tests pass. The settings
  font selector now enumerates installed families without testing glyph support
  for every family while QML is being created; validation still tests the single
  font a user chooses. The Qt QML shell smoke passed with the normal host font
  inventory in 6.41 seconds. A clean direct offscreen launch with
  `HCB_BENCHMARK_EXIT_AFTER_LOAD=1` exited zero in 2.79 seconds at 144,052 KiB
  maximum RSS. These observations are Fedora-specific and do not establish
  packaged-app or current-core behavior.
- **Python bridge client:** `src/core/PythonBridgeClient` validates an owner-only
  loopback descriptor and supports asynchronous workspace-summary, task-page,
  and date-bounded-event reads with a timeout and cancellation. Its eight Qt
  tests cover descriptor privacy, endpoint/token validation, request shape,
  error handling, and invalid inputs. `PythonBridgeProjection` has five more
  tests that map v1 JSON into existing task-list, calendar, task, and event
  model records while rejecting cross-account or malformed data. Both are
  intentionally unconnected to `AppController`, so they cannot mix the
  historical Qt SQLite/Google state with the Python-owned HCB account.

Focused follow-up command:

```sh
cmake -S frontends/qt -B /tmp/hcb-qt-tests \
  -DHCB_NATIVE_DEPENDENCY_MODE=system -DBUILD_TESTING=ON
cmake --build /tmp/hcb-qt-tests --target \
  hcb_python_bridge_client_tests hcb_timeline_model_tests \
  hcb_native hcb_native_qml_shell_smoke -j4
QT_QPA_PLATFORM=offscreen ctest --test-dir /tmp/hcb-qt-tests \
  --output-on-failure \
  -R '^(hcb_python_bridge_client_tests|hcb_timeline_model_tests|hcb_native_qml_shell_smoke)$'
```
