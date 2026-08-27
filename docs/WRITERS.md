# Writer orchestration

Writer delegation is a review boundary, not a merge engine. A primary Execute
run may use one serial writer or exactly two concurrent writers, with two
children total. Every child starts in a retained worktree built from the same
snapshot of the parent's current isolated state. It cannot write to the parent
worktree or recursively delegate another writer.

## Role policy

A project `writer` role may add instructions and an optional restrictive
`policy`. Gator first applies the developer-selected parent profile, then meets
the role policy against that result. The role may:

- force `sandbox: strict` or `network: deny`;
- lower `max_steps`;
- force Plan mode (which makes a writer non-mutating);
- omit a closed set of capability families.

It cannot enable network, turn off strict sandboxing, restore an omitted tool,
raise a budget, or change a `readonly` role into a writer. Invalid widening
definitions make `.gator/agents.json` fail closed before delegation.

## Durable manifest

The parent writes a private atomic child manifest before worktree creation,
before child execution, and at the terminal transition. It stores no delegated
task text or patch body. It records:

- parent/child/batch identity, provider/model/profile, and lifecycle status;
- immutable baseline, retained worktree, and child run record;
- task, verifier, and patch SHA-256 values plus a ref-free patch commit;
- role, declared paths, actual paths, and effective mode/sandbox/network/step
  and omitted-tool policy;
- owner PID, heartbeat, deadline, timestamps, and terminal error;
- patch size/availability and the unconditional review requirement.

The owner, heartbeat, and deadline make interrupted foreground work
diagnosable. They do not imply that PID existence proves liveness.

## Comparison and conflict review

Parallel assignments must declare non-overlapping repository-relative paths.
After completion Gator reports out-of-scope changes and actual changed-path
overlap. It then performs a real three-way textual comparison:

1. Apply each exported binary patch to the common baseline in a temporary Git
   index.
2. Write an unreachable tree and commit with no branch or ref update.
3. Run `git merge-tree --write-tree --messages` on the sibling commits.
4. Persist `clean`, `conflict`, or `unavailable`, the clean result tree when
   present, and bounded diagnostics.

The exit status is authoritative: Git documents status 0 as clean, 1 as
conflicted, and other values as execution failure:
<https://git-scm.com/docs/git-merge-tree#_exit_status>.

No step changes a worktree, index, or ref. `/manage` and `gator child batch`
show both path evidence and Git comparison evidence. A clean result proves only
that Git found no textual conflict. It does not detect incompatible API,
schema, migration, behavioral, or test assumptions. The parent must inspect and
explicitly apply each compatible patch; Gator never auto-merges.

## Background lifecycle contract

Writer execution remains foreground and parent-synchronous in this release.
Detaching it safely requires a process-local registry analogous to terminal
detachment, but with stricter result collection:

1. `start` must atomically publish the manifest before launching and retain the
   exact immutable parent snapshot and effective policy.
2. `status` must read structured registry state; it must not infer completion
   from a PID alone.
3. `cancel` must cancel the child context, wait for teardown, and publish a
   terminal manifest.
4. `collect` may return only the bounded summary, patch, and conflict evidence;
   it must not apply or merge.
5. Session shutdown must cancel and join every owned writer. A crash may leave
   a retained worktree and stale running manifest, but no unowned model process.
6. Restart recovery may expose stale evidence for review; it must not revive
   approvals, model calls, or processes.

These invariants are the prerequisite for a future detached writer tool. A
fire-and-forget goroutine without explicit ownership, cancellation, and
collection would be a regression, not background orchestration.
