import { expect, test } from "bun:test"
import { createSignal } from "solid-js"
import { testRender } from "@opentui/solid"
import { ThemeProvider, terminalPalette } from "../src/context/theme"
import { ConnectionStatus } from "../src/gator/connection-status"
import { displayDate, folderLabel } from "../src/gator/display"

test("connection dots keep semantic terminal palette slots and a readable label", async () => {
  const [connection, setConnection] = createSignal("starting")
  const [error, setError] = createSignal(false)
  const setup = await testRender(() => <ThemeProvider><ConnectionStatus connection={connection()} error={error()} /></ThemeProvider>, { width: 40, height: 3 })
  try {
    for (const [state, label, slot] of [["starting", "connecting", 3], ["connected", "connected", 2], ["disconnected", "disconnected", 1]] as const) {
      setConnection(state); await setup.flush()
      expect(setup.captureCharFrame()).toContain(`● ${label}`)
      const dot = setup.captureSpans().lines.flatMap(line => line.spans).find(span => span.text.includes("●"))!
      expect(dot.fg.intent).toBe(process.env.NO_COLOR ? "default" : "indexed")
      if (!process.env.NO_COLOR) expect(dot.fg.slot).toBe(slot)
    }
    setConnection("connected"); setError(true); await setup.flush(); expect(setup.captureCharFrame()).toContain("● disconnected")
    for (const noColor of [true, false]) {
      const palette = terminalPalette(noColor)
      expect(palette.text.base.intent).toBe("default")
      expect(palette.background.base.intent).toBe("default")
      expect(palette.background.raised.high.intent).toBe("default")
      if (noColor) expect(palette.text.feedback.error.base.intent).toBe("default")
    }
  } finally { await setup.dispose() }
})

test("display labels omit missing dates and shorten both Windows and Mac paths", () => {
  expect(displayDate("0001-01-01T00:00:00Z")).toBe("")
  expect(displayDate("not a timestamp")).toBe("")
  expect(folderLabel("C:\\Users\\Alex\\work\\Gator\\")).toBe("Gator")
  expect(folderLabel("/Users/Alex/work/Gator/")).toBe("Gator")
  expect(displayDate("2025-03-14T12:00:00Z", new Date("2026-10-29T12:00:00Z"))).toContain("2025")
})
