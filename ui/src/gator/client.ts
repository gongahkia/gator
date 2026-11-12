import { EventEmitter } from "node:events"
import { randomUUID } from "node:crypto"
import { Application } from "@gator/application-host/application"
import type { HostSnapshot, Draft, UploadedFile, FrontendFrame } from "@gator/application-host/contracts/frontend"
import { errorMessage } from "../util/error"
import type { BackendState, ChatUpdate } from "@gator/application-host/contracts/protocol"
import { uploadLimits } from "@gator/application-host/attachment-limits"
import type { FileSelection } from "./file-selection"
import { frontendDefaults, type FrontendPreferenceValues } from "@gator/application-host/preferences"
import type { ReadingAnchor } from "./transcript-reading"

// All views read this projection. IDs, admission, permissions and persistence
// remain Gator-owned; there is no upstream client, server or session database.
export class GatorClient extends EventEmitter {
  snapshot: HostSnapshot
  drafts = new Map<string, Draft>()
  pending = new Set<string>()
  operations = new Map<symbol, { context: string; label: string }>()
  async track<T>(label: string, operation: Promise<T>): Promise<T> {
    const key = Symbol(label); this.operations.set(key, { context: this.context, label }); this.emit("change")
    try { return await operation } finally { this.operations.delete(key); if (!this.#closed) this.emit("change") }
  }
  issue = ""
  preferences = frontendDefaults()
  voicePhase = "idle"
  voiceDestination = ""
  fileSelections = new Map<string, FileSelection>()
  filePositions = new Map<string, { sheet: number; row: number; col: number }>()
  readingPositions = new Map<string, number>()
  readingAnchors = new Map<string, ReadingAnchor>()
  readingStatus?: { context: string; following: boolean; count: number; notice: string }
  setReadingStatus(value?: GatorClient["readingStatus"]) {
    if (JSON.stringify(this.readingStatus) === JSON.stringify(value)) return
    this.readingStatus = value
    if (!this.#closed) this.emit("change")
  }
  #readingLoaded = new Map<string, Promise<void>>()
  #readingWrites = new Map<string, ReadingAnchor>()
  #readingTimer?: ReturnType<typeof setTimeout>
  loadReading(context: string) {
    const previous = this.#readingLoaded.get(context)
    if (previous) return previous
    const pending = this.host.reading.read(context).then(anchor => {
      if (anchor && !this.readingAnchors.has(context)) this.readingAnchors.set(context, anchor)
    }).catch(error => { if (!this.#closed) this.fail(error) })
    this.#readingLoaded.set(context, pending)
    return pending
  }
  rememberReading(context: string, anchor: ReadingAnchor) {
    if (this.#closed || JSON.stringify(this.readingAnchors.get(context)) === JSON.stringify(anchor)) return
    this.readingAnchors.set(context, anchor); this.#readingWrites.set(context, anchor)
    if (!this.#readingTimer) this.#readingTimer = setTimeout(() => { void this.flushReading() }, 200)
  }
  async flushReading() {
    clearTimeout(this.#readingTimer); this.#readingTimer = undefined
    const writes = [...this.#readingWrites]; this.#readingWrites.clear()
    await Promise.all(writes.map(async ([context, anchor]) => {
      try { await this.host.reading.save(context, anchor) }
      catch { if (!this.#closed) this.fail(new Error("Reading position could not be saved. History and drafts are kept; /latest returns to the newest content.")) }
    }))
    await this.host.reading.flush()
  }
  activityExpanded = new Set<string>()
  updateFeedback = new Map<string, { id: string; saving: boolean; message: string }>()
  actions: Record<string, () => unknown> = {}
  isLocalCommand(text: string) { return text !== "/activity" && /^\/[\w-]+$/.test(text) && (Object.hasOwn(this.actions, text.slice(1)) || ["/following", "/saved", "/settings"].includes(text)) }
  route: "chat" | "following" | "saved" | "settings" = "chat"
  requestQuit?: () => Promise<void>
  navigationGuard?: () => Promise<boolean>
  #loads = new Map<string, Promise<void>>()
  #results = new Set<() => void>()
  #admissions = new Map<string, { key: string; request: string; queueOnly?:boolean }>()
  #admissionKey(draft = this.draft) { return JSON.stringify([draft.text, draft.uploads?.map(file => file.id) ?? [], draft.reply_to_update_id, !!draft.plan_first]) }
  get canRetrySubmission() { return !this.pending.has(this.context) && this.#admissions.get(this.context)?.key === this.#admissionKey() }
  async reconnect() {
    if (!await this.mayLeave()) return
    await this.track("Reconnecting this chat…", this.host.sessions.restart())
    await this.loadDraft()
    this.issue = ""; this.emit("change")
  }
  #closing?: Promise<void>
  #closed = false
  #listener = () => {
    this.snapshot = this.host.snapshot()
    const terminal = this.state?.terminal_preferences
    if (terminal?.initialized && !terminal.issue) this.preferences = terminal.values
    this.emit("change")
    if (!this.#closed) {
      void this.loadDraft().catch(error => this.fail(error))
    }
  }
  constructor(readonly host: Application) { super(); this.snapshot = host.snapshot(); host.on("change", this.#listener) }
  get state() { return this.snapshot.state }
  get context() { return this.host.context }
  get draft(): Draft { return this.drafts.get(this.context) ?? this.empty() }
  empty(context = this.context, state = this.state): Draft { return { context, title: state?.title ?? "", source: state?.source ?? "", text: "", uploads: [] } }
  fail(error: unknown) { this.issue = errorMessage(error); this.emit("change") }
  async start(conversationID?: string, goalID?: string) {
    await this.host.start(goalID ? { conversation_id: goalID, goal: true } : conversationID ? { conversation_id: conversationID } : undefined); await this.loadDraft()
    try { this.preferences = await this.host.preferences.read(); this.emit("change") }
    catch (error) { this.fail(error) }
  }
  async configurePreferences(patch: Partial<FrontendPreferenceValues>) {
    const saved = await this.host.preferences.configure(patch), current = this.state?.terminal_preferences
    this.preferences = current?.initialized && !current.issue ? current.values : saved
    this.emit("change")
  }
  async loadDraft() {
    const context = this.context
    if (!context || this.drafts.has(context) || this.#closed) return
    const pending = this.#loads.get(context)
    if (pending) return pending
    const empty = this.empty(context), load = (async () => {
      const saved = await this.host.drafts.request({ action: "list" })
      if (this.#closed || this.drafts.has(context)) return
      const draft = saved.drafts.find(d => d.context === context)
      // `updated` belongs to the disk record, not the editable/save contract.
      this.drafts.set(context, draft ? { context, title: draft.title, source: draft.source, text: draft.text, uploads: draft.uploads, reply_to_update_id: draft.reply_to_update_id, plan_first: draft.plan_first } : empty)
      this.emit("change")
    })()
    this.#loads.set(context, load)
    try { await load } finally { this.#loads.delete(context) }
  }
  async edit(text: string, uploads = this.draft.uploads ?? [], reply: string | null | undefined = this.draft.reply_to_update_id, planFirst = this.draft.plan_first) {
    if (Buffer.byteLength(text) > 128 * 1024 || uploads.length > uploadLimits.count || new Set(uploads.map(f => f.id)).size !== uploads.length || uploads.reduce((n, f) => n + f.size, 0) > uploadLimits.total) throw new Error("Draft exceeds Gator's text or attachment limits.")
    const current = this.draft
    const draft = { context: current.context, title: current.title, source: current.source, text, uploads, reply_to_update_id: reply ?? undefined, plan_first: planFirst || undefined }
    if (!draft.context) throw new Error("Wait for this chat to open before editing.")
    this.drafts.set(draft.context, draft); this.emit("change")
    await this.host.drafts.request({ action: "save", draft })
  }
  async setPlanFirst(enabled: boolean, context = this.context) {
    const sameChat = () => { if (this.context !== context) throw new Error("The conversation changed. Set plan-first in the intended chat.") }
    sameChat(); await this.loadDraft(); sameChat()
    await this.edit(this.draft.text, this.draft.uploads, this.draft.reply_to_update_id, enabled)
  }
  async attach(paths: string[]) {
    const context = this.context
    const files = await this.host.importFiles(paths, context)
    if (this.context !== context) throw new Error("The conversation changed. Your original draft is kept.")
    await this.appendFiles(files, context)
  }
  async restoreEditor(before: Draft, text: string) {
    const current = this.drafts.get(before.context) ?? before
    const changed = current.text !== before.text
    const value = changed ? [current.text, text].filter(Boolean).join("\n\n") : text
    if (Buffer.byteLength(value) > 128 * 1024) throw new Error("The edited draft exceeds 128 KiB. Your previous draft is kept; shorten the editor text before importing it.")
    const draft = { ...current, text: value }
    this.drafts.set(before.context, draft); this.emit("change")
    await this.host.drafts.request({ action: "save", draft })
    if (changed) this.fail(new Error("The draft changed during the editor handoff. Its edited text was appended so both versions are kept."))
  }
  async appendFiles(files: UploadedFile[], context = this.context) {
    if (this.context !== context) throw new Error("The conversation changed. Your original draft is kept.")
    await this.edit(this.draft.text, [...this.draft.uploads ?? [], ...files])
  }
  async stage(file: UploadedFile, selection = "") { await this.edit(selection ? [this.draft.text, selection].filter(Boolean).join("\n\n") : this.draft.text, [...this.draft.uploads ?? [], file]) }
  async clearAdmitted(draft: Draft, preservePlan = false) {
    if (this.drafts.get(draft.context) !== draft) return
    const next = { ...draft, text: "", uploads: [], reply_to_update_id: undefined, plan_first: preservePlan ? draft.plan_first : undefined }
    this.drafts.set(draft.context, next)
    await this.host.drafts.request({ action: "save", draft: next })
  }
  async guide() {
    if (["recording", "transcribing"].includes(this.voicePhase)) throw new Error("Finish or cancel dictation before guiding active work.")
    const draft = this.draft, context = draft.context, id = this.state?.guidance_id
    if (draft.plan_first) throw new Error("Turn off plan-first before guiding active work, or send a new reviewed request.")
    if (!id || !this.state?.running) throw new Error("There is no active task to guide.")
    if (!draft.text.trim() || draft.uploads?.length || draft.reply_to_update_id) throw new Error("Guide active work with text only. Send attachments and update replies as a queued follow-up.")
    if (this.pending.has(context)) throw new Error("Wait for the current message acknowledgement.")
    const request_id = randomUUID(); this.pending.add(context); this.emit("change")
    try {
      await this.result({ type: "queue", queue: { id, request_id, action: "guide", text: draft.text } }, request_id, s => s.queue_result)
      await this.clearAdmitted(draft)
    } finally { this.pending.delete(context); this.emit("change") }
  }
  async submit(queueOnly = false) {
    const draft = this.draft, context = draft.context
    if (/^\/(exit|quit)$/.test(draft.text.trim())) {
      if (!this.requestQuit) throw new Error("Quit is not ready yet.")
      await this.requestQuit(); return
    }
    if (this.pending.has(context)) throw new Error("This message is awaiting confirmation. Check the conversation before retrying.")
    if (!draft.text.trim() && !draft.uploads?.length) return
    const command = draft.text.trimStart().startsWith("/")
    if (!command && ["recording", "transcribing"].includes(this.voicePhase)) throw new Error("Finish or cancel dictation before sending the draft.")
    if (command && (draft.uploads?.length || draft.reply_to_update_id)) throw new Error("Send attachments and update replies with a message or queued follow-up.")
    if (!command && !this.state?.running && (!this.state?.provider || this.state?.composer?.readiness && this.state.composer.readiness.state !== "configured")) {
      await this.navigate(this.actions.setup ? "/setup" : "/model"); return
    }
    this.pending.add(context); this.issue = ""; this.emit("change")
    try {
      if (command) {
        if (this.isLocalCommand(draft.text.trim())) await this.navigate(draft.text.trim())
        else await this.send({ type: "submit", text: draft.text })
      }
      else {
        const files = draft.uploads?.map(f => f.id) ?? [], plan_first = draft.plan_first, key = this.#admissionKey(draft)
        // An uncertain acknowledgement can be explicitly retried with the same
        // admission ID. Go's durable request journal prevents duplicate work.
        if (this.#admissions.get(context)?.key !== key) this.#admissions.set(context, { key, request: randomUUID(), queueOnly })
        const admission=this.#admissions.get(context)!
        await this.send({ type: "upload_submit", upload: { request_id: admission.request, context, text: draft.text, files, reply_to_update_id: draft.reply_to_update_id, ...(plan_first ? { plan_first: true } : {}), ...(admission.queueOnly ? {queue_only:true}: {}) } })
        this.#admissions.delete(context)
      }
      // An acknowledgement only clears the submitted snapshot, never newer input.
      await this.clearAdmitted(draft, command)
    } finally { this.pending.delete(context); this.emit("change") }
  }
  async send(frame: FrontendFrame) { await this.host.send(frame) }
  async mayLeave() {
    if (this.updateFeedback.get(this.context)?.saving) { this.fail(new Error("Wait for Gator to confirm this feedback before leaving the chat.")); return false }
    return this.navigationGuard ? this.navigationGuard() : true
  }
  async rateUpdate(update: ChatUpdate, rating: "useful" | "noise" | "wrong" | "clear") {
    const context = this.context, conversation_id = this.state?.conversation_id
    if (!conversation_id || !update.can_rate || update.status !== "completed") throw new Error("Only a completed update accepts usefulness feedback.")
    if (this.updateFeedback.get(context)?.saving) throw new Error("Wait for the previous feedback acknowledgement.")
    this.updateFeedback.set(context, { id: update.id, saving: true, message: "Saving feedback…" }); this.emit("change")
    try {
      const request_id = randomUUID()
      await this.result({ type: "update", update: { conversation_id, id: update.id, version: update.version, rating_version: update.rating_version, action: "rate", rating, request_id } }, request_id, state => state.chat_update_result)
      this.updateFeedback.set(context, { id: update.id, saving: false, message: rating === "clear" ? "Feedback cleared." : "Feedback saved." })
    } catch (error) { this.updateFeedback.set(context, { id: update.id, saving: false, message: errorMessage(error) }); throw error }
    finally { this.emit("change") }
  }
  async openSession(id: string) { if (await this.mayLeave()) await this.host.openSession(id) }
  async openConversation(id: string, list_id: string) { if (await this.mayLeave()) await this.send({ type: "conversation", conversation: { id, list_id } }) }
  async openGoal(id: string) {
    const goal = this.state?.following_goals?.find(goal => goal.id === id)
    if (!goal) throw new Error("This goal is no longer available. Refresh Following before opening it.")
    if (goal.status !== "active") throw new Error("Resume this goal before opening its conversation.")
    if (!await this.navigate("/goals")) return false
    const panel = this.state?.panel, row = panel?.rows?.findIndex(row => row.identity === id) ?? -1
    if (!panel?.id || panel.title !== "Ongoing goals" || row < 0) throw new Error("This goal changed or is unavailable. Refresh Following and select it again.")
    // Reuse the canonical picker and its live panel digest. Equal-looking goals
    // never select each other, and opening one does not start a task or check.
    await this.send({ type: "panel", panel: { id: panel.id, action: "activate", row } })
    return true
  }
  async prepareGoalCheck(id: string) {
    const goal = this.state?.following_goals?.find(goal => goal.id === id)
    if (!goal || goal.status !== "active") throw new Error("Refresh Following and choose an active goal before setting up its check.")
    // Busy conversations open the existing isolated management owner. Go then
    // reads the selected goal, rather than inheriting that owner's workspace.
    if (!await this.navigate("/followup")) return false
    if (this.state?.running) throw new Error("Wait for the check management view to open.")
    const request_id = randomUUID()
    await this.result({ type: "check", check: { request_id, action: "setup", id } }, request_id, state => state.check_result)
    this.route = "chat"; this.emit("change")
    return true
  }
  async navigate(text: string) {
    if (/^\/(exit|quit)$/.test(text.trim()) && this.requestQuit) { await this.requestQuit(); return true }
    if (!await this.mayLeave()) return false
    this.emit("navigation")
    if (text === "/chat") { if (this.state?.panel) await this.send({ type: "navigate", text: "/chat" }); this.route = "chat"; this.emit("change"); return true }
    if (text === "/following" || text === "/saved" || text === "/settings") { this.route = text === "/following" ? "following" : text === "/saved" ? "saved" : "settings"; this.emit("change"); return true }
    if (this.isLocalCommand(text)) { this.emit("action", text.slice(1)); return true }
    this.route = "chat"; this.emit("change"); await this.send({ type: "navigate", text })
    return true
  }
  async result<T extends { request_id: string; error?: string }>(frame: FrontendFrame, requestID: string, read: (state: BackendState) => T | undefined): Promise<T> {
    return this.observe(state => {
      const value = read(state)
      if (value?.request_id !== requestID) return
      if (value.error) throw new Error(value.error)
      return value
    }, () => this.send(frame))
  }
  async waitFor(predicate: (state: BackendState) => boolean) { await this.observe(state => predicate(state) ? true : undefined, async () => {}) }
  private async observe<T>(read: (state: BackendState) => T | undefined, start: () => Promise<void>): Promise<T> {
    if (this.#closed || this.#closing) throw new Error("Gator is closing. Check the saved result after reopening.")
    const owner = this.context
    return new Promise<T>((resolve, reject) => {
      let finished = false
      const finish = (error?: Error, value?: T) => { if (finished) return; finished = true; clearTimeout(timer); this.off("change", changed); this.#results.delete(cancel); if (error) reject(error); else if (value) resolve(value) }
      const cancel = () => finish(new Error("Gator closed before the action was confirmed. Check its saved state after reopening."))
      const changed = () => {
        if (this.context !== owner || this.snapshot.connection === "disconnected") return finish(new Error("Return to the original chat and check the saved result before retrying."))
        try { const value = this.state && read(this.state); if (value !== undefined) finish(undefined, value) }
        catch (error) { finish(error instanceof Error ? error : new Error(String(error))) }
      }
      const timer = setTimeout(() => finish(new Error("The action was not confirmed. Check its saved state before retrying.")), 30000)
      this.#results.add(cancel)
      this.on("change", changed)
      void start().then(changed).catch(error => finish(error instanceof Error ? error : new Error(String(error))))
    })
  }
  async close() {
    if (this.#closed) return
    if (this.#closing) return this.#closing
    this.#closing = (async () => {
      // A failed draft flush or live external step leaves the UI subscribed.
      this.emit("remember-reading")
      await this.flushReading()
      await this.host.close()
      this.#closed = true
      this.host.off("change", this.#listener)
      for (const cancel of this.#results) cancel()
      this.removeAllListeners()
    })()
    try { await this.#closing } finally { this.#closing = undefined }
  }
}
