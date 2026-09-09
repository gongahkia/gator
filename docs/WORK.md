# Gator Work architecture

## Product contract

Gator is a terminal-native, local-first work agent. A work run accepts a clear
objective and an explicit set of sources, performs bounded work through approved
capabilities, and returns a reviewable artifact bundle. It does not silently
overwrite source material or perform an external action merely because a model
requested it.

The product is designed for technical operators, founders, researchers,
analysts, and developers who already use terminals and want automation that is
composable, inspectable, and provider-independent.

```text
objective + sources + outcome contract
                  |
                  v
       isolated local work session
                  |
        +---------+----------+
        |                    |
        v                    v
  staged artifacts     proposed actions
        |                    |
        +---------+----------+
                  v
       evidence-backed review
                  |
       export / apply / approve
```

The unit of value is **proof-carrying work**: every deliverable is accompanied
by a machine-readable manifest that records its media type, content digest,
source provenance, validation results, and whether any external action was
taken. Model prose alone is never completion evidence.

## Product boundary

The current product handles substantial local work that turns folders,
documents, structured data, and bounded web research into finished files. Its
first supported workflows are:

1. turn a folder of source material into a cited brief or report;
2. clean, compare, and summarize CSV or spreadsheet data; and
3. research a bounded subject and produce a source-backed deliverable.

Coding remains a first-class capability, but it is not a second user-facing
workflow. Gator owns the conversation and delegates bounded implementation to
its internal Git-worktree Code specialist. General work uses an ordinary
directory as a read-only source and a private, isolated output workspace.

Gator now includes durable local conversations, immutable local-source
snapshots, document/workbook renderers, connected services, and a manual
foreground scheduler. Cloud execution, cross-device continuation, and arbitrary
desktop control remain outside the local-first boundary.

## Domain model

### Workspace

A workspace is not assumed to be a Git repository. It has four explicit roots:

- `source`: one immutable snapshot of the selected directory, exposed read-only;
- `output`: a private run directory and the only default writable root;
- `previous`: the sealed parent revision's output on a continuation, read-only;
- `scratch`: private ephemeral process state, never included in a deliverable;
- `state`: private durable metadata, conversation, manifests, and evidence.

The agent addresses source files through a stable `source/...` namespace and
creates deliverables under `output/...`. The source and output roots must never
overlap. Symlinks and concurrent replacement are checked at the descriptor
boundary using the existing `workspace.Root` primitives.

Before model execution, Gator copies the selected, bounded source set into a
private content-addressed store. It records every path, byte count, mode,
modification time, and SHA-256 digest. Symlinks, VCS state, dependency/build
caches, known credential files, and `.gatorignore` patterns are excluded and
reported. Refreshing a source creates a new snapshot; it never changes an old
revision's evidence.

Work conversations form an immutable revision tree. Back and Forward move a
small head pointer; they do not delete revisions. Sending from an older revision
creates a branch automatically and seeds the new output copy-on-write.

### Outcome contract

An outcome contract is developer-owned structured data, not prompt text. It can
require:

- one or more artifact paths;
- allowed output media types;
- per-artifact and aggregate byte limits;
- required validations; and
- a disposition for external actions (`forbid`, `draft`, or `approve`).

Profiles and project configuration may only narrow a contract. A model cannot
weaken it. Completion requires a fresh inspection of the output directory and a
passing result for every required deterministic validation.

The first validators are deliberately small and reliable:

- `artifact_exists`: a bounded regular file exists under `output`;
- `non_empty`: the artifact contains data;
- `utf8`: the artifact is valid UTF-8 without NUL bytes;
- `json`: the artifact is one complete JSON value;
- `csv`: the artifact is parseable CSV with a stable, non-empty header; and
- `contains`: a bounded text artifact contains a required literal marker;
- `docx` and `xlsx`: the artifact is a structurally complete OOXML package; and
- `pdf`: the artifact has a complete PDF envelope.

DOCX/PDF use a shared semantic document specification, while XLSX uses a
semantic workbook specification. This keeps provider prompts and artifact
contracts independent of renderer libraries. See [rich artifacts](ARTIFACT_FORMATS.md).

### Artifact bundle

Every run produces `manifest.json` beside its private output directory. The
manifest is generated by trusted Gator code after model execution and contains:

- run and workspace identity;
- objective and outcome-contract digest;
- each artifact's relative path, media type, size, and SHA-256 digest;
- validation status and bounded diagnostics;
- source references used by the run;
- proposed and completed external actions;
- bounded specialist identity, lifecycle, and digest evidence; and
- timestamps and terminal run status.

Artifacts are never inferred from model claims. Gator walks the bounded output
root after the run and generates the manifest from filesystem state.

### External actions

Reading data and causing an external side effect are distinct authorities.
Every capability is classified as one of:

1. `source_read`: read local or connected data;
2. `artifact_write`: write only to the isolated output root;
3. `process_execute`: start a sandboxed local process;
4. `network_read`: retrieve data from an approved public origin;
5. `connected_read`: read from an authenticated service;
6. `connected_mutate`: change data in an authenticated service; or
7. `publish`: send, share, submit, or otherwise cross a human communication
   boundary.

`connected_mutate` and `publish` always require a fresh approval carrying a
human-readable preview. They cannot use an allow-always decision inherited from
a lower-risk operation. Drafting an email or proposed issue is an artifact
write; sending or creating it is a separate action.

## Execution modes

Gator Work has three monotonic permission modes:

- `inspect`: source and connector reads only; no artifacts or processes;
- `draft`: inspect plus writes to the isolated output root and approved
  sandboxed processes; and
- `act`: draft plus individually approved connected mutations or publishing.

Configuration can force a lower mode but cannot elevate one. Existing Plan mode
maps to `inspect`; existing Execute mode for coding maps to `draft` unless an
external action is separately approved.

## Review and transfer

General work review is artifact-oriented rather than Git-oriented. The review
surface shows:

- an artifact tree and media-aware previews;
- content hashes, sizes, and validation outcomes;
- source provenance and unresolved claims;
- a text or structured diff when replacing an existing target; and
- proposed external actions with their exact target and payload summary.

`gator work` never writes to the source directory. `gator apply` performs a
preflight against an artifact manifest, refuses changed targets unless the user
explicitly resolves the conflict, and then publishes files atomically where the
platform supports it. `gator export` copies or archives the immutable reviewed
bundle without touching the source.

## Connectors

MCP remains one connector transport, not the user-facing connector model. A
connector descriptor has a stable ID, declared actions, risk class per action,
authentication state, and bounded input/output schemas. Credentials remain in
Gator's private credential store and are keyed to the canonical remote
resource.

Connector installation, authentication, source selection, and action approval
are separate operations. Repository content cannot add a connector, redirect a
stored credential, or promote a read action to a mutation. Every connector
result is untrusted source data and carries a provenance record.

First-party adapters cover Slack, Google Drive/Docs/Sheets, Jira/Confluence,
and Notion. A typed remote-MCP mapping brings other services through the same
resource, permission, provenance, and exact-action-approval boundary. Connected
read responses are retained inside the Work bundle and verified against their
provenance digest.

Authentication supports resource-bound bearer tokens and developer-owned
public OAuth 2.0 apps using authorization code + PKCE, a registered loopback
redirect, bounded scopes, and refresh-token rotation. Gator never borrows a
vendor CLI's OAuth client identity. Services that require a confidential client
secret should use a separately minted bearer access token in this version.

## Lifecycle and scheduling

The foreground lifecycle is:

1. resolve and validate source, output, contract, and capabilities;
2. create a private isolated work directory;
3. execute the bounded manager loop, optionally delegating fresh-context
   specialist tasks, while journaling safe event metadata;
4. inspect artifacts and run deterministic validators;
5. generate and seal the manifest;
6. present the review surface; and
7. explicitly export, apply, or approve an external action.

Scheduled work uses a separate manually started foreground supervisor. It owns
durable job definitions, timezone-aware cron evaluation, missed-run policy,
single-instance locking, bounded retries, immutable attempts, an inbox, and an
authenticated loopback control endpoint. Jobs can inspect or draft but can
never approve external actions. See [Durable jobs](JOBS.md).

## CLI

The command hierarchy is:

```text
gator work [DIRECTORY] TASK       run a general local work task
gator inspect [DIRECTORY] TASK    produce analysis without artifacts
gator code TASK                   compatibility route requiring an internal Code patch
gator review RUN                  inspect artifacts, evidence, and actions
gator export RUN                  export a sealed artifact bundle
gator apply RUN                   copy reviewed artifacts to explicit targets
gator connector ...               manage connected sources and authentication
gator job ...                     manage scheduled work and its supervisor
gator inbox                       inspect completed and attention-needed jobs
gator snapshot ...                inspect or collect unreferenced snapshots
```

`gator code` and `gator run` are compatibility routes through the same Gator
manager. They require Code-specialist evidence but never open a separate Code
TUI. RPC and ACP retain explicit workflow and outcome-contract fields.

Headless operation is a primary product surface. Structured output, stable exit
codes, stdin-compatible task input, and immutable manifests make Gator useful in
shell pipelines and CI without weakening interactive approvals.

## Package boundaries

The implementation is split around durable concepts:

- `internal/workspace`: canonical filesystem roots and non-Git work isolation;
- `internal/artifact`: contracts, artifact inspection, validation, manifests,
  export, and apply;
- `internal/workrun`: general-work orchestration built on `internal/agent`;
- `internal/orchestrator`: bounded manager-as-tools specialist execution and
  evidence;
- `internal/action`: capability classification and external-action approval;
- `internal/connector`: connector registry, schemas, provenance, and auth
  references;
- `internal/snapshot`, `internal/worksession`, `internal/jobs`, and
  `internal/inbox`: immutable Work inputs, revision history, schedules, and
  result routing; and
- `internal/run`: the backend-only isolated coding executor used by Gator's
  Code specialist and protocol integrations.

Gator Work can call the retained coding workflow as a narrowly scoped `code`
specialist. It receives the exact immutable Work source snapshot, runs in a
private Git worktree, and returns a patch artifact; it never edits the selected
source. See [Work orchestration](ORCHESTRATION.md).

Provider adapters, the agent runner, sandbox, journal transport, terminal
manager, browser controller, attachment parser, and event stream remain shared.
Format-specific artifact code must not leak into the agent runner or provider
adapters.

## Compatibility and state

`gator run` remains an alias for the `gator code` compatibility route. New
coding requests use Work conversations and revisions. Historical Code run
records remain readable for review/export compatibility but are never silently
reinterpreted as Gator conversations. Snapshot collection is explicit and does
not delete data; `gator snapshot gc --yes` is the only snapshot reclamation
path.
