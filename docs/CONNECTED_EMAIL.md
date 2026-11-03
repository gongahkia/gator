# Connected email composition

November 2026 implementation of part of competitive item 5. The same Work
connector tools serve the foreground desktop conversation. Gmail sending still
requires a selected account with the send OAuth scope and exact foreground
approval. No real account is connected or message sent by this implementation.

## Reading and replying

`gmail_get` supplies one message's headers and content. `gmail_thread_get` takes
`resource_id` and returns the selected thread in full format, subject to the
existing 256 KiB JSON response bound. Oversized threads fail explicitly; selected
individual messages remain readable. Thread reads require the Gmail read scope
and inherit any message-content denial/review requirement on `gmail_get`.

## Enabling sending

Initial Google setup remains read-only. On the saved connection, choose
**Enable email sending**, review the permission and account, and complete the
browser sign-in. This requests the existing active scopes plus `gmail.send`
using PKCE and a fresh ephemeral loopback callback. It uses a stored desktop
client secret, or the distribution's client material where configured, without
putting either into the renderer. No terminal command is required.

Google's returned grant must explicitly confirm `openid` if requested, all
requested non-identity Google API scopes, and sending. A new offline refresh
grant is required. Identity email/profile aliases are not inferred; use
**Check account & read access** to verify the selected account afterward. Partial
or missing grants, rejection, cancellation, changed settings and newer saved
credentials prevent publication. This reauthorizes the requested installed-app
scope set; it does not use Google's unsupported desktop incremental flow.

The previous configuration stays in place until authorization succeeds. The
credential is replaced with compare-and-swap before expanded settings are
published. If settings publication fails, Gator attempts to restore the prior
credential using compare-and-swap and reports any concurrent change. These are
separate durable stores: a crash between them can leave a new grant with the old
narrower local surface. It cannot publish sending before obtaining the grant.
Existing account/grant pins on follow-ups still apply.

**Turn off email sending** removes the send operation for new tasks, retaining
reading and saved credentials. Active tasks retain their reviewed connection
snapshot until completion. The UI states this limitation; each send still needs
exact approval. This local change does not revoke Google's external grant.
Reverting the native controls commit likewise does not undo prior permission
changes or revoke grants; turn off sending before rollback if that is intended.

## Composing

For `gmail_send`, the action payload is typed JSON. A legacy single-address `to`
string remains supported; use explicit arrays for multiple recipients. Cc/Bcc
are optional arrays. At least one To address and at most 50 total distinct
recipients are supported. Each address is one plain ASCII mailbox address;
comma-separated strings and display-name syntax are rejected.

```json
{
  "payload": {
    "from": "me@example.com",
    "to": ["one@example.com", "two@example.com"],
    "cc": ["copy@example.com"],
    "bcc": ["private-copy@example.com"],
    "subject": "Project review",
    "body": "Here is the report we discussed.",
    "reply": {
      "thread_id": "GMAIL_THREAD_ID",
      "in_reply_to": "<parent-message@example.com>",
      "references": ["<parent-message@example.com>"]
    },
    "attachments": [{"source": "output/report.pdf"}]
  }
}
```

Omit `reply` for a new conversation. For replies, read the original thread,
preserve its subject and supply Gmail's thread ID plus the actual RFC Message-ID
of the parent. References must end with that parent and contain at most 32 valid,
distinct Message-IDs. Gmail requires matching subject, thread ID and reply
headers; Gator supplies both the MIME headers and API `threadId`. A successful
HTTP response that does not confirm the requested thread remains uncertain.
Do not invent header IDs or infer the parent from an email address.

The text body is bounded to 10,000 UTF-8 bytes and the subject to 200 bytes. These
fields and all envelope/attachment metadata together retain the existing 12 KiB
approval-payload limit. This implementation does not add HTML authoring.

## Attachments

References can address actual bytes in this Work run's selected `source/`, staged
`output/`, or retained `previous/` mount. Absolute paths, traversal, other mounts
and links escaping the owned root are refused. A model cannot supply arbitrary
encoded attachment bytes through this operation.

The host resolves references before review and freezes private copies. It fills
the filename, media type, byte count and SHA-256 into the exact approval preview.
The caller may provide a different ordinary filename or media type; supplied
size/hash constraints must match. Media type is a filename-based hint unless
explicitly supplied, not proof of file-format correctness. Send construction and
execution revalidate metadata against the frozen bytes. Later changes to the
source buffer cannot change the approved message. There are at most ten files,
8 MiB per file and 16 MiB total. Multipart MIME uses encoded attachment filenames
and bounded-line base64 without placing file data in the approval or manifest.

These are selected file mounts, not arbitrary access to the user's upload store.
An Office document's extracted text must never be sent while claiming it is the
original document. Original uploads not materialized in a selected file mount
need a separate original-byte selection path; that integration remains open.

## Approval, receipts and recovery

All recipients, subject, body, reply metadata and attachment metadata belong to
one exact reviewed payload and target. Draft mode records a proposal without a
Gmail request. Denial sends nothing. A send obtains its existing unique
correlation/Message-ID; a new preparation remains a distinct send even when its
content is identical. Unconfirmed sends are never automatically retried.

The existing action journal flushes before dispatch. Provider receipt and Sent
lookup semantics remain: API acceptance and a Sent copy do not establish
recipient delivery. Rich envelope summaries survive sealed-result projection
and interrupted-action recovery without copying body/attachment contents into
the chat's receipt projection. The full approved preview stays in the private
action record.

Rollback adds no new canonical manifest/configuration fields. Older builds can
read sealed records but may omit recipient summaries for list-shaped envelopes
and cannot prepare these richer sends. They must not replay uncertain actions.

## Verification and remaining work

Tests parse the produced MIME independently with `net/mail` and
`mime/multipart`, checking recipients, threading, Unicode body/filename and exact
binary bytes. They cover source mutation after preparation, malformed headers,
wrong parent references, unselected paths, changed hashes and size limits.
Production Work tests generate an output and attach both selected original bytes
and generated bytes, exercising approved, denied and draft-only paths through
the real action broker/journal. Thread mismatch remains uncertain; thread read
provenance, existing permissions and receipt privacy have regressions.

Native permission changes have loopback callback and production-control tests
covering successful publication, partial/omitted grants, missing offline access,
denial, cancellation, stale settings/account/removal, failed settings publication,
local disable and bundled/custom client review. Browser callback tests read the
whole success/error page while the waiting host closes the listener; the page
is flushed with a known length before completion is signalled.

Gmail draft creation/editing/sending, original upload selection, calendar workflows
and real-account acceptance remain open in the larger item 5. Passing local
fixture tests does not close those requirements.

API behavior checked against Google's first-party documentation on 3 November:
[thread requirements](https://developers.google.com/workspace/gmail/api/guides/threads),
[sending messages and attachments](https://developers.google.com/workspace/gmail/api/guides/sending),
[send endpoint](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/send),
and [thread reads](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.threads/get).
Permission-grant checks and installed-app behavior follow
[Google's desktop OAuth guidance](https://developers.google.com/identity/protocols/oauth2/native-app)
and [Gmail scopes](https://developers.google.com/identity/protocols/oauth2/scopes#gmail).
