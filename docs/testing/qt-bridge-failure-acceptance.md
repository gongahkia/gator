# Qt bridge failure acceptance

`tools/qt_bridge_failure_acceptance.py` starts the native Qt app against a
temporary authenticated loopback server implemented by the test itself. It removes
inherited `HCB_GOOGLE_*` and bridge environment values, redirects XDG paths and the
credential file into a temporary directory, and does not start the Python core or
contact Google.

The app loads a minimal workspace, then the test verifies that an operation poll
does not hold the UI busy state, a second sync request cancels the active operation,
rejected task and calendar mutations leave models unchanged, an invalid successful
task-list response remains an error, malformed initial and polled operation responses
are rejected, and the app exits while a local operation poll is still in flight.

Run it after building the native executable:

```sh
PYTHONDONTWRITEBYTECODE=1 uv run python tools/qt_bridge_failure_acceptance.py \
  --native '/tmp/hcb-qt-search-verify/native/Hot Cross Buns'
```

`make qt-bridge-failure-acceptance` uses the established `/tmp/hcb-qt-tests` build
path. This is a bridge/UI boundary acceptance only; it does not validate Google
OAuth or real synchronization.
