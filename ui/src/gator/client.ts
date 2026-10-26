import { EventEmitter } from "node:events"
import { randomUUID } from "node:crypto"
import { Application } from "@gator/application-host/application"
import type { HostSnapshot, Draft, UploadedFile, FrontendFrame } from "@gator/application-host/contracts/frontend"
import { errorMessage } from "../util/error"

// All views read this projection. IDs, admission, permissions and persistence
// remain Gator-owned; there is no upstream client, server or session database.
export class GatorClient extends EventEmitter {
  snapshot: HostSnapshot
  drafts = new Map<string, Draft>()
  pending = new Set<string>()
  issue = ""
  #load = 0
  #listener = () => { this.snapshot = this.host.snapshot(); this.emit("change"); void this.loadDraft().catch(error => this.fail(error)) }
  constructor(readonly host: Application) { super(); this.snapshot = host.snapshot(); host.on("change", this.#listener) }
  get state() { return this.snapshot.state }
  get context() { return this.host.context }
  get draft(): Draft { return this.drafts.get(this.context) ?? this.empty() }
  empty(): Draft { return { context: this.context, title: this.state?.title ?? "", source: this.state?.source ?? "", text: "", uploads: [] } }
  fail(error: unknown) { this.issue = errorMessage(error); this.emit("change") }
  async start() { await this.host.start(); await this.loadDraft() }
  async loadDraft() {
    const context = this.context
    if (!context || this.drafts.has(context)) return
    const generation = ++this.#load, saved = await this.host.drafts.request({ action: "list" })
    if (generation !== this.#load || this.drafts.has(context)) return
    this.drafts.set(context, saved.drafts.find(d => d.context === context) ?? this.empty())
    this.emit("change")
  }
  async edit(text: string, uploads = this.draft.uploads ?? [], reply = this.draft.reply_to_update_id) {
    const draft = { ...this.draft, text, uploads, reply_to_update_id: reply }
    if (!draft.context) throw new Error("Wait for this chat to open before editing.")
    this.drafts.set(draft.context, draft); this.emit("change")
    await this.host.drafts.request({ action: "save", draft })
  }
  async attach(paths: string[]) {
    const context = this.context, current = this.draft
    const files = await this.host.importFiles(paths, context)
    if (this.context !== context) throw new Error("The conversation changed. Your original draft is kept.")
    await this.edit(current.text, [...current.uploads ?? [], ...files])
  }
  async stage(file: UploadedFile, selection = "") { await this.edit(selection || this.draft.text, [...this.draft.uploads ?? [], file]) }
  async submit() {
    const draft = this.draft, context = draft.context
    if (this.pending.has(context)) throw new Error("This message is awaiting confirmation. Check the conversation before retrying.")
    if (!draft.text.trim() && !draft.uploads?.length) return
    this.pending.add(context); this.issue = ""; this.emit("change")
    try {
      if (draft.text.startsWith("/") && !draft.uploads?.length) await this.send({ type: "submit", text: draft.text })
      else await this.send({ type: "upload_submit", upload: { request_id: randomUUID(), context, text: draft.text, files: draft.uploads?.map(f => f.id) ?? [], reply_to_update_id: draft.reply_to_update_id } })
      // An acknowledgement only clears the submitted snapshot, never newer input.
      if (this.drafts.get(context) === draft) {
        this.drafts.set(context, { ...draft, text: "", uploads: [], reply_to_update_id: undefined })
        await this.host.drafts.request({ action: "remove", context })
      }
    } finally { this.pending.delete(context); this.emit("change") }
  }
  async send(frame: FrontendFrame) { await this.host.send(frame) }
  async navigate(text: string) { await this.send({ type: "navigate", text }) }
  async close() { this.host.off("change", this.#listener); await this.host.close(); this.removeAllListeners() }
}
