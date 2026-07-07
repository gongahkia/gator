# MCP Server

`paw mcp serve` runs a stdio Model Context Protocol server for context and digest tools.
It is a sidecar for hosts that want repository context; it is not the full agent runner.

## Host Config

For hosts that use an `mcpServers` JSON object:

```json
{
  "mcpServers": {
    "paw": {
      "command": "paw",
      "args": ["mcp", "serve"]
    }
  }
}
```

If `paw` is not on the host process `PATH`, use the absolute binary path:

```json
{
  "mcpServers": {
    "paw": {
      "command": "/usr/local/bin/paw",
      "args": ["mcp", "serve"],
      "env": {
        "PAW_DRONE_TRANSPORT": "ollama",
        "PAW_DRONE_BASE_URL": "http://localhost:11434",
        "PAW_DRONE_MODEL": "qwen3:8b"
      }
    }
  }
}
```

For deterministic local smoke checks without a model call, add `--disable-compress`:

```json
{
  "mcpServers": {
    "paw": {
      "command": "paw",
      "args": ["mcp", "serve", "--disable-compress"]
    }
  }
}
```

The server reads the same config files and `PAW_*` environment variables as the CLI. For local
defaults, start Ollama and pull the drone model before using `compress` or `digest`:

```sh
ollama serve
ollama pull qwen3:8b
```

## Tools

`gather`

- Input: `cwd` and `instruction`.
- Output: a `paw.env/1` Envelope with `stage="gather"` and `raw` context.
- Use when the host wants raw repository slices and search hits.

`compress`

- Input: either `envelope`, or `cwd`, `instruction`, and `raw`.
- Output: a `paw.env/1` Envelope with `stage="compress"` and `digest`.
- Use when the host already has raw context and wants paw's validated digest contract.

`digest`

- Input: `cwd` and `instruction`.
- Output: a `paw.env/1` Envelope after `gather` then `compress`.
- Use as the default sidecar call when the host needs compact context for a task.

Tool results include text JSON plus structured content containing the same Envelope. Invalid tool
arguments return JSON-RPC `InvalidParams` errors. Stage/runtime failures return MCP tool results with
`isError=true`.

## Safety Contract

MCP server mode exposes only context tools:

- no `plan` tool
- no `edit` tool
- no `verify` tool
- no patch application
- no verification command execution

`gather` can read files under the requested `cwd` using the normal gather stage. `compress` and
`digest` can call the configured drone model to produce a digest. They do not call the brain model
or execute the full `gather | compress | plan | edit | verify` loop.

## Sidecar vs `paw run`

Use MCP sidecar mode when an MCP host already owns the conversation, planning, editing, and user
approval flow, and only needs paw's repository context or validated digest.

Use native `paw run` when paw should own the full agent pipeline, including planning, patch
generation, patch application, and verification.

## Smoke Test

With MCP Inspector:

```sh
npx -y @modelcontextprotocol/inspector paw mcp serve --disable-compress
```

In the Inspector UI, list tools and call `digest` with:

```json
{
  "cwd": "/absolute/path/to/repo",
  "instruction": "inspect the project"
}
```

Plain stdio smoke:

```sh
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"smoke","version":"dev"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  | paw mcp serve
```

The response should list `gather`, `compress`, and `digest`.

Protocol reference: <https://modelcontextprotocol.io/specification/2025-11-25>.
