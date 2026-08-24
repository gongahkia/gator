# Curated local coding models

`gator local` provides first-class management for a small reviewed catalog of
local coding models. It uses an already-installed [Ollama](https://ollama.com/)
runtime at its documented loopback address and its OpenAI-compatible Chat
Completions API. Gator still owns the agent loop, tools, worktree, sandbox,
approvals, verifier, sessions, TUI, JSONL RPC, app server, and ACP server.
Only inference moves to the selected local model.

## Start

Install Ollama from its official download page, then start its local server if
your platform did not already start it as a service:

```sh
ollama serve
# equivalent foreground helper; it does not create an unsupervised daemon
gator local serve
```

Gator intentionally does not download and execute an operating-system
installer. That would require a separate provenance, checksum, privilege, and
upgrade policy. It does manage model packages through the loopback runtime:

```sh
gator local status
gator local list
gator local pull qwen2.5-coder-7b --yes
gator local use qwen2.5-coder-7b
gator doctor --provider gator-local

# The selected model is now the persisted default for every native interface.
gator run --verify 'go test ./...' 'Add a focused feature with tests'
```

`pull` shows the package source and approximate download size, then requires
`--yes`; interrupted Ollama pulls resume through Ollama. `remove` likewise
requires `--yes` and removes the selected model from Gator's local provider
catalog. Use a normal custom provider for a self-hosted model or endpoint that
is not listed here.

## Reviewed catalog

| Gator ID | Ollama tag | Published package | Context | Intended use |
| --- | --- | --- | --- | --- |
| `qwen2.5-coder-7b` | `qwen2.5-coder:7b` | 4.7 GB | 32K | smallest recommended code-focused option |
| `qwen2.5-coder-14b` | `qwen2.5-coder:14b` | 9.0 GB | 32K | larger code-focused option |
| `devstral-24b` | `devstral:24b` | 14 GB | 128K | agentic coding with tool use |
| `qwen3-coder-30b` | `qwen3-coder:30b` | 19 GB | 256K | long-context agentic coding |

The catalog maps only to the linked [Qwen2.5-Coder](https://ollama.com/library/qwen2.5-coder),
[Devstral](https://ollama.com/library/devstral), and
[Qwen3-Coder](https://ollama.com/library/qwen3-coder) packages. It is not a
live search of Hugging Face, and Gator does not clone, execute, or download
arbitrary model repositories. That keeps model provenance, package size, and
the tool-calling capability required by Gator's native loop reviewable in
source.

Package size is not a hardware guarantee: available RAM/VRAM or unified
memory, quantization, configured context, and other loaded applications affect
whether a model runs usefully. Review the upstream model card and license
before downloading. Gator does not make a performance or benchmark claim for
any catalog entry.

## Runtime boundary

Management commands accept `--url` only for `http://localhost`, `127.0.0.1`,
or another literal loopback address. The chosen URL is persisted only when
`gator local use` selects a model, as the `gator-local` provider's
`/v1/chat/completions` endpoint. No API key is stored or sent.

This restriction is deliberate. A remote or shared endpoint belongs to the
existing `gator provider add` flow, where its authentication and data-handling
contract can be declared explicitly. `gator local` never enables Ollama's
insecure pull option and never uploads models.

Ollama documents Chat Completions streaming and tool support, which is why the
catalog can retain Gator's tool loop rather than delegating to another harness.
The selected coding models are text models, so do not use `--image` with them;
text attachments retain the normal OpenAI-compatible provider behavior while
PDF inputs remain unavailable on that generic protocol.
