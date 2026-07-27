# Handoffs

Gator creates a new target-provider session for every handoff. It transfers a reviewed, local Gator artifact rather than claiming an opaque Claude, Codex, Pi, or ACP session can be migrated across providers.

The review includes the target transport/workspace, context byte count, original context bundle, Gator-owned chat transcript when available, a live source-workspace diff, and an immutable file-snapshot manifest. The manifest records the source base SHA, staged/unstaged state, add/modify/delete/rename/mode metadata, content hashes, exclusions, target conflicts, and per-conflict decisions. Included changed text files are copied to the target workspace at:

```text
.gator/handoffs/<bundle-id>/files/
```

The target prompt names that directory. Before launch, you explicitly choose `Apply snapshots and launch` or `Retain snapshots only`; cancellation creates no target provider session. Applying snapshots renders a real target-worktree diff and detects target base/dirty-path/symlink collisions. Each collision requires an `apply` or `skip` decision. Applied snapshots preserve worktree-per-writer isolation; deleted source files are removed from that target workspace. The retained `.gator` artifact follows Gator's redaction policy, while the local target workspace receives its original local file content. Binary, unreadable, oversized, deleted, submodule, symlink, secret-like untracked, and count-limited files are not silently copied; the review labels their reason. Configure the bounds with `context.handoff.max_files` and `context.handoff.max_file_chars`; set either to `0` to disable that part of the snapshot.

`summary-first` remains restricted to active Gator chat runs because it asks the source agent for a summary; terminal output is never scraped. The run graph records the target worktree, context bundle, transcript availability, provider-reported usage when available, and budget state.

When `review.commands` is non-empty, accepting a review requires passed immutable evidence for the exact displayed base SHA and diff hash. Requesting changes or creating a further handoff remains explicit; Gator never interprets provider prose as test evidence.
