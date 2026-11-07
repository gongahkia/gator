import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { ScrollBoxRenderable } from "@opentui/core"
import { mkdtemp, rm } from "node:fs/promises"
import { join } from "node:path"
import { tmpdir } from "node:os"
import type { BackendBundle } from "@gator/application-host/contracts/protocol"
import type { FileRequest } from "@gator/application-host/contracts/fileProtocol"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost, state } from "./fixture"

test("work details expose latest-only outputs, checked proposals and failures without mixing current selections or chats", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-work-details-"))
  class Host extends FixtureHost {
    fileRequests: FileRequest[] = []
    async file(request: FileRequest): Promise<never> { this.fileRequests.push(request); throw new Error("Retained file is unavailable; original record is kept.") }
  }
  const host = new Host({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } })
  const earlier: BackendBundle = { Path: "/retained/first", Status: "completed", RunID: "work-first", Artifacts: [{ Path: "report.md", MediaType: "text/markdown", Valid: true, Preview: "Earlier retained report" }, { Path: "slides.pdf", MediaType: "application/pdf", Valid: true }], Candidates: [{ ID: "change-one", Status: "verified", ChangedPaths: ["src/example.ts"] }] }
  const latest: BackendBundle = { Path: "/retained/latest", Status: "failed", RunID: "work-failed", Sources: ["notes/venue.md"], Connections: [{ ID: "used-drive", Operations: ["fetch"] }], Artifacts: [{ Path: "failed-report.md", MediaType: "text/markdown", Valid: false, Preview: "This retained draft needs review.", Truncated: true }, { Path: "prototype.html", MediaType: "text/html", Valid: true, Preview: "<script>untrusted_preview()</script><h1>Source only</h1>" }], Candidates: null }
  host.current = { ...state(), source: "/selected/workspace", messages: [...Array.from({ length: 40 }, (_, n) => ({ role: "Gator", text: `Earlier message ${n}\nRetain the reading position and draft.` })), { role: "Deliverables", text: "Original output", bundle: earlier }], work_details: { mode: "auto", attachments: ["budget.md"], connector_ids: ["next-calendar"], latest_bundle: latest } }
  const client = new GatorClient(host)
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 130, height: 50 })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const choose = async (label: string) => { await setup.mockInput.typeText(label); await flush(); setup.mockInput.pressEnter(); await flush() }
  const command = async (name: string) => { setup.mockInput.pressKey("p", { ctrl: true }); await flush(); await choose(name) }
  const close = async () => { setup.mockInput.pressEscape(); await flush() }
  const transcript = () => { const node = setup.renderer.root.findDescendantById("transcript"); if (!(node instanceof ScrollBoxRenderable)) throw new Error("Missing transcript"); return node }
  try {
    await client.start(); await client.edit("Keep work-details draft"); await flush(); setup.mockInput.pressKey("\x1b[5~"); await flush(); const reading = transcript().scrollTop
    await command("work-details"); await choose("Current selections")
    for (const text of ["Selected workspace files: budget.md", "Selected connections: next-calendar", "Sources captured for that result: notes/venue.md", "used-drive: fetch", "Latest result: failed"]) expect(setup.captureCharFrame()).toContain(text)
    await close(); expect(transcript().scrollTop).toBe(reading)
    await command("outputs")
    for (const text of ["report.md", "slides.pdf", "Change change-one", "failed-report.md", "prototype.html", "Needs review", "Proposal"]) expect(setup.captureCharFrame()).toContain(text)
    await choose("failed-report.md"); await choose("Inspect retained preview")
    for (const text of ["Retained from a failed run.", "Showing a shortened preview.", "This retained draft needs review.", "File checks do not confirm external delivery."]) expect(setup.captureCharFrame()).toContain(text)
    expect(host.fileRequests).toHaveLength(0)
    await close(); await command("outputs"); await choose("prototype.html"); await choose("Inspect retained preview"); expect(setup.captureCharFrame()).toContain("<script>untrusted_preview()</script>"); await close()
    await command("outputs"); await choose("Change change-one"); expect(setup.captureCharFrame()).toContain("src/example.ts"); expect(setup.captureCharFrame()).toContain("this view does not apply them"); await close()
    await command("outputs"); await choose("report.md"); await choose("Show origin in chat"); expect(setup.captureCharFrame()).toContain("Original output"); expect(client.draft.text).toBe("Keep work-details draft")
    await client.navigate("/files"); await flush(); expect(setup.captureCharFrame()).toContain("failed-report.md"); await choose("failed-report.md")
    expect(host.fileRequests).toHaveLength(1); expect(host.fileRequests[0]).toMatchObject({ context: "conversation:first", ref: { kind: "output", bundle: latest.Path, path: "failed-report.md" } }); expect(client.issue).toContain("Retained file is unavailable")
    await command("outputs"); await choose("failed-report.md"); await choose("Inspect retained preview")
    host.change({ ...state("second"), following_goals: [{ id: "goal-saved", title: "Saved intention", status: "active" }], checks: [], checks_issue: "Check health is unavailable: schedule could not be read" }); await flush()
    expect(setup.captureCharFrame()).not.toContain("This retained draft needs review."); expect(setup.captureCharFrame()).not.toContain("used-drive"); expect(client.draft.text).toBe("")
    await client.navigate("/following"); await flush(); expect(setup.captureCharFrame()).toContain("Check health is unavailable"); expect(setup.captureCharFrame()).toContain("Check status unavailable"); expect(setup.captureCharFrame()).not.toContain("No recurring checks configured.")
    host.change({ ...host.current, checks_issue: undefined }); await flush(); expect(setup.captureCharFrame()).toContain("No recurring check attached"); expect(setup.captureCharFrame()).toContain("No recurring checks configured.")
    expect(host.sent).toHaveLength(0); expect(client.drafts.get("conversation:first")?.text).toBe("Keep work-details draft")
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 30000)
