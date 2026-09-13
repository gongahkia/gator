import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ok } from "@shared/ipc/result";
import App from "./App";
import { installHcb, now, seededHcb, todayDate } from "./test/appTestHelpers";

describe("Command palette search", () => {
  function addTestUtcDays(day: string, offset: number): string {
    const date = new Date(`${day}T00:00:00.000Z`);

    date.setUTCDate(date.getUTCDate() + offset);
    return date.toISOString().slice(0, 10);
  }

  function rangeOverlaps(request: { start: string; end: string }, startsAt: string, endsAt: string): boolean {
    return request.start < endsAt && request.end > startsAt;
  }

  it("falls back to planner search when the query matches no commands", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await user.keyboard("{Meta>}p{/Meta}");
    const dialog = await screen.findByRole("dialog", { name: "Command palette" });
    const input = within(dialog).getByRole("searchbox", { name: "Filter commands" });

    await user.type(input, "review");

    expect(await within(dialog).findByRole("option", { name: /Renderer acceptance review/ })).toBeInTheDocument();
    expect(api.search.query).toHaveBeenCalledWith({ query: "review", limit: 30 });
    expect(within(dialog).queryByText("Text: review")).not.toBeInTheDocument();
  });

  it("shows task snooze metadata in palette search results", async () => {
    const api = seededHcb();
    api.search.query = vi.fn(async () =>
      ok({
        items: [{
          id: "task-inbox-rules",
          domain: "tasks" as const,
          title: "Draft inbox triage rules",
          snippet: "Task in Inbox",
          snoozeUntil: new Date("2026-05-25T11:30").toISOString(),
          updatedAt: now
        }],
        page: { limit: 30, totalKnown: 1 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await user.keyboard("{Meta>}p{/Meta}");
    const dialog = await screen.findByRole("dialog", { name: "Command palette" });
    await user.type(within(dialog).getByRole("searchbox", { name: "Filter commands" }), "triage");

    expect(await within(dialog).findByRole("option", { name: /Task in Inbox.*Snoozed/ })).toBeInTheDocument();
  });

  it("opens the matching section from a palette search result", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await user.keyboard("{Meta>}p{/Meta}");
    const dialog = await screen.findByRole("dialog", { name: "Command palette" });

    await user.type(within(dialog).getByRole("searchbox", { name: "Filter commands" }), "triage");
    await user.click(await within(dialog).findByRole("option", { name: /Draft inbox triage rules/ }));

    expect(screen.queryByRole("dialog", { name: "Command palette" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Tasks" })).toBeInTheDocument();
    const inspector = await screen.findByTestId("inspector-shell");
    expect(inspector).toHaveAttribute("data-inspector-kind", "task");
    expect(inspector).toHaveAttribute("data-inspector-id", "task-inbox-rules");
  });

  it("opens event details from a palette search result", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await user.keyboard("{Meta>}p{/Meta}");
    const dialog = await screen.findByRole("dialog", { name: "Command palette" });

    await user.type(within(dialog).getByRole("searchbox", { name: "Filter commands" }), "review");
    await user.click(await within(dialog).findByRole("option", { name: /Renderer acceptance review/ }));

    expect(screen.queryByRole("dialog", { name: "Command palette" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Calendar" })).toBeInTheDocument();
    const inspector = await screen.findByTestId("inspector-shell");
    expect(inspector).toHaveAttribute("data-inspector-kind", "event");
    expect(inspector).toHaveAttribute("data-inspector-id", "event-review");
  });

  it("loads and opens an old event search result outside the visible calendar window", async () => {
    const api = seededHcb();
    const oldDay = addTestUtcDays(todayDate, -14);
    const oldEvent = {
      id: "event-old-retro",
      calendarId: "cal-product",
      title: "Ancient retro",
      startsAt: `${oldDay}T14:00:00.000Z`,
      endsAt: `${oldDay}T14:30:00.000Z`,
      allDay: false,
      updatedAt: now
    };
    api.search.query = vi.fn(async () =>
      ok({
        items: [{
          id: oldEvent.id,
          domain: "calendar" as const,
          title: oldEvent.title,
          snippet: "Old calendar event",
          updatedAt: now
        }],
        page: { limit: 30, totalKnown: 1 }
      })
    );
    api.calendar.get = vi.fn(async () =>
      ok({
        ...oldEvent,
        calendarTitle: "Product",
        deepLink: `hotcrossbuns://calendar/${oldEvent.id}`
      })
    );
    api.calendar.listEvents = vi.fn(async (request) =>
      ok({
        items: rangeOverlaps(request, oldEvent.startsAt, oldEvent.endsAt) ? [oldEvent] : [],
        page: { limit: 500, totalKnown: 1 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await user.keyboard("{Meta>}p{/Meta}");
    const dialog = await screen.findByRole("dialog", { name: "Command palette" });

    await user.type(within(dialog).getByRole("searchbox", { name: "Filter commands" }), "ancient");
    await user.click(await within(dialog).findByRole("option", { name: /Ancient retro/ }));

    const inspector = await screen.findByTestId("inspector-shell");
    expect(inspector).toHaveAttribute("data-inspector-kind", "event");
    expect(inspector).toHaveAttribute("data-inspector-id", oldEvent.id);
    expect(api.calendar.get).toHaveBeenCalledWith({ id: oldEvent.id });
    await waitFor(() => {
      expect(vi.mocked(api.calendar.listEvents).mock.calls.some(([request]) =>
        rangeOverlaps(request, oldEvent.startsAt, oldEvent.endsAt)
      )).toBe(true);
    });
  });

  it("opens note details from a palette search result", async () => {
    const api = seededHcb();
    api.search.query = vi.fn(async () =>
      ok({
        items: [{
          id: "task-note-startup",
          domain: "notes" as const,
          title: "Startup data flow",
          snippet: "Note updated from sync",
          updatedAt: now
        }],
        page: { limit: 30, totalKnown: 1 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await user.keyboard("{Meta>}p{/Meta}");
    const dialog = await screen.findByRole("dialog", { name: "Command palette" });

    await user.type(within(dialog).getByRole("searchbox", { name: "Filter commands" }), "source:notes startup");
    await user.click(await within(dialog).findByRole("option", { name: /Startup data flow/ }));

    expect(screen.queryByRole("dialog", { name: "Command palette" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Notes" })).toBeInTheDocument();
    const inspector = await screen.findByTestId("inspector-shell");
    expect(inspector).toHaveAttribute("data-inspector-kind", "note");
    expect(inspector).toHaveAttribute("data-inspector-id", "task-note-startup");
  });

  it("keeps command matches on the command path instead of searching", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await screen.findByText("Planner shell standup");
    vi.mocked(api.search.query).mockClear();

    await user.keyboard("{Meta>}p{/Meta}");
    const dialog = await screen.findByRole("dialog", { name: "Command palette" });

    await user.type(within(dialog).getByRole("searchbox", { name: "Filter commands" }), "new task");

    expect(within(dialog).getByRole("option", { name: /New task/ })).toBeInTheDocument();
    await new Promise((resolve) => window.setTimeout(resolve, 80));
    expect(api.search.query).not.toHaveBeenCalled();
  });

  it("supports structured search syntax inside the palette", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await screen.findByText("Planner shell standup");
    vi.mocked(api.search.query).mockClear();

    await user.keyboard("{Meta>}p{/Meta}");
    const dialog = await screen.findByRole("dialog", { name: "Command palette" });
    const query = "source:tasks status:open due:today priority:high list:Inbox notes:yes triage";

    fireEvent.change(within(dialog).getByRole("searchbox", { name: "Filter commands" }), {
      target: { value: query }
    });

    expect(await within(dialog).findByRole("option", { name: /Draft inbox triage rules/ })).toBeInTheDocument();
    expect(within(dialog).getByText("Source: tasks")).toBeInTheDocument();
    await waitFor(() => {
      expect(api.search.query).toHaveBeenCalledWith({ query, limit: 30 });
    });
  });

  it("shows invalid search syntax without executing a search", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await screen.findByText("Planner shell standup");
    vi.mocked(api.search.query).mockClear();

    await user.keyboard("{Meta>}p{/Meta}");
    const dialog = await screen.findByRole("dialog", { name: "Command palette" });

    fireEvent.change(within(dialog).getByRole("searchbox", { name: "Filter commands" }), {
      target: { value: "duration>=30m" }
    });

    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Use duration>30m");
    await new Promise((resolve) => window.setTimeout(resolve, 80));
    expect(api.search.query).not.toHaveBeenCalled();
  });
});
