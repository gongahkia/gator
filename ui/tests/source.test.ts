import { test, expect } from "bun:test"
import { readFileSync } from "node:fs"
import { createHash } from "node:crypto"
import { resolve } from "node:path"
import { moveSelection, revealSelectionOffset } from "../src/ui/select-controller"
import { literal } from "../src/gator/text"

test("recorded upstream files match their pinned source hashes", () => {
  const root = resolve(import.meta.dir, "../upstream")
  const hashes: Record<string, string> = JSON.parse(readFileSync(resolve(root, "manifest.json"), "utf8"))
  for (const [file, hash] of Object.entries(hashes)) expect(createHash("sha256").update(readFileSync(resolve(root, file))).digest("hex")).toBe(hash)
})
test("upstream selection wraps and reveals bounded list positions", () => {
  expect(moveSelection(0, { count: 5, delta: -1, policy: "wrap" })).toBe(4)
  expect(moveSelection(0, { count: 0, delta: 1, policy: "wrap" })).toBe(0)
  expect(revealSelectionOffset(0, { count: 200, limit: 10, selected: 150 })).toBe(141)
})
test("untrusted text cannot emit terminal commands", () => {
  expect(literal("🐊\x1b[2Jhello\x1b]52;c;secret\x07\nworld\x00")).toBe("🐊hello\nworld")
})
