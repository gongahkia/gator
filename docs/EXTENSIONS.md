# Extensions

Gator extensions add reusable prompt guidance, skills, and optional executable
tools to native Gator runs. There is one lifecycle:

```sh
gator extension install /path/to/extension
gator extension list
gator extension disable extension-id
gator extension enable extension-id
gator extension remove extension-id --yes
```

Installed bundles live below `$XDG_DATA_HOME/gator/extensions/` (or
`~/.local/share/gator/extensions/`). The non-secret enabled state is in the
single Gator configuration file, `$XDG_CONFIG_HOME/gator/config.json`. An
installation is enabled immediately; replacement requires `--replace`, and
removal requires `--yes`.

## Project extensions and trust

A repository can keep extensions at:

```text
.gator/extensions/<extension-id>/gator-extension.json
```

They do nothing until the developer runs `gator extension trust` from that Git
checkout. Trust is canonical-path-specific and can be revoked with `gator
extension untrust`. This is deliberate: cloning or opening a repository must
not make its files executable merely because Gator starts a task there.

## Manifest

Every bundle has a `gator-extension.json` file:

```json
{
  "version": 1,
  "id": "review-helper",
  "name": "Review helper",
  "skills": ["skills/review.md"],
  "prompts": ["prompts/team-style.md"],
  "tools": [
    {
      "name": "lookup",
      "description": "Look up a reviewed internal convention.",
      "parameters": {
        "type": "object",
        "additionalProperties": false,
        "required": ["topic"],
        "properties": {"topic": {"type": "string"}}
      },
      "command": ["bin/lookup"],
      "timeout_seconds": 30
    }
  ]
}
```

IDs use lower-case letters, digits, and hyphens. Resource and command paths
are exact relative paths—no globs, absolute paths, symlinks, implicit startup
hooks, or shell command strings are accepted. Skill and prompt files must be
`.md` or `.txt` files. Gator places their contents in the native model’s system
guidance with the extension ID shown.

Extension tools are available only in Execute mode and appear to the model as
`extension_<extension-id>_<tool-name>`. Their declared parameter schema is
passed through to the direct model adapter.

## Sidecar tool protocol

An executable tool receives one JSON object on standard input and emits exactly
one JSON object on standard output. Gator invokes the command with argv, never
a shell, from the isolated worktree. A representative request is:

```json
{
  "version": 1,
  "method": "tool",
  "extension": "review-helper",
  "tool": "lookup",
  "arguments": {"topic": "migrations"},
  "repository": "/path/to/isolated-worktree",
  "worktree": "/path/to/isolated-worktree"
}
```

The required response is:

```json
{"content":"Use a reversible migration and test downgrade behavior."}
```

Tool responses are limited to 64 KiB and commands to two minutes (or a shorter
manifest timeout). A malformed response, nonzero exit, or timeout fails the
tool call visibly.

## Trust boundary

An executable extension is code selected by the developer. It is outside
Gator's native `run_command` verifier allowlist and can use the permissions of
the Gator process. Global extensions therefore require an explicit install;
project extensions require explicit per-repository trust. Gator's own
worktree, path validation, patching, and verifier policies are unchanged for
native tools and direct model runs.
