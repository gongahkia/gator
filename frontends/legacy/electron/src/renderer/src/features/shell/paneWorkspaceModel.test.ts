import { describe, expect, it } from "vitest";
import { movePaneToEdge, sanitizeStoredPaneWorkspace, splitPaneWebUrl, type PaneNode } from "./paneWorkspaceModel";

describe("paneWorkspaceModel", () => {
  it("moves panes to top and bottom edges as vertical splits", () => {
    const root: PaneNode = {
      id: "root",
      kind: "split",
      direction: "row",
      ratio: 0.5,
      children: [
        { id: "left", kind: "leaf", content: { kind: "section", sectionId: "tasks" } },
        { id: "right", kind: "leaf", content: { kind: "section", sectionId: "calendar" } }
      ]
    };

    const top = movePaneToEdge(root, "left", "right", "top");
    expect(top.node).toMatchObject({ kind: "split", direction: "column" });

    const bottom = movePaneToEdge(root, "left", "right", "bottom");
    expect(bottom.node).toMatchObject({ kind: "split", direction: "column" });
  });

  it("normalizes typed webpage targets", () => {
    expect(splitPaneWebUrl("example.com", "https://app.local")).toBe("https://example.com/");
    expect(splitPaneWebUrl("https://example.com/docs", "https://app.local")).toBe("https://example.com/docs");
    expect(splitPaneWebUrl("javascript:alert(1)", "https://app.local")).toBeNull();
  });

  it("migrates old web panes and ignores old recent webpages", () => {
    const stored = sanitizeStoredPaneWorkspace({
      focusedPaneId: "web",
      recentWebPages: [{ id: "old", title: "Old", url: "https://old.example/" }],
      root: {
        id: "web",
        kind: "leaf",
        content: { kind: "web", title: "Example", url: "https://example.com/docs" }
      }
    });

    expect(stored).not.toBeNull();
    expect(stored && "recentWebPages" in stored).toBe(false);
    expect(stored?.root).toMatchObject({
      kind: "leaf",
      content: {
        kind: "web",
        tabs: [{ title: "Example", url: "https://example.com/docs" }]
      }
    });
  });
});
