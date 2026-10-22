# Everyday overview, restore, and migration

**Interface status:** Gator is a desktop app. Shell examples below are source-checkout developer operations with an explicit internal opt-in; see [development instructions](DEVELOPMENT.md). They do not install or enable a public CLI.

## Product decision

The everyday overview answers four questions together:

1. **What changed** — retained follow-up results and completed/failed Work since
   the last explicit overview review (the last 24 hours before the first review).
2. **What needs me** — unread results, proposals, due goal reviews and steps,
   and decisions still connected to a live Work process.
3. **What happens next** — upcoming checks, goal reviews, next steps, and running Work.
4. **What is blocked** — offline checks, recorded failures, shared limits, missing
   model setup or source folders, blocked steps, interrupted Work, stale decision
   records, and unavailable underlying stores.

These are projections of retained records and observed local liveness. They do
not establish that a result is correct, every change was found, or a scheduled
check will execute while the computer is off.

Home keeps all four summaries together using existing Kit typography and
colors. `/overview` or Ctrl+O opens the picker for sections, items, and direct
actions. Reading it requires no model call or connected source poll. Local
supervisor health is probed separately. It refreshes about every 15 seconds and
supports explicit refresh. Background refresh keeps the selected item and
scroll position, with no repeated loading flash. Esc returns through the
section levels to the original home.

Results open their exact inbox entry. Plans open the selected goal. Work opens
its retained history and output. Recovery targets the selected check. Missing
sources open the relink form. Limits and model setup open existing controls.
A live decision hands the terminal to `gator work answer RUN ID`; that command
inspects the exact action and requires a response. A pending record without
confirmed liveness never becomes an answerable approval.

Marking changes reviewed advances only the overview review time. It does not
mark results read, accept proposals, resolve goal reviews, or approve actions.
The review time is the displayed snapshot time, so later changes remain new.

Lists show up to 50 items per section and retain the full count. Original stores
remain available for full history. Read errors become blockers rather than
healthy empty states. Deleting history through the current session invalidates
cached overview content so a delayed async response cannot bring it back.

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator overview
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator overview --json
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator overview seen
```

## Restore into a fresh profile

`/data` offers **Restore into a new profile**. Choose the archive, a directory
that does not exist, and optional source mappings. The form review verifies the
archive and produces a second, exact restore agreement. Only confirmation of
that agreement creates the profile.

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator data restore /private/backup.tar.gz --to /private/gator-restored
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator data restore /private/backup.tar.gz --to /private/gator-restored --map-source /old/project=/new/project
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator data restore-apply REVIEW_ID --yes
```

Source mappings are repeatable and their destinations must exist. The review
binds the archive checksum, original root, destination, mappings and observed
source availability, file count, and recovery changes. It expires after one
hour. Changes to these facts require a new preview.

Existing destinations, including empty directories and symlinks, are never
merged or replaced. The destination parent must exist. The new profile must be
separate from the current profile and selected sources.

Restore verifies a private staging copy, extracts through `os.Root`, migrates
navigation metadata, validates records and sealed artifact/snapshot bytes, and
syncs before publication. The final rename refuses existing destinations on
macOS and Linux. Cancellation or error before publication leaves no partial
destination. A process killed during staging can leave a private
`.gator-restore-*` directory in the destination parent. It is not an active
profile and can be removed after confirming the process stopped.

One archive parser handles verification and extraction. It rejects traversal,
links, duplicate entries, runtime capabilities, unexpected roots, invalid
manifests, checksum failures, and oversized entries. Bounds are 1 GiB per file,
10 GiB uncompressed content, 32 MiB for the manifest, 12 GiB compressed input,
and 500,000 files. Extraction follows the Go project's
[traversal-resistant archive guidance](https://go.dev/blog/osroot).

### Migration behavior

- Private bundle and evidence navigation paths point to the new root.
- Source mappings update navigation in goals, conversations, checks, snapshots,
  and historical workspace metadata.
- Sealed manifests, drafts, transcript text, replay messages, content hashes,
  and frozen project configuration remain historical. Original host metadata
  is retained privately under `gator/migrations`.
- Replay execution settings are cleared. Imported configuration cannot
  reactivate old grants, browser sessions, or connector permissions.
- All checks start paused, OAuth pins are cleared, and interrupted Work becomes
  failed history. Runtime approval access is excluded and never resumed.
- Model/embedding requests start paused. Retention starts at keep-until-deleted.
  Credentials, provider settings, connectors, browser profiles, usage counters,
  and login services are not restored.
- Restored conversations require a fresh source capture on their next normal
  continuation. Explicit frozen snapshot selection remains user control. A new
  revision clears the refresh requirement.

The current profile stays active after restore. Success prints a shell-quoted
command selecting both the new state and configuration roots:

```sh
GATOR_STATE_DIR='/private/gator-restored' GATOR_CONFIG_DIR='/private/gator-restored/config' gator
```

Without an explicit configuration override, a restored profile uses its own
`config` directory. Explicit `GATOR_CONFIG_DIR` remains user control. `/data`
shows the receipt, current source availability, and setup guidance. Set up
`/model` and selected connectors, review `/usage`, and review paused `/followup`
agreements before resuming. Finishing setup review dismisses its reminder; it
does not enable anything.

Missing external projects do not prevent recovery of historical drafts,
transcripts, and knowledge. Those projects remain blocked until relinked to
available folders. Original project directories are outside the backup.

## Relink a moved project

`/data` offers **Relink a moved source folder**. A missing-source blocker in
`/overview` opens that form directly. Choose the old path and an existing new
folder, review affected metadata, then confirm.

```sh
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator data relink /old/project /new/project
GATOR_INTERNAL_COMMANDS=1 go run ./cmd/gator data relink-apply REVIEW_ID --yes
```

Relinking updates matching source references, pauses affected checks, and
requires a fresh capture before continuing. Source files, artifact bytes,
transcript text, and evidence hashes remain unchanged. The current workspace
follows the relink without erasing its transcript.

An exclusive profile guard prevents concurrent Work or maintenance. A private
journal binds old and new metadata bytes. A stopped, uncommitted relink rolls
back when the profile opens; a committed journal is cleaned up. Unexpected
external edits during recovery block startup for inspection. Stale or expired
reviews fail.

## Verification

Tests cover joined state, corrupt-store blockers, live versus stale decisions,
bounded lists, review timestamps, exact navigation, explicit handoffs,
secret-free bridge projections, source moves, fresh configuration isolation,
archive rejection, fresh-only publication, cancellation, source mapping,
interrupted Work, paused checks, draft/transcript preservation, conversation
continuation, second-generation migration, and relink recovery.

Automated and local fake-provider checks establish these mechanics. They do
not establish live model quality or useful alert relevance.
