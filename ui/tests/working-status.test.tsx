import { expect, test } from "bun:test"
import { createSignal } from "solid-js"
import { testRender } from "@opentui/solid"
import { ConfigProvider } from "../src/config"
import { ThemeProvider } from "../src/context/theme"
import { WorkingStatus } from "../src/gator/working-status"
import { SPINNER_FRAMES } from "../src/component/spinner-frames"

test("spinner advances, approval and cancellation stop it, and reduced motion stays still", async () => {
  const previous = process.env.GATOR_REDUCED_MOTION, noColor = process.env.NO_COLOR
  delete process.env.GATOR_REDUCED_MOTION; delete process.env.NO_COLOR
  const [phase, setPhase] = createSignal("working")
  const render = () => testRender(() => <ThemeProvider theme="dark"><ConfigProvider><WorkingStatus status="Working from an immutable snapshot…" elapsed="4s" attention={phase() === "approval"} cancelling={phase() === "stopping"} /></ConfigProvider></ThemeProvider>, { width: 60, height: 5 })
  let setup = await render()
  try {
    const frames = new Set<string>()
    for (let n = 0; n < 5; n++) { await setup.flush(); frames.add(setup.captureCharFrame()); await Bun.sleep(90) }
    expect(frames.size).toBeGreaterThan(1)
    expect([...frames].every(frame => frame.includes("Thinking…") && frame.includes("4s") && SPINNER_FRAMES.some(spinner => frame.includes(spinner)))).toBe(true)
    setPhase("approval"); await setup.flush(); expect(setup.captureCharFrame()).toContain("Waiting for approval")
    expect(SPINNER_FRAMES.some(frame => setup.captureCharFrame().includes(frame))).toBe(false)
    setPhase("stopping"); await setup.flush(); expect(setup.captureCharFrame()).toContain("Stopping…")
    await setup.dispose(); process.env.GATOR_REDUCED_MOTION = "1"; setPhase("working"); setup = await render(); await setup.flush()
    const still = setup.captureCharFrame(); await Bun.sleep(180); await setup.flush()
    expect(setup.captureCharFrame()).toBe(still); expect(still).toContain("⋯ Thinking…")
  } finally {
    await setup.dispose()
    if (previous === undefined) delete process.env.GATOR_REDUCED_MOTION; else process.env.GATOR_REDUCED_MOTION = previous
    if (noColor === undefined) delete process.env.NO_COLOR; else process.env.NO_COLOR = noColor
  }
})
