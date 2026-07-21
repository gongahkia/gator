# Execution policy

`paw` merges the `[policy]` TOML table from the standard config sources. The current schema is
`paw.policy/1`; policy validation rejects another version.

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

Omitted values retain the built-in defaults. Environment overrides use the `PAW_POLICY_*` variables
listed in the main configuration reference.
