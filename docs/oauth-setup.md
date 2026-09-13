# Bring-your-own Google OAuth setup

This guide is for the current Python CLI/TUI on Linux and macOS. Use a Google
Cloud project and Google account that you control; use a disposable account for
the [live acceptance procedure](testing/live-google-tui-smoke.md). HCB does not
provide a shared OAuth client. The future desktop app's distribution model is
still a separate decision in the [development checklist](../CHECKLIST.md).

## Configure Google Cloud

1. Create or select a Google Cloud project. In **APIs & Services > Library**,
   enable the **Google Tasks API**, **Google Calendar API**, and **Google Drive
   API** for that project. HCB uses Drive only for metadata. Google documents
   the [API enablement steps and service names](https://developers.google.com/workspace/guides/enable-apis).
2. In **Google Auth platform > Branding**, configure the consent screen with an
   app name, support email, and contact email. In **Audience**, choose
   **External** and leave the app in **Testing** for a personal test project;
   add the Google account that will sign in under **Test users**. An
   organization-only project may use **Internal** when the account belongs to
   that organization. See Google's [consent-screen instructions](https://developers.google.com/workspace/guides/configure-oauth-consent)
   and [audience rules](https://support.google.com/cloud/answer/15549945?hl=en).
3. For an External app, add HCB's requested scopes in **Data Access**. Check
   that the browser consent screen requests the same access:

   | Scope | Access used by HCB |
   | --- | --- |
   | `openid` | Identify the signed-in account. |
   | `https://www.googleapis.com/auth/userinfo.email` | Read its email address. |
   | `https://www.googleapis.com/auth/tasks` | Read and change Google Tasks. |
   | `https://www.googleapis.com/auth/calendar` | Read and change Google Calendar. |
   | `https://www.googleapis.com/auth/drive.metadata.readonly` | Read Drive file metadata, not file contents. |

   These are the scopes in `src/hcb/auth.py`. Google explains how to
   [configure consent scopes](https://developers.google.com/workspace/guides/configure-oauth-consent).
4. In **Google Auth platform > Clients**, create a client of type **Desktop
   app**. Copy its client ID and, if Google supplies one, its client secret
   into your local credential file. HCB uses a browser, PKCE, and a
   `127.0.0.1` callback on an available port; it does not use a Web client or
   a fixed redirect URI. See Google's [Desktop client setup](https://developers.google.com/workspace/guides/create-credentials)
   and [installed-app loopback flow](https://developers.google.com/identity/protocols/oauth2/native-app).

## Connect HCB

Create an owner-only file outside the repository. The default path is
`~/.config/hcb/personal.env`; use `--env-file PATH` or `HCB_ENV_FILE` when you
want another path. Enter the values as assignments, without `export` or JSON:

```text
HCB_GOOGLE_CLIENT_ID=YOUR_DESKTOP_CLIENT_ID
HCB_GOOGLE_CLIENT_SECRET=YOUR_DESKTOP_CLIENT_SECRET
```

The secret line can be omitted when Google did not supply a secret. Replace
the placeholders locally; do not paste real values into a repository, issue,
chat, or support log. To create and use the default file:

```sh
credential_file="$HOME/.config/hcb/personal.env"
umask 077
mkdir -p "${credential_file%/*}"
${EDITOR:-vi} "$credential_file"
chmod 600 "$credential_file"
hcb --env-file "$credential_file" auth connect personal you@example.com
hcb --env-file "$credential_file" auth status
hcb --env-file "$credential_file" sync
```

Use your Google account email in place of `you@example.com`. `auth connect`
opens your system browser. The browser and HCB must run on the same computer
so the temporary loopback callback reaches HCB. `--no-browser` prints the
authorization URL when automatic browser launch is unavailable; open it in a
browser on that same computer and do not share it.

After browser approval, HCB checks Google's verified account email against the
email supplied to `auth connect` before saving the refresh token. Select the
intended account in Google's account chooser; if they differ, retry with the
account's primary email. Google documents the verified email field in its
[OpenID Connect UserInfo response](https://developers.google.com/identity/openid-connect/reference#userinfo_endpoint).

HCB rejects a credential file accessible to group or other users. After
authorization, it stores an encrypted refresh token in that file and its
encryption key in the OS keyring; access tokens remain in memory. Keep the
keyring available and unlocked. `auth disconnect` removes local credentials
but keeps cached data and does not revoke Google's grant remotely. These
behaviors are implemented in `src/hcb/credentials.py`, `src/hcb/auth.py`, and
`src/hcb/runtime.py`.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| `access_denied` or a test-user warning | Confirm **Audience** is External/Testing, the signing-in account is listed under **Test users**, and Workspace policy permits access. Review the project name and scopes before consenting. |
| `redirect_uri_mismatch` | Confirm the client type is **Desktop app** and the browser is returning to HCB's loopback callback on the same computer. Do not substitute a Web client. |
| API disabled or permission error after connecting | Check the selected Cloud project, all three enabled APIs, and the requested scopes in **Data Access**. |
| Credential file permission error | Put the file outside the repository and run `chmod 600` on it. |
| Missing keyring key or token decryption error | Unlock the OS keyring used when connecting. The `.env` file alone cannot decrypt its refresh token on another machine. |
| Refresh token expires after several days | An External app in **Testing** has seven-day authorizations, including offline refresh tokens, when it requests scopes beyond basic identity. Reconnect with the test account; production publication and verification are separate decisions. |
| No refresh token returned | Revoke the previous grant in your Google account's connected-app settings, then run `auth connect` again. |

Google documents [Testing-mode token expiry](https://support.google.com/cloud/answer/15549945?hl=en)
and [Desktop OAuth errors](https://developers.google.com/identity/protocols/oauth2/native-app).
Do not attach the credential file, client JSON, authorization URL, tokens, raw
Google responses, or unredacted diagnostics to a public report. For the live
release gate, record only the redacted result described in the
[acceptance procedure](testing/live-google-tui-smoke.md).
