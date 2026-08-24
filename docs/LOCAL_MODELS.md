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

Gator detects missing prerequisites and offers checked-in installation guidance
in `gator doctor` and the native TUI. On Fedora, for example, doctor can point
to the package needed for Git or Bubblewrap; it links Ollama to its official
download page. Gator intentionally does not download and execute an
operating-system installer, package-manager command, or remote install script.
That would require a separate provenance, checksum, privilege, and upgrade
policy. It does manage model packages through the loopback runtime:

```sh
gator doctor
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

## Native TUI

The terminal UI has one model-management command: start `gator`, then enter
`/model` in the composer. Its Cloud section shows direct/cloud provider
readiness from local credential/configuration evidence. It lists the complete
checked-in OpenCode Zen and Go gateway model catalogs, plus one stable default
for every other direct provider; it does not claim to discover every model
enabled by an account. Press `c` to configure the selected built-in cloud
provider: model, masked credential where applicable, endpoint, and provider
metadata. The form includes Azure endpoint/version, Bedrock region/profile,
Vertex project/location/ADC path, Cloudflare account/gateway metadata, and
Radius gateway routing. Press `l` to start an eligible provider sign-in and
`u` or Enter to select a configured cloud model. The separate Claude Code entry
uses `c` to record an Anthropic API key for the delegated harness; it never
reuses a Claude.ai or Claude Code subscription credential.

Press Tab to open the Local section. It checks the loopback runtime and shows
the same reviewed catalog and installed state as `gator local status`. It also
shows the detected host and marks models that Gator has disabled because they
do not satisfy its local admission guardrail.

- `↑`/`↓` chooses a catalog model; `p` shows the source and approximate size,
  then `y` or Enter confirms its download. A native animated spinner and
  bounded Ollama progress update while it downloads.
- `u` or Enter selects an installed model and immediately updates the current
  TUI's provider and model for the next task. `Esc` returns to the composer;
  use `/verify` to edit the verification allowlist, then send the task as
  normal.
- `e` assigns a persistent local display label to either cloud or local model
  entries; it does not change the identifier sent to the provider. An empty
  label restores the original label. `x` requires a second confirmation before
  removing local model data; `r` refreshes runtime and inventory state.

When the Local section finds that the Ollama executable is missing, it asks to
open installation help. Press Enter or `y` for the default choice to see the
official source and platform guidance, or press `n` or Esc to install it
yourself. Press `i` at any time in the Local section to review all detected
missing local prerequisites. Gator does not execute those installers.

When the executable is installed but its loopback runtime is unavailable, the
TUI instead asks how to start it. Press Enter or `y` for the default **Start
with Gator** choice: Gator starts `ollama serve` as a child of that TUI session,
waits for its loopback API, and stops only that child when Gator exits. Press
`n` or Esc to start it yourself with `ollama serve` or `gator local serve`,
then press `r` to refresh. Gator never adopts or stops an Ollama service that
was already running.

## Host eligibility guardrail

`gator doctor` detects the operating system, architecture, total RAM, currently
available RAM where the platform reports it, and free space in the Ollama model
filesystem. It reports each reviewed model as enabled or disabled and explains
the exact blocking condition. Gator recognizes Linux, macOS, and Windows on
`amd64` and `arm64`; an unrecognized OS or architecture is disabled rather than
assumed compatible.

The published pull size is the only stable per-model resource fact in this
catalog. Gator therefore uses a deliberately conservative admission policy,
not an upstream hardware claim: it requires twice that published size in both
physical RAM and (when measurable) currently available RAM, plus 20% free disk
headroom for the pull. The current catalog evaluates to:

| Gator ID | Gator RAM guardrail | Gator free-disk guardrail |
| --- | ---: | ---: |
| `qwen2.5-coder-0.5b` | 759 MiB | 455 MiB |
| `qwen2.5-coder-1.5b` | 1.8 GiB | 1.1 GiB |
| `qwen2.5-coder-3b` | 3.5 GiB | 2.1 GiB |
| `qwen2.5-coder-7b` | 8.8 GiB | 5.3 GiB |
| `qwen2.5-coder-14b` | 16.8 GiB | 10.1 GiB |
| `qwen2.5-coder-32b` | 37.3 GiB | 22.4 GiB |
| `devstral-24b` | 26.1 GiB | 15.6 GiB |
| `qwen3-coder-30b` | 35.4 GiB | 21.2 GiB |

Gator blocks `local pull`, `local use`, TUI download/selection, and every
managed-local executor setup (including a previously selected model) when the
host fails this policy. It leaves rename and removal available, so a disabled
model can be removed to reclaim disk. This is intentionally a capacity safety
check: it does not guarantee useful speed or full context support.

Gator uses `OLLAMA_MODELS` when it is present. Otherwise it follows Ollama's
documented default model storage location for the detected OS. If Ollama runs
as a Linux service, set `OLLAMA_MODELS` in that service's environment as well
as the invoking environment when using a non-default disk. See the
[Ollama FAQ](https://docs.ollama.com/faq) for model locations and service
environment setup.

GPU compatibility, available VRAM, context length, quantization, and parallel
requests are not safely derivable from a generic laptop inspection. Ollama
chooses its runtime backend; after a model is loaded, run `ollama ps` to see
whether it is using CPU, GPU, or both. Larger context and parallel requests can
raise memory use beyond Gator's guardrail, so keep them conservative on a
resource-constrained machine.

## Reviewed catalog

| Gator ID | Ollama tag | Published package | Context | Intended use |
| --- | --- | --- | --- | --- |
| `qwen2.5-coder-0.5b` | `qwen2.5-coder:0.5b` | 398 MB | 32K | minimal coding option for constrained hardware |
| `qwen2.5-coder-1.5b` | `qwen2.5-coder:1.5b` | 986 MB | 32K | compact code-focused option for low-memory hosts |
| `qwen2.5-coder-3b` | `qwen2.5-coder:3b` | 1.9 GB | 32K | small code-focused option for everyday laptops |
| `qwen2.5-coder-7b` | `qwen2.5-coder:7b` | 4.7 GB | 32K | balanced code-focused option |
| `qwen2.5-coder-14b` | `qwen2.5-coder:14b` | 9.0 GB | 32K | larger code-focused option |
| `qwen2.5-coder-32b` | `qwen2.5-coder:32b` | 20 GB | 32K | largest Qwen2.5-Coder option for high-memory hosts |
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
whether a model runs usefully. The Gator guardrail above protects against clear
capacity shortfalls, but cannot make a performance or benchmark claim for any
catalog entry. Review the upstream model card and license before downloading.

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
