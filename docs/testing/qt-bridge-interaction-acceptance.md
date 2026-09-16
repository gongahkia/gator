# Qt bridge interaction acceptance

`tools/qt_bridge_interaction_acceptance.py` starts an actual `hcb bridge serve`
process and the native Qt executable against a new temporary 1,000-task/1,500-event
SQLite fixture. It removes inherited `HCB_GOOGLE_*` values, points all XDG paths at
the temporary directory, and creates an empty credential file there. It does not
read or use a configured Google account.

The native app's explicit acceptance probe waits for its initial bridge workspace,
searches the Python core index, then creates, edits, and completes a task; creates
and edits an event; creates, renames, and deselects a task list after proving its
task is hidden from the displayed model but retained by the bridge; creates and updates a
calendar's details and visibility; and queues then removes a synthetic calendar
subscription. It also creates a managed recurring task, reconfigures it, completes
it, stops its next occurrence, and splits a second series. It creates a task
hierarchy, reorders siblings through the bridge controller, and moves one
child into a deselected task list. It then completes, moves, and deletes two
tasks through one atomic bridge request for each bulk action. It verifies the
core-provided conflict list and resolves a synthetic normal conflict by keeping
Google's version. It then verifies the refreshed search projection, shuts down
the bridge, checks descriptor cleanup,
restarts the bridge against the same database, verifies the surviving task, event,
task-list, calendar, recurrence markers, hierarchy, persisted bulk deletions, and
conflict resolution, and starts Qt a second time to verify it loads the persisted workspace.

Run it after building the native executable:

```sh
PYTHONDONTWRITEBYTECODE=1 uv run python tools/qt_bridge_interaction_acceptance.py \
  --native '/tmp/hcb-qt-search-verify/native/Hot Cross Buns'
```

`make qt-bridge-interaction-acceptance` uses the established `/tmp/hcb-qt-tests`
build path. The acceptance probe is enabled only by the temporary
`HCB_BRIDGE_INTERACTION_ACCEPTANCE_FILE` report path. The offscreen probe exercises
the native controller rather than physical pointer input; the QML suite separately
checks typing, bridge-mode search controls, and result routing.
