# Gator — Product Direction, Architectural Handoff, and Six-Tranche Roadmap

> **Purpose of this document**
>
> This is a standalone handoff for a fresh local Codex agent working inside the Gator repository. It captures the product direction, architectural decisions, completed checkpoints, non-negotiable constraints, development philosophy, evaluation standards, and the next six implementation tranches.
>
> **Do not treat this document as a substitute for inspecting the repository.** It records the agreed direction and the last reported state. The repository is authoritative for current code, Git state, and implementation details.
>
> The intended workflow is:
>
> 1. Read this entire document once.
> 2. Inspect the repository and current Git state.
> 3. Reconcile this handoff with what actually exists.
> 4. Execute **one tranche at a time**.
> 5. Stop after each tranche and produce the required product checkpoint report.
> 6. Do not automatically continue to the next tranche until the human has reviewed the checkpoint.
>
> This roadmap is deliberately designed to allow reversal. If a tranche makes Gator less coherent, more bloated, less auditable, harder to evaluate, or materially worse at completing Work, say so and stop.

---

## 1. Repository Identity

Project: **Gator**

Last reported repository location:

```text
/Users/gabrielongtemasek/Desktop/personal/gator
```

Last explicitly reported branch:

```text
main
```

A previously reported HEAD before the evaluation-consolidation tranche was:

```text
86ba4c9b28e4abb65437f57113985ab364164265
```

This hash is **historical context only**.

Before doing anything:

```bash
git status --short --branch
git rev-parse HEAD
git rev-parse --abbrev-ref HEAD
git rev-list --left-right --count HEAD...@{upstream}
```

where applicable.

Never reset or discard work merely because the current tree differs from this document.

---

# 2. What Gator Is Becoming

The agreed product thesis is:

> **Gator is a lightweight, local-first runtime for personal AI work. It executes work inside bounded, auditable transactions, learns from their outcomes, and lets the user inspect what happened and why future behavior changed.**

The intended user is primarily **one person operating their own machine and resources**.

Do not distort the UX around enterprise IAM, multi-tenant security models, approval bureaucracy, or administration of the user's own computer.

Security and authority boundaries exist to prevent mistakes and constrain agents, not to force the user through constant confirmation screens.

The primary product abstraction is:

```text
WORK
```

Internally, each meaningful execution of Work should be treated as a bounded:

```text
WORK TRANSACTION
```

The transaction concept is primarily architectural. Ordinary users should not need to think like database operators.

Good normal-user vocabulary includes:

```text
Work
History
Result
Changes
Review
Apply
Retry
Learnings
Jobs
Settings
```

---

# 3. Core Product Model

The long-term conceptual lifecycle is:

```text
user intent
    ↓
Work transaction begins
    ↓
capture relevant initial state/context
    ↓
bounded authority is established
    ↓
agent / specialists / tools operate
    ↓
observable actions and evidence are recorded
    ↓
artifacts / intended changes are produced
    ↓
verification
    ↓
delivery / apply
    ↓
outcome
    ↓
human feedback and/or observed failure
    ↓
candidate learning
    ↓
evaluation
    ↓
active learning or rejection
```

The product should become increasingly good at answering:

```text
What did Gator attempt?
Why did it do that?
What information did it use?
What did it actually change?
What verification occurred?
What succeeded?
What failed?
What was uncertain?
What did the user accept, reject, or correct?
What did Gator learn from that?
Why does that learning affect future Work?
```

This is the basis of the product's differentiation.

---

# 4. The Central Technical Thesis

Gator's strongest potential niche is not:

- “another coding agent”;
- “another ChatGPT/Claude clone in the terminal”;
- “another multi-provider wrapper”;
- “another OpenClaw/Hermes-style memory bot”;
- “another workflow engine”.

The central technical thesis is:

> **Transactional personal AI work creates unusually strong auditability, and that auditability can become the substrate for inspectable self-improvement.**

A bounded Work transaction naturally produces structured experience:

```text
intent
  ↓
initial state
  ↓
actions
  ↓
observations
  ↓
result
  ↓
verification
  ↓
delivery
  ↓
feedback
  ↓
outcome
```

That is a much better learning source than unstructured chat history.

The future self-improvement loop should resemble:

```text
REAL WORK
   ↓
transaction evidence
   ↓
success / failure / correction
   ↓
observation
   ↓
candidate lesson
   ↓
evaluation / replay
   ↓
promote or reject
   ↓
better future Work
```

Not:

```text
LLM makes a guess
   ↓
silently rewrites its own instructions
```

---

# 5. Settled Product Decisions

These decisions have already been made. Do not reopen them casually.

## 5.1 Work is the primary abstraction

Chat, provider sessions, coding runs, jobs, browser sessions, and specialists are subordinate concepts.

A conversation may contain multiple Work transactions.

A job should create Work rather than become a second execution runtime.

Coding should be a strong Work capability rather than a second product.

---

## 5.2 General-purpose Work, strong Code

Gator should remain capable of:

- research;
- file-oriented work;
- artifact production;
- document/data tasks;
- coding;
- browsing where useful;
- desktop/computer interaction where justified;
- scheduled Jobs.

But Code remains unusually important and should retain strong capabilities such as:

- repository isolation;
- patch candidates;
- verification;
- testing;
- code-specific tools;
- LSP;
- MCP where useful.

Do not weaken Code merely to claim genericity.

---

## 5.3 Authority-centric, not prompt-for-everything

Conceptually:

```text
inspect
  broad reading
  no persistent mutation

draft
  isolated changes/artifacts
  no uncontrolled persistent external effects

act
  operate within granted authority
  ask only at genuinely consequential boundaries
```

These are conceptual semantics, not necessarily exact code names.

Gator should be usable without dozens of confirmations.

---

## 5.4 Transactions are observable, not magically reversible

Do not claim universal ACID semantics.

Different effects have different properties:

```text
local file creation
local file replacement
Git patch application
file move/delete
browser action
calendar mutation
email send
external API mutation
```

Some are safely replayable.

Some can be protected by preconditions.

Some may be irreversible.

Some may become uncertain after a network failure.

Gator's promise is:

> **bounded, observable Work with explicit outcomes**

not:

> “everything can always be rolled back.”

---

## 5.5 Production and delivery are separate

A valid result can exist even if applying it fails.

Conceptually:

```text
agent generates intended result
        ↓
verification
        ↓
delivery/apply
```

A delivery failure should not automatically force the model to regenerate Work.

Safe delivery should be retryable from persisted intent.

---

## 5.6 Learning is inspectable and reversible

The desired lifecycle is:

```text
historical evidence
      ↓
observation
      ↓
candidate lesson
      ↓
evaluation / evidence
      ↓
active learning
```

Candidate learning types initially include:

```text
preference
environment fact
procedure / skill
failure-prevention rule
```

Learnings must be:

- inspectable;
- attributable;
- scoped;
- reversible/disableable;
- directly editable where useful;
- distinguishable from immutable historical facts.

A user reversing a learning must **not** rewrite history.

Example:

```text
HISTORICAL FACT:
User rejected PDF output in Work X.

INFERENCE:
User globally prefers Markdown.
```

The inference can be disabled. The historical event remains true.

---

## 5.7 User-authored knowledge outranks inference

The user must be able to accelerate learning.

If the user explicitly edits a preference/rule/skill, Gator should not require repeated behavioral observation before respecting it.

Conceptually:

```text
explicit current user instruction
        >
explicit user-maintained knowledge
        >
high-confidence learned rule
        >
weak inferred observation
```

Exact precedence must ultimately be explicit and testable.

---

## 5.8 Learning must be scoped

A preference observed in one repository must not casually become global truth.

Useful scopes include:

```text
global/user
project
repository
directory
task type
conversation
```

Do not force all scopes into the first implementation if the repository suggests a smaller coherent start, but the architecture must avoid uncontrolled global generalization.

---

## 5.9 Google context is useful, not product identity

If Gator already has appropriate access to Google Drive and Google Calendar, use them as Work context sources.

Examples:

- upcoming meetings;
- related documents;
- calendar availability;
- Drive files relevant to a task.

Do not turn Gator into “a Google assistant.”

Do not add channel sprawl merely for feature breadth.

---

## 5.10 Provider count is not a KPI

Gator owns:

- orchestration;
- Work semantics;
- context;
- tools;
- evidence;
- verification;
- delivery;
- learning.

Models/providers are intelligence backends.

A small number of high-quality integrations is better than provider-logo accumulation.

Do not add providers simply to increase breadth.

---

## 5.11 Lightweight matters

Prefer, where practical:

```text
single binary
fast startup
low idle RAM
no mandatory daemon
SSH-friendly operation
local-first state
human-readable state where useful
minimal background machinery
```

“Lightweight” is a strong product property but not the entire product thesis.

Do not introduce databases, daemons, event buses, workflow engines, or background services without a compelling demonstrated need.

---

## 5.12 CLI/TUI parity is a hard rule

Every meaningful user-facing CLI capability should have an equivalent TUI interaction path, unless it is explicitly:

- developer plumbing;
- low-level maintenance;
- or intentionally fully hands-off automation.

The correct architecture is:

```text
             shared core capability
               /             \
             CLI             TUI
```

Never:

```text
CLI implementation       separate TUI implementation
```

The TUI must not shell out to the CLI as its semantic backend.

The CLI must not be treated as the “real” product while the TUI is decorative.

---

## 5.13 The TUI should stay simple

Do not build a dashboard because terminal AI products are expected to have dashboards.

Rich UI is especially useful for:

- active Work;
- review;
- diffs;
- evidence;
- History;
- Learnings;
- Jobs;
- Settings.

Simple task submission should remain simple.

A desirable eventual top-level information architecture is approximately:

```text
Work
History
Learnings
Jobs
Settings
```

This is directional, not a mandatory menu layout.

---

## 5.14 Be brutal about bloat

Existing code has no presumption of survival.

Be willing to delete:

- legacy runtimes;
- duplicate UIs;
- compatibility layers;
- obsolete commands;
- redundant protocols;
- provider breadth;
- optional integrations;
- state machinery;
- abstractions created for hypothetical futures.

Ask of major subsystems:

> If this did not exist today, knowing the current product direction, would we choose to build it in the next six months?

If the answer is no, strongly consider deleting or optionalizing it.

Do not delete mature useful code merely for aesthetics.

---

# 6. Development Philosophy

The preferred migration pattern is:

```text
canonicalize
    ↓
collapse
    ↓
migrate
    ↓
delete
```

Avoid:

```text
add abstraction
    ↓
support old and new forever
```

Temporary compatibility code should have explicit deletion criteria.

The burden of proof is on complexity.

---

# 7. Product Checkpoints Are Mandatory

After every major tranche, stop and explicitly reassess:

1. Is Gator simpler?
2. Is Work more coherent?
3. Did output quality improve or remain intact?
4. Are transactions more auditable?
5. Are they more replayable/recoverable?
6. Are evals giving us more confidence?
7. Did we add infrastructure instead of product value?
8. Is CLI/TUI parity intact?
9. Is Gator still lightweight/local-first?
10. What can now be deleted?

A roadmap is not sacred.

If repository evidence suggests a better boundary, say so.

Do not blindly execute the next tranche.

---

# 8. Verification Discipline

Use narrow tests while iterating.

Before declaring a tranche complete, broaden verification appropriately.

Preferred pattern:

```text
focused tests
    ↓
affected package tests
    ↓
representative evals
    ↓
go test ./...
    ↓
go vet ./...
    ↓
build
    ↓
gofmt verification
    ↓
git diff --check
```

Never claim a test passed if it was not actually observed passing.

Never normalize or weaken an oracle merely to get green.

---

# 9. Git Discipline

Do not:

- reset user work;
- discard unrelated changes;
- clean destructively;
- force checkout;
- rebase without instruction;
- amend unrelated commits;
- push;
- force-push.

The working tree may intentionally contain several uncommitted tranches.

Preserve it.

At the start and end of every tranche, report Git state.

---

# 10. Privacy / Auditability Boundary

Auditability does **not** mean preserving hidden chain-of-thought or copying all raw data into history.

Prefer durable structured evidence such as:

```text
objective summary/digest
authorized resources
source/snapshot IDs
observable tool/action events
artifact references
verification results
delivery attempts
human feedback
outcomes
derived learning references
```

Avoid unnecessarily persisting:

- raw model reasoning;
- full prompts where not needed;
- attachment bytes duplicated into logs;
- raw connector payloads;
- credentials;
- giant tool outputs.

Auditability should be evidence-oriented and privacy-bounded.

---

# 11. Reported Completed Architectural Checkpoints

Treat these as **reported state that must be re-verified against the repository**, not assumptions to implement again.

## 11.1 Canonical Work history / transaction spine

Reported complete.

A small durable Work-history index was introduced under approximately:

```text
$GATOR_STATE_DIR/gator/work-history/<work-id>.json
```

Reported `workhistory.Record` includes references such as:

- Work ID;
- bounded objective summary/digest;
- mode;
- external-action disposition;
- policy digest;
- conversation/revision/parent revision;
- snapshot;
- manifest;
- trace;
- interactions;
- tasks;
- candidate evidence;
- artifact/verification state;
- running/completed/failed timestamps.

Important design property:

> The history record references existing evidence rather than duplicating payloads.

Reported behavior:

- `workrun.Executor.Execute` writes/finalizes it;
- interactive Work, jobs, CLI, TUI, and eval-created services use the same execution path;
- CLI/TUI History use the same underlying model;
- meaningful failed Work retains a failed history record;
- process crash may leave a running record;
- full raw prompts/tool/connector payloads are not copied into the record.

Do not create another transaction runtime.

---

## 11.2 Code collapsed beneath Work

Reported complete.

Current direction reportedly became:

```text
Work
  ↓
internal/codeexec
  ↓
Code capability
```

Active Work no longer depends on `internal/run`.

Legacy human-facing Code TUI/review/session surfaces were reportedly removed.

CLI and TUI share the Work Code path.

Do not rerun or reapply this destructive migration.

Do not resurrect a second Code product.

---

## 11.3 Durable delivery/retry

Reported complete.

The repository reportedly contains `internal/delivery` with durable:

```text
delivery.Record
delivery.Effect
delivery.Attempt
```

and states including:

```text
pending
applied
failed
unknown
superseded
```

Reported behavior:

- artifact delivery remains conflict-aware and atomic;
- code candidate delivery preserves baseline/hash/tree checks;
- retry operates on already-produced Work without rerunning the model;
- applied effects are skipped;
- unknown external actions are not automatically retried;
- CLI and TUI use the same `delivery.Store`;
- history references delivery state instead of copying it.

External connector actions reportedly remain immediate approval-bound actions with intentionally non-persisted executable payloads.

This conservative boundary is intentional.

---

## 11.4 Current evaluation-consolidation tranche

At the time this document was created, the next active tranche was:

> **Transaction fidelity and evaluation consolidation**

A separate prompt was prepared to:

- distinguish task success from transaction correctness;
- distinguish verification from delivery;
- evaluate partial delivery;
- verify safe retry;
- protect held-out cases from accidental normal runs;
- consolidate legacy evaluator paths;
- create fast deterministic product gates.

**The six prompts below assume this tranche has just been completed or is being completed separately.**

Before Prompt 1, inspect its final report and repository state.

If the evaluation-consolidation tranche is incomplete, do not pretend otherwise.

---

# 12. Target Development Sequence After Evaluation Consolidation

The intended remaining sequence is:

```text
1. Inspectable learning foundation
        ↓
   PRODUCT CHECKPOINT
        ↓
2. Learning from transaction outcomes
        ↓
   PRODUCT CHECKPOINT
        ↓
3. Learning evals + anti-overgeneralization
        ↓
   PRODUCT CHECKPOINT
        ↓
4. Product/UX simplification
        ↓
   PRODUCT CHECKPOINT
        ↓
5. Lightweight/bloat/runtime pass
        ↓
   PRODUCT CHECKPOINT
        ↓
6. Final product checkpoint + regression hardening
```

These are six substantial tranches.

Do them sequentially.

---

# 13. Model Guidance

Recommended local model effort:

| Tranche | Suggested effort |
|---|---|
| 1. Inspectable learning foundation | Terra XHIGH |
| 2. Learning from transactions | Terra XHIGH |
| 3. Learning evals | Terra XHIGH |
| 4. Product/UX simplification | Terra XHIGH initially; HIGH for small follow-ups |
| 5. Lightweight/bloat/runtime pass | Terra HIGH, escalate to XHIGH if architectural |
| 6. Final product checkpoint/hardening | Terra XHIGH |

Use XHIGH where architecture, deletion, evaluation design, or cross-cutting semantics are involved.

Use HIGH for narrow repairs after a boundary is already established.

---

# PROMPT 1 — GTR-LEARN-01: Inspectable Learning Foundation

## Entry condition

Begin only after the transaction/evaluation-consolidation tranche has been reviewed.

Do not assume the current evaluator structure from this document. Inspect it.

## Objective

Introduce the **smallest inspectable learning substrate** that can later learn from Work outcomes.

Do **not** implement autonomous self-improvement yet.

The goal is to establish explicit, user-controllable primitives for:

```text
facts / observations
candidate learnings
active learnings
```

with:

```text
provenance
scope
confidence where inferred
status
precedence
enable/disable
editability
```

without creating an opaque memory system.

---

## 1. First inspect the current repository

Search for existing concepts such as:

```text
memory
preference
profile
instructions
agents.json
skills
rules
conversation summaries
user settings
project instructions
feedback
review feedback
history-derived context
```

Also inspect:

```text
workhistory
worksession
delivery
evaluation
configuration
state conventions
TUI settings/history surfaces
```

Determine what already exists and can be reused.

Do not create a parallel preference system if a coherent explicit-user guidance primitive already exists.

---

## 2. Required conceptual distinction

Do not conflate:

```text
HISTORICAL FACT
```

with:

```text
ACTIVE LEARNING
```

Examples:

Historical fact:

```text
Work 183 was rejected because the user wanted Markdown.
```

Candidate inference:

```text
User may prefer Markdown for research reports.
```

Active explicit rule:

```text
For research reports, default to Markdown.
```

Disabling the rule must not erase the historical feedback.

---

## 3. Initial learning types

Support the smallest useful set aligned with the product thesis:

```text
preference
environment fact
procedure / skill
failure-prevention rule
```

Do not build arbitrary ontology infrastructure.

If one or two shared structural fields can support all four, prefer that.

---

## 4. Scope

Every learning must be scoped.

Support the smallest coherent subset of:

```text
global/user
project
repository
directory
task type
conversation
```

based on existing product abstractions.

At minimum, avoid a design where every learned rule silently becomes global.

Scope resolution must be deterministic and testable.

---

## 5. Provenance

Every inferred/candidate learning should be able to answer:

```text
Why does this exist?
Which Work transaction(s) contributed?
Was it user-authored or inferred?
What feedback/evidence supported it?
When was it created/changed?
```

References are preferable to copying full payloads.

---

## 6. Precedence

Implement an explicit precedence model.

The direction is:

```text
current explicit user instruction
        >
explicit user-maintained rule/preference
        >
active learned rule
        >
weak candidate/observation
```

Exact integration with model context should remain minimal in this tranche.

Do not silently inject every stored learning into every prompt.

The main task is to establish trustworthy primitives and resolution.

---

## 7. User control

A user must be able to:

```text
list learnings
inspect one
see provenance
enable
disable
edit where appropriate
remove/reject candidate inference
add explicit learning directly
```

Use simple local-first state.

Human-readable Markdown or JSON/YAML may be appropriate depending on existing repository conventions.

Do not introduce a database unless current architecture already strongly requires it.

If direct file editing is part of the design, define how invalid edits are handled safely.

---

## 8. CLI/TUI parity

Every normal learning-management action exposed in CLI must have a TUI equivalent.

Normal user surface should likely include:

```text
Learnings
```

Do not build a giant knowledge-management UI.

A focused list/detail/edit/enable-disable flow is sufficient.

Both interfaces must use one shared service.

No TUI shelling out to CLI.

---

## 9. No autonomous derivation yet

This is strict.

Do NOT yet make Gator automatically inspect every transaction and create candidate lessons.

Do NOT build reflection prompts.

Do NOT auto-promote rules.

Prompt 2 will connect transactions to learning.

Prompt 1 builds the substrate and user controls.

---

## 10. Context integration

Add only the smallest safe mechanism required to prove active explicit learnings can influence future Work.

Prefer a bounded context projection such as:

```text
applicable active learnings for this Work scope
```

Do not dump the full learning store into model context.

Do not consume candidates as if they were active truth.

---

## 11. Deterministic tests

At minimum prove:

1. user-authored learning can be created;
2. candidate learning is distinguishable from active learning;
3. disabling an active learning stops it from being selected;
4. historical provenance remains after disable;
5. project/repository-scoped rule does not affect unrelated scope;
6. explicit user-authored rule outranks conflicting inferred rule;
7. applicable active learning can be projected into Work context;
8. candidate learning is not automatically projected;
9. persistence round-trips;
10. CLI/TUI use the same service;
11. no raw transaction payload is duplicated unnecessarily.

---

## 12. Avoid overengineering

Do not create:

- vector databases;
- embeddings;
- semantic memory stores;
- RAG infrastructure;
- graph databases;
- generic policy engines;
- ontology frameworks;
- automatic reflection agents;
- autonomous promotion.

This is a **trustworthy inspectable state model**, not “AI memory platform v1”.

---

## 13. Required final report

Return:

### A. Classification
`COMPLETE`, `PARTIAL`, or `BLOCKED`

### B. Previous Guidance/Memory State
What existed before.

### C. Learning Model
Exact types/files/statuses/scopes.

### D. Provenance
How each learning links back to evidence or user authorship.

### E. Precedence
Exact deterministic order.

### F. User Control
CLI and TUI interactions.

### G. Work Context Integration
What active learning is projected and when.

### H. Tests / Evals
Exact commands/results.

### I. Complexity Delta
Files/lines/concepts added/removed.

### J. Product Checkpoint
Answer:

1. Can every active learning be inspected?
2. Can it be disabled/reversed?
3. Can the user accelerate learning directly?
4. Are facts separate from inference?
5. Is scope explicit?
6. Did we accidentally build opaque memory infrastructure?
7. Is CLI/TUI parity intact?
8. What should be simplified before automatic learning?

### K. Recommended Next Tranche
Assess readiness for Prompt 2.

### L. Git Status
Do not commit/push unless explicitly instructed.

Stop after the report.

---

# PROMPT 2 — GTR-LEARN-02: Learn From Transaction Outcomes

## Objective

Connect Gator's existing Work evidence to the learning substrate.

The goal is:

> **Turn trustworthy transaction outcomes and explicit feedback into candidate learnings without silently changing behavior.**

The desired loop is:

```text
Work transaction
      ↓
outcome / verification / delivery / feedback
      ↓
structured observation
      ↓
candidate learning
      ↓
user/eval review
```

Do not auto-promote consequential learnings.

---

## 1. Inspect current feedback signals

Trace which signals actually exist after the previous tranches:

```text
Work success/failure
verification pass/fail
delivery applied/failed/unknown
review acceptance/rejection
user edits
retry
conversation correction
explicit learning edits
```

Do not invent signals the product cannot observe.

Classify signals by reliability.

Examples:

High reliability:

```text
test failed
user explicitly rejected result
delivery conflict occurred
user explicitly wrote preference
```

Lower reliability:

```text
user immediately requested another format
user rephrased task
model guessed dissatisfaction
```

Do not treat ambiguous behavior as hard truth.

---

## 2. Observation model

Add the smallest structured representation needed for learning-relevant observations.

An observation should answer:

```text
what happened?
which Work transaction?
which evidence supports it?
what scope might it belong to?
how reliable is the signal?
```

Do not copy full Work state.

Do not make every event into an observation.

Only extract signals useful for future behavior.

---

## 3. Candidate generation

Create candidate learnings from bounded rules or model-assisted reflection only where justified.

Prefer deterministic candidate derivation for obvious cases.

Examples:

```text
repeated user output-format corrections
repository-specific command conventions
verification failure caused by missing required check
explicit user correction
repeated successful procedure
```

If using a model to propose a candidate:

- feed it bounded evidence;
- require structured output;
- make the candidate visibly inferred;
- never treat the model proposal as active truth;
- retain provenance.

---

## 4. Promotion policy

Do not create one universal threshold.

Start conservatively.

Suggested principle:

```text
explicit user-authored:
  active immediately

explicit user approval of candidate:
  active

strong repeated evidence:
  candidate may become recommended for promotion

single inferred event:
  candidate only
```

Automatic activation should be extremely limited in this tranche.

If the simplest trustworthy implementation has **no automatic promotion**, that is acceptable.

---

## 5. Rejection / correction

A rejected candidate should remain attributable as historical decision if useful but must not continue influencing Work.

If a user edits a candidate before activation, record the resulting explicit rule as user-authored or user-confirmed rather than pretending the inference was perfect.

---

## 6. Failure learning

This is particularly important.

For a failure such as:

```text
Code change violated repository invariant Y.
Test Z caught it.
```

Gator should be able to derive a candidate such as:

```text
Scope: repository X
Rule: when changing subsystem A, run test Z before broader verification.
```

But only if evidence supports that conclusion.

Do not fabricate causal explanations from one failure.

---

## 7. Delivery learning

Delivery outcomes may teach operational lessons.

Examples:

```text
destination path repeatedly conflicts
certain file should never be overwritten
external action outcome became unknown
```

Do not learn unsafe “retry harder” rules from uncertain external effects.

---

## 8. User feedback UX

Create a very low-friction feedback path.

Examples conceptually:

```text
accept
reject
correct
"remember this"
"don't learn this"
```

Do not force users to manage formal schemas during ordinary Work.

The TUI should make high-value feedback easy.

The CLI should have equivalent capability.

---

## 9. “Remember this” acceleration

Support an explicit user action that promotes a correction/preference into durable learning.

For example, the user should be able to say conceptually:

```text
Remember: for this repo, use pnpm.
```

and have that become explicit scoped knowledge rather than waiting for repeated inference.

Integrate with the existing user-authored learning path rather than inventing a second shortcut store.

---

## 10. Transaction linkage

Every candidate derived from Work must reference the relevant Work transaction(s).

A future `why` surface should be able to explain:

```text
This rule exists because:
- Work A was rejected
- Work B repeated the same correction
- user approved the candidate
```

Do not implement elaborate “why” UI if not necessary yet, but preserve the data needed.

---

## 11. Deterministic tests

At minimum prove:

1. explicit negative feedback creates an observation;
2. observation links to Work history;
3. obvious deterministic correction can create a candidate;
4. candidate is not active by default;
5. user approval activates it;
6. user rejection prevents use;
7. user direct “remember this” creates explicit scoped learning;
8. Work failure and delivery failure remain distinct signals;
9. `unknown` external outcome does not generate unsafe retry learning;
10. repeated evidence can be represented without duplicating candidates uncontrollably;
11. provenance remains inspectable;
12. future Work receives only active applicable learning.

---

## 12. Do not build learning evals deeply yet

Add focused regression tests, but Prompt 3 is specifically for evaluating whether learning improves behavior and avoids overgeneralization.

Do not let this tranche expand into a giant benchmark project.

---

## 13. Required final report

### A. Classification

### B. Signals Consumed
List actual trustworthy signals.

### C. Observation Model

### D. Candidate Derivation
Deterministic vs model-assisted.

### E. Promotion / Rejection

### F. User Feedback UX
CLI + TUI parity.

### G. “Remember This”

### H. Transaction Provenance

### I. Safety
Especially uncertain external actions and over-inference.

### J. Tests / Verification

### K. Complexity Delta

### L. Product Checkpoint
Answer:

1. Does Gator learn from actual evidence rather than chat vibes?
2. Can one bad event silently become global behavior?
3. Can the user override/accelerate learning easily?
4. Is candidate vs active state clear?
5. Can every inferred rule explain its provenance?
6. Did output quality or UX regress?
7. Is the system ready to evaluate learning effectiveness?

### M. Recommended Next Tranche
Prompt 3 if ready.

### N. Git Status

Stop.

---

# PROMPT 3 — GTR-LEARN-EVAL-01: Learning Effectiveness and Anti-Overgeneralization

## Objective

Prove that Gator's learning system actually improves relevant future Work without contaminating unrelated Work.

This is where “memory” becomes an evaluated product capability rather than a storage feature.

The key experimental structure is:

```text
Episode A
  mistake

Feedback
  correction

Learning
  candidate / activation

Episode B
  relevant similar task

Expected:
  mistake does not recur

Episode C
  superficially similar but materially different task

Expected:
  learning does NOT overgeneralize
```

---

## 1. Evaluation principles

Prefer deterministic fixtures where possible.

Do not require live expensive model inference for every learning invariant.

Separate:

```text
learning-mechanism correctness
```

from:

```text
semantic behavioral improvement with real models
```

The first should be fast and deterministic.

The second can be slower and selectively run.

---

## 2. Required evaluation dimensions

At minimum:

```text
retention
relevance
scope
precedence
correction
reversal
non-overgeneralization
behavioral improvement
provenance
```

Do not reduce all learning evaluation to one score.

---

## 3. Core scenarios

Create a focused suite covering cases such as:

### Scenario A — Repository convention

Episode A:

```text
Gator uses npm.
User corrects: this repo uses pnpm.
```

Learning:

```text
repo-scoped package manager preference
```

Episode B, same repo:

Expected: pnpm.

Episode C, unrelated repo:

Expected: no forced pnpm assumption.

---

### Scenario B — Output-format preference

Episode A:

```text
research report delivered in PDF
user explicitly asks for Markdown in this task category
```

Test whether the learned preference is correctly scoped.

Do not assume global preference unless user made it global.

---

### Scenario C — Failure-prevention rule

Episode A:

```text
change breaks a specific subsystem
targeted test catches it
user/explicit logic approves candidate rule
```

Episode B:

Relevant change should trigger that targeted verification.

Episode C:

Unrelated work should not unnecessarily run the rule.

---

### Scenario D — User reversal

Active learning exists.

User disables/reverses it.

Future Work must stop applying it.

Historical evidence remains intact.

---

### Scenario E — Explicit user knowledge overrides inference

Inferred learning conflicts with direct user-maintained rule.

Expected:

```text
explicit user rule wins
```

---

### Scenario F — Conflicting scoped rules

Repository-specific rule conflicts with global preference.

Expected behavior should follow explicit precedence/specificity policy.

Test it.

---

### Scenario G — Candidate only

Candidate exists but was never activated.

Future Work must not treat it as active.

---

### Scenario H — Delivery uncertainty

Unknown external delivery outcome must not produce or activate a rule encouraging automatic retry.

---

## 4. Counterfactual testing

Where practical, run behavior with and without the active learning.

The evaluator should be capable of answering:

```text
Did the learning actually cause improvement?
```

rather than merely:

```text
Was the learning present?
```

Use ablation if existing eval machinery supports it.

---

## 5. Avoid benchmark gaming

Do not tune the learning engine directly to exact fixture wording.

Prefer semantic/scoped signals.

Keep held-out learning scenarios genuinely held out where practical.

---

## 6. Held-out discipline

Use the split/holdout mechanism established in evaluation consolidation.

Do not consume all held-out learning cases during ordinary development.

A small held-out set should test:

- generalization;
- non-overgeneralization;
- conflicting rules;
- reversal.

---

## 7. Regression from real failures

Establish a simple manual path for converting a real sanitized failure into a development regression case.

Do not auto-generate eval cases autonomously yet.

The desired workflow can be something like:

```text
select Work failure
  ↓
sanitize/minimize
  ↓
create development fixture
  ↓
run regression
```

Keep it simple.

---

## 8. Diagnostics

When a learning eval fails, report why.

Prefer:

```text
FAIL: scope_leak
learning repo:X applied to repo:Y
```

over:

```text
learning score: 0.42
```

---

## 9. Semantic evals

If model-based end-to-end evals are used:

- keep them separate from the fast deterministic suite;
- make provider/model explicit;
- avoid treating one run as statistically conclusive;
- preserve reproducibility metadata;
- use repeated trials only where useful.

Do not turn this into a research benchmark framework.

---

## 10. Required final report

### A. Classification

### B. Learning Eval Architecture

### C. Deterministic Scenarios
Report each required scenario.

### D. Counterfactual/Ablation Support

### E. Held-Out Strategy

### F. Real-Failure Regression Workflow

### G. Fast vs Slow Suite

### H. Findings
What actually improves and what remains weak.

### I. Overgeneralization Findings

### J. Tests / Verification

### K. Complexity Delta

### L. Product Checkpoint
Answer:

1. Can we prove a correction changes future relevant behavior?
2. Can we prove unrelated tasks remain unaffected?
3. Can user reversal be verified?
4. Does explicit knowledge reliably beat inference?
5. Are candidates prevented from leaking into behavior?
6. Can failures become regressions?
7. Are the tests measuring product behavior rather than storage internals?
8. Is learning now good enough to become a core user-facing feature?

### M. Recommended Next Tranche
Proceed to UX simplification only if the learning model has earned it.

### N. Git Status

Stop.

---

# PROMPT 4 — GTR-PRODUCT-UX-01: Collapse the Product Surface Around Work

## Objective

Now that Work, History, delivery, evaluation, and Learnings have coherent semantics, simplify the user-facing product.

This tranche is not about making the TUI prettier.

It is about ensuring Gator has **one obvious way to do important things**.

Target mental model:

```text
Work
History
Learnings
Jobs
Settings
```

with advanced functionality available without polluting the normal path.

---

## 1. Audit current CLI/TUI again

List every top-level CLI family and meaningful TUI surface.

Classify each as:

```text
NORMAL
ADVANCED
INTERNAL
DELETE
MERGE
```

Do not preserve implementation terminology in normal navigation.

Examples of concepts that often belong behind advanced/internal surfaces:

```text
RPC
ACP
provider transports
MCP trust internals
LSP internals
extension plumbing
state repair
low-level snapshot commands
protocol servers
```

---

## 2. CLI/TUI parity matrix

Build a fresh matrix after all previous architectural changes.

For each meaningful capability:

```text
capability
CLI path
TUI path
shared service?
normal/advanced/internal
recommended action
```

Fix actual parity gaps.

Do not create TUI duplicates for commands that should instead become internal.

---

## 3. Work submission should be trivial

A user should be able to do:

```bash
gator "research X and produce a report"
```

or open:

```bash
gator
```

and start Work naturally.

Do not require understanding provider topology, specialist identity, or transaction internals.

---

## 4. History

History should expose:

- Work objective summary;
- status;
- verification;
- delivery state;
- relevant artifacts;
- feedback;
- active/candidate learnings derived from it where useful.

Do not turn History into a raw state-file inspector.

Advanced drill-down can expose IDs/evidence.

---

## 5. Learnings

The Learnings UX should make it easy to:

```text
list
filter by scope/status
inspect provenance
enable
disable
edit
approve/reject candidate
add explicit rule
```

Do not build a knowledge-base product.

---

## 6. Jobs

Jobs should remain:

```text
schedule/repeat Work
```

not a second runtime.

Ensure normal job creation/edit/run/history is coherent in both CLI and TUI.

Advanced supervisor/platform integration can remain hidden.

---

## 7. Settings

Settings should separate normal preferences from advanced plumbing.

Normal settings might include:

- default provider/model;
- default mode;
- output preferences;
- learning controls;
- UI preferences.

Advanced settings may include:

- connector setup;
- MCP/LSP trust;
- browser/desktop configuration;
- telemetry/export;
- experimental integrations.

Do not expose every config key as equal product concepts.

---

## 8. TUI simplification

Measure current TUI complexity.

Delete screens/panels/commands that exist only for removed architecture.

Prefer contextual actions over giant command palettes.

Do not sacrifice keyboard-first efficiency.

The TUI should remain usable over SSH and in ordinary terminal sizes.

---

## 9. Provider UX

Provider selection/setup should be simple and shared across CLI/TUI.

Do not do provider-pruning in depth yet unless the UX audit reveals obviously dead surfaces.

Hide protocol details from normal users.

---

## 10. Product terminology

Normal users should not need to understand:

```text
transaction record
delivery ledger
candidate ID
revision DAG
RPC server
agent topology
```

unless they choose advanced inspection.

Translate implementation detail into coherent product language.

---

## 11. Deletion requirement

This tranche should likely remove meaningful UI/command bloat.

Track:

```text
commands removed
aliases removed
TUI commands/screens removed
duplicate UI logic removed
shared services reused
```

Do not merely reorganize menus while retaining every old concept.

---

## 12. Output quality safeguard

UX refactoring must not change core Work semantics.

Run representative:

- general Work;
- Code Work;
- review/apply/retry;
- learning;
- jobs.

---

## 13. Required final report

### A. Classification

### B. Before Product Surface

### C. After Product Surface

### D. CLI/TUI Parity Matrix

### E. Normal vs Advanced vs Internal

### F. Commands/UI Removed

### G. Work Submission UX

### H. History UX

### I. Learnings UX

### J. Jobs UX

### K. Settings UX

### L. Tests / Evals

### M. Complexity Delta

### N. Product Checkpoint
Answer:

1. Can a new user understand Gator without knowing implementation architecture?
2. Is there one obvious way to start Work?
3. Can every meaningful normal CLI operation be done in TUI?
4. Are advanced/internal concepts hidden appropriately?
5. Did we remove UI rather than merely rearrange it?
6. Is Gator still keyboard-first and lightweight?
7. Did output quality remain intact?

### O. Recommended Next Tranche
Proceed to lightweight/bloat pass.

### P. Git Status

Stop.

---

# PROMPT 5 — GTR-LIGHT-01: Resource, Dependency, and Bloat Reduction

## Objective

Make Gator materially lighter to build, run, maintain, and understand without sacrificing the product thesis.

This is where “lightweight” becomes an engineering property rather than a slogan.

Do not optimize tiny allocations while large architectural costs remain.

---

## 1. Establish measurable baseline

Measure, where practical:

```text
binary size
startup latency
idle RAM for ordinary TUI
steady-state RAM during simple Work
package/dependency count only where meaningful
background processes
external runtimes required
test duration
build duration
```

Do not obsess over microbenchmarks.

Use reproducible commands and report environment.

---

## 2. Audit heavy subsystems

Inspect especially:

```text
browser / Playwright / Chromium
desktop integration
app/server processes
RPC/ACP remnants
extensions
MCP
LSP
provider breadth
Google helper/mirror state
telemetry exporters
optional cloud integrations
```

Classify each as:

```text
CORE
OPTIONAL
EXPERIMENTAL
DELETE
```

---

## 3. Provider pruning

Reassess actual provider surface.

Ask:

> If this provider integration disappeared, would Gator's core product thesis materially weaken?

Prefer:

- a curated set of excellent native providers;
- one compatible/custom endpoint path where useful.

Do not preserve adapters merely for logo count.

Do not delete a provider with demonstrated user value without reporting the tradeoff.

---

## 4. Browser/desktop

These can be valuable Work capabilities but are expensive.

Ensure they are:

- optional;
- lazily initialized;
- not mandatory for ordinary startup;
- not running background processes when unused;
- isolated from the core binary/runtime as much as practical.

If one has low product value relative to burden, recommend deletion.

---

## 5. Servers/protocols

Revisit any remaining:

```text
RPC
ACP
app server
daemon/supervisor
```

Now that Work is canonical, ask whether each serves an actual user or integration.

Delete obsolete transports aggressively.

If ACP/editor integration remains valuable, ensure it projects Work rather than creating a second runtime.

---

## 6. Google scope

Retain Drive/Calendar context if useful.

Reassess product drift such as:

```text
reminders
quick capture
task-manager behavior
mirrored SQLite state
assistant-specific workflows
```

If those remain and do not strengthen the Work thesis, simplify or remove them.

---

## 7. State cleanup

Audit persistent roots again.

Aim for understandable organization centered around:

```text
Work/history
artifacts/evidence
deliveries
learnings
jobs
config/credentials
optional capabilities
```

Do not undertake gratuitous migration if state layout is already adequate.

Remove obsolete state writers/readers from deleted features.

---

## 8. Dependency cleanup

Run Go dependency/package analysis.

Remove dependencies made dead by earlier deletions.

Avoid adding replacements.

Check whether optional heavy integrations force dependencies into ordinary execution unnecessarily.

---

## 9. Startup

Trace normal:

```bash
gator
gator "simple task"
gator work list
```

Ensure ordinary paths do not initialize:

- browser;
- desktop;
- remote integrations;
- heavyweight protocol servers;
- provider adapters not needed.

---

## 10. Resource targets

Do not invent arbitrary vanity targets.

Report actual before/after values.

The desired qualitative direction is:

```text
fast startup
low idle memory
no hidden daemon requirement
few child processes when idle
single local binary as the center
```

---

## 11. Testing

Ensure optionalization does not break:

- Work;
- Code;
- History;
- delivery/retry;
- Learnings;
- evals;
- Jobs;
- CLI/TUI parity.

Add startup/lazy-initialization tests where reasonable.

---

## 12. Required final report

### A. Classification

### B. Baseline Measurements

### C. Highest-Cost Subsystems

### D. Deletions

### E. Optionalizations/Lazy Initialization

### F. Provider Surface

### G. Browser/Desktop

### H. Server/Protocol Cleanup

### I. Google Scope

### J. State/Dependency Cleanup

### K. Before/After Measurements

### L. Tests / Evals

### M. Complexity Delta

### N. Product Checkpoint
Answer:

1. Is ordinary Gator materially lighter?
2. Did startup improve or remain excellent?
3. Are heavy capabilities optional?
4. Is a daemon required? If yes, why?
5. Did we remove providers/integrations with weak value?
6. Did we preserve output quality?
7. Is the codebase easier to develop in?
8. What substantial bloat remains?

### O. Recommended Next Tranche
Final hardening only.

### P. Git Status

Stop.

---

# PROMPT 6 — GTR-RELEASE-01: Final Product Checkpoint and Regression Hardening

## Objective

Perform a final cross-product review of Gator against the agreed thesis.

This is not a feature tranche.

This is a **prove, simplify, and harden** tranche.

The goal is to establish whether Gator now behaves like one coherent product:

> **a lightweight local-first runtime for auditable personal AI Work that can learn from outcomes without opaque self-modification.**

---

## 1. Reconstruct the current architecture from code

Do not rely on this handoff's historical diagrams.

Map the actual current system.

Show:

```text
CLI / TUI / Jobs
        ↓
Work
        ↓
transaction/history
        ↓
agent + capabilities + Code
        ↓
artifacts / evidence
        ↓
verification
        ↓
delivery/retry
        ↓
feedback
        ↓
learning
        ↓
evaluation
```

Call out any parallel runtimes or duplicate lifecycle models that remain.

---

## 2. Product thesis audit

Assess each claim:

### Lightweight
Is it actually lightweight?

### Local-first
Can ordinary use remain local?

### General Work
Can it perform useful non-code Work?

### Strong Code
Is Code still excellent rather than degraded?

### Auditable
Can a user inspect what happened?

### Transactional
Are execution and delivery outcomes explicit?

### Replayable
Can safe delivery retry without regeneration?

### Learnable
Can evidence become scoped candidate learning?

### Reversible learning
Can the user disable/edit what Gator learned?

### Evaluated
Can the system prove learning helped rather than merely storing it?

---

## 3. End-to-end regression suite

Build or consolidate a representative product-level suite.

At minimum cover:

### General Work
Produce a bounded artifact from supplied context.

### Code Work
Modify code, verify it, produce candidate, deliver safely.

### Failed Work
History remains truthful.

### Verified but undelivered
Result persists.

### Partial delivery
Statuses remain truthful.

### Retry
No Work regeneration.

### Conflict
User state protected.

### Unknown external effect
No unsafe automatic retry.

### Feedback
User correction becomes candidate learning.

### Activation
Approved learning affects relevant Work.

### Scope
Unrelated Work is unaffected.

### Reversal
Disabled learning stops affecting Work.

### Explicit override
User-authored rule outranks inference.

### Job
Scheduled Work uses same semantics.

### CLI/TUI parity
Representative operations match.

---

## 4. Real output quality

Do not only test plumbing.

Select a small representative set of real or realistic Work tasks that exercise:

- research;
- artifact production;
- coding;
- follow-up correction;
- learning.

Use current evaluation infrastructure.

If semantic quality is model-dependent, report provider/model explicitly.

Do not overstate statistical conclusions from tiny samples.

---

## 5. Failure review

Inspect recent failures from development/evals.

For each important failure ask:

```text
Is this:
- product bug?
- model limitation?
- missing context?
- bad evaluation?
- poor UX?
- unsafe learning?
- unnecessary complexity?
```

Fix only high-value issues that fit this tranche.

Do not start major new architecture.

---

## 6. Delete remaining fossils

Run one final brutality test:

> If this subsystem did not exist today, would we build it for this product?

Identify and remove remaining clearly obsolete:

- commands;
- packages;
- adapters;
- state writers;
- compatibility shims;
- server paths;
- UI remnants.

Do not destabilize the product merely to improve deletion counts.

---

## 7. Documentation / discoverability

Ensure the code and user-facing help explain the product that now exists.

Normal user documentation/help should emphasize:

```text
Work
History
Review/Apply/Retry
Learnings
Jobs
Settings
```

and not center obsolete architecture.

Do not turn this into a documentation rewrite if docs are out of scope; at minimum update misleading surfaces touched by the product.

---

## 8. Performance regression check

Repeat baseline measurements from Prompt 5.

Ensure final hardening did not regress:

- startup;
- idle RAM;
- binary size materially without reason;
- background process count;
- test/runtime ergonomics.

---

## 9. Security/authority sanity

Because Gator is single-user, do not add enterprise complexity.

But verify:

- inspect/draft/act-style authority remains coherent;
- external unknown effects are conservative;
- learning cannot silently grant authority;
- user knowledge cannot inject hidden executable payloads without normal tool/authority checks;
- credentials remain private.

---

## 10. Learning safety sanity

Verify:

```text
observation ≠ active truth
candidate ≠ active truth
disabled means disabled
scope means scope
explicit user instruction wins
historical evidence remains immutable
```

This is critical.

---

## 11. Final architecture scorecard

Do not give arbitrary numeric scores.

Use:

```text
STRONG
ADEQUATE
WEAK
MISSING
```

for technical areas, not as political-style ranking.

Assess:

```text
Work coherence
Code capability
Auditability
Delivery/recovery
Learning
Evaluation
CLI/TUI parity
Resource footprint
Development simplicity
Optional integrations
```

Explain evidence.

---

## 12. Final required report

### A. Classification
`READY`, `READY WITH KNOWN GAPS`, or `NOT READY`

### B. Current Architecture

### C. Product Thesis Audit

### D. End-to-End Regression Results

### E. Output Quality Findings

### F. Learning Findings

### G. Evaluation Findings

### H. Resource Measurements

### I. CLI/TUI Parity

### J. Remaining Bloat

### K. Deletions Made

### L. Known Gaps
Only real gaps, not wishlists.

### M. What Not To Build Yet
Explicitly list seductive but premature directions.

Likely examples may include:

- autonomous policy rewriting;
- huge memory/RAG infrastructure;
- many more providers;
- social/chat channel expansion;
- complex workflow DSL;
- distributed/enterprise multi-user authorization.

Adapt based on current evidence.

### N. Next Product Research Questions
Do not automatically implement them.

### O. Git Status

Stop.

---

# 14. Guidance for Fresh Codex Agents

A fresh Codex agent must follow these rules.

## 14.1 Repository beats handoff

If this document says one thing and the code clearly says another:

1. inspect carefully;
2. determine whether the repo represents a deliberate newer change;
3. trust current working code;
4. report the discrepancy.

Do not “restore” old architecture from this document.

---

## 14.2 Do not rerun completed destructive migrations

Do not reintroduce:

- legacy Code product runtimes;
- legacy Code TUI/review/session surfaces;
- old `internal/run` dependencies in active Work;
- duplicated CLI/TUI apply paths.

If they are already gone, keep them gone unless evidence shows a regression.

---

## 14.3 Preserve uncommitted work

Several tranches may exist together in the working tree.

Never assume uncommitted means disposable.

---

## 14.4 Prefer deletion over compatibility scaffolding

This is an early product.

Do not build extensive compatibility infrastructure for obsolete development-state users without concrete evidence of need.

---

## 14.5 Do not confuse auditability with chain-of-thought logging

Do not persist hidden model reasoning.

Persist observable structured evidence.

---

## 14.6 Do not let “learning” become prompt accumulation

A giant text blob appended to every request is not the target.

Learning must be:

- scoped;
- inspectable;
- attributable;
- reversible;
- evaluated.

---

## 14.7 Do not let evals become benchmark theater

The most valuable eval is one that catches a real product regression.

Prefer deterministic product invariants where possible.

Model judges are supplementary.

---

## 14.8 Do not let optional integrations dominate architecture

Browser, desktop, MCP, LSP, Google, extensions, ACP, and similar integrations exist to make Work better.

Work does not exist to justify integrations.

---

# 15. Desired End-State Mental Model

A normal user should eventually be able to understand Gator roughly as:

```text
Gator
│
├── Work
│     ├── ask Gator to do something
│     ├── inspect progress/result
│     ├── review changes
│     ├── apply/retry
│     └── give feedback
│
├── History
│     ├── what happened
│     ├── evidence
│     ├── verification
│     └── delivery outcome
│
├── Learnings
│     ├── explicit preferences
│     ├── inferred candidates
│     ├── active rules
│     ├── scopes
│     └── provenance / disable / edit
│
├── Jobs
│     └── repeated or scheduled Work
│
└── Settings
      ├── provider/model
      ├── authority defaults
      ├── integrations
      └── advanced configuration
```

Code, browser, desktop, Google context, MCP, LSP, specialists, providers, and protocol adapters are capabilities beneath this product—not competing product identities.

---

# 16. Desired Learning Explanation

A long-term user-facing explanation should be possible in a form like:

```text
Why did Gator do this?

1. This repository explicitly says to use pnpm.
2. You approved learning L-42 after correcting Work W-183.
3. Two subsequent Work transactions confirmed the same convention.
4. The rule is scoped to this repository.
5. It does not apply to unrelated repositories.
```

That is the target:

> **auditable personalization**

not magical hidden memory.

---

# 17. Desired Failure-to-Improvement Loop

The eventual system should make this natural:

```text
Gator makes mistake
      ↓
verification/user catches it
      ↓
transaction retains evidence
      ↓
candidate lesson is proposed
      ↓
user/eval approves or rejects it
      ↓
regression case proves future behavior
      ↓
relevant future Work improves
```

This is the core self-improvement story.

No model retraining is required for the first useful version.

Improvement can come from:

- better context selection;
- explicit preferences;
- repository facts;
- reusable procedures;
- failure-prevention rules;
- tool-selection heuristics;
- verification requirements;
- provider/model choice later, if measured.

---

# 18. What Success Looks Like After All Six Tranches

Gator should feel like one coherent product, not a collection of agent infrastructure.

A successful end state should satisfy:

```text
ONE canonical Work runtime
ONE coherent user-facing CLI/TUI model
ONE audit root per Work execution
DURABLE verification and delivery state
SAFE retry without unnecessary regeneration
INSPECTABLE scoped learning
USER control over learned behavior
EVALS proving learning helps and does not overgeneralize
STRONG coding capability beneath Work
OPTIONAL heavy integrations
LOW operational overhead
NO mandatory daemon
NO opaque self-modification
```

And, importantly:

> The system should be easier to understand after these changes than before them.

If Gator becomes more sophisticated but substantially harder to understand, the architecture has likely gone wrong.

---

# 19. Final Instruction to the Local Codex Agent

Do not treat this as a six-prompt batch job to execute unattended.

Execute **one tranche**.

Return the required report.

Then stop.

The human will review whether the product is still going in the right direction.

The correct response to a discovered bad assumption is not to preserve the roadmap.

It is to surface the evidence and change course.

The product direction is more important than completing the plan.

---

# GTR-LEARN-01 Checkpoint Report — 2026-09-24

## A. Classification

`COMPLETE`

## B. Previous Guidance/Memory State

Before this tranche, repository guidance came from project-owned instruction
files and selected Code profiles (`internal/instructions`), while Work history
retained privacy-bounded transaction evidence (`internal/workhistory`). Neither
was a user-controlled, scoped learning store, and no candidate or active
learning was projected into ordinary future Work.

The evaluation-consolidation prerequisite is present in the repository:
`internal/eval/transaction_test.go` separates Work, verification, delivery,
recovery, and unknown external outcomes; the release evidence documents
development/held-out evaluation separation. No legacy evaluator was revived.

## C. Learning Model

The new file-backed store is `internal/learning`, rooted at:

```text
$GATOR_STATE_DIR/gator/learnings/<learning-id>.json
```

The directory is mode `0700`; each readable, indented JSON record is mode
`0600` and is atomically updated. The initial model is deliberately small:

```text
types:       preference | environment_fact | procedure | failure_prevention
statuses:    candidate | active | disabled | rejected
origins:     user | inferred
scopes:      global | project (exact absolute selected source path)
```

Inferred records require a 1–100 confidence value and start as `candidate`.
They cannot become active without a recorded explicit user confirmation. Direct
user additions are active immediately. `remove` is deliberately reversible: it
disables a record rather than erasing it.

This tranche represents environment facts as a typed, mutable learning and
keeps immutable historical facts in their existing Work/evidence stores. The
dedicated observation-extraction model remains intentionally deferred to Prompt
2, which is where the roadmap assigns transaction-to-observation derivation.

## D. Provenance

Each record has origin, creation/update timestamps, optional contributing Work
IDs, optional evidence references, and—when a candidate is enabled—the user
confirmation timestamp. These are references only; no raw prompts, tool
arguments/results, attachment bytes, connector payloads, or transaction copies
are added to the learning store.

Hand-editing is supported by the human-readable JSON format, but invalid,
oversized, non-regular, symlinked, unknown-field, or otherwise invalid records
fail closed: they are not projected into Work and the local command reports the
repairable state error. Disabling/rejecting does not modify the referenced Work
history.

## E. Precedence

The deterministic resolution order is:

```text
developer-owned Work contract and current explicit user request
    > active user-authored learning
    > active inferred learning explicitly confirmed by the user
    > candidate / disabled / rejected learning (never selected)
```

For a conflict with the same `(type, key)` identity, user-authored wins; within
the same origin, exact-project scope wins over global; then later update time
and lexical ID break ties. The Work prompt explicitly states that stored
learnings are context rather than authority and that the current request and
contract override them.

## F. User Control

CLI:

```text
gator learnings list
gator learnings show LEARNING_ID
gator learnings add --type TYPE --key KEY --scope global|project[=PATH] TEXT
gator learnings enable|disable|remove|reject LEARNING_ID
gator learnings edit LEARNING_ID [--key KEY] TEXT
```

The Work TUI exposes the same in-process service through the `/learnings`
palette entry and `/learnings …` command. It does not shell out to the CLI.
List/show/add/edit/enable/disable/remove/reject flow through the same
`internal/learning.Store` operations.

## G. Work Context Integration

`applyLearningContext` resolves only active applicable records, renders at most
16 records / 16 KiB, and sets `workrun.Request.LearningContext`. `workrun`
places that bounded block in the manager system prompt before optional
developer additions, with its explicit precedence guard.

The projection is used by direct CLI Work, the interactive TUI, Jobs via
`executeConfiguredWork`, headless Work, and configured live evaluation runs.
Candidates are not projected; a project learning only matches the exact
absolute selected source path. Scripted deterministic evaluator fixtures remain
isolated unless they deliberately supply a context, preserving their fixture
control.

## H. Tests / Evals

Observed successful commands:

```sh
go test ./cmd/gator -run 'Test(Learning|ApplyLearning|WorkCLIProjects)' -count=1
go test ./internal/learning ./internal/workrun ./internal/worktui -count=1
go test ./...
go vet ./...
go build ./cmd/gator
git diff --check
```

The focused tests prove creation, candidate separation, explicit candidate
enablement, disable-with-provenance retention, project-scope isolation,
user-over-inference precedence, rejection/removal, invalid direct-edit
fail-closed behavior, persistence, bounded prompt projection, direct CLI
projection, and the shared TUI callback. The full Go suite passed.

## I. Complexity Delta

The implementation adds one small local package and two CLI files, plus narrow
integration points in the shared Work request/prompt, main dispatch, and TUI
command palette. It does not add a database, daemon, embeddings, vector store,
RAG path, reflection prompt, policy engine, or autonomous promotion loop.

Relative to the roadmap handoff commit `4867f9125`, the observed implementation
commit pair added 1,281 lines and removed 19 lines across 14 files before the
small final correctness/doc corrections left uncommitted for review. The core
new concepts are exactly record, scope, provenance, status, deterministic
selection, and bounded projection.

## J. Product Checkpoint

1. Every active learning is inspectable through JSON or `show`: **yes**.
2. It can be disabled/reversed without deleting Work evidence: **yes**.
3. The user can accelerate learning directly: **yes**, with `learnings add`.
4. Facts are separate from inference: **yes**; immutable Work evidence remains
   separate while mutable typed learnings carry explicit origin.
5. Scope is explicit: **yes**; only global and exact-project scope are enabled.
6. Opaque memory infrastructure was added: **no**.
7. CLI/TUI parity is intact: **yes**, through one in-process store/service path.
8. Simplify before automatic learning: retain only these two scopes until real
   evidence requires more; keep candidate creation tied to explicit bounded
   observations rather than adding free-form reflection or prompt accumulation.

## K. Recommended Next Tranche

Proceed to `GTR-LEARN-02` only after human review. It should consume only
observable, reliable transaction signals, create candidates by conservative
rules, reference retained Work IDs/evidence, and add low-friction feedback. It
must not broaden scope or auto-promote candidates merely because the new store
exists.

## L. Git Status

Initial inspection found clean `main` at `4867f9125`, two commits ahead of its
then-upstream. During this tranche, repository drift occurred: `main` advanced
through `87277b495` and `050cdfc47` (both titled
`newinternalearningperformanceadded`) and local `origin/main` recorded an
update-by-push to `050cdfc47`. Those commits contain the learning implementation
and formatting changes; they were preserved rather than reset, amended, or
rewritten.

At checkpoint creation, no commit or push was intentionally performed by this
tranche. The working tree contains the final correctness, test, documentation,
and this report changes for review; it has not been committed. A local ignored
`./gator` binary was produced by the build verification command.

Stop here. Do not start Prompt 2 without human review.
