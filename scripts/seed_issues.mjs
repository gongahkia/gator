import { spawn } from "node:child_process";

const repo = process.env.GATOR_REPO || "gongahkia/gator";
const milestones = {
  "M0 Foundation": "Runnable plugin, state model, UI shell, and adapter contracts.",
  "M1 Agent and context": "Core-six providers, sessions, context, and local indexing.",
  "M2 Safe execution": "Worktrees, review, policy, and merge evidence.",
  "M3 Collaboration and SDK": "GitHub bridge, shared artifacts, and public extensions.",
  "M4 Hardening": "Telemetry, performance, platform validation, and future-adapter research.",
};
const colors = {
  foundation: "1D76DB", core: "1D76DB", ui: "5319E7", adapters: "0E8A16", context: "FBCA04",
  indexer: "FBCA04", workspace: "D93F0B", review: "D93F0B", policy: "B60205", github: "24292F",
  extensions: "5319E7", telemetry: "006B75", performance: "006B75", future: "C2E0C6",
};
const milestoneFor = {
  foundation: "M0 Foundation", core: "M0 Foundation", ui: "M0 Foundation", adapters: "M0 Foundation",
  context: "M1 Agent and context", indexer: "M1 Agent and context",
  workspace: "M2 Safe execution", review: "M2 Safe execution", policy: "M2 Safe execution",
  github: "M3 Collaboration and SDK", extensions: "M3 Collaboration and SDK",
  telemetry: "M4 Hardening", performance: "M4 Hardening", future: "M4 Hardening",
};

function run(args, input) {
  return new Promise((resolve, reject) => {
    const child = spawn("gh", args, { stdio: ["pipe", "pipe", "pipe"] });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (data) => { stdout += data; });
    child.stderr.on("data", (data) => { stderr += data; });
    child.on("error", reject);
    child.on("close", (code) => code === 0 ? resolve(stdout) : reject(new Error(`${args.join(" ")}\n${stderr}`)));
    child.stdin.end(input);
  });
}

async function api(method, path, payload) {
  const args = ["api", "--method", method, path];
  if (payload !== undefined) args.push("--input", "-");
  const output = await run(args, payload === undefined ? undefined : JSON.stringify(payload));
  return output.trim() ? JSON.parse(output) : undefined;
}

const tasks = [];
function add(area, title, objective, depends = "foundation") {
  tasks.push({ area, title, objective, depends });
}

// Foundation: 10
add("foundation", "Bootstrap the Gator Lua module boundaries", "Create load-safe public, core, UI, adapter, context, workspace, review, and policy module roots.");
add("foundation", "Add package-manager-agnostic plugin loading", "Support native package loading and lazy loading without requiring a specific plugin manager.");
add("foundation", "Build a deterministic Neovim test harness", "Run isolated headless tests with controlled runtimepath, temp state, and fixture helpers.");
add("foundation", "Build recorded agent-stream fixture helpers", "Provide reusable JSON-RPC, JSONL, terminal, and process fixture runners for adapters.");
add("foundation", "Add CI for Lua, Rust, formatting, and fixture tests", "Validate the plugin and sidecar on every push without authenticated provider access.");
add("foundation", "Add protected live-agent E2E workflow", "Create a manually dispatched protected workflow that verifies authenticated providers without exposing credentials.");
add("foundation", "Implement developer command targets", "Provide repeatable test, format, lint, sidecar, fixture, and issue-seeding targets.");
add("foundation", "Implement structured Gator error primitives", "Define typed errors and user-facing notifications with actionable recovery details.");
add("foundation", "Implement a modular checkhealth framework", "Expose dependency and configuration health checks that adapters and providers can extend.");
add("foundation", "Add version and compatibility gates", "Fail fast on unsupported Neovim versions and expose capability degradation clearly.");

// Core: 10
add("core", "Define the persistent task entity", "Model task identity, objective, lifecycle, workspace, linked sessions, and evidence references.");
add("core", "Implement task lifecycle transitions", "Enforce valid draft, planned, running, awaiting-review, merged, discarded, and failed transitions.");
add("core", "Define linked provider-native sessions", "Link provider-native session identifiers to a Gator task without rewriting their history.");
add("core", "Persist provider-native session metadata", "Store resumable session metadata locally with provider version and capability provenance.");
add("core", "Implement Gator-owned cross-provider threads", "Store optional normalized task conversations alongside, not instead of, native sessions.");
add("core", "Define the context-pack entity", "Model ordered context entries, provenance, trust, token estimate, and transfer eligibility.");
add("core", "Define scoped policy entities", "Model global, project, repository, file, and run policy overlays with explicit provenance.");
add("core", "Define run and event entities", "Persist process, provider, workspace, state, timing, usage, and streamed event records.");
add("core", "Implement SQLite schema migrations", "Version and migrate local state safely without losing active sessions or task evidence.");
add("core", "Implement local retention and cleanup", "Prune configurable stale transcripts, indices, worktree records, and telemetry safely.");

// UI: 12
add("ui", "Implement adaptive workspace layout control", "Open, resize, focus, and restore Gator panels without corrupting user window layouts.", "core");
add("ui", "Implement the persistent conversation sidebar", "Render linked task sessions with provider identity, streaming state, and input routing.", "core");
add("ui", "Implement the task dashboard", "Browse, filter, and open tasks by lifecycle, provider, workspace, and review state.", "core");
add("ui", "Implement the context-pack inspector", "Show ordered context, provenance, trust, token estimates, and editable inclusion before send.", "core");
add("ui", "Implement buffer selection context actions", "Capture visual selections into a task or session through discoverable native commands.", "core");
add("ui", "Implement a streaming markdown renderer", "Render agent text incrementally with stable cursor behavior and syntax-aware code blocks.", "adapters");
add("ui", "Implement an agent tool-call timeline", "Present provider tool calls, arguments, approvals, outputs, and failures with collapse controls.", "adapters");
add("ui", "Implement the native diff review panel", "Open changed files and real diff splits from an agent run with task context.", "core");
add("ui", "Implement the workspace and worktree dashboard", "Inspect workspace state, linked tasks, dirty files, and agent activity.", "workspace");
add("ui", "Implement the Gator command palette", "Expose actions, adapters, task commands, and provider commands through native completion.", "adapters");
add("ui", "Implement native picker fallbacks", "Provide complete picker workflows without optional UI dependencies.", "ui");
add("ui", "Implement keymap and accessibility configuration", "Support user-owned keymaps, keyboard-only workflows, and screen-reader-friendly text buffers.", "ui");

// Adapter foundation: 8
add("adapters", "Define the public adapter capability contract", "Specify transport, auth, session, permission, model, command, tool, context, and usage capabilities.", "core");
add("adapters", "Implement adapter process lifecycle management", "Launch, monitor, cancel, restart, and clean up agent subprocesses safely.", "foundation");
add("adapters", "Implement stdio JSON-RPC transport", "Provide framed request, response, notification, cancellation, and timeout handling for ACP-like transports.", "foundation");
add("adapters", "Implement structured JSONL stream parsing", "Normalize provider JSONL events, malformed records, partial lines, and stream completion.", "foundation");
add("adapters", "Implement attachable terminal fallback", "Support visible terminal sessions with explicit degraded capability reporting.", "ui");
add("adapters", "Implement CLI-owned authentication discovery", "Detect logged-in agent CLIs without reading, writing, or displaying credentials.", "foundation");
add("adapters", "Implement session resume capability fallback", "Resume native sessions where supported and preserve useful orphan metadata elsewhere.", "core");
add("adapters", "Implement a unified model and usage facade", "Expose available model and usage data only when an adapter can prove it is current.", "core");

function addProvider(name, slug, transport) {
  add("adapters", `Implement ${name} executable and version probe`, `Detect the ${name} CLI, supported version range, and ${transport} capability profile.`, "adapters");
  add("adapters", `Implement ${name} launch and authentication adapter`, `Launch ${name} using existing CLI login and map current working directory safely.`, "adapters");
  add("adapters", `Implement ${name} session and resume adapter`, `Create, list, resume, and close ${name} sessions without fabricating unsupported history.`, "adapters");
  add("adapters", `Implement ${name} permission and mode mapping`, `Map Gator policy to ${name} native modes while refusing unsafe or unavailable translations.`, "policy");
  add("adapters", `Implement ${name} context and command integration`, `Send files, selections, diagnostics, diffs, and advertised ${name} commands with provenance.`, "context");
  add("adapters", `Add ${name} fixtures and authenticated E2E verification`, `Record representative streams and require protected live verification for ${name}.`, "adapters");
}
addProvider("Codex", "codex", "structured CLI/RPC");
addProvider("Claude Code", "claude-code", "structured CLI");
addProvider("Gemini CLI", "gemini", "ACP");
addProvider("Copilot CLI", "copilot", "ACP");
addProvider("OpenCode", "opencode", "ACP");
addProvider("Pi", "pi", "RPC");

// Context: 14
add("context", "Capture current-file context entries", "Add a stable, content-addressed file reference to a context pack.", "core");
add("context", "Capture visual-selection context entries", "Capture ranges with buffer revision, language, and surrounding context metadata.", "core");
add("context", "Capture diagnostic context entries", "Attach current diagnostics with source, severity, location, and staleness checks.", "core");
add("context", "Capture Git diff context entries", "Attach staged, unstaged, or task-worktree diff context with base commit provenance.", "core");
add("context", "Implement editable context-pack ordering", "Add, remove, reorder, annotate, and pin context entries before agent submission.", "core");
add("context", "Implement provider-aware token estimation", "Estimate context cost conservatively and mark estimates as unavailable when unsupported.", "adapters");
add("context", "Render context provenance and trust", "Show source path, revision, retrieval source, trust state, and policy decision for every entry.", "policy");
add("context", "Implement manual context mode", "Require explicit user selection and preserve exact submission order by default.", "core");
add("context", "Implement inspect-before-send context mode", "Suggest retrieved context in an editable pack that requires confirmation before send.", "context");
add("context", "Implement automatic retrieval context mode", "Attach policy-approved retrieval results automatically and record a complete audit trail.", "context");
add("context", "Implement lexical repository retrieval", "Use Git-aware ripgrep and Tree-sitter signals for fast local context search.", "foundation");
add("context", "Implement agent-managed retrieval handoff", "Expose Gator-provided context tools to agents that support native retrieval or MCP.", "adapters");
add("context", "Implement project instruction discovery", "Discover AGENTS.md, CLAUDE.md, provider rules, and project policy without silently elevating trust.", "policy");
add("context", "Implement configurable context handoff modes", "Support manual transfer, editable explicit summaries, and opt-in automatic summaries between providers.", "core");

// Indexer: 8
add("indexer", "Define the Lua-to-indexer protocol", "Specify versioned local request, response, cancellation, and health messages for gator-index.", "adapters");
add("indexer", "Implement indexer process lifecycle", "Start, stop, restart, and recover the optional Rust sidecar without blocking Neovim.", "foundation");
add("indexer", "Implement Git-aware repository scanning", "Index only policy-approved tracked and selected untracked files while honoring ignore rules.", "context");
add("indexer", "Implement language-aware document chunking", "Create stable chunks using Tree-sitter when available with textual fallback.", "context");
add("indexer", "Implement local embedding-provider integration", "Use a configurable local embedding command/provider without transmitting repository data.", "indexer");
add("indexer", "Implement cloud embedding-provider integration", "Use external credential sources and explicit privacy consent for remote embedding requests.", "policy");
add("indexer", "Implement hybrid lexical and vector ranking", "Rank retrieval results deterministically with explainable lexical, semantic, and recency signals.", "indexer");
add("indexer", "Implement indexer health and fallback behavior", "Fall back to lexical/manual context on sidecar failure and expose repair actions.", "indexer");

// Workspace: 12
add("workspace", "Implement Git repository and root detection", "Resolve repository, worktree, branch, and base revision deterministically.", "foundation");
add("workspace", "Implement project workspace policy resolution", "Select current-checkout or worktree behavior from scoped Gator policy.", "policy");
add("workspace", "Implement isolated Git worktree creation", "Create named worktrees and branches with collision-safe paths and rollback on launch failure.", "core");
add("workspace", "Implement workspace branch naming policy", "Generate configurable, traceable branch names from Gator task identity.", "workspace");
add("workspace", "Implement write-run scheduler", "Queue write-capable runs with a default concurrency cap of one.", "core");
add("workspace", "Implement live workspace process monitoring", "Track agent liveness, workspace dirtiness, changed paths, and detached processes.", "adapters");
add("workspace", "Link tasks, runs, and worktrees", "Maintain bidirectional task/run/worktree references through restarts.", "core");
add("workspace", "Implement worktree switching and buffer rebinding", "Open a workspace without losing user buffers or confusing path-local state.", "ui");
add("workspace", "Implement stale workspace cleanup", "Identify recoverable stale worktrees and require confirmation before destructive cleanup.", "workspace");
add("workspace", "Implement changed-path collision detection", "Warn when active worktrees modify overlapping paths or generated artifacts.", "workspace");
add("workspace", "Implement pre-run Git snapshot evidence", "Record base revision and working-tree state before every write-capable run.", "core");
add("workspace", "Implement interrupted-run recovery", "Reconnect or safely classify agent runs after Neovim or sidecar restart.", "adapters");

// Review: 8
add("review", "Implement changed-file inventory", "Build a task-aware changed-file list with path status and workspace provenance.", "workspace");
add("review", "Implement hunk-level review navigation", "Navigate, stage decisions, and annotate agent-generated hunks inside native diffs.", "ui");
add("review", "Implement structured feedback routing", "Send selected diff feedback and context back to a linked native agent session.", "adapters");
add("review", "Implement configurable test-command execution", "Run policy-approved validation commands in the relevant workspace and stream evidence.", "policy");
add("review", "Persist review and test evidence", "Store reviewers, test output references, approvals, and revisions with a task.", "core");
add("review", "Implement accept and reject run actions", "Accept a task for merge or discard it with reversible local evidence handling.", "core");
add("review", "Implement guarded merge and rebase strategies", "Support configured merge modes, rebase retry, conflict reporting, and no silent merge.", "workspace");
add("review", "Implement read-only agent review runs", "Launch a policy-constrained reviewer against a task diff without granting write access.", "adapters");

// Policy: 8
add("policy", "Implement global Gator settings", "Load and validate user defaults without storing provider credentials.", "core");
add("policy", "Implement project policy configuration", "Load project-local shared policy from opt-in Git-tracked artifacts.", "core");
add("policy", "Implement repository policy overlays", "Resolve repository-specific rules separately from broad project defaults.", "policy");
add("policy", "Implement file-rule matching", "Apply ordered file/path rules with explainable outcomes and no glob ambiguity.", "policy");
add("policy", "Implement run-level policy overrides", "Allow explicit per-run narrowing or mode selection with persistent audit provenance.", "policy");
add("policy", "Implement permission escalation UI", "Require visible acknowledgement when Gator requests a provider mode broader than baseline policy.", "ui");
add("policy", "Implement secret redaction and safe logging", "Redact known credentials and user-configured patterns from prompts, logs, diagnostics, and telemetry.", "foundation");
add("policy", "Implement context trust modes", "Support provenance default, repository trust, and strict manual trust with visible policy decisions.", "context");

// GitHub and team: 6
add("github", "Implement gh CLI detection and capability checks", "Detect authenticated GitHub CLI support without adding a Gator account system.", "foundation");
add("github", "Implement GitHub issue-to-task import", "Import issue body, labels, references, and selected comments into a local task context pack.", "context");
add("github", "Implement pull-request context import", "Attach PR diff, review state, checks, and selected discussion context to a task.", "context");
add("github", "Implement opt-in shared task artifacts", "Serialize task plans, handoffs, and review evidence into Git-trackable .gator artifacts.", "core");
add("github", "Implement shared artifact synchronization", "Detect Git changes, merge conflicts, and stale local copies of opt-in Gator artifacts.", "workspace");
add("github", "Implement optional PR review evidence publishing", "Publish selected Gator review/test evidence through gh with user confirmation.", "review");

// Extensions: 8
add("extensions", "Implement extension discovery and lifecycle", "Discover, validate, load, unload, and isolate public Gator extensions.", "foundation");
add("extensions", "Publish the adapter SDK", "Expose stable adapter types, capabilities, event contracts, and fixture requirements.", "adapters");
add("extensions", "Publish the retrieval-provider SDK", "Expose local, cloud, lexical, and agent-managed retrieval provider contracts.", "context");
add("extensions", "Publish the policy-check SDK", "Expose policy evaluators that may narrow actions and explain their decision.", "policy");
add("extensions", "Publish the UI-panel SDK", "Allow extensions to register panels and actions without coupling to private layout internals.", "ui");
add("extensions", "Publish the workflow SDK", "Allow extensions to add task templates, workflows, and review actions with capability declarations.", "core");
add("extensions", "Implement the public event API", "Emit versioned task, session, context, run, workspace, review, and policy events.", "core");
add("extensions", "Build extension contract tests", "Provide a reusable conformance suite for public extension APIs.", "extensions");

// Telemetry: 6
add("telemetry", "Implement structured local diagnostic logs", "Write rotated local logs with task-safe identifiers, levels, and redaction.", "foundation");
add("telemetry", "Implement detailed health reporting", "Report adapter, indexer, Git, workspace, policy, and telemetry readiness.", "foundation");
add("telemetry", "Implement explicit telemetry consent", "Require explicit enablement before any anonymous telemetry collection or transmission.", "policy");
add("telemetry", "Implement telemetry event redaction", "Apply strict schema allowlists and secret/path redaction before telemetry leaves the machine.", "policy");
add("telemetry", "Implement anonymous telemetry aggregation", "Aggregate only consented, non-identifying product events without transcript or code content.", "telemetry");
add("telemetry", "Implement telemetry export transport", "Send opt-in telemetry with retry, disable, and inspect controls.", "telemetry");

// Performance and compatibility: 6
add("performance", "Enforce lazy startup and zero eager agent launches", "Measure startup impact and defer all provider, indexer, and GitHub work until invoked.", "foundation");
add("performance", "Keep streaming and indexing off the Neovim UI loop", "Profile and prevent blocking UI operations during large streams and repository scans.", "indexer");
add("performance", "Benchmark large-repository context retrieval", "Measure lexical and vector retrieval latency, memory, and cancellation on representative large repositories.", "context");
add("performance", "Test indexer crash and restart resilience", "Verify no data loss or UI hang when the optional sidecar exits unexpectedly.", "indexer");
add("performance", "Implement macOS Linux and WSL preflight checks", "Detect platform-specific executable, filesystem, worktree, and sidecar requirements.", "foundation");
add("performance", "Build a repeatable performance benchmark suite", "Track startup, context, stream, worktree, diff, and indexer regressions in CI artifacts.", "foundation");

// Future agents researched before implementation: 8
for (const agent of ["Cursor", "Cline", "Kimi CLI", "Mistral Vibe", "Goose", "Aider", "Amp", "Droid"]) {
  add("future", `Implement ${agent} capability probe and recorded transport fixture`, `Research current ${agent} CLI behavior, implement a version/capability probe, and record a conformance fixture for a future first-class adapter.`, "adapters");
}

if (tasks.length !== 160) throw new Error(`Expected 160 tasks, got ${tasks.length}`);

function body(task) {
  return [
    "## Objective", task.objective,
    "", "## Scope", `Implement this single ${task.area} unit without unrelated refactors.`,
    "", "## Acceptance criteria",
    "- Expose the behavior through the relevant Gator module, UI, or extension contract.",
    "- Handle capability absence and failures explicitly; do not silently downgrade behavior.",
    "- Preserve provider-native credentials and session ownership.",
    "", "## Tests",
    "- Add focused deterministic coverage using fixtures where applicable.",
    "- Add or update protected live E2E coverage when this affects a core provider.",
    "", "## Dependency", `Depends on: ${task.depends}.`,
  ].join("\n");
}

async function ensureLabels() {
  const existing = await api("GET", `repos/${repo}/labels?per_page=100`);
  const names = new Set(existing.map((label) => label.name));
  const labels = [
    ["status:backlog", "EDEDED", "planned engineering work"],
    ["type:feature", "0E8A16", "implementation feature"],
    ["type:infrastructure", "1D76DB", "tooling or platform work"],
    ["priority:high", "B60205", "required for v1"],
    ...Object.entries(colors).map(([area, color]) => [`area:${area}`, color, `${area} subsystem`]),
  ];
  for (const [name, color, description] of labels) {
    if (!names.has(name)) await api("POST", `repos/${repo}/labels`, { name, color, description });
  }
}

async function ensureMilestones() {
  const existing = await api("GET", `repos/${repo}/milestones?state=all&per_page=100`);
  const result = new Map(existing.map((milestone) => [milestone.title, milestone.number]));
  for (const [title, description] of Object.entries(milestones)) {
    if (!result.has(title)) {
      const milestone = await api("POST", `repos/${repo}/milestones`, { title, description });
      result.set(title, milestone.number);
    }
  }
  return result;
}

async function parallel(items, limit, action) {
  let cursor = 0;
  await Promise.all(Array.from({ length: limit }, async () => {
    while (cursor < items.length) {
      const item = items[cursor++];
      await action(item);
    }
  }));
}

async function main() {
  const existing = await run(["issue", "list", "--repo", repo, "--state", "all", "--limit", "1000", "--json", "title"]);
  const titles = new Set(JSON.parse(existing).map((issue) => issue.title));
  await ensureLabels();
  const milestoneNumbers = await ensureMilestones();
  const missing = tasks.filter((task) => !titles.has(task.title));
  const limit = Number(process.env.GATOR_ISSUE_LIMIT || missing.length);
  const batch = missing.slice(0, limit);
  await parallel(batch, 1, async (task) => {
    const infrastructure = ["foundation", "indexer", "extensions", "telemetry", "performance"].includes(task.area);
    await api("POST", `repos/${repo}/issues`, {
      title: task.title,
      body: body(task),
      labels: ["status:backlog", infrastructure ? "type:infrastructure" : "type:feature", "priority:high", `area:${task.area}`],
      milestone: milestoneNumbers.get(milestoneFor[task.area]),
    });
  });
  console.log(`Gator backlog: ${tasks.length - missing.length} existing, ${batch.length} created, ${tasks.length} total.`);
}

main().catch((error) => { console.error(error.stack || error); process.exit(1); });
