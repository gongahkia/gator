import assert from "node:assert/strict";
import test from "node:test";
import { panelGeometry, wrapTerminalText } from "../src/layout.js";
import { searchMessages } from "../src/search.js";
import { graphemeAt, nextGrapheme, previousGrapheme } from "../src/text.js";
import { paletteForTheme } from "../src/theme.js";
import { synchronizeKitTheme } from "../src/theme.js";
import { getTheme } from "@claude-code-kit/ui";

test("composer cursor and deletion preserve complete emoji and combining graphemes", () => {
  const value = "A👨‍👩‍👧‍👦e\u0301🐊B";
  const familyEnd = nextGrapheme(value, 1);
  assert.equal(graphemeAt(value, 1), "👨‍👩‍👧‍👦");
  assert.equal(previousGrapheme(value, familyEnd), 1);
  assert.equal(graphemeAt(value, familyEnd), "e\u0301");
  assert.equal(value.slice(0, 1) + value.slice(familyEnd), "Ae\u0301🐊B");
  const emojiStart = previousGrapheme(value, previousGrapheme(value, value.length));
  assert.equal(graphemeAt(value, emojiStart), "🐊");
});

test("transcript search follows visible cards and counts repeated occurrences", () => {
  const messages = [
    { text: "hidden raw output", result: { kind: "approval" as const, title: "Review send", fields: [{ label: "Recipient", value: "alex@example.com" }], preview: { subject: "Launch brief" }, footer: "/approve or /deny" } },
    { text: "", bundle: { Path: "/tmp/run", Status: "verified", Artifacts: [{ Path: "brief.docx", MediaType: "document", Valid: true }], Candidates: [{ ID: "c17", Status: "ready", ChangedPaths: ["notes.md"] }] } },
    { text: "brief brief" },
  ];
  assert.deepEqual(searchMessages(messages, "alex").map((match) => match.index), [0]);
  assert.deepEqual(searchMessages(messages, "brief").map((match) => match.index), [0, 1, 2, 2]);
  assert.deepEqual(searchMessages(messages, "Verified deliverables").map((match) => match.index), [1]);
  assert.deepEqual(searchMessages(messages, "Change c17").map((match) => match.index), [1]);
  assert.deepEqual(searchMessages(messages, "hidden"), []);
});

test("narrow panels stay inside the terminal and wrapped details remain reachable", () => {
  const narrow = panelGeometry(20, 12, true, true, true);
  assert.ok(narrow.panelWidth + narrow.paddingX * 2 <= 20);
  assert.ok(narrow.bodyRows >= 1);
  assert.equal(narrow.showComposerAnchor, false);
  assert.deepEqual(wrapTerminalText("ab🐊cd", 4), ["ab🐊", "cd"]);
  assert.equal(wrapTerminalText("a long explanation", 5).join(""), "a long explanation");
  assert.equal(panelGeometry(80, 24, false, false, true).showComposerAnchor, true);
});

test("saved themes produce distinct UI accents", () => {
  assert.notEqual(paletteForTheme("gator").accent, paletteForTheme("contrast").accent);
  assert.notEqual(paletteForTheme("contrast").accent, paletteForTheme("mono").accent);
  assert.equal(paletteForTheme("unknown"), paletteForTheme("gator"));
  synchronizeKitTheme(paletteForTheme("mono"));
  assert.equal(getTheme("dark").permission, paletteForTheme("mono").accent);
  assert.equal(getTheme("dark").link, paletteForTheme("mono").accent);
  synchronizeKitTheme(paletteForTheme("gator"));
});
