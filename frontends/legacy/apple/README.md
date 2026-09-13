# Apple App

Hot Cross Buns macOS app: a native SwiftUI client for Google Tasks and Google Calendar.

## Direction

- Google Tasks and Google Calendar are the source of truth.
- Local storage is cache, settings, sync checkpoints, and pending offline mutations.
- Google Drive is out of scope.
- Mac-only. iOS/iPadOS targets have been removed from this project.

## Requirements

- Xcode 15+ or newer
- XcodeGen 2.45+
- macOS 14+ target

## Generate The Project

```bash
cd apps/apple
xcodegen generate
open HotCrossBuns.xcodeproj
```

## Build From CLI

```bash
xcodebuild -project HotCrossBuns.xcodeproj -scheme HotCrossBunsMac -destination 'platform=macOS' build CODE_SIGNING_ALLOWED=NO
xcodebuild -project HotCrossBuns.xcodeproj -scheme HotCrossBunsMac -destination 'platform=macOS' test CODE_SIGNING_ALLOWED=NO
```

## Package macOS DMG

```bash
../../scripts/package-macos-dmg.sh
```

The script creates an unsigned DMG under `build/apple/` by default. If `CODE_SIGN_IDENTITY` is set, it signs the app bundle and DMG. If `NOTARIZE=1` is also set, it submits the DMG with `xcrun notarytool` using `APPLE_ID`, `APPLE_TEAM_ID`, and `APP_SPECIFIC_PASSWORD`, then staples the result.

## Google Integration

The app uses the native Google Sign-In SDK and requests these Google scopes during sign-in:

- `https://www.googleapis.com/auth/tasks`
- `https://www.googleapis.com/auth/calendar`

Hot Cross Buns does not run a hosted OAuth proxy. Users can connect a downloaded DMG through their own Google Cloud project by creating a `Desktop app` OAuth client, then pasting that client ID and optional client secret into the in-app Google OAuth client setup card.

There are two supported Google OAuth paths.

### Path A: Downloaded DMG + Runtime Desktop OAuth

Setup checklist:

1. Create a Google Cloud project.
2. Enable the Google Tasks API and Google Calendar API.
3. Configure the Google Auth platform / OAuth consent screen. For personal Gmail use, choose `External` and add yourself as a test user while setting up.
4. Create a `Desktop app` OAuth client.
5. Paste the desktop client ID and optional client secret into Hot Cross Buns.
6. Click Connect Google and finish the browser consent flow.

For ongoing personal use, publish the OAuth app to `In production` after setup. Google's `Testing` publishing status issues refresh tokens that expire after 7 days for the Tasks and Calendar scopes. An unverified personal production client may still show Google's warning screen, but it avoids using the maintainer's Google Cloud project and avoids the testing-mode seven-day refresh-token expiry.

### Path B: Source Build + Embedded Native Google Sign-In

Source builds can also embed a native Google Sign-In client. Create a Google Cloud OAuth client for iOS/macOS, then provide these build settings locally:

- `GOOGLE_MACOS_CLIENT_ID`
- `GOOGLE_MACOS_REVERSED_CLIENT_ID`

The committed defaults are intentionally blank in `Configuration/GoogleOAuth.xcconfig`, and that file optionally includes the ignored `Configuration/GoogleOAuth.local.xcconfig` when present. Use `Configuration/GoogleOAuth.example.xcconfig` as the template for your local override and do not commit real OAuth client IDs. You can also pass values directly to `xcodebuild` if you prefer.

Public DMGs are packaged locally. They can ship without embedded OAuth values because users can add a Desktop OAuth client at runtime. Do not upload a public DMG with a private personal embedded OAuth client unless you intend to complete Google's verification path for that client.

```bash
xcodebuild \
  -project HotCrossBuns.xcodeproj \
  -scheme HotCrossBunsMac \
  -destination 'platform=macOS' \
  GOOGLE_MACOS_CLIENT_ID='your-client-id.apps.googleusercontent.com' \
  GOOGLE_MACOS_REVERSED_CLIENT_ID='com.googleusercontent.apps.your-reversed-client-id' \
  build
```

## Mac Shell

The app uses a NavigationSplitView sidebar shell with a full CommandMenu (`⌘N` new task, `⌘⇧N` new event, `⌘R` refresh, `⌘⇧R` force full resync, `⌘F` search, `⌘1…⌘5` section switching) and window-state restoration. A menu-bar extra and dock overdue-count badge are available as optional user settings.

## Local Cache

The current cache is a SQLite database in Application Support under `~/Library/Application Support/com.gongahkia.hotcrossbuns.mac/`. It preserves account metadata, task/calendar mirrors, sync checkpoints, pending mutation placeholders, and user settings so launch does not depend on an immediate Google round trip. Existing `cache-state.json` and `cache-events.json` sidecars are migrated into the database on first load, then kept as legacy fallback inputs.

OAuth tokens live in the macOS Keychain. Optional cache encryption uses AES-256-GCM for the local cache files only; Google remains the source of truth for synced tasks and events. The app does not ship a third-party analytics SDK or cloud crash reporter.

## Onboarding

First launch presents a setup flow for Google Sign-In, sync mode, task-list/calendar selection, and local reminders. The setup state is persisted in the local cache and can be reset from Settings.

## Search

The Search section queries the local cache for synced tasks and calendar events by title, notes/details, source list/calendar, and status. Search is intentionally local-first so it remains fast and does not spend Google API quota per keystroke. Tasks and events are also indexed in Spotlight for system-wide Mac search.

## Error UX

Sync and Google connection failures surface in a global banner with dismiss and retry actions. Google API status codes are translated into user-actionable messages where possible.

## Diagnostics And Recovery

Settings includes a Diagnostics and Recovery sheet for daily-use support. It shows account, sync, cache, selection, checkpoint, and pending-write state; can copy a diagnostic summary; can refresh immediately; can force a full resync by clearing checkpoints; and can clear cached Google data on this device before reloading from Google. These controls do not delete data from Google.

## Sync

Manual refresh performs authenticated read-sync against Google Tasks task lists/tasks and Google Calendar calendar lists/events. Initial sync performs full reads for selected resources; later syncs use Google Tasks `updatedMin` checkpoints and Google Calendar `nextSyncToken` checkpoints. Tombstones are purged from the local cache after each successful sync. Near-real-time polling uses jittered exponential backoff on `429` and `5xx` responses.

Sync modes:

- Manual only syncs when the user taps refresh.
- Balanced syncs after launch/restore and when the scene becomes active.
- Near real-time does the balanced behavior plus foreground polling every 90 seconds with backoff.

Settings persist selected calendars and selected task lists. Empty selections are respected after the user has configured them, rather than falling back to Google defaults.

## Local Notifications

Local reminders are opt-in from Settings. When enabled, the app requests notification permission and schedules up to 64 pending device-local notifications for incomplete due tasks and upcoming non-cancelled Calendar events. Task reminders fire at 9:00 AM on the due date; timed event reminders fire 15 minutes before start; all-day event reminders fire at 9:00 AM.

## App Intents

The app exposes App Shortcuts for opening the task editor, opening the event editor, and opening Today.

## Local MCP Agent Access

Settings includes an opt-in local MCP server for user-configured AI agent clients. It binds to `127.0.0.1`, uses a Keychain-backed bearer token, and routes tools through the same `AppModel` mutation paths as the UI. See [Local MCP agent access](../../docs/mcp.md).

## Task Writes

The Tasks section includes online task create, edit, complete/reopen, and delete flows backed by Google Tasks `tasks.insert`, `tasks.patch`, and `tasks.delete`. It also supports creating, renaming, and deleting task lists.

## Calendar Writes

The Calendar section includes online timed and all-day event create, edit, and delete flows backed by Google Calendar `events.insert`, `events.patch`, and `events.delete`. Event forms support custom popup reminders.

## Distribution And Updates

Preview DMGs were distributed through GitHub Releases. Each release published a matching SHA-256 checksum file alongside the DMG.

Unsigned preview builds do not support in-place auto-updates. Hot Cross Buns can check GitHub Releases for a newer DMG and open the download, but installation remains a manual replace.
