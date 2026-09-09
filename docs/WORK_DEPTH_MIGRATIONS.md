# Work state compatibility

No historical Code state is deleted or automatically converted into Work.
Back up the private state directory before deploying a new binary. New state is
written in current versions; old files are read under the semantics below.
Downgrading a binary after writing newer state is not supported.

| Store/schema | Current | Older-state behavior |
| --- | --- | --- |
| Source captures | 2 | Version 1 remains readable; its ID also identifies its tree. Existing origin/time/exclusions are preserved as found. Earlier overwritten origin metadata cannot be reconstructed. |
| Work conversations/revisions | 2 | Version 1 remains readable. Revisions without replay synthesize only the retained user objective and assistant final text; no historical tool actions or attachments are invented. |
| Private replay/compaction | 1 / 1 | Unsupported replay versions fail explicitly. Provider changes remove opaque adapter state while preserving normalized correlation. |
| Captured project configuration | 1 | Missing old captures stay missing on frozen continuations. Refresh explicitly to capture current selected configuration. |
| Settings | 4 | Versions 2/3 migrate through the existing settings loader. Optional role entries are version 1. Credentials remain in the existing separate store. |
| Jobs/attempts | 2 | Version 1 definitions and old flat result files remain readable. Existing refresh flags retain their meaning; newly frozen definitions capture source and selected configuration when saved. |
| Specialist checkpoints | 1 | Queued/running checkpoints recovered by their owner become interrupted; completed results are retained. Policy/source mismatches are rejected. |
| Artifact manifests | 3 | Versions 1/2 remain readable under their original required fields. Version 3 adds selected evidence, candidates, usage and effective policy. |
| Work protocol | 1 | Separate command; legacy RPC/ACP/app-server semantics remain unchanged. |
| Work dataset/trial/experiment/rubric | 1 | New Work target is `work.v1`; existing direct Code fixtures and reports retain their old paths/schema. |

Capture IDs and content tree IDs now differ. Snapshot garbage collection retains
shared trees referenced by any capture and frozen job snapshots. Source refresh
creates a new capture and never replaces an earlier revision's evidence.

Private paths below `STATE/gator/` include conversations, tasks/RUN, interactions/RUN,
candidates/RUN, traces/RUN, jobs/definitions, and jobs/history/JOB/ATTEMPT. Work
bundles contain deliverables and selected evidence. Portable export excludes raw
replay and executable project configuration. Trace files default to bounded
metadata only.

Scheduled execution writes intent before the schedule watermark, then writes a
stable `try-N.json` Work run reference before each retry. Recovery treats an
existing slot intent as already claimed, even if the watermark write was lost.
A foreground supervisor reconciles interrupted attempts into history/inbox;
it does not repeat unknown effects. Inspect `gator work tasks RUN_ID` and job
history before deliberately starting new work. `start_agent` may use `continue`
with a completed task ID in the current run or `PARENT_RUN/subagent-NNN` from the
selected parent revision. Interrupted/active retained outcomes cannot be retried
through that shortcut.

The execution model is bounded durable state, not instruction-level checkpoint
replay or exactly-once external execution. Pending approval records preserve the
request; restarting does not grant approval. macOS runtime verification and
hosted service checks are listed separately in the verification record.
