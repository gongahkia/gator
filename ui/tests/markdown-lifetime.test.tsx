import { expect, spyOn, test } from "bun:test"
import { Show, createSignal } from "solid-js"
import { testRender } from "@opentui/solid"
import { CodeRenderable, getTreeSitterClient, type Renderable } from "@opentui/core"
import { ThemeProvider } from "../src/context/theme"
import { Markdown } from "../src/gator/markdown"

test("native markdown consumers are destroyed before styles during navigation and theme changes with pending highlighting", async () => {
  const warnings: string[] = [], pending: (() => void)[] = []
  const warning = spyOn(console, "warn").mockImplementation((...args) => { warnings.push(args.map(String).join(" ")) })
  const highlighting = spyOn(getTreeSitterClient(), "highlightOnce").mockImplementation(async () => {
    await new Promise<void>(resolve => { pending.push(resolve) })
    return { highlights: [[0, 5, "markup.strong"]] }
  })
  let show = (_value: boolean) => {}, theme = (_value: string) => {}, update = (_value: string) => {}
  const body = "**Booking** [evidence](https://example.com)\n\n```json\n{\"price\": 2000}\n```\n\n| Venue | State |\n| --- | --- |\n| One | Pending |"
  const setup = await testRender(() => {
    const [visible, setVisible] = createSignal(true), [appearance, setAppearance] = createSignal("gator"), [text, setText] = createSignal(body)
    show = setVisible; theme = setAppearance; update = setText
    return <ThemeProvider theme={appearance()}><Show when={visible()}><Markdown text={text()} streaming /></Show></ThemeProvider>
  }, { width: 80, height: 24 })
  const codes = (node: Renderable): CodeRenderable[] => [...(node instanceof CodeRenderable ? [node] : []), ...node.getChildren().flatMap(codes)]
  const release = async () => { for (const resolve of pending.splice(0)) resolve(); await Promise.resolve(); await Promise.resolve() }
  try {
    await setup.flush()
    const initial = codes(setup.renderer.root); expect(initial.length).toBeGreaterThan(0); expect(pending.length).toBeGreaterThan(0)
    const oldStyle = initial[0]!.syntaxStyle, settled = initial.map(node => node.highlightingDone)
    // No renderer flush between removing Solid content and releasing its parser.
    // This reproduces the interval in which deferred native removal used to let
    // an asynchronous highlight borrow a style already destroyed by cleanup.
    show(false)
    for (const node of initial) expect(node.isDestroyed).toBe(true)
    expect(() => oldStyle.getStyle("default")).toThrow("destroyed")
    await release(); await Promise.all(settled); expect(warnings).toHaveLength(0)
    show(true); await setup.flush()
    for (const appearance of ["light", "contrast", "mono", "terminal", "gator"]) {
      const before = codes(setup.renderer.root), styles = before.map(node => node.syntaxStyle), done = before.map(node => node.highlightingDone)
      theme(appearance)
      for (const node of before) expect(node.isDestroyed).toBe(true)
      for (const style of styles) expect(() => style.getStyle("default")).toThrow("destroyed")
      await release(); await Promise.all(done); await setup.flush()
      const live = codes(setup.renderer.root); expect(live.length).toBeGreaterThan(0)
      for (const node of live) expect(node.syntaxStyle.getStyle("default")).toBeDefined()
      update(body + "\n\nChanged after " + appearance); await setup.flush()
    }
    show(false); await release(); await setup.flush(); expect(warnings).toHaveLength(0)
  } finally { show(false); await release(); await setup.dispose(); highlighting.mockRestore(); warning.mockRestore() }
})
