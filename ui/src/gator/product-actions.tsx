import { randomUUID } from "node:crypto"
import { useClient } from "../context/client"
import { useData } from "../context/data"
import type { useActions } from "./actions"
import type { ChatUpdate, CheckAction, FollowingCheck, FollowingGoal, GoalAction, PlanContent, PlanInputRef, TaskPlanReview, TaskPlanSummary } from "@gator/application-host/contracts/protocol"
import { useReadView } from "./read-view"

// Optional management stays behind commands. All mutations use the current
// Go protocol, including its live tokens, revisions and acknowledgements.
export function useProductActions(controls: ReturnType<typeof useActions>) {
  const client = useClient(), data = useData()
  const readView = useReadView()
  const { menu, prompt, details, confirm, actions } = controls
  const planEdits = new Map<string, PlanContent>()
  let goalFailure: { id: string; context: string; message: string } | undefined
  const owner = () => {
    const id = client.state?.conversation_id
    if (!id) throw new Error("Wait for this conversation to open.")
    return id
  }
  actions.appearance = async () => {
    client.preferences = await client.host.preferences.read(); client.emit("change")
    menu("Appearance", [
    ...([ ["Follow terminal colors", "terminal"], ["Dark", "gator"], ["Light", "light"], ["High contrast", "contrast"], ["Monochrome", "mono"] ] as const).map(([title, theme]) => ({ title, description: client.preferences.theme === theme ? "Current appearance" : undefined, run: async () => {
      await client.configurePreferences({ theme })
    } })),
    { title: "Keyboard shortcuts", run: () => client.send({ type: "navigate", text: "/appearance" }) },
    ])
  }
  actions["terminal-settings"] = () => menu("Terminal appearance and navigation", [
    { title: "Appearance", description: client.preferences.theme, run: () => client.navigate("/appearance") },
    { title: client.preferences.sidebarCollapsed ? "Show navigation rail" : "Hide navigation rail", description: "Shown when the terminal has enough room", run: () => client.configurePreferences({ sidebarCollapsed: !client.preferences.sidebarCollapsed }) },
    { title: client.preferences.groupChatsByFolder ? "Show flat recent chats" : "Group recent chats by folder", run: () => client.configurePreferences({ groupChatsByFolder: !client.preferences.groupChatsByFolder }) },
    { title: "Reload saved terminal settings", run: async () => { client.preferences = await client.host.preferences.read(); client.issue = ""; client.emit("change") } },
    ...(!client.state?.terminal_preferences?.initialized ? [{ title: "Use default display settings", description: "Review defaults while keeping the original retained settings file", run: () => confirm("Use default display settings?", "Use default display and navigation preferences. A theme explicitly saved through Gator is preserved. The original retained settings file is kept; conversations, tasks, approvals and drafts stay intact.", async () => {
      if (!client.host.preferences.initializeDefaults) throw new Error("Display recovery is unavailable in this runtime.")
      client.preferences = await client.host.preferences.initializeDefaults(); client.issue = ""; client.emit("change")
    }) }] : []),
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
  async function planAction(plan: TaskPlanSummary | TaskPlanReview, action: "inspect" | "edit" | "remove_inputs" | "approve" | "start" | "cancel", content?: PlanContent, inputs?:PlanInputRef[]) {
    const request_id = randomUUID()
    const result = await client.result({ type: "task_plan", task_plan: { conversation_id: owner(), task_id: plan.id, revision: plan.revision, digest: plan.digest, request_id, action, content, inputs } }, request_id, s => s.task_plan_result)
    return result.review
  }
  const planText = (plan: TaskPlanReview) => [plan.phase === "revising" ? "A revision is being prepared. The previous proposal below is retained for reference; approval is disabled." : "", plan.task, plan.content?.outcome,
    plan.correction ? `${plan.edit_state === "applied" ? "Retained user correction" : "Requested correction (not applied)"}:\n${plan.correction}` : "", plan.issue,
    ...(plan.content?.steps ?? []).map((step, i) => `${i + 1}. ${step.text}`),
    `Effects: ${plan.content?.effects.join(", ") || "None"}`, `Sources: ${plan.content?.sources.join(", ") || "None"}`,
    `Selected inputs: ${plan.input_names?.join(", ") || "None"}`,
    plan.input_additions?.length ? `Added inputs requiring this fresh review: ${plan.input_additions.join(", ")}. Original inputs remain retained. Accounts, permissions and original budget are unchanged.` : "",
    plan.input_removals?.length ? `Inputs withdrawn from future task use: ${plan.input_removals.join(", ")}. Originals remain in history; earlier runs may already have processed them.` : "",
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
      { title: "Remove selected inputs", disabled: !editable || !!client.state?.running || !plan.inputs?.length, run: () => menu("Selected task inputs", (plan.inputs ?? []).map(input => ({ title: input.name || input.id, description: input.kind === "upload" ? "Selected upload" : "Selected source file", run: () => confirm("Remove this input from future execution?", `${input.name || input.id}\nThe original stays in history. A new exact plan review is required before execution. Earlier runs may already have processed it.`, async () => { await planAction(plan,"remove_inputs",undefined,[{kind:input.kind,id:input.id}]); const next=client.state?.task_plans?.find(item=>item.id===plan.id); if(next){ const review=await planAction(next,"inspect");if(review)reviewPlan(review) } }) })),true) },
      { title: "Edit plan", disabled: !editable || !!client.state?.running, run: () => editPlan(plan) },
      { title: "Approve plan and start", disabled: !editable || !!client.state?.running, run: () => confirm("Approve this plan and start?", planText(plan), () => planAction(plan, "approve")) },
      { title: "Start approved work", disabled: plan.state !== "queued" || plan.phase !== "approved" || !!client.state?.running, run: () => confirm("Start reviewed work?", planText(plan), () => planAction(plan, "start")) },
      { title: "Cancel retained request", disabled: !["queued", "interrupted"].includes(plan.state), run: () => confirm("Cancel this request?", plan.task, () => planAction(plan, "cancel")) },
      { title: "Reuse request in message box", run: () => client.edit([client.draft.text, plan.task].filter(Boolean).join("\n\n")) },
    ], true)
  }
  actions["request-recovery"] = () => menu("Task setup and recovery", (data.state?.request_recoveries ?? []).map(task => ({ title: task.text, description: task.recovery.summary, run: () => {
    const act = async (action: "prepare_retry" | "discard") => {
      const request_id = randomUUID()
      await client.result({type:"request_recovery",request_recovery:{conversation_id:owner(),task_id:task.task_id,token:task.token,action,request_id}},request_id,s=>s.request_recovery_result)
    }
    menu("Original task recovery", [
      {title:"Inspect failure and repair",run:()=>details("Original task",[task.text,`${task.provider} · ${task.model}`,task.recovery.summary,task.recovery.repair,"No task is replayed automatically. A prepared retry still requires a fresh exact plan review."].join("\n\n"))},
      {title:"Repair original model setup",run:()=>client.navigate("/models")},
      {title:"Prepare retry for review",disabled:!task.recovery.retry_safe || !!client.state?.running,run:()=>confirm("Prepare this task for fresh review?",`${task.text}\n\nKeep its original inputs, scope and remaining budget. This prepares a plan; it does not execute the task.`,()=>act("prepare_retry"))},
      {title:"Inspect original receipts",run:()=>client.navigate("/receipts")},
      {title:"Review prepared task plans",run:()=>client.navigate("/task-plans")},
      {title:"Discard retained retry",run:()=>confirm("Discard this retry?","Keep the original results and receipts. The task will not be retried.",()=>act("discard"))},
    ],true)
  }})))
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
  async function goalAction(value: FollowingGoal, action: GoalAction["action"]) {
    if (action !== "inspect" && client.state?.running && !await client.navigate("/followup")) return
    if (action !== "inspect" && client.state?.running) throw new Error("Wait for the goal management view to open.")
    const request_id = randomUUID()
    const goal: GoalAction = action === "inspect" ? { request_id, id: value.id, action } : { request_id, id: value.id, action, digest: value.digest! }
    const context = client.context, previousFailure = goalFailure
    try {
      const result = await client.result({ type: "goal", goal }, request_id, state => state.goal_result)
      if (!result.review || result.review.id !== value.id) throw new Error("The selected goal was not confirmed. Refresh Following before retrying.")
      if (goalFailure === previousFailure && goalFailure?.id === value.id && goalFailure.context === context && goalFailure.message === client.issue) { client.issue = ""; goalFailure = undefined }
      if (action !== "inspect") client.route = "following"
      client.emit("change")
      return result.review
    } catch (error) {
      if (client.context === context) goalFailure = { id: value.id, context, message: error instanceof Error ? error.message : String(error) }
      throw error
    }
  }
  function goalCommitment(goal: FollowingGoal) {
    const current = () => client.state?.following_goals?.find(item => item.id === goal.id)
    const exact = () => {
      const value = current()
      if (!value?.controllable || !value.digest || value.digest !== goal.digest) {
        const message = "This goal changed. Refresh Following and review its current responsibility."
        goalFailure = { id: goal.id, context: client.context, message }
        throw new Error(message)
      }
      return value
    }
    const checks = client.state?.checks?.filter(check => check.goal_id === goal.id) ?? []
    menu(`Goal · ${goal.title}`, [
      ...checks.map(check => ({ key: `check:${check.id}`, title: check.title, description: `${check.state} · ${check.next_action}`, run: () => commitment(check) })),
      { key: "inspect-goal", title: "Inspect goal and retained plan", description: "Current outcome, workspace, plan steps and check coverage", disabled: !goal.controllable, run: () => readView({
        title: "Inspect " + goal.title, context: client.context, description: "Reading this goal and its retained plan on this device. Current work and drafts are kept.",
        back: () => { const value = current(); if (value) goalCommitment(value) },
        read: async () => { const review = await goalAction(goal, "inspect"); if (!review) throw new Error("The goal read was not confirmed."); return review },
        show: review => details(review.title, review.lines.join("\n\n"), () => { const value = current(); if (value) goalCommitment(value) }),
      }) },
      { key: "goal-chat", title: "Goal and plan", description: goal.status === "paused" ? "Resume before starting goal work; inspection remains available" : checks.length ? "Intended work and retained progress" : "Not monitored automatically", disabled: goal.status !== "active", run: () => client.openGoal(goal.id) },
      { key: "goal-status", title: goal.status === "paused" ? "Resume this goal" : "Pause this goal", description: "Review the exact goal; saved check agreements stay intact", disabled: !goal.controllable, run: () => {
        const value = exact(), pause = value.status === "active", context = client.context
        confirm(`${pause ? "Pause" : "Resume"} goal: ${value.title}?`, [value.outcome, `Goal: ${value.id}`, `Workspace: ${value.source || "Unavailable"}`,
          ...checks.map(check => `Check: ${check.title} · ${check.id} · ${check.schedule} · ${check.timezone}`),
          pause ? "Future checks for this goal will not start. A foreground task or check already running may finish. Other goals and checks stay unchanged." : "The goal becomes active. Its enabled checks use their current saved agreements; paused or ended checks stay paused or ended. This does not start a foreground task or the local service. Checks require an awake computer and accessible sources.",
          "Saved plans, history, drafts and check settings are retained."].filter(Boolean).join("\n\n"), async () => {
          const review = await goalAction(value, pause ? "pause" : "resume")
          if (!review) return
          controls.resetMenus()
          details(value.title, pause ? "Goal paused. Future checks for this goal are stopped; current work may finish. Its saved plan and check agreements are retained." : review.enabled_checks ? "Goal active. Enabled checks use their saved agreements when the service and sources are available. Paused or ended checks are not re-enabled." : "Goal active. No recurring check is enabled, so it is not monitored automatically. Attach or enable a reviewed check in Following.")
        }, () => { if (client.context === context) goalCommitment(value) })
      } },
      ...(!checks.length ? [{ key: "attach-check", title: "Attach a recurring check", description: goal.status === "active" ? "Review sources and timing for this goal" : "Resume the goal before attaching a check", disabled: goal.status !== "active", run: () => client.prepareGoalCheck(goal.id) }] : []),
    ], true)
  }
  function commitment(check: FollowingCheck) {
    const current = () => client.state?.checks?.find(item => item.id === check.id)
    const goal = client.state?.following_goals?.find(goal => goal.id === check.goal_id)
    const exact = () => { const value = current(); if (!value || value.digest !== check.digest) throw new Error("This check changed. Refresh Following and review its current agreement."); return value }
    menu(check.title, [
      { title: "Inspect agreement and check health", description: check.issue || check.health, run: () => checkAction(exact(), "inspect") },
      { title: "Edit check agreement", description: check.editable ? "Review outcome, sources or supported timing changes before saving" : "Check editing is unavailable", disabled: !check.editable, run: () => checkAction(exact(), "edit") },
      { title: check.enabled ? "Pause this check" : "Resume this check", description: check.state === "goal paused" ? "Resume the goal first" : !check.enabled && check.state === "ended" ? "Review a new end date before resuming" : check.enabled ? "Stops future checks; a running check may finish" : "Uses the same saved agreement and existing service",
        disabled: !check.enabled && ["goal paused", "ended"].includes(check.state), run: () => {
          confirmCheckPause(exact())
        } },
      { title: "Review local service", run: () => client.navigate("/recovery") },
      ...(check.goal_id ? [{ title: "Goal and plan", description: "Open this commitment's own conversation", disabled: goal?.status === "paused", run: () => client.openGoal(check.goal_id!) }] : []),
      ...(goal?.controllable ? [{ title: "Goal controls and retained plan", description: "Inspect, pause or resume this exact goal", run: () => goalCommitment(goal) }] : []),
    ], true)
  }
  function confirmCheckPause(value: FollowingCheck) {
    const pause = value.enabled
    confirm(`${pause ? "Pause" : "Resume"} ${value.title}?`, [value.outcome, `Check: ${value.id}`, `Sources: ${[value.source, ...(value.connectors ?? [])].filter(Boolean).join(", ") || "None"}`, `Timing: ${value.schedule} · ${value.timezone}`, pause ? "Future checks will stop. A check already running may finish. Its retained results remain available. Other checks keep their saved agreements." : "Resume future checks using this saved agreement. This does not start the local service or run a check immediately. Checks require this computer to be awake and their sources accessible."].join("\n\n"), async () => {
      await checkAction(value, pause ? "pause" : "resume")
      controls.resetMenus()
      details(value.title, pause ? "Check paused. Future checks are stopped; a check already running may finish. Other checks keep their saved agreements." : client.state?.supervisor_running ? "Check enabled. The local service will evaluate its next due time while this computer is awake and its sources are accessible." : "Check enabled, but the local service is offline. Open /recovery to review starting it.")
    })
  }
  function reviewNoise(update: ChatUpdate, context: string) {
    const retained = client.state?.chat_updates?.find(item => item.id === update.id)
    if (client.context !== context || !retained || retained.version !== update.version || retained.job_id !== update.job_id) throw new Error("Return to the original update and review its current check.")
    const check = client.state?.checks?.find(check => !!update.job_id && check.id === update.job_id)
    if (!check) { details("Feedback saved", "Noise feedback was saved. The original check is not available here, so its behavior has not changed. Refresh Following to inspect the saved check before changing it."); return }
    const exact = () => {
      if (client.context !== context) throw new Error("Return to the original update before changing this check.")
      const current = client.state?.checks?.find(item => item.id === check.id)
      if (!current || current.digest !== check.digest) throw new Error("This check changed. Refresh Following and review its current agreement.")
      return current
    }
    menu("Reduce interruptions", [
      { title: "Review notification conditions and timing", description: `${check.title} · Review before saving; other checks stay unchanged`, disabled: !check.editable, run: () => checkAction(exact(), "edit") },
      { title: "Review pausing this check", description: `${check.title} · Stops future checks; a running check may finish`, disabled: !check.enabled, run: () => confirmCheckPause(exact()) },
      { title: "Inspect affected check", description: `${check.title} · ${check.id}`, run: () => checkAction(exact(), "inspect") },
      { title: "Keep feedback only", description: "Your assessment is saved; no check behavior has changed", run: () => details("Feedback saved", `Noise feedback is saved for ${update.title}. No check behavior was changed. You can review ${check.title} in Following later.`) },
    ], true)
  }
  return { commitment, goalCommitment, reviewNoise, refreshChecks: () => checkAction(undefined, "refresh") }
}
