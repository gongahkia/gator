import { Application } from "@gator/application-host/application"
import type { BackendState } from "@gator/application-host/contracts/protocol"
import type { FrontendFrame } from "@gator/application-host/contracts/frontend"

export function state(id = "first"): BackendState {
  return { mode: "chat", messages: [], commands: [], actions: [], title: id, source: "topic:" + id, conversation_id: id,
    provider: "fixture", theme: "gator", status_line: [], status_line_enabled: false, running: false, cancelling: false, loading_frame: 0,
    queued: 0, queue: [], home: false, first_run: false, supervisor_needs_start: false }
}
export class FixtureHost extends Application {
  current = state()
  closeIssue = ""
  closed = false
  sent: FrontendFrame[] = []
  dispatch: (frame: FrontendFrame) => Promise<void> = async () => {}
  context = "conversation:first"
  snapshot() { return { ...super.snapshot(), state: this.current, connection: "connected" } }
  change(next: BackendState) { this.current = next; this.context = "conversation:" + next.conversation_id; this.emit("change") }
  async start() { this.emit("change"); return this.snapshot() }
  async send(frame: FrontendFrame) { this.sent.push(frame); await this.dispatch(frame) }
  async close() { if (this.closeIssue) throw new Error(this.closeIssue); await this.drafts.flush(); this.closed = true }
}
