# Memory evolution capability audit

Run from the repository root, choosing a new report filename in an existing directory:

```sh
go run ./scripts/memory-evolution /tmp/gator-memory-evolution.json
```

This calls the real memory tools/stores and learning store using disposable,
synthetic profiles. It exercises passive save boundaries, corrections, repeated
confirmation, aliases, forgetting, expiry, source changes, corruption and learning
candidate lifecycle. One synthetic record is aged in its temporary store; the
machine clock and real profile are never changed. No model or network is used.

`baseline-2026-10-29.json` records **14 met capabilities and 8 gaps across 22
probes**. These counts are not a memory-quality score, live-model extraction rate
or generalization benchmark. The runner intentionally reports unmet capabilities;
a completed audit exits successfully even when it discovers gaps. Setup/runtime
errors fail the command. It is not a passing CI acceptance gate. Inspect each
finding and its evidence; do not use a zero exit code to claim memory is complete.

The passive suitability probes deliberately call the tool with choices a model
is instructed not to make. They measure host enforcement, not how often a live
model makes those choices. The repeated-failure probe records observations and
checks for candidates without calling `Propose`; it measures the absence of an
automatic proposal step, not a malfunction of explicit proposal creation.

The report never edits implementation to turn gaps into passes. Product changes
should be followed by a new report and targeted regression coverage. See the
[evaluation and priorities](../../docs/MEMORY_EVOLUTION_EVALUATION_2026-10-29.md).
