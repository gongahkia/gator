# Onibi tmux inspection adaptation

Copyright 2026 gongahkia. Apache License, Version 2.0; full original license and
copyright notice are preserved in [LICENSE](LICENSE).

Source: https://github.com/gongahkia/onibi, local reference commit
`48523fe9514d6e4cd7d2e310934328df77d90b1d`.

Adapted files:

- `internal/tmux/attach.go`: runner/controller and capture/list parsing patterns,
  now in Gator `internal/executiontarget/tmux/controller.go`.
- `internal/render/text.go`: escape stripping and bounded tail rendering patterns,
  now in Gator `internal/executiontarget/tmux/text.go`.
- Relevant cases from `internal/tmux/attach_test.go`, `internal/mux/mux_test.go`,
  and `internal/render/text_test.go` inform Gator's adapter tests.

All adapted source files identify modifications. Gator replaces unbounded output,
session-name targeting, environment/config discovery and ANSI-preserving output
with bounded reads, explicit native pane identity, OS incarnation evidence,
hook-aware/no-start command guards and conservative plain text. Input, lifecycle,
hooks, daemon, intake, database, web UI and project configuration were not copied.
Onibi's third-party fonts and rattles assets/notices are outside this extraction.

This notice licenses the identified adapted material; it does not assign an
Apache-2.0 license to the remainder of Gator. No root project license was present
when this adaptation was made.

T4 also adapts the already selected attach.go literal SendText/SendKey patterns in
`internal/executiontarget/tmux/input.go`. Gator adds control-review/grant gates,
finite keys and input bounds, trailing-semicolon argv escaping, exact native
identity checks, partial-delivery semantics and no automatic input retries.
