import { expect, test } from "bun:test"
import { createEffect } from "solid-js"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { ClientProvider } from "../src/context/client"
import { DataProvider, useData } from "../src/context/data"
import { GatorClient } from "../src/gator/client"
import { FixtureHost } from "./fixture"

test("work snapshots update only changed fields and never mutate an open exact review", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-field-subscriptions-"))
  const host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } })
  const client = new GatorClient(host)
  let messageUpdates = 0, connectionUpdates = 0, statusUpdates = 0
  let read!: ReturnType<typeof useData>
  const Probe = () => {
    const data = useData(); read = data
    createEffect(() => { data.state?.messages; messageUpdates++ })
    createEffect(() => { data.snapshot.connection; connectionUpdates++ })
    createEffect(() => { data.state?.status; statusUpdates++ })
    return <box><text>{data.state?.messages[0]?.text}</text><text>{data.state?.status}</text></box>
  }
  const setup = await testRender(() => <ClientProvider client={client}><DataProvider><Probe /></DataProvider></ClientProvider>, { width: 80, height: 24 })
  try {
    await client.start()
    host.change({ ...host.current, running: true, status: "Working 0", messages: [{ role: "Gator", text: "The exact reviewed draft" }], pending_approval: { id: 1, token: "original-token", message_index: 0 } })
    await setup.flush()
    const capturedReview = read.state!.pending_approval!, capturedMessage = read.state!.messages[0]!
    const before = { messageUpdates, connectionUpdates, statusUpdates }
    for (let tick = 1; tick <= 40; tick++) host.change({ ...JSON.parse(JSON.stringify(host.current)), status: `Working ${tick}`, loading_frame: tick })
    await setup.flush()
    expect(messageUpdates).toBe(before.messageUpdates)
    expect(connectionUpdates).toBe(before.connectionUpdates)
    expect(statusUpdates).toBe(before.statusUpdates + 40)
    expect(read.state!.pending_approval).toBe(capturedReview)
    expect(read.state!.messages[0]).toBe(capturedMessage)
    expect(setup.captureCharFrame()).toContain("Working 40")
    host.change({ ...host.current, messages: [{ role: "Gator", text: "A different proposed action" }], pending_approval: { id: 2, token: "replacement-token", message_index: 0 } })
    await setup.flush()
    expect(read.state!.pending_approval).not.toBe(capturedReview)
    expect(capturedReview.id).toBe(1); expect(capturedReview.token).toBe("original-token")
    expect(capturedMessage.text).toBe("The exact reviewed draft")
    expect(setup.captureCharFrame()).toContain("A different proposed action")
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})
