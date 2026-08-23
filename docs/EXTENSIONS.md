# Extensions

Gator extensions add reusable prompt guidance, skills, and optional executable
tools to native Gator runs. There is one lifecycle:

```sh
gator extension install /path/to/extension
gator extension install https://github.com/example/review-helper.git
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

`install` accepts a local directory or an explicit `https`, `ssh`, or `git@`
repository URL. Git sources are cloned shallowly only for that explicit
installation; re-run the same command with `--replace` to update an installed
package. Gator never refreshes executable extension code in the background.

## Project extensions and trust

A repository can keep extensions at:

```text
.gator/extensions/<extension-id>/gator-extension.json
```

They do nothing until the developer runs `gator extension trust` from that Git
checkout. Trust records the canonical repository path and a SHA-256 hash of the
entire extension bundle: any manifest, prompt, executable, helper, or file-mode
change disables the project bundle until it is reviewed and trusted again. Use
`gator extension status` to inspect that state and `gator extension untrust` to
revoke it. This is deliberate: cloning or opening a repository must not make
its files executable merely because Gator starts a task there.

## Manifest

Every bundle has a `gator-extension.json` file:

```json
{
  "version": 1,
  "id": "review-helper",
  "name": "Review helper",
  "skills": ["skills/review.md"],
  "prompts": ["prompts/team-style.md"],
  "commands": [
    {
      "name": "focused_review",
      "description": "Load the focused review prompt into the composer.",
      "prompt": "prompts/focused-review.md"
    }
  ],
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

An extension command appears in the terminal palette as
`/extension-id:command-name`. Selecting it loads its declared prompt file into
the composer for review and editing; it never sends a run automatically. This
is the standard prompt-template interface for packages and keeps user control
visible at the send boundary.

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
manifest timeout). Every model-requested sidecar invocation requires the same
allow-once, allow-this-exact-tool, or deny approval used for trusted MCP tools.
The executable runs under the native run's strict sandbox, filesystem roots,
filtered environment, and network mode; `--sandbox off` remains the explicit
host-access escape hatch for the entire run. A malformed response, nonzero
exit, timeout, or post-load bundle change fails the tool call visibly.

## Trust boundary

An executable extension is code selected by the developer. Global extensions
require an explicit local install; project extensions require explicit
hash-pinned trust. Neither bypasses the native run's process sandbox or the
per-tool developer approval boundary. Extension sidecars can write only where
the run sandbox already permits writes; any resulting worktree change remains
subject to Gator's final diff, verifier, and explicit patch handoff review.
Gator rejects symlinks and non-regular files in every extension bundle, and
rehashes a bundle before reading its guidance or launching its sidecar.
