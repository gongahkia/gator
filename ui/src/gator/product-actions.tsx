import { randomUUID } from "node:crypto"
import { useClient } from "../context/client"
import { useData } from "../context/data"
import type { useActions } from "./actions"
import type { CheckAction, FollowingCheck, PlanContent, TaskPlanReview, TaskPlanSummary } from "@gator/application-host/contracts/protocol"

// Optional management stays behind commands. All mutations use the current
// Go protocol, including its live tokens, revisions and acknowledgements.
export function useProductActions(controls: ReturnType<typeof useActions>) {
  const client = useClient(), data = useData()
  const { menu, prompt, details, confirm, actions } = controls
  const planEdits = new Map<string, PlanContent>()
  const owner = () => {
    const id = client.state?.conversation_id
    if (!id) throw new Error("Wait for this conversation to open.")
    return id
  }
  actions.appearance = () => menu("Appearance", [
    ...([ ["Follow terminal colors", "terminal"], ["Dark", "gator"], ["Light", "light"], ["High contrast", "contrast"], ["Monochrome", "mono"] ] as const).map(([title, theme]) => ({ title, description: client.preferences.theme === theme ? "Current appearance" : undefined, run: async () => {
      if (theme !== "terminal") await client.send({ type: "submit", text: `/theme ${theme}` })
      await client.configurePreferences({ theme })
    } })),
    { title: "Keyboard shortcuts", run: () => client.send({ type: "navigate", text: "/appearance" }) },
  ])
  actions.sources = async () => {
    let sources = client.state?.sources
    if (!sources) throw new Error("Sources are unavailable. Open /connections to configure an account.")
    const request_id = randomUUID()
    await client.result({ type: "sources", sources: { action: "refresh", id: sources.id, request_id } }, request_id, s => s.sources_result)
    const show = () => {
      sources = client.state?.sources
      if (!sources) return
      const current = sources
      menu("Sources for new messages", [
        ...current.choices.map(choice => ({ title: `${current.selected.includes(choice.id) ? "✓ " : ""}${choice.label}`,
          description: current.disabled || choice.unavailable || `${choice.service} · ${choice.status} · ${choice.access}`,
          disabled: !!current.disabled || !!choice.unavailable && !current.selected.includes(choice.id) || current.selected.length >= 16 && !current.selected.includes(choice.id),
          run: async () => {
            const request_id = randomUUID(), ids = current.selected.includes(choice.id) ? current.selected.filter(id => id !== choice.id) : [...current.selected, choice.id]
            await client.result({ type: "sources", sources: { action: "select", id: current.id, request_id, ids } }, request_id, s => s.sources_result)
            show()
          } })),
        { title: "Manage connections", run: () => client.navigate("/connections") },
      ], true)
    }
    show()
  }
  actions["plan-first"] = () => {
    const context = client.context
    menu("Plan before work", [
      { title: "Prepare a plan for my next message", description: "Review the proposal before implementation. Later external actions still require approval.", run: () => client.setPlanFirst(true, context) },
      { title: "Work directly on my next message", run: () => client.setPlanFirst(false, context) },
    ])
  }
  async function planAction(plan: TaskPlanSummary | TaskPlanReview, action: "inspect" | "edit" | "approve" | "start" | "cancel", content?: PlanContent) {
    const request_id = randomUUID()
    const result = await client.result({ type: "task_plan", task_plan: { conversation_id: owner(), task_id: plan.id, revision: plan.revision, digest: plan.digest, request_id, action, content } }, request_id, s => s.task_plan_result)
    return result.review
  }
  const planText = (plan: TaskPlanReview) => [plan.phase === "revising" ? "A revision is being prepared. The previous proposal below is retained for reference; approval is disabled." : "", plan.task, plan.content?.outcome,
    plan.correction ? `${plan.edit_state === "applied" ? "Retained user correction" : "Requested correction (not applied)"}:\n${plan.correction}` : "", plan.issue,
    ...(plan.content?.steps ?? []).map((step, i) => `${i + 1}. ${step.text}`),
    `Effects: ${plan.content?.effects.join(", ") || "None"}`, `Sources: ${plan.content?.sources.join(", ") || "None"}`,
    `Original inputs: ${plan.input_names?.join(", ") || "None"}`,
    `Execution limits: manager ${plan.budget.max_steps || "default"} steps; coding assignment ${plan.budget.code_max_steps || "default"} steps.`,
    `Revision ${plan.revision} · ${plan.state} · ${plan.phase}`,
    "A plan describes intended work. Activity and retained outputs show actual results. External actions keep their separate approvals.",
  ].filter(Boolean).join("\n\n")
  function editPlan(plan: TaskPlanReview) {
    if (!plan.content) return
    const key = `${owner()}:${plan.id}:${plan.revision}`, draft = planEdits.get(key) ?? structuredClone(plan.content)
    planEdits.set(key, draft)
    const show = () => menu("Edit proposed plan", [
      { title: "Intended outcome", description: draft.outcome, run: () => prompt("Intended outcome", draft.outcome, text => { draft.outcome = text; show() }) },
      ...draft.steps.map((step, index) => ({ title: `Step ${index + 1}: ${step.text}`, run: () => menu("Proposed step", [
        { title: "Edit step", run: () => prompt("Proposed step", step.text, text => { step.text = text; show() }) },
        { title: "Remove step", disabled: draft.steps.length <= 1, run: () => { draft.steps = draft.steps.filter(item => item.id !== step.id); show() } },
      ], true) })),
      { title: "Add step", disabled: draft.steps.length >= 24, run: () => prompt("New proposed step", "", text => { draft.steps.push({ id: randomUUID(), text }); show() }) },
      { title: "Effects", description: draft.effects.join(", "), run: () => menu("Allowed effects", ["read", "draft", "code", "checks", "connected_write", "settings", "browser"].map(effect => ({ title: `${draft.effects.includes(effect) ? "✓ " : ""}${effect.replaceAll("_", " ")}`, run: () => { draft.effects = draft.effects.includes(effect) ? draft.effects.filter(item => item !== effect) : [...draft.effects, effect]; show() } }))) },
      { title: "Connected sources", description: draft.sources.join(", ") || "None", run: () => menu("Plan sources", plan.sources.map(source => ({ title: `${draft.sources.includes(source) ? "✓ " : ""}${source}`, run: () => { draft.sources = draft.sources.includes(source) ? draft.sources.filter(item => item !== source) : [...draft.sources, source]; show() } }))) },
      { title: "Save revision for review", run: async () => { const review = await planAction(plan, "edit", draft); planEdits.delete(key); if (review) reviewPlan(review) } },
      { title: "Discard edits", run: () => confirm("Discard plan edits?", "The retained plan stays unchanged.", () => { planEdits.delete(key); reviewPlan(plan) }) },
    ], true)
    show()
  }
  function reviewPlan(plan: TaskPlanReview) {
    const editable = plan.state === "queued" && plan.phase === "review" && !!plan.content
    menu("Task plan review", [
      { title: "Read exact proposed plan", run: () => details("Proposed plan", planText(plan)) },
      { title: "Edit plan", disabled: !editable || !!client.state?.running, run: () => editPlan(plan) },
      { title: "Approve plan and start", disabled: !editable || !!client.state?.running, run: () => confirm("Approve this plan and start?", planText(plan), () => planAction(plan, "approve")) },
      { title: "Start approved work", disabled: plan.state !== "queued" || plan.phase !== "approved" || !!client.state?.running, run: () => confirm("Start reviewed work?", planText(plan), () => planAction(plan, "start")) },
      { title: "Cancel retained request", disabled: !["queued", "interrupted"].includes(plan.state), run: () => confirm("Cancel this request?", plan.task, () => planAction(plan, "cancel")) },
      { title: "Reuse request in message box", run: () => client.edit([client.draft.text, plan.task].filter(Boolean).join("\n\n")) },
    ], true)
  }
  actions["task-plans"] = () => menu("Reviewed task plans", (data.state?.task_plans ?? []).map(plan => ({ title: plan.outcome || plan.task, description: `${plan.state} · ${plan.phase} · revision ${plan.revision}${plan.issue ? " · " + plan.issue : ""}`, run: async () => { const review = await planAction(plan, "inspect"); if (review) reviewPlan(review) } })))
  actions.specialists = () => menu("Specialists in this task", (data.state?.specialists ?? []).map(task => ({ title: task.name, description: task.status, run: () => {
    const act = (action: "guide" | "stop", text?: string) => {
      if (!task.token) throw new Error("This worker no longer accepts live controls.")
      const request_id = randomUUID()
      return client.result({ type: "specialist", specialist: { request_id, id: task.id, token: task.token, action, text } }, request_id, s => s.specialist_result)
    }
    menu(task.name, [
      { title: "Assignment and observed status", run: () => details(task.name, [task.description, task.assignment, task.status].filter(Boolean).join("\n\n")) },
      { title: "Guide this specialist", disabled: !task.can_guide, run: () => prompt("Guide " + task.name, "", text => act("guide", text)) },
      { title: "Stop this specialist", disabled: !task.can_stop, run: () => confirm("Stop " + task.name + "?", "Other work can continue. Retained results remain available.", () => act("stop")) },
      { title: "All retained task details", run: () => client.navigate("/tasks") },
    ], true)
  } })))
  actions["original-chat"] = async () => { const id = client.state?.branch?.conversation_id; if (!id) throw new Error("This chat has no original branch."); await client.openSession(id) }
  actions.branch = () => {
    const conversation_id = owner()
    menu("Branch from a saved answer", (client.state?.messages ?? []).filter(message => message.role === "Gator" && message.text && !message.result && message.revision_id && message.run_id).map(message => ({ title: message.text.slice(0, 100), description: message.created_at, run: () => confirm("Create an independent chat?", "Earlier messages and retained file references are inherited through this answer. Actions are not replayed. Select connections again; queues, approvals, workers and schedules are not copied.", async () => {
      if (client.state?.conversation_id !== conversation_id) throw new Error("Return to the original conversation before branching.")
      await client.host.branch({ conversation_id, revision_id: message.revision_id!, run_id: message.run_id!, request_id: randomUUID() })
      client.route = "chat"; client.emit("change")
    }) })))
  }
  actions.evidence = () => {
    const context = client.context
    menu("Sources retained for an answer", (client.state?.messages ?? []).filter(message => message.bundle?.RunID && message.bundle.ManifestSHA256).map(message => ({ title: message.text.slice(0, 100) || message.bundle!.RunID!, run: async () => {
      const run = message.bundle!.RunID!, catalog = await client.host.sourceCatalog({ context, run })
      menu("Retained sources", catalog.references.map(reference => ({ title: reference.label, description: reference.locator, run: async () => {
        const source = await client.host.sourcePreview({ context, run, path: "evidence:" + reference.id })
        details(reference.label, [source.notice, `Captured: ${source.captured_at || "Unknown"}`, source.reads.length ? "Observed reads: " + source.reads.map(read => `${read.start}–${read.end}${read.truncated ? " (partial)" : ""}`).join(", ") : "Exact read ranges unavailable.", source.truncated ? "Showing part of the retained source." : "", source.text.split("\n").map((line, index) => `${source.firstLine + index}  ${line}`).join("\n")].filter(Boolean).join("\n\n"))
      } })))
    } })))
  }
  actions.changes = () => {
    const context = client.context
    menu("Retained code changes", (client.state?.messages ?? []).flatMap(message => (message.bundle?.Candidates ?? []).map(candidate => ({ title: candidate.ID, description: `${candidate.Status} · ${candidate.ChangedPaths.join(", ")}`, run: async () => {
      const change = await client.host.codePreview({ context, run: message.bundle!.RunID!, candidate: candidate.ID })
      details("Code changes", [change.status, ...change.verification.map(check => `${check.passed ? "Passed" : "Failed"}: ${check.argv.join(" ")}`), change.truncated ? "Showing part of the retained patch." : "", change.patch].filter(Boolean).join("\n\n"))
    } }))))
  }
  async function checkAction(check: FollowingCheck | undefined, action: Exclude<CheckAction["action"], "setup">) {
    // Managed views have their own owner while a foreground task is busy.
    // The existing session host keeps that task and its approval in place.
    if (client.state?.running) await client.navigate("/followup")
    if (client.state?.running) throw new Error("Wait for the check management view to open.")
    const request_id = randomUUID()
    const value: CheckAction = action === "refresh" ? { request_id, action } : { request_id, action, id: check!.id, digest: check!.digest }
    await client.result({ type: "check", check: value }, request_id, s => s.check_result)
    client.route = action === "inspect" || action === "edit" ? "chat" : "following"; client.emit("change")
  }
  function commitment(check: FollowingCheck) {
    const current = () => client.state?.checks?.find(item => item.id === check.id)
    const exact = () => { const value = current(); if (!value || value.digest !== check.digest) throw new Error("This check changed. Refresh Following and review its current agreement."); return value }
    menu(check.title, [
      { title: "Inspect agreement and check health", description: check.issue || check.health, run: () => checkAction(exact(), "inspect") },
      { title: "Edit outcome, sources and timing", description: check.editable ? "Review changes before saving" : "This schedule uses advanced settings; open /jobs", disabled: !check.editable, run: () => checkAction(exact(), "edit") },
      { title: check.enabled ? "Pause this check" : "Resume this check", description: check.state === "goal paused" ? "Resume the goal first" : !check.enabled && check.state === "ended" ? "Review a new end date before resuming" : check.enabled ? "Stops future checks; a running check may finish" : "Uses the same saved agreement and existing service",
        disabled: !check.enabled && ["goal paused", "ended"].includes(check.state), run: () => {
          const value = exact(), pause = value.enabled
          confirm(`${pause ? "Pause" : "Resume"} ${value.title}?`, [value.outcome, `Sources: ${[value.source, ...(value.connectors ?? [])].filter(Boolean).join(", ") || "None"}`, `Timing: ${value.schedule} · ${value.timezone}`, pause ? "Future checks will stop. A check already running may finish. Its retained results remain available." : "Resume future checks using this saved agreement. This does not start the local service or run a check immediately. Checks require this computer to be awake and their sources accessible."].join("\n\n"), async () => {
            await checkAction(value, pause ? "pause" : "resume")
            details(value.title, pause ? "Check paused. Future checks are stopped; a check already running may finish." : client.state?.supervisor_running ? "Check enabled. The local service will evaluate its next due time while this computer is awake and its sources are accessible." : "Check enabled, but the local service is offline. Open /recovery to review starting it.")
          })
        } },
      { title: "Review local service", run: () => client.navigate("/recovery") },
      ...(check.goal_id ? [{ title: "Goal and plan", description: "Open this commitment's own conversation", run: () => client.openGoal(check.goal_id!) }] : []),
    ], true)
  }
  return { commitment, refreshChecks: () => checkAction(undefined, "refresh") }
}
