# Learning evaluation fixture workflow

This deterministic corpus evaluates whether active scoped learnings change a
future Work result and whether inactive, reversed, conflicting, or unrelated
learnings do not. Every probe executes the normal Work agent loop: a
deterministic model receives the ordinary Work system/user context, writes a
sealed `decision.txt` artifact through the normal artifact tool, and the
evaluator grades the artifact's observed behavior. It never gives the model
the learning store or fixture identity directly.

Failure-prevention cases currently evaluate the generated verification plan,
not host-side verification execution: learnings are prompt context and do not
yet modify deterministic `codeexec` verification policy. Development cases are
the normal iteration set; held-out cases require an explicit selection.

To turn a real failure into a regression case:

1. Select a retained Work or observation with a concrete, reviewable failure.
2. Remove prompts, source content, credentials, personal data, and unrelated
   details; retain only the minimal scope, correction, and expected future Work
   behavior.
3. Add a **development** case to `dataset.json` with a stable ID, a scoped
   learning seed or observation seed, an ablated relevant probe, and an
   unrelated/reversal probe when applicable.
4. Run `go test ./internal/eval -run Learning -count=1` during development.
   Run the held-out split only when intentionally checking generalization.

Do not generate cases from Work history automatically. A reviewer must decide
that the sanitized fact is worth preserving as a development regression.
