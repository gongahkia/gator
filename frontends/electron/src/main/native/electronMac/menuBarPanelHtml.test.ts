import { describe, expect, it } from "vitest";
import type { NativeMenuBarSnapshot } from "../types";
import { menuBarPanelDataUrl, menuBarPanelHtml } from "./menuBarPanelHtml";

describe("menu bar panel HTML", () => {
  it("escapes snapshot text before rendering the panel", () => {
    const snapshot: NativeMenuBarSnapshot = {
    panelStyle: "adaptive",
    iconName: "calendar",
    calendarIconId: "calendar",
    calendarDoneMode: "visibleTodayDone",
    customMenuBarIcons: [],
    calendarDone: false,
    primaryClickAction: "open-menu",
      title: "<script>&\"'",
      syncLabel: "Sync <ok>",
      tooltip: "Tooltip",
      sections: [
        {
          title: "Danger & Co",
          items: [
            {
              label: "Buy <milk>",
              detail: "Use \"oat\" & tea",
              action: "refresh"
            }
          ]
        }
      ]
    };

    const html = menuBarPanelHtml(snapshot);

    expect(html).toContain("&lt;script&gt;&amp;&quot;&#39;");
    expect(html).toContain("Sync &lt;ok&gt;");
    expect(html).toContain("Danger &amp; Co");
    expect(html).toContain("Buy &lt;milk&gt;");
    expect(html).toContain("Use &quot;oat&quot; &amp; tea");
    expect(html).not.toContain("Buy <milk>");
    expect(menuBarPanelDataUrl(snapshot)).toMatch(/^data:text\/html;charset=utf-8,/);
  });

  it("renders agenda rows without leading icons", () => {
    const snapshot: NativeMenuBarSnapshot = {
    panelStyle: "adaptive",
    iconName: "calendar",
    calendarIconId: "calendar",
    calendarDoneMode: "visibleTodayDone",
    customMenuBarIcons: [],
    calendarDone: false,
    primaryClickAction: "open-menu",
      title: "Agenda",
      syncLabel: "Synced",
      tooltip: "Tooltip",
      sections: [
        {
          title: "Today",
          items: [
            {
              label: "Focus block",
              detail: "9:00 AM-10:00 AM",
              route: { kind: "event", id: "event-1" }
            }
          ]
        }
      ]
    };

    const html = menuBarPanelHtml(snapshot);

    expect(html).toContain("Focus block");
    expect(html).not.toContain("row-icon");
  });

  it("renders the account block with an avatar slot instead of the old chevron", () => {
    const snapshot: NativeMenuBarSnapshot = {
      panelStyle: "adaptive",
      iconName: "calendar",
      calendarIconId: "calendar",
      calendarDoneMode: "visibleTodayDone",
      customMenuBarIcons: [],
      calendarDone: false,
      primaryClickAction: "open-menu",
      title: "Agenda",
      syncLabel: "Synced",
      tooltip: "Tooltip",
      sections: [],
      account: {
        displayName: "Google account",
        connectionState: "connected"
      }
    };

    const html = menuBarPanelHtml(snapshot);

    expect(html).not.toContain("class=\"chevrons\"");
    expect(html).not.toContain(">v</span>");
    expect(html).toContain("class=\"account-avatar\"");
    expect(html).toContain("<span class=\"account-name\">Connected</span>");
  });

  it("renders the Google account profile image when present", () => {
    const snapshot: NativeMenuBarSnapshot = {
      panelStyle: "adaptive",
      iconName: "calendar",
      calendarIconId: "calendar",
      calendarDoneMode: "visibleTodayDone",
      customMenuBarIcons: [],
      calendarDone: false,
      primaryClickAction: "open-menu",
      title: "Agenda",
      syncLabel: "Synced",
      tooltip: "Tooltip",
      sections: [],
      account: {
        displayName: "Gabriel Ong",
        email: "angryapplegravy@gmail.com",
        avatarUrl: "https://lh3.googleusercontent.com/avatar.png",
        connectionState: "connected"
      }
    };

    const html = menuBarPanelHtml(snapshot);

    expect(html).toContain("img-src https: data:");
    expect(html).toContain("referrerpolicy=\"no-referrer\"");
    expect(html).toContain("src=\"https://lh3.googleusercontent.com/avatar.png\"");
  });
});
