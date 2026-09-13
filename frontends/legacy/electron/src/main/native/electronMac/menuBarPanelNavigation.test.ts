import { describe, expect, it } from "vitest";
import { panelActionHref, panelRouteHref } from "./menuBarPanelHtml";
import { parseMenuBarPanelUrl } from "./menuBarPanelNavigation";

describe("menu bar panel navigation", () => {
  it("round-trips route hrefs with ids and queries", () => {
    const href = panelRouteHref({ kind: "task", id: "task-1", query: "due today" });

    expect(parseMenuBarPanelUrl(href)).toEqual({
      kind: "route",
      route: {
        kind: "task",
        id: "task-1",
        query: "due today"
      }
    });
  });

  it("round-trips action hrefs", () => {
    expect(parseMenuBarPanelUrl(panelActionHref("refresh"))).toEqual({
      kind: "action",
      action: "refresh"
    });
  });

  it("rejects unknown panel URLs", () => {
    expect(parseMenuBarPanelUrl("https://example.com")).toBeNull();
    expect(parseMenuBarPanelUrl("hcb-panel://route?kind=unknown")).toBeNull();
    expect(parseMenuBarPanelUrl("hcb-panel://action?name=unknown")).toBeNull();
    expect(parseMenuBarPanelUrl("hcb-panel://other?kind=today")).toBeNull();
  });
});
