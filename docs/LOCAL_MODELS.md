# Local models in the Work TUI

Start `gator`, enter `/model`, then press Tab for **Local**. The same screen is
available from the command palette and first-run model selection. All local model
management happens here; the former `gator local` CLI commands are retired and
return directions to this screen. Automation and headless Work can still use the
persisted model selection.

Gator uses the existing Ollama runtime and its loopback Chat Completions API.
Its Work service, tools, specialist boundaries, evidence, and approvals still
own execution. The current catalog is text-only; unsupported image/PDF inputs
are reported explicitly. No model-quality claim follows from being in the catalog.

## Download, select, and delete

1. Choose a model with ↑/↓. The screen shows installed state, source, approximate
   size, and host eligibility.
2. Press `p` to review its download source and size, then Enter or `y` to confirm.
   Progress appears while the UI remains responsive. Esc or Ctrl+C requests
   cancellation; a later pull uses Ollama's resumable download behavior.
3. Press `u` or Enter to select an installed model. The selection is saved for
   the next Work run and later sessions. Press `e` to edit its display name.
4. Press `x` to review deletion, then Enter or `y` to confirm, or `n`/Esc to
   decline. Deletion updates the selected provider; deleting its last configured
   model clears that selection. Model files belong to the selected Ollama runtime,
   so deletion also affects other clients using the same runtime.
5. Press Esc to return to the same conversation or draft. First-run setup keeps
   the pending prompt in the composer; press Enter to start it after selection.

`r` refreshes inventory, `i` opens prerequisite help, and F1 shows shortcuts.
Model management waits until active Work has finished or been cancelled, so a
model being used by that run cannot be deleted from this screen mid-run.
Download and deletion always require their own TUI confirmation.

## Runtime setup

If Ollama is installed but stopped, the Local screen offers **Start with Gator**.
Enter or `y` starts `ollama serve` as a child of the Work TUI. It remains running
when the model screen closes and stops when Gator exits. Gator does not adopt or
stop a runtime that was already running. Press `s` to revisit runtime startup.
Closing the TUI cancels a pending model operation or sign-in.

If the Ollama executable is missing, the screen offers installation help with the
[official download source](https://ollama.com/download) and platform guidance.
Installing the operating-system runtime remains a prerequisite: Gator does not
run privileged package installers or remote installation scripts. Once installed,
refresh from the Local screen; model downloads, selection, deletion and runtime
startup require no separate Gator CLI commands.

## Cloud and custom models

Tab switches back to Cloud. Press `c` to configure a provider/model with masked
credential entry and provider-specific endpoint fields, `l` for an available
native OAuth sign-in, and `u`/Enter to select a model. The selection is persisted
for Work. The screen reports credential/configuration readiness, not a live
account entitlement check. Sign-ins requiring an unconfigured OAuth client are
reported as unavailable.

Press `n` to add a custom Chat Completions endpoint, `g` to preview model discovery,
`e` to rename an entry, `d` to remove a stored Gator credential, and `x` to remove a
custom provider. The reserved `gator-local` entry is managed from Local. This
screen reuses the retained model-management component; it does not reopen the
historical Code composer or read/write its draft state.

## Host eligibility guardrail

The Local model screen detects the operating system, architecture, total RAM, currently
available RAM where the platform reports it, and free space in the Ollama model
filesystem. It reports each reviewed model as enabled or disabled and explains
the exact blocking condition. Gator supports Linux and macOS on `amd64` and
`arm64`; an unrecognized OS or architecture is disabled rather than assumed
compatible.

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

Gator blocks TUI download/selection and every
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
chooses its runtime backend; the model-management screen does not currently report CPU/GPU placement. Larger context and parallel requests can
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

The manager uses a loopback Ollama runtime. Selecting a model persists the
`gator-local` provider and its `/v1/chat/completions` endpoint. Existing configured
loopback endpoints remain readable; remote addresses are rejected. No API key is
stored or sent to the managed local runtime.

This restriction is deliberate. A remote or shared endpoint belongs to the
Cloud section’s custom-provider form (`n`), where its authentication and data-handling
contract can be declared explicitly. Gator never enables Ollama's
insecure pull option and never uploads models.

Ollama documents Chat Completions streaming and tool support, which is why the
catalog can retain Gator's tool loop rather than delegating to another harness.
The selected coding models are text models, so do not use `--image` with them;
text attachments retain the normal OpenAI-compatible provider behavior while
PDF inputs remain unavailable on that generic protocol.

## Verification of the Work TUI integration

September 10, 2026: `GOTOOLCHAIN=go1.25.13 make check` passed all 52 packages,
formatting, vet and build. Race checks passed for `internal/localmodel`,
`internal/tui`, `internal/worktui`, and `cmd/gator`. Darwin arm64 cross-compilation
passed; macOS runtime behavior was not exercised locally. Browser code did not
change, so the Chromium integration was not repeated for this change.

The assembled Bubble Tea test uses local HTTP fixtures to exercise download
confirmation, progress, cancellation, a completed download, persistent selection,
an actual Work-service artifact run, declined deletion, confirmed deletion, and
clearing the last selected model. Other regressions cover short-terminal
confirmations, preserving conversations and historical Code drafts, cancelled
refreshes, stale panel responses, active-run exclusion and runtime shutdown.
An initial race run exposed test synchronization against streamed text rather
than terminal completion; the corrected test passed three race-enabled repeats
and the final full race group. The initial UI text assertion was also updated to
the actual confirmation label before passing verification.

A separate real-terminal smoke check started the installed Ollama executable
from Work, listed installed models, selected `qwen2.5-coder:0.5b`, and cancelled a
live Work task. Its retained manifest reports four requests, 8,861 input tokens,
130 output tokens, and no completed artifact before cancellation on turn four.
The Ollama server and runner were absent after Gator exited. This verifies the
runtime lifecycle and cancellation, not successful live task quality. No existing
model weights were downloaded or deleted in that smoke check; those operations
were verified with local HTTP fixtures.
