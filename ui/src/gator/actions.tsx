import { createEffect, onCleanup } from "solid-js"
import { randomUUID } from "node:crypto"
import { resolve } from "node:path"
import { useClient } from "../context/client"
import { useDialog } from "../ui/dialog"
import { DialogPrompt } from "../ui/dialog-prompt"
import { DialogConfirm } from "../ui/dialog-confirm"
import { literal } from "./text"
import { useFiles } from "./files"
import { PathPicker } from "./path-picker"
import { Details } from "./details"
import { ActionPrompt } from "./action-prompt"
import { useData } from "../context/data"
import { activityKey, activitySummary, activityText, searchableMessage } from "./activity"
import { useKeyHints } from "./key-hints"
import { requestContext } from "./request-context"
import productGuide from "../../../internal/productguide/capabilities.json"
import { useMenuNavigation } from "./menu-navigation"

export type Choice = { title: string; key?: string; description?: string; category?: string; disabled?: boolean; run(): unknown }
export function useActions() {
  const client = useClient(), dialog = useDialog(), data = useData(), hint = useKeyHints()
  let previousContext = client.context
  createEffect(() => {
    const context = data.context
    if (context !== previousContext) { previousContext = context; navigation.reset(); dialog.clear() }
  })
  const attempt = (operation: unknown, label = "Working…") => { if (operation instanceof Promise) void client.track(label, operation).catch(e => client.fail(e)) }
  const navigation = useMenuNavigation(attempt), menu = navigation.menu
  const prompt = (title: string, value: string, save: (text: string) => unknown, description?: string) => { const token = Symbol(title); dialog.replace(() => <ActionPrompt token={token} title={title} value={value} save={save} description={description} />, undefined, { key: token }) }
  const details = (title: string, text: string, back?: () => void) => { const restore = navigation.back(back); dialog.replace(() => <Details title={title} text={text} back={restore} />, undefined, { size: "xlarge" }) }
  const confirm = (title: string, message: string, run: () => unknown, cancel?: () => void) => dialog.replace(() => <DialogConfirm title={title} message={literal(message)} onCancel={cancel ? () => queueMicrotask(cancel) : undefined} onConfirm={() => { try { attempt(run(), literal(title) + "…") } catch (error) { client.fail(error) } }} />)
  async function drafts() {
    const saved = await client.host.drafts.request({ action: "list" })
    menu("Unsent drafts", [
      { title: saved.enabled ? "Disable draft retention" : "Enable draft retention", run: () => confirm("Change draft retention?", saved.enabled ? "All retained composer drafts will be cleared from disk. Current unsent input remains editable until this session closes." : "Keep unsent drafts on this device for up to seven days.", async () => { await client.host.drafts.request({ action: "configure", enabled: !saved.enabled }); client.emit("change") }) },
      ...saved.drafts.map(draft => ({ title: draft.title || draft.context, description: [draft.plan_first ? "Plan first" : "", draft.text.slice(0, 120) || (draft.uploads?.length ? `${draft.uploads.length} attachments` : "No message yet")].filter(Boolean).join(" · "), run: () => menu("Saved draft", [
        { title: "Recover in original chat", run: async () => {
          if (draft.context.startsWith("conversation:")) await client.host.sessions.open({ conversation_id: draft.context.slice(13) })
          else if (client.context !== draft.context) throw new Error("Open this workspace's original chat to recover its draft.")
          const { updated, ...editable } = draft
          client.drafts.set(draft.context, editable); client.route = "chat"; client.emit("change")
        } },
        { title: "Remove this draft", run: () => confirm("Remove unsent draft?", draft.text, async () => { await client.host.drafts.request({ action: "remove", context: draft.context }); client.drafts.delete(draft.context); client.emit("change") }) },
      ]) })),
    ])
  }
  function queue() {
    menu("Queued and interrupted messages", (client.state?.queue ?? []).map(item => ({ title: item.text || `${item.inputs?.length ?? 0} attached files`, description: item.interrupted ? "Interrupted; explicit resume required" : "Queued", run: () => {
      const act = async (action: "edit" | "remove" | "earlier" | "later" | "run" | "guide", text?: string) => {
        const request_id = randomUUID()
        await client.result({ type: "queue", queue: { id: action === "guide" ? client.state?.guidance_id ?? "" : item.id, request_id, action, text } }, request_id, s => s.queue_result)
      }
      menu("Queue message", [
        { title: "Edit message", run: () => prompt("Edit queued message", item.text, text => act("edit", text)) },
        { title: "Move earlier", run: () => act("earlier") }, { title: "Move later", run: () => act("later") },
        { title: "Remove", run: () => confirm("Remove queued message?", item.text, () => act("remove")) },
        { title: "Run / resume", run: () => confirm("Run this retained message?", item.text, () => act("run")) },
        { title: "Guide active work with this text (keep queued copy)", disabled: !client.state?.guidance_id || !!item.inputs?.length || !!item.reply_to_update_id, run: () => act("guide", item.text) },
      ])
    } })))
  }
  function memories() {
    const conversation_id = client.state?.conversation_id
    if (!conversation_id) return
    menu("Saved memory receipts", (client.state?.chat_memories ?? []).map(memory => ({ title: memory.text, description: `${memory.scope} · ${memory.updated}`, run: () => {
      const act = async (action: "correct" | "forget" | "undo", text?: string) => {
        const request_id = randomUUID(); await client.result({ type: "memory", memory: { conversation_id, id: memory.id, updated: memory.updated, action, text, request_id } }, request_id, s => s.chat_memory_result)
      }
      menu("Memory and provenance", [
        { title: "Inspect source / quote", description: memory.quote || memory.run_id, run: () => details("Memory provenance", `${memory.scope}\n${memory.quote ?? ""}\n${memory.text}`) },
        { title: "Correct", run: () => prompt("Correct saved memory", memory.text, text => act("correct", text)) },
        { title: "Forget", run: () => confirm("Forget this memory?", memory.text, () => act("forget")) },
        { title: "Undo original memory save", disabled: !memory.can_undo, run: () => act("undo") },
      ])
    } })))
  }
  function models() {
    const chooseModel = () => menu("Models › Choose model", () => {
      const composer = data.state?.composer
      if (!composer) return []
      return composer.models.map(model => ({ key: `model:${model.provider}:${model.model}`, title: model.label, category: model.provider,
        description: composer.disabled || model.unavailable || (model.provider === data.state?.provider && model.model === data.state?.model ? "Current model" : undefined), disabled: !!composer.disabled || !!model.unavailable, run: async () => {
        const request_id = randomUUID(); await client.result({ type: "composer", composer: { action: "model", id: composer.id, request_id, provider: model.provider, model: model.model } }, request_id, s => s.composer_result)
      } }))
    }, true, models)
    const chooseEffort = () => menu("Models › Choose effort", () => {
      const composer = data.state?.composer
      if (!composer) return []
      return (["low", "standard", "high"] as const).map(effort => ({ key: `effort:${effort}`, title: `Effort: ${effort}`, description: composer.disabled || (composer.effort === effort ? "Current effort" : undefined), disabled: !!composer.disabled, run: async () => {
        const request_id = randomUUID(); await client.result({ type: "composer", composer: { action: "effort", id: composer.id, request_id, effort } }, request_id, s => s.composer_result)
      } }))
    }, true, models)
    menu("Model and effort for future messages", () => [
      { title: "Choose model", description: data.state?.composer?.disabled || `${data.state?.provider || "No provider"} / ${data.state?.model || "Not selected"}`, run: chooseModel },
      { title: "Choose effort", description: data.state?.composer?.disabled || data.state?.composer?.effort || "standard", run: chooseEffort },
      { title: "Processing and work limits", description: data.state?.composer?.readiness?.issue || "Where requests go and what effort limits mean", run: () => details("Request context and limits", requestContext(client.state, client.draft)) },
      { title: "Configure providers and local models", run: () => client.navigate("/model") },
    ])
  }
  function catalog() {
    const panel = client.state?.panel, row = panel?.selected ?? 0, item = panel?.rows?.[row]
    if (!panel?.catalog?.can_organize || !panel.id || !item) throw new Error("Open Conversations and select a retained chat first.")
    const id = panel.id
    const change = async (action: "rename" | "pin" | "unpin" | "archive" | "restore", text?: string) => {
      await client.send({ type: "panel", panel: { id, row, action, ...(text === undefined ? {} : { text }) } })
      await client.waitFor(state => !state.catalog_busy && !state.panel?.catalog?.loading)
      const state = client.state
      if (state?.notice || state?.panel?.notice_tone === "error") throw new Error(state.notice || state.panel?.notice || "The conversation changed. Refresh and review before retrying.")
    }
    menu(item.label, [
      { title: "Conversation details", run: () => details(item.label, [item.source ? `Folder: ${item.source}` : "", item.updated_at ? `Updated: ${item.updated_at}` : "", item.detail, ...(item.metadata ?? []).map(field => `${field.label}: ${field.value}`)].filter(Boolean).join("\n\n")) },
      { title: "Rename conversation", run: () => prompt("Rename retained conversation", item.label, text => change("rename", text)) },
      { title: item.pinned ? "Unpin conversation" : "Pin conversation", run: () => change(item.pinned ? "unpin" : "pin") },
      { title: item.archived ? "Restore conversation" : "Archive conversation", run: () => confirm(item.archived ? "Restore this conversation?" : "Archive this conversation?", item.label, () => change(item.archived ? "restore" : "archive")) },
    ])
  }
  function chats() {
    const grouped = client.preferences.groupChatsByFolder
    menu(grouped ? "Chats grouped by folder" : "Open and recent chats", () => {
      const activity = data.snapshot.chatActivity, recent = data.state?.recent_conversations ?? []
      return [
        ...activity.map(chat => ({ key: `chat:${chat.id}`, title: recent.find(item => item.id === chat.id)?.title || chat.title, description: chat.status, category: grouped ? "Activity" : undefined, run: () => client.openSession(chat.id) })),
        ...recent.filter(chat => !activity.some(active => active.id === chat.id)).map(chat => ({ key: `chat:${chat.id}`, title: chat.title, description: `${chat.source} · ${chat.updated_at}`, category: grouped ? chat.source : undefined, run: () => client.openConversation(chat.id, client.state?.recent_id ?? "") })),
        { key: "grouping", title: grouped ? "Show flat recent chats" : "Group recent chats by folder", run: async () => { await client.configurePreferences({ groupChatsByFolder: !grouped }); chats() } },
        { key: "catalog", title: "Search all retained conversations", run: () => client.navigate("/conversations") },
      ]
    })
  }
  const actions: Record<string, () => unknown> = {
    "queue-message":()=>prompt("Message to run after current work",client.draft.text.startsWith("/")?"":client.draft.text,async text=>{await client.edit(text);await client.submit(true)},"Keep this task and its selected files in the current owner's explicit queue. Ordinary messages stay conversational."),
    tasks: () => menu("Work in this conversation", () => [
      ...(client.state?.dialogue_task ? [{title:"Return to the requesting conversation",run:()=>client.openSession(client.state!.dialogue_task!.parent_id)}] : []),
      ...(client.state?.dialogue ?? []).filter(turn=>turn.task_chat).map(turn=>({key:turn.id,title:turn.text,description:client.state?.dialogue_peers?.find(peer=>peer.conversation_id===turn.task_chat)?.state || "Outcome unconfirmed",run:()=>client.openSession(turn.task_chat!)})),
    ]),
    "earlier-messages":async()=>{const state=client.state,before=state?.dialogue?.[0]?.sequence;if(!state?.dialogue_more || !before) return details("Earlier messages","All retained conversation messages are loaded.");await client.send({type:"dialogue_page",dialogue_before:before})},
    latest: async () => { if (await client.navigate("/chat")) client.emit("transcript-latest") },
    "catch-up": async () => { if (await client.navigate("/chat")) client.emit("transcript-catch-up") },
    drafts, queue, memories, models,
    recover: () => {
      const context = client.context, disconnected = client.snapshot.connection === "disconnected"
      const sameChat = () => { if (client.context !== context) throw new Error("The conversation changed. Reopen recovery in the intended chat.") }
      menu(disconnected ? "Reconnect this chat" : "Recover this action", [
        ...(disconnected ? [{ title: "Reconnect and review", description: "Keep the draft. Recovered tasks stay paused; external actions are not resent.", run: () => { sameChat(); return client.reconnect() } }] : []),
        ...(!disconnected && client.state?.terminal_preferences && !client.state.terminal_preferences.initialized ? [{ title: "Recover terminal settings", run: () => { sameChat(); return client.navigate("/terminal-settings") } }] : []),
        ...(!disconnected && client.state?.composer?.readiness && client.state.composer.readiness.state !== "configured" ? [{ title: "Repair model setup", description: client.state.composer.readiness.issue, run: () => { sameChat(); return client.navigate("/setup") } }] : []),
        ...(!disconnected && client.canRetrySubmission ? [{ title: "Retry this message acknowledgement", description: "Uses the original request identity. An already accepted message will not start a second task.", run: () => { sameChat(); if (!client.canRetrySubmission) throw new Error("The draft changed. Send it as a new message when ready."); return client.submit() } }] : []),
        ...(!disconnected ? [{ title: "Inspect retained requests", description: "Review queued or interrupted work before resuming.", run: () => { sameChat(); return queue() } },
          { title: "Inspect external receipts", description: "Check uncertain sends before considering another action.", run: () => { sameChat(); return actions.receipts() } }] : []),
        { title: "Inspect error details", run: () => details("Action details", client.issue || client.snapshot.error || client.snapshot.sessionIssue || "The connection is unavailable.") },
        ...(!disconnected && client.issue ? [{ title: "Dismiss error and keep editing", run: () => { sameChat(); client.issue = ""; client.emit("change") } }] : []),
      ])
    },
    setup: () => menu("Set up a model for this request", [
      { title: "Connect or repair a model", description: client.state?.composer?.readiness?.issue || "Choose a local model or a cloud provider. Your draft is kept.", run: () => client.navigate("/model") },
      { title: "Why setup is needed", description: "Read the complete issue and recovery instructions", run: () => details("Model setup issue", (client.state?.composer?.readiness?.issue || "Choose a model for this request.") + "\n\nYour draft and attachments stay in this chat. Setup does not submit the request.") },
      ...((client.state?.composer?.models.length ?? 0) ? [{ title: "Choose an already configured model", run: models }] : []),
      { title: "Request context and limits", description: "Processing destination, selected sources and work budgets", run: () => details("Request context and limits", requestContext(client.state, client.draft)) },
      { title: "What can Gator help with?", description: "Files, web research, optional accounts and reviewed changes", run: () => details("Capabilities and optional setup", productGuide.connections_setup + "\n\n" + productGuide.help) },
      { title: "Return to my draft", run: () => client.navigate("/chat") },
    ]),
    "request-context": () => details("Request context and limits", requestContext(client.state, client.draft)),
    "draft-context": () => menu("Draft details", [
      { title: "Request context and limits", description: client.state?.composer?.readiness?.issue || "Model processing, workspace, accounts and work budgets", run: () => details("Request context and limits", requestContext(client.state, client.draft)) },
      { title: `Plan first: ${client.draft.plan_first ? "on" : "off"}`, description: "Review a proposal before implementation; external actions still require approval.", run: () => client.navigate("/plan-first") },
      { title: `Attachments: ${client.draft.uploads?.length || 0}`, description: client.draft.uploads?.map(file => file.name).join(" · ") || "No private files attached", run: () => actions.attachments() },
      ...(client.draft.reply_to_update_id ? [{ title: "Clear reply link", description: client.state?.chat_updates?.find(update => update.id === client.draft.reply_to_update_id)?.title || "Replying to a retained update", run: () => client.edit(client.draft.text, client.draft.uploads, null) }] : []),
    ]),
    activity: () => {
      const context = client.context, live = client.state?.activity
      menu("Public work activity", [
      ...(live ? [{ title: "Inspect live public activity", run: () => details("Live public activity", activityText(client.context === context ? client.state?.activity ?? live : live)) }] : []),
      ...(client.state?.messages ?? []).flatMap((message, index) => message.activity ? [{ title: `${index + 1}. ${activitySummary(message.activity)}`, description: message.text.slice(0, 100), run: () => {
        const key = activityKey(context, message, index), trace = message.activity!
        menu("Retained public activity", [
          { title: client.activityExpanded.has(key) ? "Collapse in transcript" : "Expand in transcript", run: () => { if (client.activityExpanded.has(key)) client.activityExpanded.delete(key); else client.activityExpanded.add(key); client.emit("change"); client.emit("reveal-message", index) } },
          { title: "Inspect all public entries", run: () => details("Retained public activity", activityText(trace)) },
        ])
      } }] : []),
      ])
    },
    "catalog-actions": catalog,
    "check-health": () => details("Check health in this chat", [(client.state?.chat_checks_issue ?? ""), ...(client.state?.chat_checks ?? []).flatMap(check => [check.title + " · " + check.state, check.last_success ? `Last successful check for current settings: ${check.last_success}.` : "No successful check recorded for current settings.", check.next_due ? `Next scheduled: ${check.next_due}.` : "No check is currently scheduled.", check.unsuccessful ? `${check.unsuccessful} consecutive unsuccessful checks. ${check.issue || ""}` : check.issue || "", check.next_action, "Checks depend on this computer being on.", check.gap_from && check.gap_to ? `Observed supervisor gap: ${check.gap_from}–${check.gap_to}. This does not establish what changed while checks were unavailable.` : "", ""])].filter(Boolean).join("\n")),
    help: () => details("Gator terminal help", [`${hint("command.palette.show")} commands · ${hint("gator.sidebar")} navigation rail · ${hint("gator.new")} new independent chat · ${hint("gator.quit")} quit`, `${hint("gator.send")} talks / steers · ${hint("gator.newline")} new line · ${hint("gator.guide")} guides the live task · ${hint("gator.cancel")} cancels/stops`, `${hint("gator.queue")} queues a draft after current work in this chat; Tab completes slash commands and navigates forms`, `${hint("gator.context-actions")} item actions · /actions chooses a result`, `${hint("gator.search")} searches the transcript · ${hint("gator.draft-context")} draft details · PgUp/PgDn scroll or move selection`, `${hint("gator.latest")} or /latest jumps to newest content · /catch-up starts at the first unseen item; earlier reading positions survive restart`, `${hint("gator.approve")} reviews the live approval · ${hint("gator.decline")} declines · Paste never sends or approves`, "/following tracks goals/checks/updates · /saved opens knowledge/history/files · /settings configures Gator", "/sources selects connections · /plan-first proposes before working · /task-plans reviews retained plans", "/specialists guides or stops one worker · /branch starts a separate chat · /evidence and /changes inspect exact retained sources", "/links edits links into drafts · /read-aloud reads a saved reply with a system voice", "/chats switches chats · /queue manages retained messages · /drafts recovers drafts · /prompt-history reuses sent text", "/files opens full files · Ctrl+S saves · Ctrl+E edits Office/cells · Ctrl+O versions/export/Ask Gator", "/voice explicitly records/transcribes or configures new-reply playback · /attach-file selects a private input", "Forms: Enter edits a field · Ctrl+R saves or reviews · Esc backs out", "Customized shortcuts are shown in the command palette. Use slash commands or Ctrl shortcuts to open actions.", "", "Gator commands", ...(client.state?.commands ?? []).map(command => `/${command.name} — ${command.description}`)].join("\n")),
    "prompt-history": () => menu("Reuse a retained message", (client.state?.messages ?? []).filter(message => message.role === "You" && message.text).slice().reverse().map(message => ({ title: message.text.slice(0, 100), description: message.created_at, run: () => menu("Stage previous message text", [
      { title: "Append text to current draft", run: () => client.edit([client.draft.text, message.text].filter(Boolean).join("\n\n")) },
      { title: "Edit text before replacing the composer", run: () => prompt("Reuse message (files in current draft stay attached)", message.text, text => client.edit(text)) },
      { title: "Inspect retained message", run: () => details("Retained message", message.text) },
    ]) }))),
    "attach-file": () => dialog.replace(() => <PathPicker kind="file" choose={path => { dialog.clear(); attempt(client.attach([path]), "Importing attachment…") }} />),
    attachments: () => menu("Draft attachments", (client.draft.uploads ?? []).map(file => ({ title: file.name, run: () => menu(file.name, [{ title: "Remove from draft", run: () => client.edit(client.draft.text, client.draft.uploads?.filter(f => f.id !== file.id)) }]) }))),
    chats,
    search: () => prompt("Search this transcript", "", text => menu("Transcript matches", (client.state?.messages ?? []).map((message, index) => ({ message, index })).filter(({ message, index }) => searchableMessage(message, client.activityExpanded.has(activityKey(client.context, message, index))).toLocaleLowerCase().includes(text.toLocaleLowerCase())).map(({ message, index }) => ({ title: `${index + 1}. ${message.text.slice(0, 100)}`, description: [message.role, message.created_at].filter(Boolean).join(" · "), run: () => { client.route = "chat"; client.emit("change"); client.emit("reveal-message", index) } })))),
    "work-details": () => {
      const state = client.state, work = state?.work_details, latest = work?.latest_bundle
      menu("Conversation information", [
        { title: "Current selections and latest result", run: () => details("Selections and evidence", [`Workspace: ${state?.source || "None selected"}`, `Future work mode: ${work?.mode || "auto"}`, `Selected workspace files: ${work?.attachments?.join(", ") || "None"}`, `Private draft files: ${client.draft.uploads?.map(file => file.name).join(", ") || "None"}`, `Selected connections: ${work?.connector_ids?.join(", ") || "None"}`, `Browser for next task: ${work?.browser_session || "None"}`, "", `Latest result: ${latest?.Status || "No retained result"}`, `Sources captured for that result: ${latest?.Sources?.join(", ") || "None recorded"}`, `Connections captured for that result: ${latest?.Connections?.map(connection => `${connection.ID}: ${connection.Operations.join(", ")}`).join("; ") || "None"}`, ...(latest?.Artifacts ?? []).map(file => `${file.Path}: ${file.Valid ? "Checks passed" : "Needs review"}`), "Future selection changes do not change queued requests or historical evidence."].join("\n")) },
        { title: "Sources and supplied memory", run: () => client.navigate("/context") },
        { title: "Connections", run: () => client.navigate("/connections") },
        { title: "Choose workspace", run: () => client.navigate("/source") },
        { title: "Files and outputs", run: () => actions.files() },
        { title: "Retained output previews and proposed changes", run: () => actions.outputs() },
        { title: "Model and effort for future messages", run: models },
      ])
    },
    "cancel-update-reply": () => client.edit(client.draft.text, client.draft.uploads, null),
    about: () => menu("About Gator terminal", [{ title: "Installed version and runtime", description: "OpenCode-derived Solid/OpenTUI frontend; Gator Go execution and storage", run: async () => details("About Gator terminal", `${await client.host.version()}\nFrontend: OpenCode f669c46c9604ea3d70b8a7ed1f0c10f3a5ac1f61\nOpenTUI 0.6.8 · Solid 1.9.15 · Bun ${Bun.version}\nProfile: ${client.host.stateRoot}\nInstallation updates are explicit; see docs/TERMINAL.md. Existing profiles are retained.`) }, { title: "Release and installation information", run: () => client.emit("open-releases") }]),
  }
  const fileWorkflows = useFiles(menu, prompt)
  actions.files = fileWorkflows.files
  actions["work-activity"] = actions.activity!
  const trigger = (name: string) => { navigation.reset(); const action = actions[name]; if (action) { try { attempt(action(), name.replaceAll("-", " ") + "…") } catch (error) { client.fail(error) } } }
  const clearNavigation = () => { navigation.reset(); dialog.clear() }
  client.on("navigation", clearNavigation); onCleanup(() => client.off("navigation", clearNavigation))
  client.on("action", trigger); onCleanup(() => client.off("action", trigger))
  client.actions = actions
  return { menu, prompt, details, confirm, actions, trigger, openFile: fileWorkflows.open, resetMenus: navigation.reset }
}
