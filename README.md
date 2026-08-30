# Gator

Gator is a native, terminal-first coding agent for developers who want an
inspectable route from a task to a tested patch.

It runs an agent inside an isolated Git worktree, streams the exact actions it
takes, and leaves the developer with a diff and verification evidence to
review. Feature work is a primary workflow: Gator is intended to explore an
unfamiliar repository, implement a bounded multi-file change, add or update
tests, and propose the resulting patch.

## Status

The runnable milestone includes a Gator-owned native loop, direct cloud-provider
adapters, worktree-local tools, command policy, durable local run storage, a
full-screen terminal application, an automation protocol, and a headless
evaluation harness. See [evaluation](docs/EVALUATION.md) and [release
evidence](docs/RELEASE_EVIDENCE.md) for its limits, the automated checks, and
the dogfood checklist. No GitHub release has been published yet; real-repository
dogfood logs are still the human gate for a daily-driver claim.

## Install and update

No GitHub release has been published yet. To run the current source, build it
from a checkout:

```sh
make build
./bin/gator
```

Gator supports macOS and Linux on `amd64` and `arm64`. The tagged-release
workflow publishes checksum-verified archives for those platforms. Once a
release exists, install with:

```sh
curl -fsSL https://raw.githubusercontent.com/gongahkia/gator/main/scripts/install.sh | sh
gator update
```

Use `gator update --check` to inspect a published update without changing the
binary.

## Development

```sh
make check
make build
./bin/gator
./bin/gator help
./bin/gator doctor
./bin/gator version
# Offline evaluation exercises harness mechanics only.
./bin/gator eval ./internal/eval/testdata/greeting --run-id greeting-script-001 \
  --report /tmp/gator-eval/greeting-script-001.json --require-resolved

# Real-model evaluation is noninteractive and writes a per-case suite report.
# See docs/EVALUATION.md for safe container execution and evidence criteria.
OPENAI_API_KEY=... ./bin/gator eval suite ./internal/eval/testdata/core-v1 \
  --live --provider openai --model gpt-5.6 \
  --environment-id 'gator-eval@sha256:IMAGE_DIGEST' --attempts 3 \
  --report-dir /tmp/gator-eval/core-001 --require-resolved
./bin/gator config set default-provider anthropic
./bin/gator config set default-model claude-sonnet-4-6
OPENAI_API_KEY=... ./bin/gator run --provider openai --verify 'go test ./...' \
  --allow-command 'go test ./internal/foo' \
  'Add a focused feature with tests'

# Explicitly include repository-local visual and document context in a native
# run. `--image` accepts PNG, JPEG, and WebP; `--attach` accepts PDFs and
# supported text or Office documents.
OPENAI_API_KEY=... ./bin/gator run --provider openai --verify 'go test ./...' \
  --image screenshot.png --attach design.pdf \
  'Match the attached design and verify the implementation'

ANTHROPIC_API_KEY=... ./bin/gator run --provider anthropic \
  --model claude-sonnet-4-6 --verify 'go test ./...' \
  'Add a focused feature with tests'

GEMINI_API_KEY=... ./bin/gator run --provider gemini \
  --model gemini-3.5-flash --verify 'go test ./...' \
  'Add a focused feature with tests'

# `connect` is the no-registration subscription route when a vendor CLI offers
# a public login. Credentials stay in that CLI's credential store.
./bin/gator connect codex            # first-party Codex browser login
./bin/gator connect copilot          # first-party Copilot device login
./bin/gator connect kimi             # first-party Kimi device login
./bin/gator connect xai              # OpenCode's provider-approved Grok OAuth
./bin/gator connect openrouter       # browser PKCE flow, mints a Gator API key

# Claude Code delegation uses a standard Anthropic API key, which `connect`
# persists in Gator's private credential file and passes only to --bare runs.
ANTHROPIC_API_KEY=... ./bin/gator connect claude

# Radius has no verified public vendor-CLI subscription route. Its API key is
# the supported no-registration path for native Gator runs.
RADIUS_API_KEY=... ./bin/gator connect radius

# Native Gator account OAuth is an advanced integration: Gator's client must
# be registered with the provider. It is distinct from the recommended `connect`
# route and Gator never impersonates another application's OAuth client.
GATOR_CODEX_OAUTH_CLIENT_ID=... ./bin/gator login codex
GATOR_COPILOT_OAUTH_CLIENT_ID=... ./bin/gator login copilot
GATOR_XAI_OAUTH_CLIENT_ID=... ./bin/gator login xai --subscription
GATOR_KIMI_CODE_OAUTH_CLIENT_ID=... ./bin/gator login kimi-coding --subscription
GATOR_RADIUS_OAUTH_CLIENT_ID=... ./bin/gator login radius --subscription

# Delegated runtimes are separate from native Gator providers. The installed
# harness owns its agent execution; Gator creates a worktree and runs required
# verification commands after the harness exits.
./bin/gator delegate codex run --verify 'go test ./...' \
  'Add a focused feature with tests'
./bin/gator delegate copilot run --verify 'go test ./...' \
  'Add a focused feature with tests'
./bin/gator delegate kimi run --verify 'go test ./...' \
  'Add a focused feature with tests'
./bin/gator delegate opencode run --model xai/grok-build --verify 'go test ./...' \
  'Add a focused feature with tests'

# Claude Code delegation intentionally uses only an API key. --bare prevents
# Claude Code from reusing Claude.ai OAuth or its stored CLI credentials.
./bin/gator delegate claude run --verify 'go test ./...' \
  'Add a focused feature with tests'

# Open a project-scoped retained-thread picker, continue the latest thread,
# or target an ID directly. The task is optional: without it, Gator opens the
# resumed conversation in the TUI.
./bin/gator resume
./bin/gator resume --last
./bin/gator resume --last --compact
./bin/gator resume thread-identifier
./bin/gator resume --all
OPENAI_API_KEY=... ./bin/gator resume --last \
  --image regression.png --attach browser-log.txt \
  'Address the failing verification and finish the patch'

# The original raw run-record form remains supported.
OPENAI_API_KEY=... ./bin/gator resume /path/to/run-record \
  'Address the failing verification and finish the patch'

# Fork a selected retained turn into a separate worktree, or clone the current
# retained head. Both retain the original source thread unchanged.
./bin/gator fork --last 'Try the smaller implementation instead'
./bin/gator clone --last 'Repeat the active direction with another constraint'

# Save a portable patch or apply it explicitly to a clean compatible checkout.
./bin/gator export /path/to/run-record > gator-review.patch
./bin/gator apply --check /path/to/run-record
./bin/gator apply /path/to/run-record

# Export a reviewable local HTML transcript. Gator does not upload it.
./bin/gator transcript /path/to/run-record > gator-transcript.html
```

`gator` opens a conversation-first terminal application when run from a Git
checkout and a real terminal. Type a task and press `Enter` to send it; the
same prompt accepts follow-up instructions after the run completes. `Ctrl+R`
also sends in every input mode. The live
conversation includes model text and tool activity, while `PgUp` and `PgDn`
browse earlier entries; the mouse wheel does the same. Use `/copy` to copy the
latest Gator response or `/copyall` to copy the complete plain-text conversation
transcript. Use `/model` to select and manage cloud or local models, and
`/verify` to edit the run configuration. During a run, `Enter` sends a steering instruction
that the agent consumes at its next model or tool boundary; it does not cancel
the run. If the agent requests a command that is not a required verifier, the
running view instead asks for approval: `y` or `Enter` allow once, `a` always
allow that exact argv for the rest of the thread, and `n` deny. Denied commands
return a tool error; the loop continues. Native commands use the configured
strict sandbox by default: macOS uses Seatbelt and Linux uses Bubblewrap, both
with a private environment, worktree-only writes, and denied network access by
default. On an unsupported platform strict mode fails closed; `--sandbox off`
is the explicit host-access escape hatch. `Tab` queues the current prompt for the next turn, and a second `Tab`
queues an already-completed slash command. Gator runs queued work in FIFO order
only after the active run succeeds. `/queue`, `/dequeue`, and `/clear-queue`
inspect or manage the bounded, 16-item in-memory queue. Failed or cancelled
runs leave queued instructions paused for the developer to inspect; the queue
is cleared when the TUI exits and is never written to a retained session.
`Ctrl+C` clears the composer message without touching the active run. During a
run, `Ctrl+X` requests cancellation while retaining the isolated worktree;
`Ctrl+Q` exits Gator and cancels an active native run during cleanup. The
running view shows an event-backed activity phase, current turn, provider mode,
verifier state, and a preview of the next queued work; it does not estimate
token use or fake percentage progress. After a run, the latest result
summarizes verifier status and the visible diff's file/addition/deletion counts.
Failure states include the reason plus the relevant next action.
Use `/review` for a retained-worktree review surface with an expanded changed
file tree, additions/deletions, hunk navigation, focused/raw rendering per
file, explicit staged/unstaged file or hunk operations, and range-scoped
change requests. `v` starts a line range, `r` opens a request, and `Ctrl+R`
sends that bounded context as a retained-thread follow-up. `e`
still shows explicit patch handoff commands; the active checkout is never
changed from review. See [Review surfaces](docs/REVIEW.md) for the full
terminal controls and the separate authenticated loopback browser surface:

```sh
./bin/gator review RUN_RECORD_PATH --open
```

`/vim` toggles Vim-style message editing. Vim Normal mode provides counts,
`h`/`j`/`k`/`l`, `0`/`^`/`$`, `w`/`b`/`e`, `gg`/`G`, insert commands
`i`/`I`/`a`/`A`/`o`/`O`, `x`/`X`/`r`, operator motions with `d`/`c`/`y`,
`p`/`P`, and `u` / `Ctrl+R` undo and redo. The composer renders Vim's hybrid
line-number style by default: the current line is absolute and all other lines
are relative. Use `:set number`, `:set relativenumber`, `:set nonumber`, or
`:set norelativenumber` to configure the gutter. `Enter` sends from Normal
mode; Insert-mode `Enter` adds a line and `Esc` returns to Normal mode. Ex
commands include `:w` to send, `:wq` or `:x` to send then exit after successful
active and queued work, `:q` / `:q!` to exit, `:set`, and `:help`.

Before sending, Gator validates the task and local prerequisites: a verifier
list in Execute mode, valid provider settings, required API-key environment
variables, and provider-specific model/base-URL
requirements. Press `F1` for the conversation, running, and review shortcut
reference. `Ctrl+O` opens recent retained threads for the current repository;
press `a` in that picker to switch between the current repository and all
locally retained repositories, or `p` to type a thread ID, unique prefix, or
run-record path. The picker shows a summary rather than private
run-record paths and validates the selected worktree when continuation begins.
`/resume TARGET [instruction…]`, `/fork TARGET [instruction…]`, and
`/clone TARGET [instruction…]` bind the same way; an instruction starts that
continuation immediately. `/tree` opens an in-terminal navigator for the active retained thread. It
shows every saved turn from root to head with its mode, provider/model, status,
timestamp, prompt summary, selected response, and any independent forks. `y`
opens the same navigator from review. Press `f` on an earlier turn to start an
alternate branch in a fresh isolated worktree; `/clone` branches from the
current retained head. Neither operation modifies the source thread.

Gator automatically summarizes older retained context once it exceeds its
local threshold, retaining the newest messages and recording a visible
`context_compacted` event. Use `/compact` (or `gator resume --compact`) to
request that summary before the next retained turn. The original private run
records remain intact for inspection and forking.

Type `/` (or `?` in an empty message) to filter and select local conversation commands;
type `@` in a task to select a repository file or directory from matching path
suggestions. In the command menu, `Tab` completes the selected command and
`Enter` executes it; path suggestions accept either key to insert the path.
Conversation
commands are `/plan`, `/execute`, `/new`, `/status`, `/effort`, `/model`, `/manage`, `/extensions`, `/verify`,
`/permissions`, `/worktree`, `/review`, `/run`, `/resume`, `/threads`,
`/recent`, `/tree`, `/fork`, `/clone`, `/compact`, `/clear`, `/queue`,
`/dequeue`, `/clear-queue`, `/help`, and `/quit`. `/plan` gives every
provider an enforced read-only tool surface and does not require a verifier;
switch the same retained thread to `/execute` when you are ready to make edits.
`/manage` opens an inspect-first local control center for execution defaults,
project hook/LSP/MCP/extension trust, retained runs and worktrees, writer-child
manifests, and installed extension state. It can save the currently selected
provider/model as the default, change strict/off sandbox and deny/allow network
defaults after an explicit confirmation, privately export a retained patch or
HTML transcript below Gator's state directory, and transfer a retained patch
only after a clean-checkout compatibility check plus a separate apply
confirmation. Trust changes, extension removal, and worktree removal also
require confirmation; project executable content remains hash-pinned.
`Ctrl+Space` (reported as `Ctrl+@`
by many terminals) reopens an active `@` path menu. Use `@path/to/file` or `@"path with spaces"`
in a task to mark repository files or directories that the agent should inspect
first. Gator validates every reference against the repository boundary before
starting. Ordinary references remain paths only. Before supported attachments
are sent, Gator shows the exact files, sizes, and selected provider and requires
an explicit confirmation.

Use `@mock.png`, `@screenshot.jpg`, or `@design.webp` to attach images, and
`@report.pdf` to attach a PDF, to a direct-provider task. The non-interactive
equivalents are `--image PATH` and `--attach PATH` on `run`, `resume`, `fork`,
and `clone`; paths are always relative to the relevant repository. Gator also accepts
`@` references to UTF-8 text/data documents (`.txt`, Markdown, CSV, JSON,
YAML, TOML, XML, HTML, logs, and common config files) plus `.docx`, `.odt`,
and `.xlsx`. Office files are extracted locally into plain text; PDFs retain
their original bytes so OpenAI, Anthropic, and Gemini can use their documented
document inputs. Text/data attachments work with every direct API adapter;
PDFs require the `openai`, `codex`, `anthropic`, or `gemini` provider because generic
Chat Completions endpoints do not share a stable document-input protocol.

When a selected subscription provider has no Gator-registered OAuth client,
the Cloud section of `/model` opens its supported vendor-owned `gator connect`
flow in the same terminal and returns to Gator afterward. That sign-in remains
owned by the vendor CLI; Gator makes the next delegated-harness action explicit
rather than silently treating the credential as a native Gator credential.

Gator permits at most four attachments per task, each up to 4 MiB, with an
8 MiB combined image/document budget. Attachments must be regular files inside
the repository and are read through a descriptor-rooted workspace boundary.
Raw attachment bytes are never written to the private `0600` continuation
session; Gator retains only a name, media type, size, and SHA-256 manifest, so
you must explicitly re-add `@` files or `--image` / `--attach` paths in a later
turn. The selected provider
receives confirmed bytes under its own data-handling and retention policy.
The byte budget limits local input size but does not cap a PDF's provider-side
page or token cost. Attachment text is framed as untrusted data, but no LLM
prompt can make a model fully immune to prompt injection.
Unsupported binary `@` paths remain ordinary references that the agent can
inspect if its tools can read them. In the running view, file reads, commands,
tool results, and patch line changes are visible as they occur. Press `t` from
review to browse the current run transcript; it shows model-emitted text and
tool activity, not hidden chain-of-thought.

The review screen deliberately does not modify the active checkout. To hand off
reviewed work, `gator export RUN_RECORD_PATH` emits a binary-safe patch to
standard output. `gator apply --check RUN_RECORD_PATH` verifies that the current
checkout is clean and compatible; omitting `--check` applies the patch. Gator
does not stage, commit, or push the result. The existing `run` and `resume`
commands remain available for scripts and CI-like usage.

`gator connect` is the shortest compliant onboarding path for subscription
providers. It invokes an installed Codex, GitHub Copilot, or Kimi CLI for that
vendor's own sign-in. `gator connect xai` invokes OpenCode's xAI provider
picker, which offers browser OAuth, device-code OAuth, or an API key. `gator
connect claude` stores an Anthropic API key for Gator's API-key Claude Code
harness route; it does not access a Claude.ai or Claude Code subscription
credential. Gator does not store, copy, or translate any credential another
harness creates.

`gator delegate` is the explicit alternative for an installed agent CLI. It
creates and retains the same Git worktree, then executes Gator's required
verification commands after the delegated agent exits. The delegated CLI owns
its own tool policy, sandbox, approvals, session history, and credentials, so
delegated runs do not support Gator steering or `gator resume`. Copilot's
non-interactive delegate grants its own tools but does not disable its path or
URL approval controls. Claude delegation passes either `ANTHROPIC_API_KEY` or
Gator's stored Anthropic API key to `claude --bare`, which prevents reuse of
Claude.ai OAuth or Keychain credentials. For another installed harness, use
`gator delegate external run --task '...' --verify '...' -- command {task}`.
`{task}` and `{worktree}` are safe whole-argument placeholders, and the process
receives `GATOR_TASK` and `GATOR_WORKTREE`; its authentication and automation
contract remain its own responsibility.

For an ACP-capable editor, use `gator acp` rather than parsing human CLI output
or private journal files. It implements the local stdio ACP v1 session and
permission lifecycle while retaining Gator's worktree and verifier policy; see
[ACP integration](docs/ACP.md). For CI or a bespoke control plane, `gator rpc`
remains Gator's versioned JSONL protocol with explicit run, resume, steer,
cancel, approve, status, and thread-listing methods; see
[RPC integration](docs/RPC.md). A local editor or controller that prefers HTTP
can run the authenticated loopback `gator serve` bridge: it accepts that same
RPC request type and streams the same events over SSE without creating a second
agent policy. It can also control a model-detached terminal within that one
authenticated server process. Use `gator serve start --token-file PATH` for an
explicit repository-scoped background bridge, or run `gator serve` in the
foreground; see [local app server](docs/APP_SERVER.md).

Gator extensions provide reusable skills, prompt guidance, and optional
language-neutral JSON sidecar tools. Install a bundle with `gator extension
install DIRECTORY`; project bundles under `.gator/extensions/` stay inactive
until `gator extension trust` records their full content hash in that
repository. Sidecar calls require approval and run inside the active sandbox,
not with implicit host access. Read the exact manifest, lifecycle, protocol,
and trust boundary in [Extensions](docs/EXTENSIONS.md).

For a local server or a provider with a compatible endpoint, add it once with
`gator provider add ID --base-url URL --model MODEL`. The endpoint must speak
OpenAI Chat Completions; the declared model list is the only selectable list.
`--api-key-env NAME` references an environment variable without storing a
secret, while omitting it is the supported keyless-local-server route.
`/model` can create, edit, discover, and remove the same metadata after
confirmation. `gator logout PROVIDER` and `/model` `d` remove only a stored
Gator credential and report remaining ambient sources; they do not unset
environment variables or vendor CLI logins.

For a reviewed local coding model running through Ollama, use the first-class
catalog instead of manually configuring the endpoint:

```sh
gator local status
gator local list
gator local pull qwen2.5-coder-7b --yes
gator local use qwen2.5-coder-7b
gator run --verify 'go test ./...' 'Add a focused feature with tests'
```

Gator manages model packages only through a literal loopback Ollama runtime;
it does not execute arbitrary model repositories or install a system runtime.
In the primary terminal UI, `/model` is the single cloud-and-local catalog: it
shows credential/configuration readiness for cloud choices, includes the
checked-in OpenCode Zen and Go model catalogs, signs into eligible providers,
provides secure built-in cloud configuration, custom OpenAI-compatible provider
editing, stored-credential removal, and reviewed local pull, selection,
display-name rename, refresh, and removal controls. Press `c` on a built-in
cloud entry to set the exact model, a masked credential where applicable, an
endpoint, and its provider-specific non-secret settings. Press `d` to remove a
stored Gator credential after confirmation; that deletes only the private
`auth.json` entry and reports any remaining environment, AWS, or ADC source
instead of claiming a full logout. Press `n` to create a custom Chat Completions
provider, `c` on a custom row to edit it, `g` to preview `/models`, and `x` to
remove its metadata. Custom providers store an environment variable name, never
an API key. Built-in cloud configuration covers Azure endpoints, Bedrock
region/profile, Vertex project/location/ADC path, and Cloudflare metadata;
credentials never enter `config.json`, a draft, or a transcript. Codex and
Copilot use `l` for account OAuth; Claude Code's `c` form stores an Anthropic
API key for the delegated harness and never reuses Claude.ai or Claude Code
subscription credentials. After selection, return to the composer and send a
verified task normally. The chosen native model becomes
the default provider, so the same TUI, tools, worktrees, sandbox, verification,
retained sessions, RPC, and ACP paths apply.
See [curated local models](docs/LOCAL_MODELS.md) for model sources, sizes,
hardware limitations, and removal.

Gator keeps terminal customization deliberately small and readable: `gator
theme set gator|contrast|mono` persists one named theme, and `/theme NAME`
applies it immediately in the terminal UI.

## Providers

Set persistent defaults with `gator config set default-provider NAME` and
`gator config set default-model NAME`. `GATOR_PROVIDER`, `GATOR_MODEL`, and
one-run flags override those settings for automation. `--base-url` overrides
an endpoint for one scripted run.

| Provider | Authentication | Protocol |
| --- | --- | --- |
| `openai` | `OPENAI_API_KEY` | OpenAI Responses API |
| `codex` | ChatGPT/Codex account OAuth using a Gator-registered client; `gator connect codex` delegates first-party CLI login | ChatGPT Codex Responses endpoint |
| `anthropic` | `ANTHROPIC_API_KEY` | Anthropic Messages API |
| `copilot` | GitHub Copilot account OAuth using a Gator-registered client; `gator connect copilot` delegates first-party CLI login | Copilot Chat Completions endpoint and account model catalog |
| `kimi-coding` | `KIMI_API_KEY` or Kimi Code account OAuth using a Gator-registered client; `gator connect kimi` delegates first-party CLI login | Kimi's Anthropic-compatible coding Messages endpoint |
| `radius` | `RADIUS_API_KEY` or Radius account OAuth using a Gator-registered client | Radius `pi-messages` gateway protocol |
| `gemini` | `GEMINI_API_KEY` | Gemini GenerateContent API |
| `azure-openai` | `AZURE_OPENAI_API_KEY` plus `--base-url` and deployment model | Azure OpenAI-compatible Chat Completions |
| `azure-openai-responses` | `AZURE_OPENAI_API_KEY` or `AZURE_OPENAI_AUTH_TOKEN` plus `--base-url`, `AZURE_OPENAI_BASE_URL`, or `AZURE_OPENAI_RESOURCE_NAME` | Azure OpenAI Responses API |
| `mistral`, `groq`, `together`, `fireworks`, `deepseek` | Provider-specific API key | OpenAI-compatible Chat Completions |
| `cerebras` | `CEREBRAS_API_KEY` | Cerebras OpenAI-compatible Chat Completions |
| `nvidia` | `NVIDIA_API_KEY` | NVIDIA NIM OpenAI-compatible Chat Completions |
| `huggingface` | `HF_TOKEN` | Hugging Face Inference Providers Chat Completions |
| `moonshotai` | `MOONSHOT_API_KEY` | Moonshot AI Kimi OpenAI-compatible Chat Completions |
| `zai` | `ZAI_API_KEY` | Z.AI GLM Coding Plan OpenAI-compatible Chat Completions |
| `zai-coding-cn` | `ZAI_CODING_CN_API_KEY` | Z.AI GLM Coding Plan China OpenAI-compatible Chat Completions |
| `minimax` | `MINIMAX_API_KEY` | MiniMax Anthropic-compatible Messages API |
| `minimax-cn` | `MINIMAX_CN_API_KEY` | MiniMax China Anthropic-compatible Messages API |
| `baseten` | `BASETEN_API_KEY` | Baseten OpenAI-compatible Chat Completions |
| `vercel-ai-gateway` | `AI_GATEWAY_API_KEY` | Vercel AI Gateway OpenAI-compatible Chat Completions |
| `ant-ling` | `ANT_LING_API_KEY` | Ant Ling OpenAI-compatible Chat Completions |
| `xiaomi` | `MIMO_API_KEY` | Xiaomi MiMo OpenAI-compatible Chat Completions |
| `moonshotai-cn` | `MOONSHOT_API_KEY` | Moonshot AI Kimi China OpenAI-compatible Chat Completions |
| `cloudflare-workers-ai` | `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` | Cloudflare Workers AI OpenAI-compatible Chat Completions |
| `cloudflare-ai-gateway` | `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`, `CLOUDFLARE_AI_GATEWAY_ID`, and `GATOR_CLOUDFLARE_GATEWAY_PROTOCOL` | Cloudflare AI Gateway native OpenAI Responses, Anthropic Messages, or Workers AI Chat Completions |
| `amazon-bedrock` | `AWS_BEARER_TOKEN_BEDROCK`, or standard ambient AWS credentials with optional `AWS_REGION` / `AWS_PROFILE` | Amazon Bedrock OpenAI-compatible Chat Completions |
| `google-vertex` | Google ADC plus `GOOGLE_CLOUD_PROJECT` and `GOOGLE_CLOUD_LOCATION`; optionally `GOOGLE_APPLICATION_CREDENTIALS` or `GATOR_VERTEX_ACCESS_TOKEN` | Google Vertex AI OpenAI-compatible Chat Completions |
| `qwen-token-plan`, `qwen-token-plan-individual` | `QWEN_TOKEN_PLAN_API_KEY` | Qwen Token Plan OpenAI-compatible Chat Completions |
| `qwen-token-plan-cn` | `QWEN_TOKEN_PLAN_CN_API_KEY` | Qwen Token Plan China OpenAI-compatible Chat Completions |
| `xiaomi-token-plan-cn`, `xiaomi-token-plan-ams`, `xiaomi-token-plan-sgp` | `MIMO_API_KEY` | Xiaomi MiMo prepaid Token Plan Chat Completions |
| `xai` | `XAI_API_KEY`, or Grok/X account OAuth using a Gator-registered client | OpenAI-compatible Chat Completions |
| `openrouter` | `OPENROUTER_API_KEY`, or a browser-minted user-controlled API key | OpenAI-compatible Chat Completions |
| `opencode` | `OPENCODE_API_KEY` | OpenCode Zen direct gateway selected by Gator's checked-in model/protocol catalog |
| `opencode-go` | `OPENCODE_API_KEY` | OpenCode Go direct gateway selected by Gator's checked-in model/protocol catalog |
| `openai-compatible` | `GATOR_COMPATIBLE_API_KEY` plus `--base-url` | Any compatible Chat Completions endpoint |

`gator login PROVIDER` stores an API-key credential, while `gator login
PROVIDER --subscription` runs a native Gator account OAuth flow. Both write
only Gator's own credential material to
`$XDG_STATE_HOME/gator/auth.json` (or `~/.local/state/gator/auth.json`) with
`0600` permissions. Explicit `--api-key` wins over the stored credential,
which wins over the provider environment variable. The Cloud section of
`/model` displays a browser or device-code URL for eligible providers and waits
for completion; `Ctrl+C` cancels the pending login. Codex, Copilot, xAI, Kimi Code, and Radius require
their corresponding `GATOR_*_OAUTH_CLIENT_ID` registration. OpenRouter's flow
does not use a client ID: it exchanges a PKCE authorization code for a
user-controlled API key. Prefer `gator connect` when it offers a public
vendor-CLI path. Claude.ai subscription OAuth is intentionally not offered by
Gator. Use provider `anthropic` or `gator delegate claude run` with an
Anthropic API key; provider billing and eligibility remain provider-defined.

The predefined compatible providers use these key variables respectively:
`MISTRAL_API_KEY`, `XAI_API_KEY`, `GROQ_API_KEY`, `OPENROUTER_API_KEY`,
`TOGETHER_API_KEY`, `FIREWORKS_API_KEY`, `DEEPSEEK_API_KEY`,
`CEREBRAS_API_KEY`, `NVIDIA_API_KEY`, `HF_TOKEN`, `MOONSHOT_API_KEY`,
`ZAI_API_KEY`, `ZAI_CODING_CN_API_KEY`, `MINIMAX_API_KEY`, `MINIMAX_CN_API_KEY`,
`OPENCODE_API_KEY`, `BASETEN_API_KEY`, `AI_GATEWAY_API_KEY`,
`ANT_LING_API_KEY`, `MIMO_API_KEY`, `QWEN_TOKEN_PLAN_API_KEY`,
`QWEN_TOKEN_PLAN_CN_API_KEY`, and `CLOUDFLARE_API_TOKEN`. Direct Workers AI
requires `CLOUDFLARE_ACCOUNT_ID` unless a full `--base-url` is supplied.
Cloudflare AI Gateway additionally requires `CLOUDFLARE_AI_GATEWAY_ID` and an
explicit `GATOR_CLOUDFLARE_GATEWAY_PROTOCOL`: `openai-responses` requires an
`openai/` model, `anthropic-messages` an `anthropic/` model, and
`workers-ai-chat-completions` an `@cf/` model. Gator uses Cloudflare's account
REST API with the `cf-aig-gateway-id` request header; it neither infers this
protocol from a model name nor alters direct Workers AI behavior.
Azure Responses uses `AZURE_OPENAI_API_KEY` when present, otherwise it accepts
the short-lived Microsoft Entra bearer token in `AZURE_OPENAI_AUTH_TOKEN` and
sends it only as `Authorization: Bearer`; the two authentication headers are
never combined. Obtain that token for the Azure Cognitive Services scope
`https://cognitiveservices.azure.com/.default` and replace it when it expires.
`gator login azure-openai-responses --bearer-token-from-env AZURE_OPENAI_AUTH_TOKEN`
may retain a provider-scoped token in Gator's private credential store, but
Gator cannot refresh it because it does not own the issuing OAuth client.
Bedrock preserves `AWS_BEARER_TOKEN_BEDROCK` as the explicit bearer-token path.
When no bearer token is supplied, Gator uses the AWS SDK's standard credential
chain (environment credentials, web identity, shared profiles, ECS task role,
and EC2 instance role) and directly SigV4-signs the Bedrock Runtime Chat
Completions request with service `bedrock`; it does not invoke the AWS CLI.
The default region is `us-east-1` when the standard AWS configuration does not
set one. `/model` can persist a region and shared-profile selection without
copying AWS credentials. Azure Responses normalizes resource roots to `/openai/v1/responses`
and uses `AZURE_OPENAI_API_VERSION` (default `v1`) when the base URL has no
`api-version` query. Vertex refreshes Google Application Default Credentials directly,
or accepts `GATOR_VERTEX_ACCESS_TOKEN` for an externally managed short-lived
token. Set `--model`
for compatible providers without a Gator default. `gator doctor --provider NAME`
reports the selected provider's prerequisite without printing a secret.

The OpenCode catalog is a versioned checked-in snapshot of its published Zen
and Go endpoint tables. It has no runtime network refresh; Gator rejects an
unknown OpenCode model with a configuration error until a maintainer refreshes
the catalog deliberately. The direct adapters preserve Gator's strict tool surface: repository reads and
writes stay inside the isolated worktree and commands are limited to the
explicit `--verify` argv entries. They replay normalized history locally; the
Gemini adapter also retains the provider content needed for thought-signature
tool-call replay in the private session file.

### Provider ownership

Native `gator run` never starts Codex, Claude Code, GitHub Copilot, Kimi, or
Cursor Agent. Its `codex`, `copilot`, `kimi-coding`, `radius`, `xai`,
`openrouter`, `opencode`, and `opencode-go` paths make direct model requests
with credentials Gator creates and stores itself; they do not reuse a vendor
CLI session or read another application's OAuth files, tokens, or API keys.
Gator refreshes a near-expiry credential before starting a native run. Cursor
remains unavailable rather than falling back to its vendor CLI: no public
direct Cursor inference contract was verified.

`gator delegate` is a separate, explicit product boundary rather than a native
provider fallback. It can launch Codex, Claude Code, Copilot, Kimi, OpenCode,
or another explicitly selected harness, and has the delegated-runtime
limitations described above.

`google-vertex` is the explicit cloud-credential exception: it reads Google
Application Default Credentials (or `GATOR_VERTEX_ACCESS_TOKEN`) and refreshes
them by direct OAuth requests. `/model` can select a non-secret absolute ADC
file path, project, and location; it never copies ADC content into Gator's
settings. It does not invoke `gcloud`, start a coding agent, or delegate
Gator's tool loop.

## Local execution and trusted project capabilities

`run_command` uses the strict sandbox by default: macOS uses Seatbelt and
Linux uses Bubblewrap, with a filtered environment, private scratch directory,
worktree-scoped writes, and denied network access. Strict mode fails closed
where no platform adapter is available. `--sandbox off`, `--network allow`,
and user-configured filesystem/environment grants are explicit capability
grants, not defaults.

Gator resolves root, `.gator`, and scope-specific `AGENTS.md` guidance for
each `--scope` path. `.gator/rules.json` adds bounded path rules without
granting executable capability. A developer may select a named declarative
profile from `.gator/agents.json` with `--profile`. The same file can define
capability-bounded roles for delegated scouts and writers; inspect them with
`gator agent list` and see [project agent roles](docs/AGENTS.md). Project hooks, LSP, and MCP configuration are
different: `gator hook trust` pins the hash of `.gator/hooks.json` plus its
declared executables; `gator lsp trust` pins `.gator/lsp.json` plus local LSP
executables; and `gator mcp trust` pins `.gator/mcp.json` plus local stdio
executables. A changed bundle is disabled until re-trusted. Trusted hooks run
at tool, compaction, verification, and session boundaries in the strict
sandbox. Trusted LSP bundles expose bounded pull diagnostics, read-only
navigation, informational completion, formatting, rename, and workspace-confined
code-action edit suggestions, and request approval
before each lookup; see
[Trusted local LSP](docs/LSP.md).
Trusted MCP bundles can expose repository-relative stdio servers or Streamable
HTTP servers; every MCP tool invocation still requests approval. Remote HTTP
servers can use explicit, standards-based OAuth login with private,
resource-bound tokens; see [Trusted MCP servers](docs/MCP.md).

Use `--scout` up to four times for parallel read-only exploration in separate
worktrees. Reports reach the primary writer as untrusted evidence. New runs
support `--base REF`, resolved to an immutable commit before creation. Opt in
to copying ignored setup files with `--copy-ignored`; the exact files must be
listed in tracked `.gator/worktreeinclude` and remain ignored. Use
`gator worktree list`, `gator worktree prune`, and the explicit destructive
`gator worktree remove RUN_ID --yes` to manage retained checkouts.

For dependencies or generated local prerequisites, `gator run` also accepts a
repeatable developer-supplied `--setup 'argv ...'`. Each value is parsed as a
whitespace-separated argv and runs once after the isolated worktree (and any
opted-in copied files) exists, before the model or scouts start. Setup follows
the selected strict sandbox and network policy, is never read from project
configuration, is unavailable to Plan mode and ACP/RPC clients, and leaves the
worktree plus an error record for review if it fails. If bootstrap needs the
network, grant it explicitly. For example:

```sh
gator run --network allow --copy-ignored --setup 'npm ci' --verify 'npm test' 'fix the parser'
```

During either native Plan or Execute mode, the model can also call
`delegate_readonly` for one to four focused inspections of the active isolated
worktree. Those fresh-context scouts run concurrently, can only read/list/search
or inspect Git state, and see the primary agent's uncommitted changes. They
cannot edit, run a command, access extensions, LSP, MCP, or recursively
delegate. Each report is bounded to 8 KiB and explicitly framed as untrusted
evidence; Gator permits at most eight dynamic scouts per primary run. A project
may add a named `readonly` role to focus one scout's fresh-context prompt, but
it cannot expand the scout tool surface.

Execute mode exposes `delegate_writer` for one independent implementation and
`delegate_writers` for exactly two parallel implementations (at most two writer
children total per primary run). Every writer starts from the same snapshot of
the isolated parent worktree in its own retained child worktree, with a clean
internal baseline. A parallel call must declare non-overlapping
repository-relative paths; Gator rejects conflicting declarations and reports
the actual changed paths and any post-run overlap or scope violation. It also
creates ref-free comparison commits and runs Git's three-way `merge-tree`
analysis without changing a ref, index, or worktree. A clean textual comparison
is still not a semantic merge decision: the primary agent stays paused,
receives only summaries and, when no larger than 512 KiB, reviewable patches,
and must use a separate `apply_patch` call for each compatible delta. Gator
never auto-merges writer output. Writer children inherit the developer-selected
profile,
verifier, sandbox, approval policy, and trusted integrations, but cannot
recursively create writers. Their lifecycle, effective narrowed policy,
owner/deadline/heartbeat, baseline, verifier and patch digests, comparison
commit, worktree, and child run record are saved privately under the parent run
record. A batch also persists its two-child schedule, changed-path evidence,
and three-way comparison result; inspect children
with `gator child list RUN_RECORD_PATH` and `gator child show RUN_RECORD_PATH
CHILD_RUN_ID`, or batches with `gator child batches RUN_RECORD_PATH` and `gator
child batch RUN_RECORD_PATH BATCH_ID`. A named project `writer` role can focus
prompt instructions and further narrow sandbox, network, step, or tool policy;
the role meet cannot restore authority removed by the parent profile. See
[writer orchestration](docs/WRITERS.md).

Execute mode also gives the native model a persistent terminal-task surface:
`terminal_start`, `terminal_read`, `terminal_write`, `terminal_list`, and
`terminal_stop`. In the native TUI and an authenticated `gator serve` process,
`terminal_detach` can keep one running task after a separate developer approval.
A started task gets a pseudo-terminal
in the same strict
sandbox, worktree, network policy, output bounds, and lifetime limit as the
run. Starting a task needs the normal command approval; each distinct input
needs its own approval and is represented to the approval UI by a digest rather
than the input bytes. Ordinary tasks stop when the agent run ends. A detached
task keeps its existing policy, is visible through `Ctrl+T` after the run in
the native TUI or controllable through the app-server's authenticated terminal
methods, expires within two hours while running, and retains at most eight
restartable histories per Gator session.
Gator's normal shutdown path stops it; it cannot be created from direct
commands, plain JSONL RPC, ACP, or a child writer.
This is an agent-mediated task manager, not an arbitrary host shell. `Ctrl+T`
opens an attachment to existing model-started tasks: developers can view
bounded output, switch tasks, send a line or interrupt, stop a task, and use
`Ctrl+R` to restart an exited task with its exact previous argv and sandbox.
`Ctrl+O` enters opt-in raw-keyboard mode for arrows, completion, control keys,
and common function keys; `Ctrl+]` returns to Gator controls. The attachment
resizes the underlying PTY to its viewport and uses a bounded VT500/xterm text
emulator: cursor and margin operations, primary and alternate screens,
scrollback, Unicode cell widths, ANSI/256/RGB color, and text attributes render
as a terminal application expects. `PgUp`, `PgDn`, `Home`, and `End` scroll the
emulator's primary-buffer history rather than a flattened transcript. Standard
device/status replies are capped and returned only to the existing task; they
are neither developer input nor model context. Direct input stays inside the
task's existing sandbox and is journaled only as byte count plus SHA-256 digest
while its owning run is active; background input is not appended to a completed
run record. The authenticated app-server controller can directly write only to
an already detached task; that input never becomes model context or a completed
run record. Terminal graphics, clipboard/window control, and mouse forwarding
remain intentionally unsupported, as do a restart-surviving terminal daemon and
ACP/client terminal multiplexer.

When the developer explicitly grants `--network allow` (or `gator config set
network allow`), Execute mode additionally exposes `http_fetch` for bounded
web research. Every exact HTTPS URL needs approval unless it was remembered for
the current run; the tool accepts only port 443, resolves and pins public DNS
addresses, rejects local/private/reserved targets, does not follow redirects,
and returns at most 256 KiB of textual content.

For JavaScript applications and visual verification, Gator also has explicit
local Chromium sessions. Install the pinned runtime with `gator browser
install`, start or attach a local session, select the exact tab(s) an agent may
see, approve origins, then pass it to an Execute-mode run with `--network
allow --browser-session ID`. Every browser mutation needs a fresh approval;
logins are user-only; screenshots require a separate session grant; uploads
must be pre-registered; and browser artifacts remain private local files. The
TUI provides the same lifecycle through `/browser`. This is local browser
control, not hosted execution or unrestricted computer use. See [local
controlled browser sessions](docs/BROWSER.md).

Set `BRAVE_SEARCH_API_KEY` to also expose `web_search` through Brave's
documented Web Search API. Each exact query needs approval unless remembered
for the current run; the fixed HTTPS endpoint uses the same proxy-free,
public-DNS-pinned transport, shares the eight-request web-research budget with
`http_fetch`, and returns at most ten title/URL/snippet records. The key stays
in the process environment and outgoing request header only: it is not written
to Gator configuration, events, or tool results. Search results and fetched
pages remain untrusted data. [Brave Web Search API](https://api-dashboard.search.brave.com/api-reference/web/search/get)

See the source-backed [terminal-harness capability audit](docs/COMPETITIVE_AUDIT.md)
for the current comparison with Codex CLI, Claude Code, Cursor CLI, Pi, and
OpenCode.

## Design principles

- Gator-owned agent loop, rather than a wrapper around another coding agent;
- isolated worktree by default; direct edits require an explicit later mode;
- a complete event trace, patch, and verifier result for every completed run;
- deterministic tests and replayable model transcripts around provider
  behavior;
- no claim of model or benchmark competitiveness without published evidence.

## Local run data

Runs leave code changes in a sibling `*-gator-runs/` worktree, never in the
active checkout. The printed run-record path defaults to
`$XDG_STATE_HOME/gator/` (or `~/.local/state/gator/`) and contains a
metadata-only event journal, final result, a portable patch snapshot, and a
private `0600` session file for `gator resume`. `gator resume --last` selects the newest retained thread
for the current repository; `--all` expands selection to the local state root.
The event log intentionally omits prompts, source text,
tool arguments, tool output, and raw attachment bytes; the worktree is the
reviewable source of truth. The TUI also keeps one private `0600` unfinished draft per repository
and lists resumable conversation threads from the same state root. Threads
retain one worktree across turns and record whether the last turn was Plan or
Execute. Each retained turn points to its immutable parent record and stores a
portable patch snapshot. `gator fork` restores that snapshot into a new
worktree at the saved base commit, so the source thread stays unchanged while
`/tree` reconstructs local lineage and visible forks without replaying the
agent.
Drafts contain only the current message, verifier text, provider, and
model, and are removed after Gator finishes a new thread. The active-turn queue
is intentionally absent from drafts and sessions, so a restart never performs
queued work automatically. Set `GATOR_STATE_DIR`
to use another local state root.

See [the architecture](docs/ARCHITECTURE.md) for the intended runtime and
acceptance criteria.
