import type { NativeRoute } from "@shared/ipc/contracts";
import type { NativeMenuBarItem, NativeMenuBarSnapshot } from "../types";

export function menuBarPanelDataUrl(snapshot: NativeMenuBarSnapshot): string {
  return `data:text/html;charset=utf-8,${encodeURIComponent(menuBarPanelHtml(snapshot))}`;
}

export function menuBarPanelHtml(snapshot: NativeMenuBarSnapshot): string {
  const panel =
    snapshot.panelStyle === "calendar" && snapshot.calendar
      ? calendarPanelMarkup(snapshot)
      : adaptivePanelMarkup(snapshot);

  return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta
      http-equiv="Content-Security-Policy"
      content="default-src 'none'; img-src https: data:; style-src 'unsafe-inline'; navigate-to hcb-panel:"
    >
    <meta name="color-scheme" content="light dark">
    <title>Hot Cross Buns menu bar panel</title>
    <style>
      :root {
        color-scheme: light dark;
        font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", "Segoe UI", system-ui, sans-serif;
        background: transparent;
        color: #262626;
        --panel: rgba(255, 255, 255, 0.98);
        --panel-border: rgba(0, 0, 0, 0.18);
        --separator: rgba(0, 0, 0, 0.11);
        --muted: rgba(0, 0, 0, 0.52);
        --faint: rgba(0, 0, 0, 0.22);
        --hover: rgba(0, 0, 0, 0.06);
        --accent: #74aef1;
      }
      * { box-sizing: border-box; }
      body {
        margin: 0;
        min-height: 100vh;
        overflow: hidden;
        background: transparent;
        -webkit-font-smoothing: antialiased;
      }
      .popover {
        position: relative;
        width: 100vw;
        height: 100vh;
        padding-top: 9px;
        overflow: hidden;
        border: 1px solid var(--panel-border);
        border-radius: 13px;
        background: var(--panel);
      }
      .popover::before {
        content: "";
        position: absolute;
        top: 3px;
        left: calc(50% - 7px);
        width: 14px;
        height: 14px;
        transform: rotate(45deg);
        border-left: 1px solid var(--panel-border);
        border-top: 1px solid var(--panel-border);
        border-top-left-radius: 3px;
        background: var(--panel);
        z-index: 1;
      }
      .panel {
        position: relative;
        width: 100%;
        height: 100%;
        display: flex;
        flex-direction: column;
        overflow: hidden;
        border: 0;
        border-radius: 0;
        background: transparent;
        box-shadow: none;
      }
      .panel-header {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 12px;
        padding: 14px 18px 9px;
      }
      .panel-header h1 {
        margin: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        font-size: 17px;
        line-height: 22px;
        font-weight: 500;
      }
      .sync-label,
      .section-count,
      .secondary {
        color: var(--muted);
      }
      .sync-label {
        font-size: 12px;
        font-weight: 600;
      }
      .scroll-body {
        min-height: 0;
        flex: 1;
        overflow-y: auto;
        padding: 0 14px 10px;
      }
      .native-section {
        margin-top: 10px;
      }
      .section-heading {
        display: flex;
        align-items: baseline;
        justify-content: space-between;
        padding: 0 2px 6px;
        color: var(--muted);
        font-size: 12px;
        font-weight: 700;
        letter-spacing: 0;
      }
      .native-row {
        display: grid;
        grid-template-columns: minmax(0, 1fr);
        min-height: 38px;
        align-items: center;
        padding: 4px 4px;
        border-radius: 7px;
        color: inherit;
        text-decoration: none;
      }
      .native-row:hover { background: var(--hover); }
      .row-title,
      .row-detail {
        display: block;
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
      .row-title {
        font-size: 13px;
        line-height: 18px;
        font-weight: 500;
      }
      .row-detail {
        color: var(--muted);
        font-size: 12px;
        line-height: 16px;
      }
      .disabled {
        pointer-events: none;
        color: var(--muted);
      }
      .divider {
        border-top: 1px solid var(--separator);
        margin: 10px 0;
      }
      .account {
        display: grid;
        grid-template-columns: minmax(0, 1fr) 32px;
        align-items: center;
        gap: 10px;
        padding: 7px 4px;
      }
      .account-kicker {
        display: block;
        font-size: 12px;
        font-weight: 700;
        color: var(--muted);
      }
      .account-name {
        display: block;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        font-size: 13px;
        line-height: 17px;
        font-weight: 700;
      }
      .account-email {
        display: block;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        color: var(--muted);
        font-size: 12px;
        line-height: 16px;
      }
      .account-avatar {
        display: inline-flex;
        width: 32px;
        height: 32px;
        align-items: center;
        justify-content: center;
        overflow: hidden;
        border: 1px solid var(--separator);
        border-radius: 999px;
        background: var(--hover);
        color: var(--muted);
        font-size: 13px;
        font-weight: 700;
        line-height: 1;
        text-transform: uppercase;
      }
      .account-avatar img {
        display: block;
        width: 100%;
        height: 100%;
        object-fit: cover;
      }
      .quick-actions {
        display: grid;
        gap: 2px;
        padding-bottom: 2px;
      }
      .quick-action {
        display: block;
        min-height: 26px;
        padding: 3px 2px;
        border-radius: 6px;
        color: var(--muted);
        font-size: 13px;
        line-height: 19px;
        text-decoration: none;
      }
      .quick-action:hover { background: var(--hover); color: inherit; }
      .calendar-wrap {
        padding: 13px 18px 0;
      }
      .calendar-header {
        display: grid;
        grid-template-columns: minmax(0, 1fr) 24px 24px 24px;
        align-items: center;
        gap: 4px;
        margin-bottom: 12px;
      }
      .calendar-title {
        color: var(--muted);
        font-size: 17px;
        line-height: 22px;
        font-weight: 400;
      }
      .calendar-control {
        display: grid;
        place-items: center;
        width: 24px;
        height: 24px;
        border-radius: 6px;
        color: var(--muted);
        text-decoration: none;
        font-size: 21px;
        line-height: 1;
      }
      .calendar-control:hover { background: var(--hover); }
      .calendar-dot {
        font-size: 18px;
      }
      .weekday-grid,
      .day-grid {
        display: grid;
        grid-template-columns: repeat(7, minmax(0, 1fr));
        gap: 4px;
      }
      .weekday {
        text-align: center;
        color: var(--muted);
        font-size: 12px;
        font-weight: 700;
      }
      .day {
        display: grid;
        place-items: center;
        height: 29px;
        border-radius: 7px;
        color: inherit;
        font-size: 15px;
        font-variant-numeric: tabular-nums;
        text-decoration: none;
      }
      .day.muted { color: var(--faint); }
      .day.selected {
        color: white;
        background: var(--accent);
        font-weight: 700;
      }
      .day.today:not(.selected) {
        background: rgba(116, 174, 241, 0.18);
      }
      .selected-agenda {
        border-top: 1px solid var(--separator);
        margin-top: 15px;
        padding-top: 12px;
      }
      .selected-header {
        display: flex;
        align-items: baseline;
        justify-content: space-between;
        gap: 10px;
        margin-bottom: 8px;
      }
      .selected-title {
        font-size: 13px;
        font-weight: 700;
      }
      .quick-add {
        display: block;
        margin-top: 10px;
        padding: 6px 8px;
        border: 1px solid var(--separator);
        border-radius: 7px;
        color: var(--muted);
        font-size: 13px;
        line-height: 17px;
        text-decoration: none;
      }
      .quick-add:hover { background: var(--hover); }
      @media (prefers-color-scheme: dark) {
        :root {
          color: #f2f2f2;
          --panel: rgba(36, 36, 36, 0.98);
          --panel-border: rgba(255, 255, 255, 0.16);
          --separator: rgba(255, 255, 255, 0.13);
          --muted: rgba(255, 255, 255, 0.58);
          --faint: rgba(255, 255, 255, 0.24);
          --hover: rgba(255, 255, 255, 0.08);
          --accent: #5e9de6;
        }
      }
    </style>
  </head>
  <body>
    <div class="popover">
      ${panel}
    </div>
  </body>
</html>`;
}

function adaptivePanelMarkup(snapshot: NativeMenuBarSnapshot): string {
  return `
    <main class="panel adaptive-panel">
      <header class="panel-header">
        <h1>${escapeHtml(snapshot.title)}</h1>
        <span class="sync-label">${escapeHtml(snapshot.syncLabel)}</span>
      </header>
      <div class="scroll-body">
        ${nativeSectionsMarkup(snapshot.sections)}
        ${accountMarkup(snapshot)}
        ${quickActionsMarkup()}
      </div>
    </main>`;
}

function calendarPanelMarkup(snapshot: NativeMenuBarSnapshot): string {
  const calendar = snapshot.calendar;

  if (!calendar) {
    return adaptivePanelMarkup(snapshot);
  }

  const days = calendar.days
    .map((day) => {
      const classes = [
        "day",
        day.inCurrentMonth ? "" : "muted",
        day.isToday ? "today" : "",
        day.isSelected ? "selected" : ""
      ].filter(Boolean).join(" ");

      return `<span class="${classes}">${escapeHtml(day.label)}</span>`;
    })
    .join("");
  const weekdays = calendar.weekdayLabels
    .map((label) => `<span class="weekday">${escapeHtml(label)}</span>`)
    .join("");

  return `
    <main class="panel calendar-panel">
      <div class="scroll-body calendar-wrap">
        <section>
          <div class="calendar-header">
            <div class="calendar-title">${escapeHtml(calendar.monthLabel)}</div>
            <a class="calendar-control" href="${panelRouteHref({ kind: "calendar" })}" aria-label="Previous month">&lsaquo;</a>
            <a class="calendar-control calendar-dot" href="${panelRouteHref({ kind: "calendar" })}" aria-label="Today">&bull;</a>
            <a class="calendar-control" href="${panelRouteHref({ kind: "calendar" })}" aria-label="Next month">&rsaquo;</a>
          </div>
          <div class="weekday-grid">${weekdays}</div>
          <div class="day-grid">${days}</div>
        </section>
        <section class="selected-agenda">
          <div class="selected-header">
            <div class="selected-title">${escapeHtml(calendar.selectedLabel)}</div>
            <div class="secondary">${escapeHtml(calendar.selectedMeta)}</div>
          </div>
          ${rowsMarkup(calendar.selectedItems)}
        </section>
        ${accountMarkup(snapshot)}
        ${quickActionsMarkup()}
      </div>
    </main>`;
}

function compactPanelMarkup(snapshot: NativeMenuBarSnapshot): string {
  return `
    <main class="panel compact-panel">
      <header class="panel-header">
        <h1>${escapeHtml(snapshot.title)}</h1>
        <span class="sync-label">${escapeHtml(snapshot.syncLabel)}</span>
      </header>
      <div class="scroll-body">
        ${nativeSectionsMarkup(snapshot.sections)}
        ${accountMarkup(snapshot)}
        ${quickActionsMarkup()}
      </div>
    </main>`;
}

function nativeSectionsMarkup(sections: NativeMenuBarSnapshot["sections"]): string {
  return sections.map((section) => {
    const count = section.items.filter((item) => item.route || item.action).length;

    return `
      <section class="native-section">
        ${section.title ? `
          <div class="section-heading">
            <span>${escapeHtml(section.title)}</span>
            ${count > 0 ? `<span class="section-count">${count}</span>` : ""}
          </div>` : ""}
        ${rowsMarkup(section.items)}
      </section>`;
  }).join("");
}

function rowsMarkup(items: NativeMenuBarItem[]): string {
  return items.map((item) => {
    const href = menuBarItemHref(item);
    const disabled = href === "#";

    return `
      <a class="native-row ${disabled ? "disabled" : ""}" href="${escapeHtml(href)}" aria-disabled="${disabled}">
        <span class="row-text">
          <span class="row-title">${escapeHtml(item.label)}</span>
          ${item.detail ? `<span class="row-detail">${escapeHtml(item.detail)}</span>` : ""}
        </span>
      </a>`;
  }).join("");
}

function accountMarkup(snapshot: NativeMenuBarSnapshot): string {
  if (!snapshot.account) {
    return "";
  }

  const hasProfileText = snapshot.account.displayName !== "Google account" || Boolean(snapshot.account.email);
  const accountName = hasProfileText ? snapshot.account.displayName : "Connected";
  const fallbackInitial = accountName.trim().charAt(0) || "G";
  const avatarMarkup = snapshot.account.avatarUrl
    ? `<span class="account-avatar"><img alt="" referrerpolicy="no-referrer" src="${escapeHtml(snapshot.account.avatarUrl)}"></span>`
    : `<span class="account-avatar" aria-hidden="true">${escapeHtml(fallbackInitial)}</span>`;

  return `
    <div class="divider"></div>
    <section class="account">
      <span class="account-copy">
        <span class="account-kicker">Google account</span>
        <span class="account-name">${escapeHtml(accountName)}</span>
        ${snapshot.account.email ? `<span class="account-email">${escapeHtml(snapshot.account.email)}</span>` : ""}
      </span>
      ${avatarMarkup}
    </section>`;
}

function quickActionsMarkup(): string {
  return `
    <div class="divider"></div>
    <nav class="quick-actions" aria-label="Menu bar actions">
      <a class="quick-action" href="${panelActionHref("showWindow")}">Open Hot Cross Buns</a>
      <a class="quick-action" href="${panelActionHref("refresh")}">Refresh</a>
      <a class="quick-action" href="${panelActionHref("openSettings")}">Settings</a>
      <a class="quick-action" href="${panelActionHref("quit")}">Quit</a>
    </nav>`;
}

function menuBarItemHref(item: NativeMenuBarItem): string {
  if (item.route) {
    return panelRouteHref(item.route);
  }

  if (item.action) {
    return panelActionHref(item.action);
  }

  return "#";
}

export function panelRouteHref(route: NativeRoute): string {
  const params = new URLSearchParams({ kind: route.kind });

  if (route.id) {
    params.set("id", route.id);
  }

  if (route.query) {
    params.set("query", route.query);
  }

  return `hcb-panel://route?${params.toString()}`;
}

export function panelActionHref(action: NonNullable<NativeMenuBarItem["action"]>): string {
  return `hcb-panel://action?name=${encodeURIComponent(action)}`;
}

export function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, (character) => {
    switch (character) {
      case "&":
        return "&amp;";
      case "<":
        return "&lt;";
      case ">":
        return "&gt;";
      case "\"":
        return "&quot;";
      case "'":
        return "&#39;";
      default:
        return character;
    }
  });
}
