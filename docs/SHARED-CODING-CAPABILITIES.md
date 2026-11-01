# Shared coding capabilities

Gator's user-facing conversation runner and bounded coding workers use the same
provider-neutral coding harness. Coding is an agent capability, not a second
product or a requirement to install Codex, Pi, or Claude. Electron remains the
primary product surface. Explicitly read-only assignments and research/review
specialists retain their narrower authority.

## Architecture and ownership

`internal/codeexec.Executor.Prepare` allocates isolated tools without calling a
model. Its `Prepared` session owns the worktree, project-policy intersection,
hooks, optional integrations and run-owned terminal cleanup. `Execute` uses this
same prepared harness for a standalone bounded assignment.

`cmd/gator/work_code_agent.go` provides one `CodeSessionFactory` and its delegated
runner adapter. Both consume a private Git repository made from the exact frozen
Work source and host-captured project configuration. They return patches rather
than write the selected live checkout. No original repository is required to be
Git initialized, and no Git history in that repository is rewritten.

The ordinary Electron path enables coding for draft/act work. It does not require
an unrelated document for a coding request. Code references such as SPEC.md or
“CSV importer” do not by themselves require an output document. Explicit inspect
mode stays read-only. Automatic classification is a convenience, not authority to
bypass the sandbox or command approvals.

`internal/workrun.conversationCode` exposes lazy `code_*` tools directly to the
existing main runner. Ordinary chat does not create a coding checkout or another
worker. Delegation is optional. The main agent keeps its actual selected model,
conversation identity, history and parent budget. A delegated coding worker gets
its own bounded assignment and private checkout through the same factory.

| Ability | Main conversation | Coding worker |
| --- | --- | --- |
| Read/list/search coding files | `code_read_file`, `code_list_files`, `code_search_files` | Same native file tools |
| Apply an isolated patch | `code_apply_patch` | Same `tools.ApplyPatch` implementation |
| Run verification/exploratory commands | `code_run_command` | Same command policy and sandbox |
| Inspect Git state | `code_git_status`, `code_git_diff` | Same Git tools |
| Run-owned PTY tasks | `code_terminal_*` when host-granted | Same terminal tools and input approvals |
| Optional trusted integrations | Exact `code_open`-advertised tools through `code_use_integration` | Same prepared LSP/MCP/extension/web/browser tools |
| Retain/verify a frozen proposal | `code_stage` | Host retains the returned patch; `integrate_code` verifies it |

Project and role policies can only narrow the host envelope. Tools omitted by the
effective policy fail closed even if their built-in descriptor was advertised
before lazy preparation. Optional integrations require the existing exact
configuration trust, capability and operation approvals; `code_use_integration`
cannot invoke an arbitrary host tool. There is no recursive delegation grant.

The main conversation's direct coding operations also have the prepared profile's
bounded operation allowance. This conservatively counts tool attempts rather
than introducing a second unbounded model loop. All actual model requests remain
under the parent budget. Native external CLI agents have separate billing and
must not be represented as covered by an internal provider-request budget.

## Staging, continuity and completion

`code_open` may explicitly select a previously verified `baseline_patch` before
the checkout opens. It cannot switch the session's baseline afterward. Verified
patches are retained in the existing Work artifact/revision lineage. A follow-up
starts a fresh bounded execution against the explicitly selected accepted patch;
this is conversation continuity, not persistent OS-process reattachment.

After changes, the main agent must run required verification, inspect status/diff
and `code_stage`. The host exports and hashes the patch and re-verifies a separate
frozen candidate. An unchanged checkout may finish without a fabricated patch.
Failures remain failures. A successful patch proposal is not an application to
live files. Existing exact application review and target-conflict checks remain
in force; terminal input does not substitute for that workflow.

`complete_code` now accepts the internal `code` worker as well as eligible native
CLI workers when host verification is configured. An optional `baseline_patch`
must resolve to an exact, retained verified candidate. Its implement → verify →
independent review → repair loop retains existing checkpoints, leases, task
budgets and conservative interrupted-owner recovery. Native CLI continuations
still require their exact native session identity. Internal repairs use fresh
bounded assignments with retained feedback and an explicit verified baseline;
they do not invent native session IDs.

Verification success is invalidated by later edits, terminal mutations,
exploratory commands, or a subsequent failed required check. Reviewer completion
now requires host-observed full byte coverage of every changed text file, not a
model claim or one-line read. Paged reads must overlap boundary lines; partial
final lines do not count as full coverage. Deleted files require observed
candidate absence and a complete source read. Unreadable, oversized or binary
files cannot pass this text-review gate silently.

A reviewed model verdict and passing bounded tests still do not prove general
correctness. Direct staging verifies a candidate but does not manufacture an
independent review result. Existing completed historical checkpoints are retained
as historical evidence, not retroactively upgraded to the new coverage claim.

## Sandbox hardening

Linux explicitly restores the shared network namespace only for an actual
`AllowNetwork` grant; Bubblewrap's `--unshare-all` otherwise keeps networking off.
This corrects the previous ineffective allow setting without weakening default
deny. Native Linux acceptance remains separate from argv and cross-build checks.

For host-selected Node/npm installed in the conventional NVM version layout,
strict execution can read that version's `bin` and npm implementation directory.
It does not grant the whole home, all of `.nvm`, other versions, user-level registry config,
or arbitrary executable parent directories. Child PATH entries are filtered to
already readable directories: inaccessible ambient PATH entries can otherwise
make native spawning stop at EPERM before reaching the system shell. Other
nonstandard runtimes may still need explicit narrowly scoped runtime roots.
Network, scratch HOME, environment filtering and user-file isolation remain.

## Electron evidence presentation

Code fences and supported text-file previews have bounded lexical syntax colours
with escaped React text, copy controls and plain-text fallback for unknown
languages. This is not an IDE grammar engine. Public narration and coding tool
activity stay in delivery order; coding history opens for inspection above the
answer. Private provider reasoning, raw command arguments and terminal output are
not copied into public activity. A nonzero command exit is shown as a failed step
without disclosing its output.

Changed-file cards open a read-only unified diff and recorded verification in the
existing right sidebar. `desktop/electron/code-preview.mjs` verifies the current
conversation, retained run, sealed manifest digest, exact candidate and patch
hash. It reads retained output, never live source or an editable file version.
Reads reject linked paths. Preview limits are 128 KiB / 2,000 lines, with a
256 KiB total response bound and explicit partial-view notice. Selection-keyed
requests and the final owner check prevent stale results replacing another
conversation's view. No Git process is launched merely to display a diff.

## Automated evaluation

Free deterministic Electron journeys use actual Go workers, real isolated edits,
real npm tests and real candidate verification. Only provider responses are
scripted. They cover implementation, red/green repair, delegated implementation
with independent read-only review, accepted-baseline follow-up, inspect-only
rejection and cancellation while awaiting approval. Unit/race tests additionally
exercise stale verification, read coverage, lazy allocation, operation limits,
omitted tools, lifecycle hooks, source isolation and preview tampering.

Opt-in real-model journeys:

```sh
npm run build --prefix desktop
npm run test:live:coding --prefix desktop
```

The command reads an existing OpenAI key locally and uses the same cumulative
`scripts/project-followup-eval/results/budget.json` as prior evaluations. It never
resets that ledger. The authorized total remains $10, with the existing $9.50
conservative cutoff, durable pre-request reservation and bounded output. The
finite aggregate request ceiling is 128 (raised from 96 to accommodate multi-step
coding, without raising the dollar allowance); the coding UI proxy permits at
most 40 requests. Ordinary UI/CI tests do not consume a key merely because .env
exists. Real keys stay in the Go proxy; Electron gets an ephemeral loopback token.
No native external coding CLI is invoked by these paid tests.

The live scenario asks the real model to implement two CommonJS modules for a CSV
budget importer, run executable assertions, show its retained diff, then adapt to
changed BOM/refund requirements using an internal worker and independent review.
Original implementation stubs remain unchanged in live source. The harness alone
updates tests/specs between phases; changed-path checks reject agent modifications
to those acceptance files. This is a guided,
small-project workflow evaluation, not an unseen production-repository benchmark
or a measured general success rate. Actual results are recorded separately below.

For free UI runs while using the computer:

```sh
GATOR_E2E_BACKGROUND=1 npm run test:e2e --prefix desktop -- coding.spec.ts
```

The real Electron windows stay visible but do not acquire keyboard focus on
supported hosts. This affects test fixtures only. Live tests disable traces to
avoid retaining authorization headers. Screenshots/reports contain only disposable
project data. There is no offline action replay or new supervisor/daemon.

## Verification record and remaining limits

Current verification results are appended after the final runs. Earlier historical
harness evidence is in `PRODUCT_DEPTH_VERIFICATION.md` and
`SUBAGENTS_AND_CLI_INTEROP.md`; their dated native-CLI smoke results are not a new
claim of broad coding reliability.

Not established by this change: native Windows strict process support, native
Linux/WSL runtime acceptance on this Mac, signed installer acceptance, physical
phone acceptance, arbitrary runtime installation layouts, automatic dependency
installation, unrestricted network access, or long-lived process survival after
its actual owner exits. External tmux remains separately enrolled and authorized;
these agent tools grant no access to unrelated terminals. Ghostty is out of scope.

### November 1 coding evaluation evidence

The first live Electron coding run is retained as
`coding-ui-2026-11-01T12-00-29-020Z.json` plus its browser report. The main agent
implemented both modules, passed actual npm acceptance checks and staged a
verified two-file candidate in 100,659 ms. The changed-requirements phase produced
another verified two-file candidate but the overall turn failed after the proxy's
original 32-request limit was exhausted; its parent received HTTP 403. There was
no independent-review checkpoint in the retained browser report, so that phase is
**incomplete**, not a passed reviewed coding workflow. The disposable profile was
cleaned up; no original source file was applied or mutation automatically replayed.

That run admitted 32 provider requests (34 incoming proxy attempts). The
cumulative ledger is 117 requests, $6.745135 estimated model allowance plus $1.92
retained search allowance: $8.665135 total against the authorized $10 cap. No new
paid run was started to hide the failure or reset accounting. Future explicitly
invoked coding runs have a 40-request per-run bound and retain bounded completion
metadata even when the parent cannot deliver its final answer. The shared
128-request/$9.50 conservative admission limits remain unchanged.
