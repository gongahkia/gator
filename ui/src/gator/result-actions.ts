import { onCleanup } from "solid-js"
import { randomUUID } from "node:crypto"
import type { BackendBundle, BackendMessage, ChatUpdate, DialogueMessage, DialoguePeer } from "@gator/application-host/contracts/protocol"
import { useClient } from "../context/client"
import { useClipboard } from "../context/clipboard"
import { useToast } from "../ui/toast"
import type { useActions } from "./actions"
import { conversationBundles } from "./outputs"
import { displayDate } from "./display"
import { useReadView } from "./read-view"
import { useReceiptActions } from "./receipt-actions"

export type ResultTarget =
  | { kind: "message"; context: string; index: number; message: BackendMessage }
  | { kind: "update"; context: string; update: ChatUpdate }
  | { kind: "file"; context: string; bundle: BackendBundle; file: NonNullable<BackendBundle["Artifacts"]>[number] }
  | { kind: "dialogue"; context: string; turn: DialogueMessage; peer?: DialoguePeer }

// Every callback retains its original conversation and immutable result. An
// equal-looking item, newer snapshot or switched chat never supplies authority.
export function useResultActions(controls: ReturnType<typeof useActions>) {
  const client = useClient(), clipboard = useClipboard(), toast = useToast(), readView = useReadView()
  const { menu, details, actions, openFile } = controls
  const receiptMenu = useReceiptActions(controls)
  const contextIsCurrent = (context: string) => {
    if (context !== client.context) throw new Error("Return to the original conversation and select this item again.")
  }
  function bundleIsCurrent(bundle: BackendBundle, context: string) {
    contextIsCurrent(context)
    if (!conversationBundles(client.state).some(current => current.Path === bundle.Path && current.RunID === bundle.RunID && current.ManifestSHA256 === bundle.ManifestSHA256)) throw new Error("This retained result changed or is unavailable. Select it again.")
  }
  function updateIsCurrent(update: ChatUpdate, context: string) {
    contextIsCurrent(context)
    if (!client.state?.chat_updates?.some(current => current.id === update.id && current.version === update.version)) throw new Error("This update changed or is unavailable. Select it again.")
  }
  const origin = (bundle: BackendBundle) => [
    `Run: ${bundle.RunID || "Not recorded"} · ${bundle.Status}`,
    bundle.FinishedAt ? `Completed: ${bundle.FinishedAt}` : "",
    `Sources: ${bundle.Sources?.join(", ") || "None recorded"}`,
    `Accounts and operations: ${bundle.Connections?.map(account => `${account.ID}: ${account.Operations.join(", ")}`).join("; ") || "None recorded"}`,
    bundle.InheritedOrigin ? `Original provenance: ${JSON.stringify(bundle.InheritedOrigin)}` : "",
    `Manifest: ${bundle.ManifestSHA256 || "Unavailable"}`,
    `Retained location: ${bundle.Path}`,
  ].filter(Boolean).join("\n")
  const attempt = (operation: () => unknown) => {
    try { const value = operation(); if (value instanceof Promise) void client.track("Opening item actions…", value).catch(error => client.fail(error)) }
    catch (error) { client.fail(error) }
  }
  async function sources(bundle: BackendBundle, context: string, back?: () => void) {
    bundleIsCurrent(bundle, context)
    const run = bundle.RunID!
    await readView({ title: "Retained sources", context, description: "Reading sealed source references on this device.", back, read: () => client.host.sourceCatalog({ context, run }), show: catalog => {
    bundleIsCurrent(bundle, context)
    menu("Retained sources", catalog.references.map(reference => ({ key: reference.id, title: reference.label, description: `${reference.kind} · ${reference.locator}`, run: () => {
      bundleIsCurrent(bundle, context)
      return readView({ title: reference.label, context, description: `Reading retained evidence: ${reference.locator}`, back: () => sources(bundle, context, back), read: () => client.host.sourcePreview({ context, run, path: "evidence:" + reference.id }), show: source => {
      bundleIsCurrent(bundle, context)
      details(reference.label, [`Source: ${source.locator}`, `Evidence: ${source.id} · ${source.sha256}`, source.notice, `Captured: ${source.captured_at || "Unknown"}`, source.reads.length ? "Observed reads: " + source.reads.map(read => `${read.start}–${read.end}${read.truncated ? " (partial)" : ""}`).join(", ") : "Exact read ranges unavailable.", source.truncated ? "Showing part of the retained source." : "", source.text.split("\n").map((line, index) => `${source.firstLine + index}  ${line}`).join("\n")].filter(Boolean).join("\n\n"), () => attempt(() => sources(bundle, context, back)))
      } })
    } })), false, back)
    if (!catalog.references.length) details("Retained sources", "This result has no supported retained source references. No current account or workspace was queried.", back)
    } })
  }
  async function change(bundle: BackendBundle, candidate: NonNullable<BackendBundle["Candidates"]>[number], context: string, back?: () => void) {
    bundleIsCurrent(bundle, context)
    if (!bundle.RunID || !bundle.ManifestSHA256) {
      details("Code changes", ["Proposed changes; this view does not apply them.", origin(bundle), `Checks: ${candidate.Status || "Not reported"}`, ...candidate.ChangedPaths, candidate.PatchPath, "This older proposal has no verifiable sealed patch preview."].filter(Boolean).join("\n\n"), back)
      return
    }
    await readView({ title: "Code changes", context, description: "Reading the retained patch on this device. This does not apply changes.", back, read: () => client.host.codePreview({ context, run: bundle.RunID!, candidate: candidate.ID }), show: result => {
    bundleIsCurrent(bundle, context)
    details("Code changes", [origin(bundle), result.status, ...result.verification.map(check => `${check.passed ? "Passed" : "Failed"}: ${check.argv.join(" ")}`), "Inspecting this retained patch does not apply it.", result.truncated ? "Showing part of the retained patch." : "", result.patch].filter(Boolean).join("\n\n"), back)
    } })
  }
  function fileMenu(bundle: BackendBundle, file: NonNullable<BackendBundle["Artifacts"]>[number], context: string, back?: () => void) {
    const check = () => {
      bundleIsCurrent(bundle, context)
      if (!conversationBundles(client.state).find(current => current.Path === bundle.Path)?.Artifacts?.some(current => current.Path === file.Path && current.SHA256 === file.SHA256)) throw new Error("This file changed or is unavailable. Select its current version.")
    }
    check()
    const again = () => attempt(() => fileMenu(bundle, file, context, back))
    menu(file.Path, [
      { title: "Open complete file / edit working version", description: "Keep the sealed original; save edits as a working version", run: () => { check(); return openFile({ kind: "output", bundle: bundle.Path, path: file.Path }, undefined, context, again) } },
      { title: "Inspect retained preview and checks", run: () => { check(); details(file.Path, [file.Valid ? "Checks passed" : "Needs review", `Retained from a ${bundle.Status} run.`, origin(bundle), `${file.MediaType} · ${file.Bytes ?? "Unknown"} bytes`, `File hash: ${file.SHA256 || "Unavailable"}`, file.Preview || "No text preview is available for this file.", file.Truncated ? "Showing a shortened preview." : "", "File checks do not confirm external delivery."].filter(Boolean).join("\n\n"), again) } },
      { title: "Inspect source evidence", disabled: !bundle.RunID || !bundle.ManifestSHA256, run: () => { check(); return sources(bundle, context, again) } },
      { title: "Show origin in chat", disabled: !client.state?.messages.some(message => message.bundle?.Path === bundle.Path), run: () => {
        check(); const index = client.state!.messages.findIndex(message => message.bundle?.Path === bundle.Path)
        if (index < 0) throw new Error("The original message is unavailable.")
        client.route = "chat"; client.emit("change"); client.emit("reveal-message", index)
      } },
    ], true, back)
  }
  function updateMenu(update: ChatUpdate, context: string, back?: () => void) {
    updateIsCurrent(update, context)
    const again = () => attempt(() => updateMenu(update, context, back))
    const receipt = async (action: "read" | "resolve" | "reopen") => {
      updateIsCurrent(update, context)
      const request_id = randomUUID(), conversation_id = client.state!.conversation_id!
      await client.result({ type: "update", update: { conversation_id, id: update.id, version: update.version, action, request_id } }, request_id, state => state.chat_update_result)
      contextIsCurrent(context)
      toast.show({ message: action === "resolve" ? "Update marked done." : action === "reopen" ? "Update reopened." : "Update marked read.", variant: "success" })
    }
    menu(update.title, [
      { title: "Inspect update", description: update.summary, run: () => { updateIsCurrent(update, context); details(update.title, [update.summary, `Status: ${update.status}`, `Original conversation: ${context}`, `Update: ${update.id} · version ${update.version}`, update.job_id ? `Check: ${update.job_id}` : "", update.goal_id ? `Goal: ${update.goal_id}` : "", ...(update.evidence ?? []), update.next_step].filter(Boolean).join("\n\n"), again) } },
      { title: "Mark read", disabled: update.read, run: () => receipt("read") },
      { title: update.resolved ? "Reopen" : "Mark done", run: () => receipt(update.resolved ? "reopen" : "resolve") },
      { title: "Reply in original chat", description: "Add reply context to your editable draft; sending remains explicit", run: async () => { updateIsCurrent(update, context); client.route = "chat"; await client.edit(client.draft.text, client.draft.uploads, update.id) } },
      ...(["useful", "noise", "wrong", "clear"] as const).map(rating => ({ title: `Feedback: ${rating}`, description: "Saved for review; this does not change your checks automatically.", disabled: !update.can_rate || !!client.updateFeedback.get(context)?.saving, run: () => {
        updateIsCurrent(update, context)
        if (client.state?.chat_updates?.find(current => current.id === update.id)?.rating_version !== update.rating_version) throw new Error("This feedback changed. Select the update again.")
        return client.rateUpdate(update, rating)
      } })),
    ], true, back)
  }
  const messageIdentity = (message: BackendMessage) => JSON.stringify([message.role, message.created_at, message.text, message.result, message.revision_id, message.run_id, message.reply_to_update_id, message.input_ids, message.bundle])
  function messageMenu(target: Extract<ResultTarget, { kind: "message" }>, back?: () => void) {
    const { context, message, index } = target, identity = messageIdentity(message)
    // Go retains the answer and its deliverables as separate messages. Join
    // only their exact run identities, never their position or similar text.
    const bundle = message.bundle ?? (message.run_id ? conversationBundles(client.state).find(bundle => bundle.RunID === message.run_id) : undefined)
    const check = () => {
      contextIsCurrent(context)
      if (!client.state?.messages[index] || messageIdentity(client.state.messages[index]!) !== identity) throw new Error("This result changed or is unavailable. Select it again.")
    }
    check()
    const again = () => attempt(() => messageMenu(target, back))
    const text = [message.text, message.result ? JSON.stringify(message.result, null, 2) : ""].filter(Boolean).join("\n\n")
    menu(`${message.role} · ${displayDate(message.created_at) || "Retained message"}`, [
      { title: "Inspect complete result", description: message.text.slice(0, 100), run: () => { check(); details("Complete result", [text, bundle ? origin(bundle) : "", `Original conversation: ${context}`, message.revision_id ? `Revision: ${message.revision_id}` : ""].filter(Boolean).join("\n\n"), again) } },
      { title: "Copy result text", disabled: !text, run: async () => { check(); await clipboard.write(text); toast.show({ message: "Result copied.", variant: "success" }) } },
      ...((message.run_id || bundle?.RunID) ? [{ title: "Inspect supplied memory and context", description: "See retained excerpts and provenance; correct or forget saved knowledge", run: () => { check(); return client.navigate("/context " + (message.run_id || bundle!.RunID)) } }] : []),
      ...(bundle?.Artifacts ?? []).map(file => ({ title: `File: ${file.Path}`, description: file.MediaType, run: () => { check(); fileMenu(bundle!, file, context, again) } })),
      ...((bundle?.RunID && bundle.ManifestSHA256) ? [{ title: "Inspect source evidence", run: () => { check(); return sources(bundle!, context, again) } }] : []),
      ...(bundle?.Candidates ?? []).map(candidate => ({ title: `Change ${candidate.ID}`, description: candidate.Status, run: () => { check(); return change(bundle!, candidate, context, again) } })),
      ...(bundle?.Actions ?? []).map(receipt => ({ title: `Request: ${receipt.Operation}`, description: `${receipt.ConnectorID} · ${receipt.Status} · ${receipt.Recipient || receipt.Target}`, run: () => { check(); receiptMenu(bundle!, receipt, context, again) } })),
      ...(message.input_ids ?? []).map((id, i) => ({ title: `Input: ${message.inputs?.[i] || id}`, run: () => { check(); return openFile({ kind: "upload", id }, undefined, context) } })),
    ], true, back)
  }
  function dialogueMenu(target: Extract<ResultTarget, { kind: "dialogue" }>, back?: () => void) {
    const { context, turn, peer } = target, identity = JSON.stringify(turn)
    const check = () => {
      contextIsCurrent(context)
      const current = client.state?.dialogue?.find(current => current.id === turn.id)
      if (!current || JSON.stringify(current) !== identity) throw new Error("This conversation item changed or is unavailable. Select it again.")
      if (peer && (peer.conversation_id !== turn.task_chat || peer.turn_id !== turn.id || !client.state?.dialogue_peers?.some(current => JSON.stringify(current) === JSON.stringify(peer)))) throw new Error("The original task result changed. Select it again.")
    }
    check()
    const again = () => attempt(() => dialogueMenu(target, back))
    const text = [turn.response, peer?.summary].filter(Boolean).join("\n\n")
    menu("Conversation item actions", [
      { title: "Inspect retained reply and task", description: turn.text, run: () => { check(); details("Conversation item", [turn.text, text, turn.issue, `Original conversation: ${context}`, `Message: ${turn.id} · ${turn.updated_at}`, `State: ${peer?.state || turn.state}`, turn.task_chat ? `Original task: ${turn.task_chat}` : "", peer?.run_id ? `Run: ${peer.run_id}` : "", turn.task_chat ? "The summary belongs to the original task. Open that task to inspect its complete results, reviews and outputs." : ""].filter(Boolean).join("\n\n"), again) } },
      { title: "Copy retained reply", disabled: !text, run: async () => { check(); await clipboard.write(text); toast.show({ message: "Reply copied.", variant: "success" }) } },
      ...(turn.task_chat ? [{ title: "Open original task, reviews and outputs", description: peer?.state || "Outcome unconfirmed", run: () => { check(); return client.openSession(turn.task_chat!) } }] : []),
      ...(turn.input_ids ?? []).map(id => ({ title: `Input: ${id.split("/").at(-1) || id}`, run: () => { check(); return openFile({ kind: "upload", id }, undefined, context, again) } })),
    ], true, back)
  }
  function show(target?: ResultTarget) {
    if (!target) return list()
    if (target.kind === "message") return messageMenu(target, list)
    if (target.kind === "update") return updateMenu(target.update, target.context, list)
    if (target.kind === "dialogue") return dialogueMenu(target, list)
    return fileMenu(target.bundle, target.file, target.context, list)
  }
  function list() {
    const context = client.context
    menu("Actions in this conversation", [
      ...(client.state?.chat_updates ?? []).toReversed().map(update => ({ key: `update:${update.id}`, title: update.title, category: "Updates", description: `${update.status} · ${update.resolved ? "done" : "open"}`, run: () => updateMenu(update, context, list) })),
      ...(client.state?.dialogue ?? []).toReversed().map(turn => ({ key: `dialogue:${turn.id}`, title: turn.text, category: "Conversation and delegated work", description: turn.response || turn.state, run: () => dialogueMenu({ kind: "dialogue", context, turn, peer: client.state?.dialogue_peers?.find(peer => peer.conversation_id === turn.task_chat && peer.turn_id === turn.id) }, list) })),
      ...(client.state?.messages ?? []).map((message, index) => ({ message, index })).filter(({ message }) => message.text || message.result || message.bundle || message.input_ids?.length).toReversed().map(({ message, index }) => ({ key: `message:${index}`, title: `${message.role}: ${message.text.slice(0, 90) || message.result?.title || "Retained result"}`, category: "Messages and results", description: message.created_at, run: () => messageMenu({ kind: "message", context, message, index }, list) })),
    ])
  }
  actions.actions = list
  actions.outputs = () => {
    const context = client.context
    menu("Retained outputs and proposed changes", conversationBundles(client.state).flatMap(bundle => [
      ...(bundle.Artifacts ?? []).map(file => ({ title: file.Path, description: `${file.Valid ? "Checks passed" : "Needs review"} · ${bundle.Status} run`, run: () => fileMenu(bundle, file, context, actions.outputs) })),
      ...(bundle.Candidates ?? []).map(candidate => ({ title: `Change ${candidate.ID}`, description: `Proposal · ${candidate.Status}`, run: () => change(bundle, candidate, context, actions.outputs) })),
    ]))
  }
  actions.updates = () => {
    const state = client.state, context = client.context
    menu("This chat's updates", [
      ...(state?.chat_updates ?? []).map(update => ({ title: update.title, description: `${update.status} · ${update.read ? "read" : "unread"} · ${update.resolved ? "done" : "open"}`, run: () => updateMenu(update, context, actions.updates) })),
      ...[state?.updates_have_older ? "older" : undefined, state?.updates_have_newer ? "newer" : undefined].filter((direction): direction is "older" | "newer" => !!direction).map(direction => ({ title: `${direction} updates`, run: async () => {
        contextIsCurrent(context)
        const update = direction === "older" ? state?.chat_updates?.[0] : state?.chat_updates?.at(-1)
        if (!update) throw new Error("Updates changed. Reopen the current page.")
        const request_id = randomUUID()
        await client.result({ type: "update", update: { conversation_id: state!.conversation_id!, id: update.id, version: update.version, action: direction, request_id } }, request_id, value => value.chat_update_result)
      } })),
    ])
  }
  actions.evidence = () => {
    const context = client.context
    menu("Sources retained for an answer", conversationBundles(client.state).filter(bundle => bundle.RunID && bundle.ManifestSHA256).map(bundle => ({ title: client.state?.messages.find(message => message.run_id === bundle.RunID && message.text)?.text.slice(0, 100) || bundle.RunID!, run: () => sources(bundle, context, actions.evidence) })))
  }
  actions.changes = () => {
    const context = client.context
    menu("Retained code changes", conversationBundles(client.state).flatMap(bundle => (bundle.Candidates ?? []).map(candidate => ({ title: candidate.ID, description: `${candidate.Status} · ${candidate.ChangedPaths.join(", ")}`, run: () => change(bundle, candidate, context, actions.changes) }))))
  }
  const open = (target?: ResultTarget) => { controls.resetMenus(); attempt(() => show(target)) }
  client.on("item-actions", open); onCleanup(() => client.off("item-actions", open))
}
