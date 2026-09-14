# Qt browser OAuth acceptance

This procedure verifies that the Linux Qt frontend starts browser authorization
through the Python-owned local bridge and receives its completed state. It
performs no HCB task, event, Calendar, or Drive mutation. Google OAuth does read
the selected account's identity to reject an unexpected account.

Use the disposable Google test account and client, never the personal HCB
profile. Create a new owner-only file outside the repository containing only the
Desktop OAuth client configuration:

```text
HCB_GOOGLE_CLIENT_ID=YOUR_DESKTOP_CLIENT_ID
HCB_GOOGLE_CLIENT_SECRET=YOUR_DESKTOP_CLIENT_SECRET
```

The secret line is optional. Do not copy an existing account file: the tool
rejects `~/.config/hcb/personal.env` and any file that already contains an HCB
refresh token. The unique file path gives the disposable account a separate
keyring entry when the bridge saves its token.

Run the acceptance from the repository root with a built Qt executable:

```sh
uv run python tools/qt_oauth_acceptance.py \
  --native "/tmp/hcb-qt-tests/native/Hot Cross Buns" \
  --env-file "$HOME/.config/hcb/qt-oauth-acceptance.env" \
  --email "disposable@example.com"
```

The tool creates a temporary HCB config, cache, and SQLite database, starts an
external Python bridge, and launches Qt in bridge mode. In the Qt window select
**Open Google setup**, then **Connect Google**. Complete consent with the
expected disposable account in the browser on the same computer. A successful
run prints only `{"qt_oauth_acceptance":{"authenticated":true}}`.

On success, the temporary HCB profile is deleted. The supplied disposable
credential file gains an encrypted refresh token and its encryption key is
stored under that file's separate OS-keyring identity. Preserve it only for
later disposable-account acceptance; remove it with HCB's disconnect flow when
finished. Do not copy its contents into logs, chat, a commit, or an issue.

If the browser is denied, the wrong account is selected, or the loopback callback
does not return, close the Qt app or press `Ctrl+C`. The temporary profile and
bridge descriptor are removed; diagnose the redacted status in the app without
printing the descriptor, browser URL, authorization code, client values, or
token values.
