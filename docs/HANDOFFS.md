# Handoffs

Gator creates a new target-provider session for every handoff. It transfers a reviewed, local Gator artifact rather than claiming an opaque Claude, Codex, Pi, or ACP session can be migrated across providers.

The review includes the original context bundle, Gator-owned chat transcript when available, a live source-workspace diff, and a file-snapshot manifest. Included changed text files are copied to the target workspace at:

```text
.gator/handoffs/<bundle-id>/files/
```

The target prompt names that directory. Included text snapshots are also applied to the target workspace after review confirmation, preserving worktree-per-writer isolation; deleted source files are removed from that target workspace. The retained `.gator` artifact follows Gator's redaction policy, while the local target workspace receives its original local file content. Binary, unreadable, oversized, deleted, and count-limited files are not silently copied; the review labels their reason. Configure the bounds with `context.handoff.max_files` and `context.handoff.max_file_chars`; set either to `0` to disable that part of the snapshot.

`summary-first` remains restricted to active Gator chat runs because it asks the source agent for a summary; terminal output is never scraped. The run graph records the target worktree, context bundle, transcript availability, provider-reported usage when available, and budget state.
