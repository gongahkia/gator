# Current Work execution

CLI, the main TUI, scheduled jobs, and `work-rpc` call `workrun.Service`. Typed
contracts reach execution intact. Provider adapters and credential lookup remain
outside the service; UI rendering and protocol framing remain outside execution.
The existing Go orchestrator owns children. No second language runtime was added.

## Conversation and evidence

A continuation resolves its explicit parent before choosing source, replay, or
prior output. Back/Forward move the head without deleting branches. `--parent`
selects a specific branch; `--refresh-source` captures current input. Otherwise
continuations use the selected parent's capture. An explicit snapshot override
is available in the typed/headless request. Source capture IDs are unique;
identical bytes share content-addressed trees and blobs.

Private revision state contains normalized messages, attachment bytes, effective
configuration, a versioned compaction summary, captured project configuration,
and accepted Code candidates. Opaque provider fields and provider-specific tool
IDs are removed when changing provider/model identity. Logical tool/result
correlation is retained. Private replay and executable configuration are excluded
from portable artifact archives.

Project profiles, selected rules and referenced files are captured separately
from ordinary source. Explicit grants select hooks, MCP, LSP, and extensions;
the original canonical project identity and matching bundle hash remain necessary
for trust. Scratch directories do not acquire trust. Inline credential fields
and referenced credential files are rejected. Revoking a grant disables its
execution even if an earlier capture contains the configuration.

Local file reads, supported extraction, selected attachments, connector reads,
and permitted web reads populate the evidence catalog. `list_evidence` is paged;
`read_evidence` checks and retains bytes; `check_claims` checks exact quotations.
A quote match does not establish semantic entailment. Local catalog reads are
bounded to 512 KiB per retained file; larger inputs still have source snapshot
identities and supported extraction/table tools. Binary or large attachments
remain private replay references when bounded text extraction is unavailable.
Web requests allow only selected HTTPS origins, use the existing public-address
DNS/pinning policy, and retain URL, time, digest, body, and truncation status.

## Deterministic tables and documents

`extract_document` uses the attachment parser on demand and reports unsupported
formats explicitly. Native PDF text extraction is unavailable; a selected PDF
can be passed to a model advertising visual input. Gator does not invent page
references. CSV/XLSX inspection retains source paths, hashes, sheets, and rows.

`reconcile_tables` joins exact keys and sums decimal amounts using rational
arithmetic. It reports duplicates and missing values, excludes missing amounts
from totals, preserves exact numeric results as text cells, and writes a workbook
and memo. An independent total comparison and reopening the generated workbook
check the result. Bounds are 4 MiB per input, 10,000 rows, and 256 columns.
Supported document/workbook rendering remains in the existing trusted renderers.

## Code candidates

The manager selects Code assignments; Code cannot recursively delegate. Each
child receives a private repository based on frozen source. `baseline_patch`
selects an accepted candidate, including one from the parent revision. Child
patches are cumulative relative to original source. Evidence records the selected
candidate and SHA-256 of the baseline Git tree identifier.

`--scope PATH` is a changed-path envelope as well as an instruction scope.
Built-in patch validation and strict process mounts enforce it. Nested trusted
integration processes inherit the Work envelope. Directory scopes allow new
files; a file scope allows writes to that existing file and may prohibit programs
that require replacing it through a temporary sibling. Additional writable roots
cannot widen a scoped child. Remote MCP also requires allowed network access.

`integrate_code` accepts only retained successful Code patches or accepted
candidates, checks composition, creates a private candidate, and runs the host's
verifiers against the combination. Failed conflicts/verifiers remain visible.
Verification that mutates candidate bytes is rejected. `artifact_reviewer` can
receive `baseline_patch` to review a private copy of exactly that candidate;
subsequent manager work does not change its view.

The review command shows candidate paths, original tree, selected patches,
verifier results, and failures. Only an explicitly selected verified candidate
can be applied as code. Target preflight checks changed-file original hashes in
addition to Git's patch checks. No agent execution applies code to live source.

## Configuration and bounds

Explicit request/CLI choices override configured manager defaults. Settings
`work_roles` entries are version 1 with `name`, optional `provider`/`model` pair,
and a narrowing `max_steps`. Unspecified routes inherit the manager. The role
registry is closed: source_researcher, artifact_reviewer, code,
connected_researcher, spreadsheet_analyst, claim_verifier. Per-role model routing
changes no authority. There is no automatic cross-provider fallback.

The effective policy digest covers the full normalized contract, mode, Code
policy, selected services, connector permissions, web origins, aggregate limits,
manager/provider identity, role routes, and project configuration digest. The
private replay retains this configuration. A foreground CLI may add detected
project verifier commands; explicit `--verify` replaces that detection.

Defaults are 24 manager turns, at most 8 turns for non-Code specialists, at most
32 Code turns, 8 child invocations, and 3 concurrent children. Shared budgets
cover manager, children, compaction, and retries: 256 adapter requests and 1,800
seconds by default. `--max-model-requests`, `--max-tokens`, and
`--timeout-seconds` narrow aggregate execution. Token limits stop subsequent
requests once reported usage reaches the limit; in-flight work can overshoot,
and unreported tokens cannot be enforced. Unknown usage and cost remain unknown.

Pending approvals are retained before presentation. Exact requests need explicit
responses; a deadline is never approval. Active cancellation joins children
before sealing. A process crash cannot guarantee a sealed final manifest, but
child checkpoints and scheduled attempt intents remain inspectable. Completed
results can be deliberately selected as continuation evidence. Unknown external
outcomes require inspection rather than automatic repetition.
