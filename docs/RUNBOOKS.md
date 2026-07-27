# Manual runbooks

A runbook is a local, explicit dependency graph for Gator-managed runs. It is not a daemon, queue, scheduler, or process monitor. Gator only marks a step ready; a user selects every start from `:GatorRuns` (`n`) or `:GatorRunbook`.

Create one through the Lua API:

```lua
require("gator").create_runbook({
  id = "fix-and-review",
  title = "Research, implement, review",
  max_concurrent = 2, -- 0 uses `runbooks.max_concurrent`; 0 there is unbounded
  max_tokens = 0, -- 0 uses `runbooks.max_tokens`; reported usage only
  steps = {
    { id = "research", role = "researcher", provider = "pi", objective = "Find the cause", depends_on = {} },
    { id = "write", role = "writer", provider = "codex", objective = "Implement the fix", depends_on = { "research" } },
    { id = "review", role = "reviewer", provider = "pi", objective = "Review the diff", depends_on = { "write" } },
    { id = "integrate", role = "integrator", provider = "codex", objective = "Converge reviewed work", depends_on = { "write", "review" } },
  },
})
```

Every step has an explicit provider, objective, role, and acyclic dependency list. A step is ready only when every dependency completed and it has not already started. Failed/stopped runs are not retried implicitly.

| Role | Workspace | Start policy |
| --- | --- | --- |
| `researcher` | Project workspace | Gator sends a read-only research instruction. |
| `writer` | New isolated Git worktree | Gator transfers bounded dependency provenance. |
| `reviewer` | Completed writer workspace | Gator opens a reviewed handoff and asks for evidence; no second writer worktree. |
| `integrator` | New isolated Git worktree | Explicit user confirmation required; every dependency is identified and its bundle/transcript/diff detail is supplied within the configured bound. |

Provider CLIs retain their own filesystem and command permissions. `researcher`/`reviewer` are Gator role instructions, not an OS sandbox or a claim that an arbitrary provider cannot write. For structured providers, native approval requests remain interactive.

Runbook usage displays exact aggregate provider-reported tokens only. If any started run has no report, the graph says `partial`; an untouched runbook says `unknown`. No price, token, or context value is inferred for another provider. A reported-token cap blocks future starts once the exact reported lower bound reaches it; it does not estimate unreported usage.
