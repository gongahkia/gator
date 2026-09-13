# Historical desktop frontends

These are review snapshots from Git history, not active entry points. The current
`hcb` CLI/TUI and Python core remain under `src/hcb/`. Do not run an old frontend
against the current HCB database or credential file: all three snapshots contain
their own storage, OAuth, and sync assumptions and have not been adapted to the
Python application interface.

| Snapshot | Source commit | Restored material | Assessment on Fedora 43 |
| --- | --- | --- | --- |
| `apple/` | `cc1e7f01abdf0e8bf0d42e3bcbc0d8fce51df5eb` | `apps/apple/` Xcode project, Swift source, tests, and source assets | The project defines a macOS app, macOS tests, and a share extension with macOS 14 deployment target. No iOS/iPadOS target was found in this commit. Xcode build and runtime behavior cannot be verified on Fedora. |
| `electron/` | `6a98dc8dadba0775f9e09056d6595661b7f06a50` | React renderer, platform adapters, package/build configuration | The original package declared Electron 33, React 18, Node 20+, and Linux/macOS/Windows adapters. This review snapshot omits the old main-process backend, preload, dependency lockfile, and release scripts, so it is not a standalone build. |
| `qt/` | `24dde3d9bb6b6c956a03634c6de3c0356e3d83d9` | CMake project, native C++/QML source, tests, benchmarks, and source brand assets | CMake configures and `hcb_native` builds on Fedora with Qt 6.10.3 and SQLite 3.50.2 in system-dependency mode. The old project declares Linux, macOS, and Windows adapters; runtime and installed-package checks remain open. |

The source commits are the parents of the changes that removed each frontend:
`e4f6318ae` (Apple), `90f34603b` (Electron), and `35604ba0d` (Qt).
`apple/Configuration/GoogleOAuth.xcconfig` was deliberately excluded because
the historical file contained non-placeholder values; only the example config
is retained. One client-ID-shaped literal in an Apple release-gate test was
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

The Electron renderer offers a cross-platform React UI with platform-specific
native adapters and a virtualized task list. Its TypeScript view-model and IPC
types describe the former backend. A prototype must define a narrow local IPC
boundary to Python, replace the former Google/storage calls, and measure startup,
memory, and calendar rendering on packaged builds.

The Qt tree has the broadest self-contained native snapshot: C++ services, QML
views, packaging, and benchmark sources. It also has a separate C++ SQLite and
OAuth implementation. Its old backend must be replaced or isolated behind a
bridge to the current Python core before it can share accounts or data with
the CLI. Reuse the QML calendar and task flows first in a small adapter prototype.

Fedora source-build check (without modifying the current Python install):

```sh
cmake -S frontends/legacy/qt -B /tmp/hcb-legacy-qt-build \
  -DHCB_NATIVE_DEPENDENCY_MODE=system -DBUILD_TESTING=OFF
cmake --build /tmp/hcb-legacy-qt-build --target hcb_native -j4
```

No port has passed a current-core integration, OAuth callback, accessibility,
large-account, or installed-package test. Choose a production frontend only
after those prototypes and platform measurements.
