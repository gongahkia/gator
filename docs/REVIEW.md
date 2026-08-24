# Review surfaces

Gator reviews only a retained run worktree. Staging, feedback, and browser
review never write to the checkout from which the run was started. Export and
apply remain the explicit handoff to that checkout.

## Terminal review

After a run, open `/review`. Gator loads one bounded review snapshot split into
three scopes:

- `1` — all changes relative to the run's recorded base commit
- `2` — unstaged changes, where `s` stages a selected hunk and `S` stages a
  selected file
- `3` — staged changes, where `s` unstages a selected hunk and `S` unstages a
  selected file

The left pane is an expanded changed-file tree with per-file additions and
deletions. The detail pane has hunk navigation and exact line numbers. `Tab`
or left/right moves among files, hunks, and lines; up/down and `j`/`k` move in
the active pane. `[` and `]` jump hunks. The visible action bar gives mouse
targets for scope, focused/raw, staging, range, request, and refresh actions;
its confirmation and send controls are clickable too. The mouse can also
select file-tree leaves, hunks, and focused diff lines; its wheel follows the
active keyboard pane.

`f` switches the selected file between a complete raw patch and a focused
selected hunk. Index mutations always show an explicit `y`/`Enter` confirmation
first. Binary and metadata-only changes have no hunk operation, so use the
whole-file command.

To request a precise revision, move to focused lines, press `v` to set the
range start, move to its end, and press `r`. Enter an instruction and press
`Ctrl+R`. Gator saves a private structured record containing the selected
file, current hunk identity, line numbers, before/after excerpts, and request,
then sends it as a constrained continuation of the retained thread. The code
excerpt is labeled untrusted context in that follow-up; the agent is told to
inspect the current worktree before editing.

`e` shows the unchanged explicit `gator export`, `gator apply --check`, and
`gator apply` handoff commands. Gator's `git_diff` tool also includes staged
changes, so a later continuation does not silently lose an index-only change.

## Browser review

Use a separate foreground local server for a retained run:

```sh
gator review RUN_RECORD_PATH --listen 127.0.0.1:0
gator review RUN_RECORD_PATH --open
```

The command prints a one-use URL. Opening it exchanges the URL credential for a
15-minute, HttpOnly, `SameSite=Strict` session cookie and redirects to a URL
without the credential. The browser page provides the same all/unstaged/staged
file and hunk navigation, raw file patches, explicit stage/unstage
confirmations, and range-scoped feedback. Browser feedback is saved beside the
retained run and returns the exact constrained continuation text for review and
copying; use `gator resume RUN_RECORD_PATH` when no native TUI is already
attached to send it.

This is intentionally not `gator serve` and not an RPC client. `gator serve`
continues to reject every browser origin. The review listener accepts literal
loopback peers only, binds only literal loopback addresses, has no CORS mode,
requires same-origin mutations, sends `no-store`, `nosniff`, no-referrer, and a
nonce-bound restrictive Content Security Policy, and never loads external
assets. It can be reached over an SSH tunnel only by deliberately forwarding a
loopback port; do not expose it with a public reverse proxy.

The page is a review and index-management surface, not a remote executor. It
does not expose provider credentials, the app-server bearer token, a generic
agent API, or a way to apply the retained patch to the active checkout.
