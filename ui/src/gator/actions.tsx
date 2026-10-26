import { onCleanup } from "solid-js"
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

export type Choice = { title: string; description?: string; disabled?: boolean; run(): unknown }
export function useActions() {
  const client = useClient(), dialog = useDialog()
  const attempt = (operation: unknown) => { if (operation instanceof Promise) void operation.catch(e => client.fail(e)) }
  const menu = (title: string, choices: Choice[]) => dialog.replace(() => <DialogSelect title={title} options={choices.map((item, index) => ({
    title: literal(item.title), description: literal(item.description), disabled: item.disabled, value: index,
    onSelect: () => { dialog.clear(); try { attempt(item.run()) } catch (error) { client.fail(error) } },
  }))} />)
  const prompt = (title: string, value: string, save: (text: string) => unknown, description?: string) => dialog.replace(() => <DialogPrompt title={title} value={value} subtitle={description} onConfirm={text => { dialog.clear(); try { attempt(save(text)) } catch (error) { client.fail(error) } }} />)
  const confirm = (title: string, message: string, run: () => unknown) => dialog.replace(() => <DialogConfirm title={title} message={literal(message)} onConfirm={() => { try { attempt(run()) } catch (error) { client.fail(error) } }} />)
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
        await client.result({ type: "queue", queue: { id: item.id, request_id, action, text } }, request_id, s => s.queue_result)
      }
      menu("Queue message", [
        { title: "Edit message", run: () => prompt("Edit queued message", item.text, text => act("edit", text)) },
        { title: "Move earlier", run: () => act("earlier") }, { title: "Move later", run: () => act("later") },
        { title: "Remove", run: () => confirm("Remove queued message?", item.text, () => act("remove")) },
        { title: "Run / resume", run: () => confirm("Run this retained message?", item.text, () => act("run")) },
        { title: "Guide active work", disabled: !client.state?.running, run: () => act("guide") },
      ])
    } })))
  }
  function updates() {
    const state = client.state
    if (!state?.conversation_id) return
    const conversation_id = state.conversation_id
    const updateAction = (update: ChatUpdate, action: "read" | "resolve" | "reopen" | "delivered" | "older" | "newer") => client.send({ type: "update", update: { conversation_id, id: update.id, version: update.version, action } })
    menu("This chat's updates", [
      ...(state.chat_updates ?? []).map(update => ({ title: update.title, description: `${update.status} · ${update.read ? "read" : "unread"} · ${update.resolved ? "done" : "open"}`, run: () => menu(update.title, [
        { title: update.summary, description: update.evidence?.join(" · "), run: () => prompt("Update details (read only)", `${update.summary}\n${update.evidence?.join("\n") ?? ""}`, () => {}) },
        { title: "Mark read", disabled: update.read, run: () => updateAction(update, "read") },
        { title: update.resolved ? "Reopen" : "Mark done", run: () => updateAction(update, update.resolved ? "reopen" : "resolve") },
        { title: "Reply in original chat", run: async () => { client.route = "chat"; await client.edit(client.draft.text, client.draft.uploads, update.id) } },
        ...(["useful", "noise", "wrong", "clear"] as const).map(rating => ({ title: `Feedback: ${rating}`, disabled: !update.can_rate, run: async () => {
          const request_id = randomUUID(); await client.result({ type: "update", update: { conversation_id, id: update.id, version: update.version, rating_version: update.rating_version, action: "rate", rating, request_id } }, request_id, s => s.chat_update_result)
        } })),
      ]) })),
      ...[state.updates_have_older ? "older" : undefined, state.updates_have_newer ? "newer" : undefined].filter((direction): direction is "older" | "newer" => !!direction).map(direction => ({ title: `${direction} updates`, run: () => {
        const update = state.chat_updates?.[0]; if (update) return updateAction(update, direction)
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
        { title: "Inspect source / quote", description: memory.quote || memory.run_id, run: () => prompt("Memory provenance (read only)", `${memory.scope}\n${memory.quote ?? ""}\n${memory.text}`, () => {}) },
        { title: "Correct", run: () => prompt("Correct saved memory", memory.text, text => act("correct", text)) },
        { title: "Forget", run: () => confirm("Forget this memory?", memory.text, () => act("forget")) },
        { title: "Undo correction", disabled: !memory.can_undo, run: () => act("undo") },
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
      prompt("Saved receipt check", JSON.stringify(result.check, null, 2), () => {})
    }
    const rows = bundles.flatMap(bundle => (bundle.Actions ?? []).map(receipt => ({ bundle, receipt })))
    rows.push(...(state.action_recoveries ?? []).map(item => ({ bundle: { Path: "", Status: item.receipt.Status, Artifacts: null, Candidates: null, RunID: item.run_id }, receipt: item.receipt })))
    menu("Connected request receipts", rows.map(({ bundle, receipt }) => ({ title: `${receipt.Operation} · ${receipt.Status}`, description: receipt.Recipient || receipt.Target, run: () => menu("Receipt: " + receipt.Status, [
      { title: "Inspect recorded request", run: () => prompt("Request receipt (read only)", JSON.stringify(receipt, null, 2), () => {}) },
      { title: "Inspect saved Sent check", disabled: !bundle.ManifestSHA256, run: () => inspect(bundle, receipt, "inspect") },
      { title: "Check Sent (read only)", disabled: receipt.Operation !== "gmail_send" || receipt.Status !== "unknown" || !bundle.ManifestSHA256, run: () => inspect(bundle, receipt, "check_sent") },
    ]) })))
  }
  function models() {
    const composer = client.state?.composer
    if (!composer) return
    menu("Model and effort for future messages", [
      ...composer.models.map(model => ({ title: model.label, description: model.unavailable, disabled: !!composer.disabled || !!model.unavailable, run: async () => {
        const request_id = randomUUID(); await client.result({ type: "composer", composer: { action: "model", id: composer.id, request_id, provider: model.provider, model: model.model } }, request_id, s => s.composer_result)
      } })),
      ...(["low", "standard", "high"] as const).map(effort => ({ title: `Effort: ${effort}`, disabled: !!composer.disabled, run: async () => {
        const request_id = randomUUID(); await client.result({ type: "composer", composer: { action: "effort", id: composer.id, request_id, effort } }, request_id, s => s.composer_result)
      } })),
      { title: "Configure providers and local models", run: () => client.navigate("/model") },
    ])
  }
  const actions: Record<string, () => unknown> = {
    drafts, queue, updates, memories, receipts, models,
    "attach-file": () => dialog.replace(() => <PathPicker kind="file" choose={path => { dialog.clear(); attempt(client.attach([path])) }} />),
    attachments: () => menu("Draft attachments", (client.draft.uploads ?? []).map(file => ({ title: file.name, run: () => menu(file.name, [{ title: "Remove from draft", run: () => client.edit(client.draft.text, client.draft.uploads?.filter(f => f.id !== file.id)) }]) }))),
    chats: () => menu("Open and recent chats", [...client.snapshot.chatActivity.map(chat => ({ title: chat.title, description: chat.status, run: () => client.host.openSession(chat.id) })), ...(client.state?.recent_conversations ?? []).map(chat => ({ title: chat.title, description: chat.source, run: () => client.send({ type: "conversation", conversation: { id: chat.id, list_id: client.state?.recent_id ?? "" } }) })), { title: "Search all retained conversations", run: () => client.navigate("/conversations") }]),
    search: () => prompt("Search this transcript", "", text => menu("Transcript matches", (client.state?.messages ?? []).map((message, index) => ({ message, index })).filter(({ message }) => message.text.toLocaleLowerCase().includes(text.toLocaleLowerCase())).map(({ message, index }) => ({ title: `${index + 1}. ${message.text.slice(0, 100)}`, description: message.role, run: () => { client.emit("reveal-message", index); prompt("Message (read only)", message.text, () => {}) } })))),
    about: () => menu("About Gator terminal", [{ title: "Version: development migration", description: "OpenCode-derived Solid/OpenTUI frontend; Gator Go execution and storage", run: () => {} }, { title: "Release and installation information", run: () => client.emit("open-releases") }]),
  }
  const fileWorkflows = useFiles(menu, prompt)
  actions.files = fileWorkflows.files
  const trigger = (name: string) => { const action = actions[name]; if (action) { try { attempt(action()) } catch (error) { client.fail(error) } } }
  client.on("action", trigger); onCleanup(() => client.off("action", trigger))
  return { menu, prompt, confirm, actions, trigger }
}
