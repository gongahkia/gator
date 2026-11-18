# P04 — MD03 native provider credentials

## Security design before implementation

Baseline HEAD: `d0de3963f56216d78cb17f613162abfc3252e08c`, clean worktree. Accepted P03 bundle is preserved. This increment is MD03 only.

Boundary: native secure entry → finite local NDJSON save request → canonical Go cloud validation → `saveTUICloudModelConfiguration` → existing `auth.Store` and `config.Store.Update` → request-specific, non-secret completion and status reread. No second authentication store, Keychain record, OAuth implementation, engine or live request is added.

A read-only review request carries configuration fields and a replace/retain intent, never the entered key. Go retains one bounded, short-lived opaque review identity with its settings/credential expectations. The exact provider/default/model/destination and add/replace/retain/removal intent are shown before explicit Save. Only Save carries a new key. Review identities are random, not credential fingerprints.

The existing callback selects the future default provider/model as well as saving its endpoint/options; the review must state this. Reviewed native changes use the existing cloud setup lock, credential comparison and settings Update lock. Failed settings publication retains compare-before-rollback behavior. Removal uses the same Go credential authority with reviewed comparison; it leaves settings, environment and unrelated providers intact. There is no invented cross-file crash journal.

Secrets remain transient in secure UI input and the required local save operation. They never enter drafts, chat/history, prompts, task traces, argv, environment mutations, status/review/result DTOs, logs, tooltips, screenshots or clipboard output. UI input is cleared on dispatch, cancellation, selection/navigation away and failure. Clearing Swift String is not a physical-memory-erasure guarantee.

The auxiliary native window uses a pinned existing managed Go owner and finite typed commands, not the terminal panel or a new owner. It does not replace the current conversation or interrupt Work. Existing captured RunOptions remain authoritative; confirmed settings refresh applies to future submissions. Quit closes admission synchronously, awaits an already accepted setup action, then uses the existing durability/shutdown transaction.

Status means configured and/or locally ready, never live verified. No network probe, discovery, vendor CLI credential import, OAuth handoff or paid inference occurs. Unsafe endpoint userinfo/query data is not projected, and destination changes require explicit override/default selection and review. Built-in provider/options allowlisting and canonical URL/adapter validation remain Go-owned.

Implementation, verification, precise provider matrix and remaining limits will be recorded below.
