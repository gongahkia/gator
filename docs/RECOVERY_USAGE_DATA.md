# Recovery, shared usage, and private data

These controls use the existing Claude Code Kit terminal pickers, forms, and
confirmation screens. Open them from chat or the command palette. Tab moves
between fields, Ctrl+U clears a field, Ctrl+R reviews an edit, Enter confirms,
and Esc cancels. A review does not itself change saved settings.

## Recovery: /recovery

Open a check from the list to inspect its last success, next due time, recent
attempts, selected connector credentials, and suggested recovery action.

- Reconnect a selected source using the existing OAuth login handoff or a masked
  bearer-token form. This preserves the selected resource and permissions.
  Changing accounts or the follow-up agreement requires a reviewed edit through
  /followup. Retry the check to verify access after reconnecting.
- Retry an enabled check against its current sources. Existing per-check and
  shared usage limits still apply.
- Start the local supervisor, or review enabling/disabling its per-user
  start-at-login service. Service commands use this profile's state directory
  and configured Gator settings. Another profile's service is not offered for
  removal or replacement. Reinstall after moving the executable.
- Inspect heartbeat gaps and recent attempts. A gap establishes that continuous
  monitoring was not confirmed. A catch-up check reads current sources; it
  cannot reconstruct intermediate changes or prove nothing was missed.

The computer must be on and its user session/user manager active. This does not
provide cloud execution. Installing a service and a live supervisor health
check are separate facts.

## Shared usage: /usage

Every process using the same GATOR_STATE_DIR shares one private, locked ledger.
Foreground chat, ambiguous-request routing, background Work, internal Code
work, embedding requests, failures, and retries use that ledger. Gemini
embeddings are counted per input because the adapter makes one HTTP request for
each input; batched adapters count each actual batch request.

Daily request, token admission, and concurrent request allowances default to
uncapped. The user can review limits or pause new model and embedding requests.
Pausing does not cancel requests already in flight, disable connector reads, or
stop inexpensive local polling.

Request attempts reserve allowance before invoking an adapter. Some failures
happen before a provider receives the request. Reported usage replaces token
estimates; failed or unreported requests retain conservative estimates.
Admission estimates use serialized input bytes and an output reservation
(default 8192). The token allowance is not a strict provider billing or output
ceiling: a response can exceed its reservation. Unknown prices are never
displayed as zero cost. Per-run and per-check limits continue to apply.

A crash frees concurrency once the process-held lease is gone, while retaining
its request and estimated token charge. Daily totals use UTC and are stored
without prompt bodies or credentials. Old usage counters are preserved by data
deletion so deletion cannot reset a shared daily allowance.

CLI equivalents:

```sh
gator usage status
gator usage limits --requests 100 --tokens 1000000 --concurrent 2
gator usage pause
gator usage resume
```

## Private data: /data

Retention defaults to **keep until explicitly deleted**. Users can review a
policy, preview its eligible files, export and verify a private backup, or
review deletion by scope.

| Scope | Contents |
| --- | --- |
| Saved knowledge | Saved facts, notes, skills, learnings, observations, retrieval cache |
| History | Conversations and revisions, retained inputs/excerpts, Work records and private output workspaces, related traces, suggestions and inbox |
| Unused snapshots | Source snapshots not needed by retained conversations, Work or checks; shared trees and blobs stay protected |
| All retained Work and knowledge | All these groups, goals and saved check definitions |

Original external project directories, credentials, provider settings,
browser/desktop profiles, shared usage counters, and external backups remain.
Forgetting a saved fact removes it from future recall; earlier transcripts and
snapshots can still contain historical copies. Use the appropriate historical
scope to remove those copies too. Local deletion does not remove provider-held
data and is not a promise of secure SSD erasure.

Automatic retention applies to completed history, saved facts/notes, and unused
snapshots. Ongoing goals and their conversations, pending suggestions, the latest
successful check per job, running/interrupted Work, and referenced snapshot
content are protected. Skills and learnings require explicit deletion.
Unindexed or interrupted private workspaces can be removed through a reviewed
history/all deletion. A policy runs on startup and daily while the supervisor
is active; busy data is deferred. /data shows the last completed automatic sweep.

Deletion reviews include exact selected files, hashes, sizes, folder counts,
and a one-hour expiry. Changed content or references require a fresh review.
Active Work blocks destructive maintenance, including across processes.
Deletion can stop partway on cancellation or filesystem errors; refresh and
preview what remains. Successful history deletion clears cached conversation
content; deleting goals also clears the active goal binding.

Backups are unencrypted tar.gz archives with a checksummed manifest and private
file permissions. They include retained source snapshots, drafts, transcripts,
and selected connected data. Credentials, provider settings, browser profiles,
runtime access tokens/leases, rebuildable retrieval indexes, and usage counters
are excluded. Choose a new destination outside Gator's private data directory;
an existing archive is never overwritten. Export and verification enforce
1 GiB per file and 10 GiB total limits, including a bounded manifest.

Verification checks file scope, checksums, and archive integrity. Reviewed
restore and source relinking are now supported through /data and the CLI.
[Everyday overview, restore, and migration](OVERVIEW_RESTORE.md) documents fresh
profiles, source mappings, paused checks, fresh configuration, and recovery.
The archive retains content rather than credentials or installation settings.

CLI equivalents:

```sh
gator data status
gator data retention --history-days 90 --knowledge-days 0 --snapshot-days 30
# Review the printed policy before repeating with --yes.
gator data cleanup
gator data preview history
gator data apply REVIEW_ID --yes
gator data export /private/path/new-backup.tar.gz
gator data verify /private/path/new-backup.tar.gz
```

## Verification boundaries

Tests exercise isolated state/config roots, fake model/embedding adapters,
cross-process admission and crash leases, heartbeat gaps, masked credentials,
stale reviews, live-data guards, source/snapshot protection, private archives,
and corruption rejection. OS-service recovery is exercised with an injected
command runner; implementation testing does not install or remove the user's
real login service.

These controls establish inspectable infrastructure. A live project-follow-up
pilot and matched model evaluation are still required to demonstrate useful
alerts, missed-change rates, recall accuracy, and acceptable attention/cost.
