import { createEffect, onCleanup } from "solid-js"
import { randomUUID } from "node:crypto"
import { resolve } from "node:path"
import { useClient } from "../context/client"
import { useDialog } from "../ui/dialog"
import { DialogSelect } from "../ui/dialog-select"
import { DialogPrompt } from "../ui/dialog-prompt"
import { DialogConfirm } from "../ui/dialog-confirm"
import type { ChatUpdate, BackendBundle, ActionReceipt } from "@gator/application-host/contracts/protocol"
import { literal } from "./text"
import { useFiles } from "./files"
import { PathPicker } from "./path-picker"
import { Details } from "./details"
import { ActionPrompt } from "./action-prompt"
import { useData } from "../context/data"
import { activityKey, activitySummary, activityText, searchableMessage } from "./activity"
import { conversationBundles } from "./outputs"

export type Choice = { title: string; key?: string; description?: string; category?: string; disabled?: boolean; run(): unknown }
export function useActions() {
  const client = useClient(), dialog = useDialog(), data = useData()
  let previousContext = client.context
  createEffect(() => {
    data.snapshot
    const context = client.context
    if (context !== previousContext) { previousContext = context; dialog.clear() }
  })
  const attempt = (operation: unknown) => { if (operation instanceof Promise) void operation.catch(e => client.fail(e)) }
  const menu = (title: string, choices: Choice[] | (() => Choice[]), showDisabled = false, back?: () => void) => dialog.replace(() => <DialogSelect title={title} showDisabled={showDisabled} onCancel={back} options={(typeof choices === "function" ? choices() : choices).map((item, index) => ({
    title: literal(item.title), description: literal(item.description), category: literal(item.category), disabled: item.disabled, value: item.key ?? index,
    onSelect: () => { dialog.clear(); try { attempt(item.run()) } catch (error) { client.fail(error) } },
  }))} />)
  const prompt = (title: string, value: string, save: (text: string) => unknown, description?: string) => { const token = Symbol(title); dialog.replace(() => <ActionPrompt token={token} title={title} value={value} save={save} description={description} />, undefined, { key: token }) }
  const details = (title: string, text: string) => dialog.replace(() => <Details title={title} text={text} />, undefined, { size: "xlarge" })
  const confirm = (title: string, message: string, run: () => unknown) => dialog.replace(() => <DialogConfirm title={title} message={literal(message)} onConfirm={() => { try { attempt(run()) } catch (error) { client.fail(error) } }} />)
  function outputs() {
    const context = client.context
    menu("Retained outputs and proposed changes", () => conversationBundles(data.state).flatMap((bundle, group) => [
      ...(bundle.Artifacts ?? []).map(file => ({ title: file.Path, description: `${file.Valid ? "Checks passed" : "Needs review"} · ${bundle.Status} run · Result ${group + 1}`, run: () => menu(file.Path, [
        { title: "Inspect retained preview and checks", run: () => details(file.Path, [file.Valid ? "Checks passed" : "Needs review", `Result ${group + 1} · ${bundle.RunID || bundle.Path}`, `Retained from a ${bundle.Status} run.`, bundle.FinishedAt || "", `${file.MediaType} · ${file.Bytes ?? "Unknown"} bytes`, file.Preview || "No text preview is available for this file.", file.Truncated ? "Showing a shortened preview." : "", "File checks do not confirm external delivery.", `Retained location: ${bundle.Path}`].filter(Boolean).join("\n\n")) },
        { title: "Open complete file / edit working version", run: () => fileWorkflows.open({ kind: "output", bundle: bundle.Path, path: file.Path }, undefined, context) },
        ...((client.state?.messages.findIndex(message => message.bundle?.Path === bundle.Path) ?? -1) >= 0 ? [{ title: "Show origin in chat", run: () => { const index = client.state?.messages.findIndex(message => message.bundle?.Path === bundle.Path) ?? -1; if (client.context !== context || index < 0) throw new Error("Return to the original conversation to inspect this result."); client.route = "chat"; client.emit("change"); client.emit("reveal-message", index) } }] : []),
      ]) })),
      ...(bundle.Candidates ?? []).map(candidate => ({ title: `Change ${candidate.ID}`, description: `Proposal · ${candidate.Status || "unreported checks"} · Result ${group + 1}`, run: () => details(`Change ${candidate.ID}`, ["Proposed changes; this view does not apply them.", `Checks: ${candidate.Status || "Not reported"}`, `Run: ${bundle.RunID || bundle.Path} · ${bundle.Status}`, ...candidate.ChangedPaths, `Retained location: ${bundle.Path}`, candidate.PatchPath || ""].join("\n\n")) })),
    ]))
  }
  async function drafts() {
    const saved = await client.host.drafts.request({ action: "list" })
    menu("Unsent drafts", [
      { title: saved.enabled ? "Disable draft retention" : "Enable draft retention", run: () => confirm("Change draft retention?", saved.enabled ? "All retained composer drafts will be cleared from disk. Current unsent input remains editable until this session closes." : "Keep unsent drafts on this device for up to seven days.", async () => { await client.host.drafts.request({ action: "configure", enabled: !saved.enabled }); client.emit("change") }) },
      ...saved.drafts.map(draft => ({ title: draft.title || draft.context, description: draft.text.slice(0, 120) || `${draft.uploads?.length ?? 0} attachments`, run: () => menu("Saved draft", [
        { title: "Recover in original chat", run: async () => {
          if (draft.context.startsWith("conversation:")) await client.host.sessions.open({ conversation_id: draft.context.slice(13) })
          else if (client.context !== draft.context) throw new Error("Open this workspace's original chat to recover its draft.")
          client.drafts.set(draft.context, draft); client.route = "chat"; client.emit("change")
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
  function updates() {
    const state = client.state
    if (!state?.conversation_id) return
    const conversation_id = state.conversation_id
    const updateAction = async (update: ChatUpdate, action: "read" | "resolve" | "reopen" | "delivered" | "older" | "newer") => {
      await client.send({ type: "update", update: { conversation_id, id: update.id, version: update.version, action } })
      if (client.state?.chat_updates_issue) throw new Error(client.state.chat_updates_issue)
    }
    menu("This chat's updates", [
      ...(state.chat_updates ?? []).map(update => ({ title: update.title, description: `${update.status} · ${update.read ? "read" : "unread"} · ${update.resolved ? "done" : "open"}`, run: () => menu(update.title, [
        { title: "Inspect update", description: update.summary, run: () => details(update.title, `${update.summary}\n${update.evidence?.join("\n") ?? ""}\n${update.next_step ?? ""}`) },
        { title: "Mark read", disabled: update.read, run: () => updateAction(update, "read") },
        { title: update.resolved ? "Reopen" : "Mark done", run: () => updateAction(update, update.resolved ? "reopen" : "resolve") },
        { title: "Reply in original chat", run: async () => { client.route = "chat"; await client.edit(client.draft.text, client.draft.uploads, update.id) } },
        ...(["useful", "noise", "wrong", "clear"] as const).map(rating => ({ title: `Feedback: ${rating}`, description: "Saved for review; this does not change your checks automatically.", disabled: !update.can_rate || !!client.updateFeedback.get(client.context)?.saving, run: () => client.rateUpdate(update, rating) })),
      ]) })),
      ...[state.updates_have_older ? "older" : undefined, state.updates_have_newer ? "newer" : undefined].filter((direction): direction is "older" | "newer" => !!direction).map(direction => ({ title: `${direction} updates`, run: () => {
        const update = direction === "older" ? state.chat_updates?.[0] : state.chat_updates?.at(-1); if (update) return updateAction(update, direction)
      } })),
    ])
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
  function receipts() {
    const state = client.state, conversation_id = state?.conversation_id
    if (!conversation_id) return
    const bundles = (state.messages ?? []).flatMap(m => m.bundle ? [m.bundle] : [])
    const inspect = async (bundle: BackendBundle, receipt: ActionReceipt, action: "inspect" | "check_sent") => {
      if (!bundle.RunID || !bundle.ManifestSHA256 || !receipt.CorrelationID) throw new Error("This retained receipt has no verifiable manifest for a Sent check. Inspect the connected service before retrying any external action.")
      const request_id = randomUUID()
      const result = await client.result({ type: "receipt", receipt: { conversation_id, run_id: bundle.RunID, action_id: receipt.ID, correlation_id: receipt.CorrelationID, manifest_sha256: bundle.ManifestSHA256, action, request_id } }, request_id, s => s.receipt_result)
      const meaning = result.check?.status === "found" ? "Found in Sent. This confirms a matching Sent message, not recipient delivery." : result.check?.status === "not_checked" ? "No saved Sent check. Inspecting this record does not contact the provider." : "Sent outcome remains unconfirmed. Verify the connected service before considering a new send."
      details("Saved receipt check", meaning + "\n\n" + JSON.stringify(result.check, null, 2))
    }
    const rows = bundles.flatMap(bundle => (bundle.Actions ?? []).map(receipt => ({ bundle, receipt })))
    rows.push(...(state.action_recoveries ?? []).map(item => ({ bundle: { Path: "", Status: item.receipt.Status, Artifacts: null, Candidates: null, RunID: item.run_id }, receipt: item.receipt })))
    menu("Connected request receipts", rows.map(({ bundle, receipt }) => ({ title: `${receipt.Operation} · ${receipt.Status}`, description: receipt.Recipient || receipt.Target, run: () => menu("Receipt: " + receipt.Status, [
      { title: "Inspect recorded request", run: () => details("Request receipt", JSON.stringify(receipt, null, 2)) },
      { title: "Inspect saved Sent check", disabled: !bundle.ManifestSHA256, run: () => inspect(bundle, receipt, "inspect") },
      { title: "Check Sent (read only)", disabled: receipt.Operation !== "gmail_send" || receipt.Status !== "unknown" || !bundle.ManifestSHA256, run: () => inspect(bundle, receipt, "check_sent") },
    ]) })))
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
    drafts, queue, updates, memories, receipts, models,
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
    help: () => details("Gator terminal help", ["Ctrl+P commands · Ctrl+B navigation rail · Ctrl+N new independent chat · Ctrl+Q quit", "Enter sends or queues · Shift+Enter/Ctrl+J new line · Ctrl+G guides the live task · Ctrl+C stops work", "Ctrl+F searches the transcript or loaded panel · PgUp/PgDn scroll or move selection", "Ctrl+Y reviews the live approval · Ctrl+D declines · Paste never sends or approves", "/following tracks goals/checks/updates · /saved opens knowledge/history/files · /settings configures Gator", "/chats switches chats · /queue manages retained messages · /drafts recovers drafts · /prompt-history reuses sent text", "/files opens full files · Ctrl+S saves · F3 edits Office/cells · F2 versions/export/Ask Gator", "/voice explicitly records/transcribes or configures new-reply playback · /attach-file selects a private input", "Forms: Enter edits a field · Ctrl+R or displayed review key reviews · F2 falls back for Ctrl+Enter · Esc backs out", "Customized shortcuts are shown in the command palette. Use slash commands and the displayed function-key fallbacks where terminal modifiers overlap.", "", "Gator commands", ...(client.state?.commands ?? []).map(command => `/${command.name} — ${command.description}`)].join("\n")),
    "prompt-history": () => menu("Reuse a retained message", (client.state?.messages ?? []).filter(message => message.role === "You" && message.text).slice().reverse().map(message => ({ title: message.text.slice(0, 100), description: message.created_at, run: () => menu("Stage previous message text", [
      { title: "Append text to current draft", run: () => client.edit([client.draft.text, message.text].filter(Boolean).join("\n\n")) },
      { title: "Edit text before replacing the composer", run: () => prompt("Reuse message (files in current draft stay attached)", message.text, text => client.edit(text)) },
      { title: "Inspect retained message", run: () => details("Retained message", message.text) },
    ]) }))),
    "attach-file": () => dialog.replace(() => <PathPicker kind="file" choose={path => { dialog.clear(); attempt(client.attach([path])) }} />),
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
        { title: "Retained output previews and proposed changes", run: outputs },
        { title: "Model and effort for future messages", run: models },
      ])
    },
    outputs,
    "cancel-update-reply": () => client.edit(client.draft.text, client.draft.uploads, null),
    about: () => menu("About Gator terminal", [{ title: "Installed version and runtime", description: "OpenCode-derived Solid/OpenTUI frontend; Gator Go execution and storage", run: async () => details("About Gator terminal", `${await client.host.version()}\nFrontend: OpenCode f669c46c9604ea3d70b8a7ed1f0c10f3a5ac1f61\nOpenTUI 0.6.8 · Solid 1.9.15 · Bun ${Bun.version}\nProfile: ${client.host.stateRoot}\nInstallation updates are explicit; see docs/TERMINAL.md. Existing profiles are retained.`) }, { title: "Release and installation information", run: () => client.emit("open-releases") }]),
  }
  const fileWorkflows = useFiles(menu, prompt)
  actions.files = fileWorkflows.files
  const trigger = (name: string) => { const action = actions[name]; if (action) { try { attempt(action()) } catch (error) { client.fail(error) } } }
  client.on("action", trigger); onCleanup(() => client.off("action", trigger))
  return { menu, prompt, confirm, actions, trigger }
}
