# Work evaluation

Gator evaluates the product through `work.v1`, the same Work service used by
the CLI, TUI, jobs, history, Code specialist, artifact verification, and local
delivery. There is no separate legacy Code evaluation runner.

See [WORK_EVALUATION.md](WORK_EVALUATION.md) for corpus format, deterministic
transaction gates, optional semantic rubric judging, and local report handling.

Evaluation is an advanced developer/automation surface. It is intentionally
CLI-only: normal Work review belongs in the TUI, while experiments need an
explicit dataset, output directory, and split selection.
