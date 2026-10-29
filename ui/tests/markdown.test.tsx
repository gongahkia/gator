import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { TextAttributes } from "@opentui/core"
import { ThemeProvider } from "../src/context/theme"
import { Markdown } from "../src/gator/markdown"

test("Markdown styles prose and preserves code, lists, links and tables at narrow widths", async () => {
  const text = '# Plan\n\n**Bold** and *italic* with `inline code`.\n\n1. First item\n2. Second item\n\n> A useful quote\n\n[Reference](https://example.com/help)\n\n```text\nconst value = "**literal**"\n```\n\n| Name | State |\n| --- | --- |\n| Report | Ready |\n\n\x1b[31mSafe text\x1b[0m'
  const setup = await testRender(() => <ThemeProvider theme="dark"><Markdown text={text} /></ThemeProvider>, { width: 70, height: 35 })
  try {
    // The bundled parser highlights asynchronously; wait for semantic rendering.
    for (let n = 0; n < 30; n++) {
      await Bun.sleep(40); await setup.flush()
      if (setup.captureSpans().lines.some(line => line.spans.some(span => span.text.includes("Bold") && !!(span.attributes & TextAttributes.BOLD)))) break
    }
    await setup.flush()
    const frame = setup.captureCharFrame(), spans = setup.captureSpans().lines.flatMap(line => line.spans)
    for (const label of ["Plan", "First item", "Second item", "A useful quote", "Reference", "Report", "Ready", "Safe text", 'const value = "**literal**"']) expect(frame).toContain(label)
    expect(spans.some(span => span.text.includes("Plan") && !!(span.attributes & TextAttributes.BOLD))).toBe(true)
    expect(spans.find(span => span.text.includes("Bold"))?.fg.intent).toBe("default")
    expect(frame).not.toContain("**Bold**"); expect(frame).not.toContain("# Plan"); expect(frame).not.toContain("```")
    expect(spans.some(span => span.text.includes("italic") && !!(span.attributes & TextAttributes.ITALIC))).toBe(true)
    setup.resize(38, 45); await setup.flush()
    expect(setup.captureCharFrame()).toContain("Report"); expect(setup.captureCharFrame()).toContain("Ready")
  } finally { await setup.dispose() }
})
