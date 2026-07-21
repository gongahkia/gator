# Execution policy

`paw` merges the `[policy]` TOML table from the standard config sources. The current schema is
`paw.policy/1`; policy validation rejects another version. Policy fields use the same precedence as
the rest of configuration: built-in defaults, user config, nearest repo `.paw/config.toml`, explicit
`--config`, then supported `PAW_POLICY_*` environment overrides. There is no separate policy file.

```toml
[policy]
version = "paw.policy/1"

[policy.provider]
allowed_transports = ["ollama", "openai"]
allowed_base_urls = ["https://api.example.test/v1"]
allow_loopback = true

[policy.command]
allow = ["go test ./..."]
deny = ["rm -rf"]
require_approval = true

[policy.risk]
max_files = 12
max_lines = 800
max_tokens = 120000
max_commands = 8

[policy.git]
allowed_remotes = ["origin"]
allow_branch = false
allow_commit = false
allow_push = false

[policy.egress]
max_files = 80
max_bytes = 524288
block_secrets = true

[policy.approval]
auto_approve = false
```

`policy.git.allowed_remotes` contains exact Git remote names. `allow_push` requires at least one
allowlisted remote; a push to any other remote is denied.

`policy.command.allow` and `policy.command.deny` contain exact shell command strings. Deny entries
take precedence. A nonempty allowlist denies every command not listed; otherwise
`require_approval` marks each non-denied command as requiring approval.

`policy.risk` limits apply independently to each milestone's file, line, token, and command risk.
An estimate exceeding any one limit is denied.

`policy.approval.auto_approve` defaults to false. Setting it to true permits automatic approval at
approval gates, but does not bypass command denies, command allowlists, remote allowlists, or risk
limits.

Provider approvals are recorded as receipts bound to the exact transport, base URL, and egress
manifest digest.

Gathered context is scanned for GitHub tokens, OpenAI keys, AWS access keys, private keys, and JWTs.
The manifest records finding kinds and counts only.

Detected secret values are redacted before compression or raw-context model requests.
When `block_secrets` is true, detected secret-bearing context also requires a matching provider
approval receipt before model egress.

High-confidence secret values are scrubbed from traces, session event data, verification output, and
doctor diagnostics.

Omitted values retain the built-in defaults. Environment overrides use the `PAW_POLICY_*` variables
listed in the main configuration reference.
