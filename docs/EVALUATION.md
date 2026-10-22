# Work evaluation

Gator evaluates the product through `work.v1`, the same Work service used by
the desktop application, jobs, history, Code specialist, artifact verification, and local
delivery. There is no separate legacy Code evaluation runner.

See [WORK_EVALUATION.md](WORK_EVALUATION.md) for corpus format, deterministic
transaction gates, optional semantic rubric judging, and local report handling.

Evaluation is an advanced developer/automation harness. Normal Work review
belongs in the desktop app. Experiments use an explicit source-checkout opt-in,
dataset, output directory and split selection; they do not restore a public CLI.
See [Developing the desktop application](DEVELOPMENT.md).
