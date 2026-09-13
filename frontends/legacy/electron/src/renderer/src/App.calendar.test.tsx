import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ok } from "@shared/ipc/result";
import App from "./App";
import {
  goToSection,
  installHcb,
  now,
  runPaletteCommand,
  seededHcb,
  seededTaskDetail,
  testDataTransfer,
  testSettings,
  todayDate,
  tomorrowIso,
  utcWeekStartDate
} from "./test/appTestHelpers";

describe("App calendar", () => {
  function addTestUtcDays(day: string, offset: number): string {
    const date = new Date(`${day}T00:00:00.000Z`);

    date.setUTCDate(date.getUTCDate() + offset);
    return date.toISOString().slice(0, 10);
  }

  function hideTaskCalendarItems(api: ReturnType<typeof seededHcb>): void {
    api.settings = {
      ...api.settings,
      get: vi.fn(async () => ok(testSettings({ selectedTaskListIds: ["list-none"] })))
    };
  }

  function rangeOverlaps(request: { start: string; end: string }, startsAt: string, endsAt: string): boolean {
    return request.start < endsAt && request.end > startsAt;
  }

  it("switches calendar agenda, day, week, and month shells", async () => {
    installHcb(seededHcb());
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    expect(await screen.findByText("Agenda view")).toBeInTheDocument();
    const agenda = await screen.findByRole("list", { name: "Calendar agenda" });
    const agendaTitle = within(agenda).getByText("Planner shell standup");

    expect(agendaTitle).toBeInTheDocument();
    expect(within(agenda).queryByText("Scheduled - No notes")).not.toBeInTheDocument();
    expect(agendaTitle.style.backgroundColor).toBe("rgb(52, 168, 83)");
    expect(screen.queryByRole("button", { name: "Share availability" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Share Availability" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Calendar context" })).not.toBeInTheDocument();
    expect(screen.queryByRole("group", { name: "Calendar visibility" })).not.toBeInTheDocument();

    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Day" }));
    expect(screen.getByRole("grid", { name: "Calendar day view" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "09:30-09:50 Planner shell standup" }).style.backgroundColor).toBe(
      "rgb(52, 168, 83)"
    );
    expect(screen.getByRole("button", { name: "Share availability" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Share Availability" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Share availability" }));
    expect(screen.getByRole("heading", { name: "Share Availability" })).toBeInTheDocument();

    await user.click(within(tabs).getByRole("tab", { name: "Multi-Day" }));
    expect(screen.getByRole("grid", { name: "Calendar multi-day view" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Share availability" })).toBeInTheDocument();

    await user.click(within(tabs).getByRole("tab", { name: "Week" }));
    expect(screen.getByRole("grid", { name: "Calendar week view" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Share availability" })).toBeInTheDocument();

    await user.click(within(tabs).getByRole("tab", { name: "Month" }));
    expect(screen.getByRole("grid", { name: "Calendar month view" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Share availability" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Share Availability" })).not.toBeInTheDocument();
  });

  it("renders a live current-time line only on timeline ranges containing today", async () => {
    installHcb(seededHcb());
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Day" }));

    let nowLine = document.querySelector<HTMLElement>("[data-calendar-now-line]");
    expect(nowLine).toBeInTheDocument();
    const actualMinute = Number(nowLine?.dataset.calendarNowMinute);
    const expectedNow = new Date();
    const expectedMinute = expectedNow.getUTCHours() * 60 + expectedNow.getUTCMinutes();

    expect(Math.abs(actualMinute - expectedMinute)).toBeLessThanOrEqual(1);
    expect(parseFloat(nowLine?.style.top ?? "0")).toBeCloseTo((actualMinute / 60) * 64, 1);

    await user.click(within(tabs).getByRole("tab", { name: "Multi-Day" }));
    expect(document.querySelector("[data-calendar-now-line]")).toBeInTheDocument();

    await user.click(within(tabs).getByRole("tab", { name: "Week" }));
    expect(document.querySelector("[data-calendar-now-line]")).toBeInTheDocument();

    await user.click(within(tabs).getByRole("tab", { name: "Day" }));
    await user.click(screen.getByRole("button", { name: "Next day" }));
    expect(document.querySelector("[data-calendar-now-line]")).not.toBeInTheDocument();
  });

  it("returns month view to today and scrolls the current day into view", async () => {
    const scrollIntoView = vi.fn();
    const originalScrollIntoView = HTMLElement.prototype.scrollIntoView;

    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      configurable: true,
      value: scrollIntoView
    });

    try {
      installHcb(seededHcb());
      const user = userEvent.setup();
      render(<App />);

      await goToSection("Calendar");
      const tabs = screen.getByRole("tablist", { name: "Calendar views" });
      await user.click(within(tabs).getByRole("tab", { name: "Month" }));

      const currentMonth = new Intl.DateTimeFormat(undefined, {
        month: "long",
        year: "numeric",
        timeZone: "UTC"
      }).format(new Date(`${todayDate}T00:00:00.000Z`));

      expect(screen.getByRole("button", { name: "Return calendar to today" })).toHaveTextContent(currentMonth);
      expect(document.querySelector("[data-calendar-month-today='true']")).toBeInTheDocument();

      await user.click(screen.getByRole("button", { name: "Next month" }));
      expect(screen.getByRole("button", { name: "Return calendar to today" })).not.toHaveTextContent(currentMonth);

      await user.click(within(tabs).getByRole("tab", { name: "Week" }));
      await user.click(within(tabs).getByRole("tab", { name: "Month" }));

      expect(screen.getByRole("button", { name: "Return calendar to today" })).toHaveTextContent(currentMonth);
      expect(document.querySelector("[data-calendar-month-today='true']")).toBeInTheDocument();
      await waitFor(() => expect(scrollIntoView).toHaveBeenCalled());
    } finally {
      Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
        configurable: true,
        value: originalScrollIntoView
      });
    }
  });

  it("places timed events in their display timezone hour on timeline views", async () => {
    const api = seededHcb();
    api.calendar.listEvents = vi.fn(async () =>
      ok({
        items: [
          {
            id: "event-singapore-evening",
            calendarId: "cal-product",
            title: "Singapore evening",
            startsAt: `${todayDate}T11:30:00.000Z`,
            endsAt: `${todayDate}T12:00:00.000Z`,
            allDay: false,
            timeZone: "Asia/Singapore",
            updatedAt: now
          },
          {
            id: "event-singapore-evening-2",
            calendarId: "cal-product",
            title: "Same time call",
            startsAt: `${todayDate}T11:30:00.000Z`,
            endsAt: `${todayDate}T12:00:00.000Z`,
            allDay: false,
            timeZone: "Asia/Singapore",
            updatedAt: now
          }
        ],
        page: { limit: 250, totalKnown: 2 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Day" }));

    await screen.findByRole("row", { name: "19:00 Open slot" });

    expect(screen.getByRole("button", { name: "19:30-20:00 Singapore evening" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "19:30-20:00 Same time call" })).toBeInTheDocument();

    const firstLayout = document.querySelector(
      '[data-calendar-event-layout="event-singapore-evening"]'
    ) as HTMLElement;
    const secondLayout = document.querySelector(
      '[data-calendar-event-layout="event-singapore-evening-2"]'
    ) as HTMLElement;

    expect(firstLayout).toHaveAttribute("data-start-minute", "1170");
    expect(firstLayout).toHaveAttribute("data-duration-minutes", "30");
    expect(firstLayout).toHaveAttribute("data-lane-count", "2");
    expect(firstLayout).toHaveAttribute("data-lane-index", "0");
    expect(firstLayout).toHaveStyle({ top: "1248px", height: "32px" });
    expect(secondLayout).toHaveAttribute("data-lane-count", "2");
    expect(secondLayout).toHaveAttribute("data-lane-index", "1");
  });

  it("lazy-loads retained past calendar events when navigating back", async () => {
    const api = seededHcb();
    hideTaskCalendarItems(api);
    const pastDay = addTestUtcDays(todayDate, -14);
    const pastEvent = {
      id: "event-past-retro",
      calendarId: "cal-product",
      title: "Past planning retro",
      startsAt: `${pastDay}T10:00:00.000Z`,
      endsAt: `${pastDay}T10:30:00.000Z`,
      allDay: false,
      updatedAt: now
    };
    const pastBirthday = {
      id: "event-past-birthday",
      calendarId: "cal-product",
      title: "Past birthday",
      startsAt: `${pastDay}T00:00:00.000Z`,
      endsAt: `${addTestUtcDays(pastDay, 1)}T00:00:00.000Z`,
      allDay: true,
      updatedAt: now
    };
    api.calendar.listEvents = vi.fn(async (request) =>
      ok({
        items: [
          ...(rangeOverlaps(request, `${todayDate}T09:30:00.000Z`, `${todayDate}T09:50:00.000Z`)
            ? [{
              id: "event-standup",
              calendarId: "cal-product",
              title: "Planner shell standup",
              startsAt: `${todayDate}T09:30:00.000Z`,
              endsAt: `${todayDate}T09:50:00.000Z`,
              allDay: false,
              updatedAt: now
            }]
            : []),
          ...(rangeOverlaps(request, pastEvent.startsAt, pastEvent.endsAt) ? [pastEvent] : []),
          ...(rangeOverlaps(request, pastBirthday.startsAt, pastBirthday.endsAt) ? [pastBirthday] : [])
        ],
        page: { limit: 500, totalKnown: 3 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Week" }));
    await user.click(screen.getByRole("button", { name: "Previous week" }));
    await user.click(screen.getByRole("button", { name: "Previous week" }));

    expect(await screen.findByRole("button", { name: "10:00 Past planning retro" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Past birthday" })).toBeInTheDocument();
    expect(vi.mocked(api.calendar.listEvents).mock.calls.some(([request]) =>
      rangeOverlaps(request, pastEvent.startsAt, pastEvent.endsAt)
    )).toBe(true);
  });

  it("separates all-day calendar events and summarizes dense month cells", async () => {
    const api = seededHcb();
    api.settings = {
      ...api.settings,
      get: vi.fn(async () => ok(testSettings({ selectedTaskListIds: ["list-planning"] })))
    };
    api.tasks.list = vi.fn(async () =>
      ok({
        items: [
          seededTaskDetail("task-done", {
            dueAt: now
          })
        ],
        page: { limit: 100, totalKnown: 1 }
      })
    );
    api.calendar.listEvents = vi.fn(async () =>
      ok({
        items: [
          {
            id: "event-launch-freeze",
            calendarId: "cal-product",
            title: "Launch freeze",
            startsAt: now,
            endsAt: tomorrowIso,
            allDay: true,
            updatedAt: now
          },
          {
            id: "event-design-sync",
            calendarId: "cal-product",
            title: "Design sync",
            startsAt: `${todayDate}T09:00:00.000Z`,
            endsAt: `${todayDate}T09:30:00.000Z`,
            allDay: false,
            updatedAt: now
          },
          {
            id: "event-roadmap-check",
            calendarId: "cal-product",
            title: "Roadmap check",
            startsAt: `${todayDate}T10:00:00.000Z`,
            endsAt: `${todayDate}T10:30:00.000Z`,
            allDay: false,
            updatedAt: now
          },
          {
            id: "event-partner-review",
            calendarId: "cal-product",
            title: "Partner review",
            startsAt: `${todayDate}T11:00:00.000Z`,
            endsAt: `${todayDate}T11:30:00.000Z`,
            allDay: false,
            updatedAt: now
          },
          {
            id: "event-release-notes",
            calendarId: "cal-product",
            title: "Release notes",
            startsAt: `${todayDate}T12:00:00.000Z`,
            endsAt: `${todayDate}T12:30:00.000Z`,
            allDay: false,
            updatedAt: now
          }
        ],
        page: { limit: 250, totalKnown: 5 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    expect(await screen.findByText("Agenda view")).toBeInTheDocument();

    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Day" }));

    const allDayLane = screen.getByRole("group", { name: /All-day events/ });
    expect(within(allDayLane).getByRole("button", { name: "Launch freeze" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "09:00-09:30 Design sync" })).toBeInTheDocument();

    await user.click(within(tabs).getByRole("tab", { name: "Month" }));

    const monthGrid = screen.getByRole("grid", { name: "Calendar month view" });
    expect(within(monthGrid).getByRole("button", { name: "Launch freeze" })).toBeInTheDocument();
    expect(within(monthGrid).getByRole("button", { name: "Design sync" })).toBeInTheDocument();
    expect(within(monthGrid).getByText("3 more")).toBeInTheDocument();

    await user.click(within(monthGrid).getByRole("button", { name: "Show 3 more calendar items" }));

    const overflowDialog = screen.getByRole("dialog", { name: /Items for/ });
    const overflowButtons = within(overflowDialog).getAllByRole("button").slice(1);

    expect(overflowButtons[0]).toHaveAccessibleName("Completed task Report shell-visible timing");
    expect(within(overflowDialog).getByRole("button", { name: "All day Launch freeze" })).toBeInTheDocument();
    expect(within(overflowDialog).getByRole("button", { name: "09:00-09:30 Design sync" })).toBeInTheDocument();
    expect(within(overflowDialog).getByRole("button", { name: "10:00-10:30 Roadmap check" })).toBeInTheDocument();
    expect(within(overflowDialog).getByRole("button", { name: "11:00-11:30 Partner review" })).toBeInTheDocument();
    expect(within(overflowDialog).getByRole("button", { name: "12:00-12:30 Release notes" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { level: 2, name: "New event" })).not.toBeInTheDocument();
  });

  it("opens all-day overflow without creating a draft in timeline views", async () => {
    const api = seededHcb();
    hideTaskCalendarItems(api);
    api.calendar.listEvents = vi.fn(async () =>
      ok({
        items: Array.from({ length: 5 }, (_, index) => ({
          id: `event-all-day-${index + 1}`,
          calendarId: "cal-product",
          title: `All-day ${index + 1}`,
          startsAt: `${todayDate}T00:00:00.000Z`,
          endsAt: `${tomorrowIso.slice(0, 10)}T00:00:00.000Z`,
          allDay: true,
          updatedAt: now
        })),
        page: { limit: 250, totalKnown: 5 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Day" }));
    await user.click(screen.getByRole("button", { name: "Show 1 more calendar items" }));

    const overflowDialog = screen.getByRole("dialog", { name: /Items for/ });
    expect(within(overflowDialog).getByRole("button", { name: "All day All-day 1" })).toBeInTheDocument();
    expect(within(overflowDialog).getByRole("button", { name: "All day All-day 5" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { level: 2, name: "New event" })).not.toBeInTheDocument();
  });

  it("opens and toggles due tasks directly from calendar views", async () => {
    const api = seededHcb();
    api.tasks.list = vi.fn(async () =>
      ok({
        items: [
          seededTaskDetail("task-inbox-rules"),
          seededTaskDetail("task-calendar-fixtures"),
          seededTaskDetail("task-done", { dueAt: now })
        ],
        page: { limit: 100, totalKnown: 3 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const agenda = await screen.findByRole("list", { name: "Calendar agenda" });
    expect(await within(agenda).findByText("Draft inbox triage rules")).toBeInTheDocument();
    expect(within(agenda).getByText("Review calendar fixture shape")).toBeInTheDocument();
    expect(within(agenda).getByText("Report shell-visible timing")).toBeInTheDocument();

    await user.click(within(agenda).getByText("Report shell-visible timing"));
    let inspector = await screen.findByTestId("inspector-shell");
    expect(inspector).toHaveAttribute("data-inspector-kind", "task");
    expect(inspector).toHaveAttribute("data-inspector-id", "task-done");
    expect(screen.getByText("Agenda view")).toBeInTheDocument();
    await user.click(within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Close" }));

    await user.click(within(agenda).getByRole("button", { name: "Mark Draft inbox triage rules complete" }));
    expect(api.tasks.complete).toHaveBeenCalledWith({ id: "task-inbox-rules" });
    expect(screen.queryByTestId("inspector-shell")).not.toBeInTheDocument();

    await user.click(within(agenda).getByRole("button", { name: "Reopen Report shell-visible timing" }));
    expect(api.tasks.reopen).toHaveBeenCalledWith({ id: "task-done" });

    const visibility = screen.getByRole("group", { name: "Sidebar calendar visibility" });
    await user.click(within(visibility).getByLabelText("Hide Product"));

    await waitFor(() => {
      expect(within(agenda).queryByText("Planner shell standup")).not.toBeInTheDocument();
      expect(within(agenda).getByText("Draft inbox triage rules")).toBeInTheDocument();
    });

    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Day" }));
    const allDayLane = screen.getByRole("group", { name: /All-day events/ });
    await user.click(within(allDayLane).getByRole("button", { name: "Draft inbox triage rules" }));

    inspector = await screen.findByTestId("inspector-shell");
    expect(inspector).toHaveAttribute("data-inspector-kind", "task");
    expect(inspector).toHaveAttribute("data-inspector-id", "task-inbox-rules");
    expect(screen.getByRole("grid", { name: "Calendar day view" })).toBeInTheDocument();
    await user.click(within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Close" }));

    await user.click(within(tabs).getByRole("tab", { name: "Month" }));
    const monthGrid = screen.getByRole("grid", { name: "Calendar month view" });
    await user.click(within(monthGrid).getByRole("button", { name: "Mark Review calendar fixture shape complete" }));
    expect(api.tasks.complete).toHaveBeenCalledWith({ id: "task-calendar-fixtures" });
  });

  it("renders linked Google Calendar task projections as timed tasks", async () => {
    const api = seededHcb();
    const title = "Timed linked task";
    api.tasks.list = vi.fn(async () =>
      ok({
        items: [
          seededTaskDetail("task-lawnet", {
            title,
            dueAt: `${todayDate}T00:00:00.000Z`,
            status: "completed",
            notes: "Task body"
          })
        ],
        page: { limit: 100, totalKnown: 1 }
      })
    );
    api.calendar.listEvents = vi.fn(async () =>
      ok({
        items: [{
          id: "event-task-projection",
          eventId: "event-task-projection",
          linkedTaskId: "task-lawnet",
          calendarId: "cal-product",
          title,
          startsAt: `${todayDate}T07:00:00.000Z`,
          endsAt: `${todayDate}T07:30:00.000Z`,
          allDay: false,
          notes: "Changes made to the title, description or attachments will not be saved. To make edits, please go to: https://tasks.google.com/task/1u__Kt1mliHUFzMf",
          updatedAt: now
        }],
        page: { limit: 500, totalKnown: 1 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Day" }));

    const timedTask = await screen.findByRole("button", { name: `07:00-07:30 ${title}` });
    expect(within(timedTask).getByText(`07:00-07:30 ${title}`)).toHaveClass("line-through");
    expect(screen.queryByRole("button", { name: title })).not.toBeInTheDocument();

    await user.click(timedTask);
    const inspector = await screen.findByTestId("inspector-shell");
    expect(inspector).toHaveAttribute("data-inspector-kind", "task");
    expect(inspector).toHaveAttribute("data-inspector-id", "task-lawnet");
    expect(screen.queryByRole("button", { name: "Delete event" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: `Reopen ${title}` }));
    expect(api.tasks.reopen).toHaveBeenCalledWith({ id: "task-lawnet" });
  });

  it("removes linked Google Calendar task projections when deleting the task from calendar", async () => {
    const api = seededHcb();
    const title = "Timed linked task";
    api.tasks.list = vi.fn(async () =>
      ok({
        items: [
          seededTaskDetail("task-lawnet", {
            title,
            dueAt: `${todayDate}T00:00:00.000Z`,
            notes: "Task body"
          })
        ],
        page: { limit: 100, totalKnown: 1 }
      })
    );
    api.calendar.listEvents = vi.fn(async () =>
      ok({
        items: [{
          id: "event-task-projection",
          eventId: "event-task-projection",
          linkedTaskId: "task-lawnet",
          calendarId: "cal-product",
          title,
          startsAt: `${todayDate}T07:00:00.000Z`,
          endsAt: `${todayDate}T07:30:00.000Z`,
          allDay: false,
          notes: "Changes made to the title, description or attachments will not be saved. To make edits, please go to: https://tasks.google.com/task/1u__Kt1mliHUFzMf",
          updatedAt: now
        }],
        page: { limit: 500, totalKnown: 1 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    await user.click(within(screen.getByRole("tablist", { name: "Calendar views" })).getByRole("tab", { name: "Day" }));
    await user.click(await screen.findByRole("button", { name: `07:00-07:30 ${title}` }));
    expect(await screen.findByTestId("inspector-shell")).toHaveAttribute("data-inspector-kind", "task");

    await user.click(within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Delete" }));

    expect(api.tasks.delete).toHaveBeenCalledWith({ id: "task-lawnet" });
    expect(api.calendar.delete).not.toHaveBeenCalled();
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: `07:00-07:30 ${title}` })).not.toBeInTheDocument();
    });
    expect(screen.queryByTestId("inspector-shell")).not.toBeInTheDocument();
  });

  it("renders multi-day all-day events as one spanning timeline segment", async () => {
    const api = seededHcb();
    api.calendar.listEvents = vi.fn(async () =>
      ok({
        items: [
          {
            id: "event-sleepover",
            calendarId: "cal-product",
            title: "Sleepover",
            startsAt: `${todayDate}T00:00:00.000Z`,
            endsAt: `${addTestUtcDays(todayDate, 3)}T00:00:00.000Z`,
            allDay: true,
            updatedAt: now
          }
        ],
        page: { limit: 250, totalKnown: 1 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Multi-Day" }));

    expect(await screen.findByRole("grid", { name: "Calendar multi-day view" })).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Sleepover" })).toHaveLength(1);

    const segment = document.querySelector('[data-calendar-all-day-segment="event-sleepover"]') as HTMLElement;

    expect(segment).toHaveAttribute("data-start-day-index", "0");
    expect(segment).toHaveAttribute("data-day-span", "3");
    expect(segment).toHaveAttribute("data-lane-index", "0");

    await user.click(within(tabs).getByRole("tab", { name: "Month" }));

    const monthGrid = await screen.findByRole("grid", { name: "Calendar month view" });

    const monthButtons = within(monthGrid).getAllByRole("button", { name: "Sleepover" });
    const monthSegments = Array.from(document.querySelectorAll(
      '[data-calendar-month-all-day-segment="event-sleepover"]'
    )) as HTMLElement[];

    expect(monthButtons).toHaveLength(monthSegments.length);
    expect(
      monthSegments.reduce((total, monthSegment) => total + Number(monthSegment.dataset.daySpan ?? 0), 0)
    ).toBe(3);
  });

  it("opens calendar creation from keyboard-focused grid cells", async () => {
    installHcb(seededHcb());
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    expect(await screen.findByText("Agenda view")).toBeInTheDocument();

    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Week" }));
    const grid = screen.getByRole("grid", { name: "Calendar week view" });
    const firstCell = within(grid).getAllByRole("gridcell")[0];

    firstCell.focus();
    fireEvent.keyDown(firstCell, { key: "Enter" });

    expect(await screen.findByRole("heading", { level: 2, name: "New event" })).toBeInTheDocument();
    expect(screen.getByTestId("inspector-shell")).toHaveAttribute("data-inspector-kind", "event");
    expect(screen.getByRole("button", { name: "New event" })).toHaveAttribute(
      "data-action-id",
      "calendar.create"
    );
  });

  it("creates all-day month range drafts from pointer drags", async () => {
    installHcb(seededHcb());
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Month" }));

    const monthGrid = screen.getByRole("grid", { name: "Calendar month view" });
    const rows = Array.from(monthGrid.querySelectorAll<HTMLElement>("[data-calendar-month-week-row]"));
    const cells = within(monthGrid).getAllByRole("gridcell");

    expect(rows).toHaveLength(6);
    rows.forEach((row, rowIndex) => {
      Object.defineProperty(row, "getBoundingClientRect", {
        configurable: true,
        value: () => ({
          bottom: (rowIndex + 1) * 140,
          height: 140,
          left: 0,
          right: 700,
          top: rowIndex * 140,
          width: 700,
          x: 0,
          y: rowIndex * 140,
          toJSON: () => ({})
        })
      });
    });
    const dispatchPointer = (
      target: Element,
      type: string,
      init: { button?: number; buttons: number; clientX: number; clientY: number; pointerId: number }
    ): void => {
      const event = new Event(type, { bubbles: true, cancelable: true });

      Object.defineProperties(event, {
        button: { value: init.button ?? 0 },
        buttons: { value: init.buttons },
        clientX: { value: init.clientX },
        clientY: { value: init.clientY },
        pointerId: { value: init.pointerId }
      });
      fireEvent(target, event);
    };

    dispatchPointer(cells[1], "pointerdown", { button: 0, buttons: 1, clientX: 150, clientY: 40, pointerId: 1 });
    dispatchPointer(cells[3], "pointermove", { buttons: 1, clientX: 350, clientY: 40, pointerId: 1 });
    dispatchPointer(cells[3], "pointerup", { button: 0, buttons: 0, clientX: 350, clientY: 40, pointerId: 1 });

    expect(await screen.findByRole("heading", { level: 2, name: "New event" })).toBeInTheDocument();
    const startsInput = screen.getByLabelText("Event starts") as HTMLInputElement;
    const endsInput = screen.getByLabelText("Event ends") as HTMLInputElement;

    expect(startsInput).toHaveAttribute("type", "date");
    expect(startsInput.value).not.toBe(endsInput.value);
  });

  it("creates timed calendar drafts from day planning slots", async () => {
    installHcb(seededHcb());
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    expect(await screen.findByText("Agenda view")).toBeInTheDocument();

    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Day" }));
    await user.click(screen.getByRole("button", { name: "Create event at 11:00" }));

    expect(await screen.findByRole("heading", { level: 2, name: "New event" })).toBeInTheDocument();
    expect(screen.getByTestId("inspector-shell")).toHaveAttribute("data-inspector-kind", "event");
    expect(screen.getByLabelText("Event starts")).toHaveValue(`${todayDate}T11:00`);
    expect(screen.getByLabelText("Event ends")).toHaveValue(`${todayDate}T12:00`);
  });

  it("preserves selected calendar timezone when saving timed event fields", async () => {
    const api = seededHcb();
    api.calendar.listCalendars = vi.fn(async () =>
      ok({
        items: [
          {
            id: "cal-product",
            title: "Product",
            selected: true,
            timeZone: "Asia/Singapore",
            backgroundColor: "#34a853",
            foregroundColor: "#ffffff",
            updatedAt: now,
            eventCount: 1
          }
        ],
        page: { limit: 100, totalKnown: 1 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    await user.click(screen.getByRole("button", { name: /New event/ }));
    await user.type(await screen.findByRole("textbox", { name: "Event title" }), "Singapore hold");
    fireEvent.change(screen.getByLabelText("Event starts"), { target: { value: "2026-06-01T10:00" } });
    fireEvent.change(screen.getByLabelText("Event ends"), { target: { value: "2026-06-02T10:00" } });
    const localRangeLabel = "2026-06-01 · 10:00-2026-06-02 · 10:00";

    await waitFor(() => {
      expect(screen.getByText(`Product · ${localRangeLabel}`)).toBeInTheDocument();
    });
    expect(screen.getByText(localRangeLabel)).toBeInTheDocument();
    expect(screen.getByTestId("inspector-shell")).not.toHaveTextContent("02:00");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(api.calendar.create).toHaveBeenCalledWith(
        expect.objectContaining({
          startsAt: "2026-06-01T02:00:00.000Z",
          endsAt: "2026-06-02T02:00:00.000Z"
        })
      );
    });
  });

  it("closes dirty birthday creation from the inspector titlebar close button", async () => {
    installHcb(seededHcb());
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    await user.click(screen.getByRole("button", { name: "New event" }));
    await user.click(await screen.findByRole("tab", { name: "Birthday" }));
    expect(await screen.findByRole("heading", { level: 2, name: "New birthday" })).toBeInTheDocument();

    await user.type(screen.getByRole("textbox", { name: "Birthday title" }), "Alex");
    expect(screen.getByLabelText("Birthday reminder")).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Event location" })).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Event guests" })).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Event notes" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Event repeat frequency")).not.toBeInTheDocument();
    await user.click(screen.getByTestId("inspector-close"));

    expect(screen.queryByTestId("inspector-shell")).not.toBeInTheDocument();
  });

  it("edits birthday events with strict birthday fields", async () => {
    const api = seededHcb();
    api.calendar.listEvents = vi.fn(async () =>
      ok({
        items: [
          {
            id: "event-birthday-edit",
            hcbKind: "birthday" as const,
            calendarId: "cal-product",
            title: "Alex",
            startsAt: `${todayDate}T00:00:00.000Z`,
            endsAt: `${tomorrowIso.slice(0, 10)}T00:00:00.000Z`,
            allDay: true,
            reminderMinutes: [15],
            recurrenceRule: "RRULE:FREQ=YEARLY",
            updatedAt: now
          }
        ],
        page: { limit: 250, totalKnown: 1 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    await user.click(within(await screen.findByRole("list", { name: "Calendar agenda" })).getByText("Alex"));
    await user.click(within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Edit" }));

    expect(await screen.findByRole("textbox", { name: "Birthday title" })).toHaveValue("Alex");
    expect(screen.getByLabelText("Birthday date")).toBeInTheDocument();
    expect(screen.getByLabelText("Birthday calendar")).toBeInTheDocument();
    expect(screen.getByLabelText("Birthday reminder")).toHaveValue("15");
    expect(screen.queryByRole("textbox", { name: "Event location" })).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Event guests" })).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Event notes" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Event repeat frequency")).not.toBeInTheDocument();
  });

  it("opens calendar events in the inspector", async () => {
    installHcb(seededHcb());
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    expect(await screen.findByText("Agenda view")).toBeInTheDocument();

    const agenda = await screen.findByRole("list", { name: "Calendar agenda" });
    await user.click(within(agenda).getByText("Planner shell standup"));

    const inspector = await screen.findByTestId("inspector-shell");
    expect(inspector).toHaveAttribute("data-inspector-kind", "event");
    expect(inspector).toHaveAttribute("data-inspector-id", "event-standup");

    const inspectorBody = within(inspector).getByTestId("inspector-body");
    expect(within(inspectorBody).getByRole("heading", { name: "Planner shell standup" })).toBeInTheDocument();
    expect(within(inspectorBody).getByText("Product")).toBeInTheDocument();
    expect(within(inspectorBody).getAllByText(new RegExp(`${todayDate}.*09:30-09:50`)).length).toBeGreaterThan(0);
    expect(within(inspectorBody).getByText("20 min")).toBeInTheDocument();
    expect(within(inspectorBody).queryByText("UTC")).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Event title" })).not.toBeInTheDocument();

    await user.click(
      within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Edit" })
    );
    expect(await screen.findByRole("textbox", { name: "Event title" })).toHaveValue("Planner shell standup");
  });

  it("duplicates events as editable drafts while clearing guests and conference data", async () => {
    const api = seededHcb();
    api.calendar.listEvents = vi.fn(async () =>
      ok({
        items: [
          {
            id: "event-duplicate-source",
            calendarId: "cal-product",
            title: "Duplicate source sync",
            startsAt: `${todayDate}T08:30:00.000Z`,
            endsAt: `${todayDate}T09:00:00.000Z`,
            allDay: false,
            timeZone: "Asia/Singapore",
            location: "Room 4",
            notes: "Bring notes.",
            guestEmails: ["ada@example.com"],
            reminderMinutes: [15],
            colorId: "9",
            recurrenceRule: "RRULE:FREQ=WEEKLY;INTERVAL=2;BYDAY=MO",
            conference: {
              solutionName: "Google Meet",
              videoUri: "https://meet.google.com/source",
              videoLabel: "meet.google.com/source"
            },
            updatedAt: now
          }
        ],
        page: { limit: 250, totalKnown: 1 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const agenda = await screen.findByRole("list", { name: "Calendar agenda" });
    await user.click(within(agenda).getByText("Duplicate source sync"));
    await user.click(within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Duplicate" }));

    expect(await screen.findByRole("heading", { level: 2, name: "New event" })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Event title" })).toHaveValue("Duplicate source sync (copy)");
    expect(screen.getByRole("textbox", { name: "Event guests" })).toHaveValue("");

    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(api.calendar.create).toHaveBeenCalledWith(
        expect.objectContaining({
          title: "Duplicate source sync (copy)",
          calendarId: "cal-product",
          colorId: "9",
          location: "Room 4",
          notes: "Bring notes.",
          guestEmails: [],
          reminderMinutes: [15],
          timeZone: "Asia/Singapore",
          recurrence: {
            frequency: "weekly",
            interval: 2,
            byDay: ["MO"],
            endsOn: null,
            count: null
          }
        })
      );
    });
  });

  it("duplicates birthday-style events as birthday drafts", async () => {
    const api = seededHcb();
    api.calendar.listEvents = vi.fn(async () =>
      ok({
        items: [
          {
            id: "event-birthday-source",
            hcbKind: "birthday" as const,
            calendarId: "cal-product",
            title: "Alex",
            startsAt: `${todayDate}T00:00:00.000Z`,
            endsAt: `${tomorrowIso.slice(0, 10)}T00:00:00.000Z`,
            allDay: true,
            recurrenceRule: "RRULE:FREQ=YEARLY",
            updatedAt: now
          }
        ],
        page: { limit: 250, totalKnown: 1 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const agenda = await screen.findByRole("list", { name: "Calendar agenda" });
    await user.click(within(agenda).getByText("Alex"));
    await user.click(within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Duplicate" }));

    expect(await screen.findByRole("heading", { level: 2, name: "New birthday" })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Birthday title" })).toHaveValue("Alex (copy)");

    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(api.calendar.create).toHaveBeenCalledWith(
        expect.objectContaining({
          title: "Alex (copy)",
          allDay: true,
          hcbKind: "birthday",
          recurrence: {
            frequency: "yearly",
            interval: 1,
            endsOn: null,
            count: null
          }
        })
      );
    });
  });

  it("hides event pending mutation state in rows and inspector", async () => {
    const api = seededHcb();
    api.calendar.listEvents = vi.fn(async () =>
      ok({
        items: [
          {
            id: "event-standup",
            calendarId: "cal-product",
            title: "Planner shell standup",
            startsAt: `${todayDate}T09:30:00.000Z`,
            endsAt: `${todayDate}T09:50:00.000Z`,
            allDay: false,
            mutationState: "queued" as const,
            updatedAt: now
          }
        ],
        page: { limit: 250, totalKnown: 1 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const agenda = await screen.findByRole("list", { name: "Calendar agenda" });
    expect(within(agenda).queryByText("Queued")).not.toBeInTheDocument();

    await user.click(within(agenda).getByText("Planner shell standup"));
    const inspector = await screen.findByTestId("inspector-shell");

    expect(within(inspector).queryByText("Queued")).not.toBeInTheDocument();
  });

  it("flags failed Google writes through notifications instead of calendar rows", async () => {
    const api = seededHcb();
    api.calendar.listEvents = vi.fn(async () =>
      ok({
        items: [
          {
            id: "event-standup",
            calendarId: "cal-product",
            title: "Planner shell standup",
            startsAt: `${todayDate}T09:30:00.000Z`,
            endsAt: `${todayDate}T09:50:00.000Z`,
            allDay: false,
            mutationState: "failed" as const,
            updatedAt: now
          }
        ],
        page: { limit: 250, totalKnown: 1 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const agenda = await screen.findByRole("list", { name: "Calendar agenda" });
    expect(within(agenda).queryByText("Failed")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Notifications,/ }));
    expect(await screen.findByText("Some changes need attention")).toBeInTheDocument();
  });

  it("renders Meet and guest details only when present", async () => {
    const api = seededHcb();
    api.calendar.listEvents = vi.fn(async () =>
      ok({
        items: [
          {
            id: "event-meet",
            calendarId: "cal-product",
            title: "Team Elefant: Stand-up",
            startsAt: `${todayDate}T08:30:00.000Z`,
            endsAt: `${todayDate}T09:00:00.000Z`,
            allDay: false,
            guestEmails: ["krishna@example.com", "gabriel@example.com"],
            conference: {
              solutionName: "Google Meet",
              videoUri: "https://meet.google.com/nrf-pwpu-cws",
              videoLabel: "meet.google.com/nrf-pwpu-cws",
              phoneUri: "tel:+14017539584,,,708190980#",
              phoneLabel: "(US) +1 401-753-9584",
              phonePin: "708 190 980#",
              moreUri: "https://tel.meet/nrf-pwpu-cws"
            },
            updatedAt: now
          }
        ],
        page: { limit: 250, totalKnown: 1 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const agenda = await screen.findByRole("list", { name: "Calendar agenda" });
    await user.click(within(agenda).getByText("Team Elefant: Stand-up"));
    const inspectorBody = within(await screen.findByTestId("inspector-shell")).getByTestId("inspector-body");

    expect(within(inspectorBody).getByText("Join with Google Meet")).toBeInTheDocument();
    expect(within(inspectorBody).getByText("meet.google.com/nrf-pwpu-cws")).toBeInTheDocument();
    expect(within(inspectorBody).getByText("Join by phone")).toBeInTheDocument();
    expect(within(inspectorBody).getByText("(US) +1 401-753-9584 PIN: 708 190 980#")).toBeInTheDocument();
    expect(within(inspectorBody).getByText("krishna@example.com")).toBeInTheDocument();
    expect(within(inspectorBody).queryByText("No guests")).not.toBeInTheDocument();
  });

  it("keeps a dirty calendar event inspector open on Escape", async () => {
    installHcb(seededHcb());
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    expect(await screen.findByText("Agenda view")).toBeInTheDocument();
    const agenda = await screen.findByRole("list", { name: "Calendar agenda" });
    await user.click(within(agenda).getByText("Planner shell standup"));

    const inspector = await screen.findByTestId("inspector-shell");
    const inspectorBody = within(inspector).getByTestId("inspector-body");
    expect(within(inspectorBody).getByRole("heading", { name: "Planner shell standup" })).toBeInTheDocument();
    expect(within(inspectorBody).queryByText("Repeat")).not.toBeInTheDocument();
    expect(within(inspectorBody).queryByText("Does not repeat")).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Event title" })).not.toBeInTheDocument();
    await user.click(
      within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Edit" })
    );
    const titleInput = await screen.findByRole("textbox", { name: "Event title" });
    await user.clear(titleInput);
    await user.type(titleInput, "Planner shell sync");
    await user.keyboard("{Escape}");

    expect(screen.getByTestId("inspector-shell")).toHaveAttribute("data-inspector-kind", "event");
    expect(screen.getByText("Unsaved")).toBeInTheDocument();
  });

  it("filters calendar views by visible calendar source", async () => {
    const api = seededHcb();
    api.calendar.listCalendars = vi.fn(async () =>
      ok({
        items: [
          {
            id: "cal-product",
            title: "Product",
            selected: true,
            timeZone: "UTC",
            backgroundColor: "#34a853",
            foregroundColor: "#ffffff",
            updatedAt: now,
            eventCount: 1
          },
          {
            id: "cal-engineering",
            title: "Engineering",
            selected: true,
            timeZone: "UTC",
            backgroundColor: "#fbbc04",
            foregroundColor: "#202124",
            updatedAt: now,
            eventCount: 1
          }
        ],
        page: { limit: 100, totalKnown: 2 }
      })
    );
    api.calendar.listEvents = vi.fn(async () =>
      ok({
        items: [
          {
            id: "event-standup",
            calendarId: "cal-product",
            title: "Planner shell standup",
            startsAt: `${todayDate}T09:30:00.000Z`,
            endsAt: `${todayDate}T09:50:00.000Z`,
            allDay: false,
            updatedAt: now
          },
          {
            id: "event-engineering-sync",
            calendarId: "cal-engineering",
            title: "Engineering sync",
            startsAt: `${todayDate}T10:30:00.000Z`,
            endsAt: `${todayDate}T11:00:00.000Z`,
            allDay: false,
            updatedAt: now
          }
        ],
        page: { limit: 250, totalKnown: 2 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const agenda = await screen.findByRole("list", { name: "Calendar agenda" });
    expect(await within(agenda).findByText("Planner shell standup")).toBeInTheDocument();
    expect(within(agenda).getByText("Engineering sync")).toBeInTheDocument();
    const status = screen.getByRole("status", { name: "Calendar status" });
    expect(status).toBeInTheDocument();
    expect(within(status).queryByText(/\d+ events/)).not.toBeInTheDocument();
    expect(within(status).queryByText(/Default timezone/)).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Calendar context" })).not.toBeInTheDocument();

    const visibility = screen.getByRole("group", { name: "Sidebar calendar visibility" });
    await user.click(within(visibility).getByLabelText("Hide Product"));

    await waitFor(() => {
      expect(within(agenda).queryByText("Planner shell standup")).not.toBeInTheDocument();
      expect(within(agenda).getByText("Engineering sync")).toBeInTheDocument();
      expect(within(visibility).getByLabelText("Show Product")).toBeInTheDocument();
    });
  });

  it("drags and resizes calendar events in the day planning grid", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    expect(await screen.findByText("Agenda view")).toBeInTheDocument();

    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Day" }));

    const eventButton = screen.getByRole("button", {
      name: "09:30-09:50 Planner shell standup"
    });
    eventButton.focus();
    await user.keyboard("{ArrowDown}");

    await waitFor(() => {
      expect(api.calendar.update).toHaveBeenCalledWith({
        id: "event-standup",
        startsAt: `${todayDate}T09:45:00.000Z`,
        endsAt: `${todayDate}T10:05:00.000Z`,
        allDay: false
      });
    });
    expect(screen.queryByTestId("inspector-shell")).not.toBeInTheDocument();
    vi.mocked(api.calendar.update).mockClear();

    eventButton.focus();
    await user.keyboard("{ArrowUp}");

    await waitFor(() => {
      expect(api.calendar.update).toHaveBeenCalledWith({
        id: "event-standup",
        startsAt: `${todayDate}T09:15:00.000Z`,
        endsAt: `${todayDate}T09:35:00.000Z`,
        allDay: false
      });
    });
    expect(screen.queryByTestId("inspector-shell")).not.toBeInTheDocument();
    vi.mocked(api.calendar.update).mockClear();

    const occupiedTransfer = testDataTransfer();
    const occupiedTarget = [...document.querySelectorAll<HTMLElement>("[data-calendar-event-layout]")]
      .find((element) => element.getAttribute("data-calendar-event-layout") !== "event-standup") ?? null;
    const occupiedDay = occupiedTarget?.closest<HTMLElement>("[data-calendar-day-events]");

    expect(occupiedTarget).not.toBeNull();
    expect(occupiedDay).not.toBeNull();
    vi.spyOn(occupiedDay as HTMLElement, "getBoundingClientRect").mockReturnValue({
      bottom: 1536,
      height: 1536,
      left: 0,
      right: 160,
      top: 0,
      width: 160,
      x: 0,
      y: 0,
      toJSON: () => ({})
    });
    fireEvent.dragStart(eventButton, { dataTransfer: occupiedTransfer });
    fireEvent.dragOver(occupiedTarget as HTMLElement, {
      clientY: 11 * 64,
      dataTransfer: occupiedTransfer
    });
    expect(document.querySelector("[data-calendar-drop-preview]")).toBeInTheDocument();
    fireEvent.drop(occupiedTarget as HTMLElement, {
      clientY: 11 * 64,
      dataTransfer: occupiedTransfer
    });

    await waitFor(() => {
      expect(api.calendar.update).toHaveBeenCalledWith({
        id: "event-standup",
        startsAt: `${todayDate}T00:00:00.000Z`,
        endsAt: `${todayDate}T00:20:00.000Z`,
        allDay: false
      });
    });
    vi.mocked(api.calendar.update).mockClear();

    const moveTransfer = testDataTransfer();
    const moveTarget = screen.getByRole("row", { name: "11:00 Open slot" });

    fireEvent.dragStart(eventButton, { dataTransfer: moveTransfer });
    fireEvent.dragOver(moveTarget, { dataTransfer: moveTransfer });
    fireEvent.drop(moveTarget, { dataTransfer: moveTransfer });

    await waitFor(() => {
      expect(api.calendar.update).toHaveBeenCalledWith({
        id: "event-standup",
        startsAt: `${todayDate}T11:00:00.000Z`,
        endsAt: `${todayDate}T11:20:00.000Z`,
        allDay: false
      });
    });
    expect(screen.queryByTestId("inspector-shell")).not.toBeInTheDocument();

    const resizeHandle = screen.getByRole("button", { name: "Resize Planner shell standup end" });
    const resizeTransfer = testDataTransfer();
    const resizeTarget = screen.getByRole("row", { name: "12:00 Open slot" });

    fireEvent.dragStart(resizeHandle, { dataTransfer: resizeTransfer });
    fireEvent.dragOver(resizeTarget, { dataTransfer: resizeTransfer });
    fireEvent.drop(resizeTarget, { dataTransfer: resizeTransfer });

    await waitFor(() => {
      expect(api.calendar.update).toHaveBeenCalledWith({
        id: "event-standup",
        endsAt: `${todayDate}T12:00:00.000Z`
      });
    });
    expect(screen.queryByTestId("inspector-shell")).not.toBeInTheDocument();
  });

  it("drags calendar events across week days while preserving time", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    expect(await screen.findByText("Agenda view")).toBeInTheDocument();

    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Week" }));

    const eventButton = screen.getByRole("button", {
      name: "09:30 Planner shell standup"
    });
    const weekGrid = screen.getByRole("grid", { name: "Calendar week view" });
    const targetDay = within(weekGrid).getAllByRole("gridcell")[0];
    const transfer = testDataTransfer();
    const targetDate = utcWeekStartDate(now);

    fireEvent.dragStart(eventButton, { dataTransfer: transfer });
    fireEvent.dragOver(targetDay, { dataTransfer: transfer });
    fireEvent.drop(targetDay, { dataTransfer: transfer });

    await waitFor(() => {
      expect(api.calendar.update).toHaveBeenCalledWith({
        id: "event-standup",
        startsAt: `${targetDate}T09:30:00.000Z`,
        endsAt: `${targetDate}T09:50:00.000Z`,
        allDay: false
      });
    });
  });

  it("creates, edits, and deletes calendar events through preload", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    expect(await screen.findByText("Agenda view")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /New event/ }));
    await user.type(screen.getByRole("textbox", { name: "Event title" }), "Design review");
    fireEvent.change(screen.getByLabelText("Event starts"), { target: { value: `${todayDate}T11:00` } });
    fireEvent.change(screen.getByLabelText("Event ends"), { target: { value: `${todayDate}T12:00` } });
    await user.type(screen.getByRole("textbox", { name: "Event location" }), "Room 3");
    await user.type(screen.getByRole("textbox", { name: "Event guests" }), "ada@example.com");
    await user.selectOptions(screen.getByLabelText("Event color"), "9");
    await user.selectOptions(screen.getByLabelText("Event reminder mode"), "custom");
    expect(screen.getByLabelText("Reminder 1 offset")).toHaveValue("10 mins");
    fireEvent.change(screen.getByLabelText("Reminder 1 offset"), { target: { value: "15 mins" } });
    await user.selectOptions(screen.getByLabelText("Event repeat frequency"), "custom");
    await user.selectOptions(screen.getByLabelText("Repeat unit"), "weekly");
    fireEvent.change(screen.getByLabelText("Repeat interval"), { target: { value: "2" } });
    await user.click(screen.getByLabelText("Repeat on date"));
    fireEvent.change(screen.getByLabelText("Repeat end date"), { target: { value: "2026-12-31" } });
    expect(screen.getByText(/Every 2 weeks/)).toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "Event notes" }), "Bring mocks.");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(api.calendar.create).toHaveBeenCalledWith(
        expect.objectContaining({
          title: "Design review",
          calendarId: "cal-product",
          startsAt: `${todayDate}T11:00:00.000Z`,
          endsAt: `${todayDate}T12:00:00.000Z`,
          allDay: false,
          colorId: "9",
          location: "Room 3",
          notes: "Bring mocks.",
          guestEmails: ["ada@example.com"],
          reminderMinutes: [15],
          reminders: [{ method: "popup", minutes: 15 }],
          remindersUseDefault: false,
          recurrence: {
            frequency: "weekly",
            interval: 2,
            byDay: expect.any(Array),
            endsOn: "2026-12-31",
            count: null
          }
        })
      );
    });

    const agenda = await screen.findByRole("list", { name: "Calendar agenda" });
    await user.click(within(agenda).getByText("Planner shell standup"));
    const inspector = await screen.findByTestId("inspector-shell");
    const inspectorBody = within(inspector).getByTestId("inspector-body");
    expect(within(inspectorBody).getByRole("heading", { name: "Planner shell standup" })).toBeInTheDocument();
    expect(within(inspector).queryByText("Attachments")).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Event title" })).not.toBeInTheDocument();
    await user.click(
      within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Edit" })
    );
    expect(await within(inspector).findByText("Attachments")).toBeInTheDocument();
    const titleInput = screen.getByRole("textbox", { name: "Event title" });
    await user.clear(titleInput);
    await user.type(titleInput, "Planner shell sync");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(api.calendar.update).toHaveBeenCalledWith(
        expect.objectContaining({
          id: "event-standup",
          title: "Planner shell sync"
        })
      );
    });

    await user.click(within(agenda).getByText("Planner shell standup"));
    const deleteButton = screen.getByRole("button", { name: "Delete event" });
    expect(deleteButton.className).toContain("ring-danger");
    await user.click(deleteButton);
    expect(api.calendar.delete).toHaveBeenCalledWith({ id: "event-standup" });
  });

  it("renders existing event notes as markdown in read-only details", async () => {
    const api = seededHcb();
    api.calendar.listEvents = vi.fn(async () =>
      ok({
        items: [
          {
            id: "event-markdown",
            calendarId: "cal-product",
            title: "Markdown planning event",
            startsAt: `${todayDate}T09:30:00.000Z`,
            endsAt: `${todayDate}T10:00:00.000Z`,
            allDay: false,
            notes: "# Plan\n\n1. **Full-time** option\n2. [Docs](https://example.com)",
            updatedAt: now
          }
        ],
        page: { limit: 250, totalKnown: 1 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const agenda = await screen.findByRole("list", { name: "Calendar agenda" });
    await user.click(within(agenda).getByText("Markdown planning event"));

    const inspector = await screen.findByTestId("inspector-shell");
    const preview = within(inspector).getByRole("region", { name: "Event notes preview" });
    expect(within(preview).getByRole("heading", { name: "Plan" })).toBeInTheDocument();
    expect(within(preview).getByText("Full-time")).toBeInTheDocument();
    const docsLink = within(preview).getByRole("link", { name: "Docs" });
    expect(docsLink).toHaveAttribute("href", "https://example.com");
    expect(within(inspector).queryByText("Attachments")).not.toBeInTheDocument();

    await user.click(docsLink);

    const webPane = await screen.findByRole("region", { name: "Docs pane" });
    expect(within(webPane).getByRole("heading", { name: "Docs" })).toBeInTheDocument();
    expect(within(webPane).getByTestId("split-webview")).toHaveAttribute("src", "https://example.com/");
  });

  it("loads existing event recurrence into the inspector and persists recurrence changes", async () => {
    const api = seededHcb();
    api.calendar.listEvents = vi.fn(async () =>
      ok({
        items: [
          {
            id: "event-recurring-review",
            calendarId: "cal-product",
            title: "Recurring release review",
            startsAt: `${todayDate}T13:00:00.000Z`,
            endsAt: `${todayDate}T14:00:00.000Z`,
            allDay: false,
            updatedAt: now,
            reminderMinutes: [420],
            recurrenceRule: "RRULE:FREQ=MONTHLY;INTERVAL=2;COUNT=4"
          }
        ],
        page: { limit: 250, totalKnown: 1 }
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const agenda = await screen.findByRole("list", { name: "Calendar agenda" });
    await user.click(await within(agenda).findByText("Recurring release review"));

    const inspector = await screen.findByTestId("inspector-shell");
    const inspectorBody = within(inspector).getByTestId("inspector-body");
    const recurrenceSummary = `Every 2 months on day ${new Date(`${todayDate}T00:00:00.000Z`).getUTCDate()}, 4 times`;
    expect(inspector).toHaveAttribute("data-inspector-kind", "event");
    expect(within(inspectorBody).getByText(recurrenceSummary)).toBeInTheDocument();
    expect(within(inspectorBody).getByText("Popup 7 hr 0 min before")).toBeInTheDocument();
    expect(screen.queryByLabelText("Event repeat frequency")).not.toBeInTheDocument();
    await user.click(
      within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Edit" })
    );
    expect(screen.getByLabelText("Event repeat frequency")).toHaveValue("custom");
    expect(screen.getByLabelText("Repeat unit")).toHaveValue("monthly");
    expect(screen.getByLabelText("Repeat interval")).toHaveValue(2);
    expect(screen.getByLabelText("Repeat count")).toHaveValue(4);
    expect(screen.getByText(recurrenceSummary)).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText("Event repeat frequency"), "none");
    expect(screen.queryByLabelText("Repeat interval")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(api.calendar.update).toHaveBeenCalledWith(
        expect.objectContaining({
          id: "event-recurring-review",
          recurrence: null
        })
      );
    });
  });

  it("generates static availability from selected calendar sources", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    expect(await screen.findByText("Agenda view")).toBeInTheDocument();
    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Day" }));
    await user.click(screen.getByRole("button", { name: "Share availability" }));
    expect(await screen.findByRole("heading", { name: "Share Availability" })).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Availability start"), {
      target: { value: todayDate }
    });
    fireEvent.change(screen.getByLabelText("Availability end"), {
      target: { value: todayDate }
    });
    await user.click(screen.getByRole("button", { name: "Generate" }));

    await waitFor(() => {
      expect(api.calendar.exportAvailability).toHaveBeenCalledWith({
        calendarIds: ["cal-product"],
        start: now,
        end: tomorrowIso,
        format: "text"
      });
    });
    expect(await screen.findByRole("textbox", { name: "Availability export" })).toHaveValue(
      `Availability from ${now} to ${tomorrowIso}`
    );
    expect(screen.getByText("2 busy blocks")).toBeInTheDocument();
  });

  it("uses timeline drags for availability slots while share availability is active", async () => {
    installHcb(seededHcb());
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    const tabs = screen.getByRole("tablist", { name: "Calendar views" });
    await user.click(within(tabs).getByRole("tab", { name: "Day" }));
    await user.click(screen.getByRole("button", { name: "Share availability" }));

    expect(await screen.findByRole("heading", { name: "Share Availability" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Create event at 11:00" })).not.toBeInTheDocument();

    const slot = screen.getByRole("gridcell", { name: /11 AM/ });
    vi.spyOn(slot, "getBoundingClientRect").mockReturnValue({
      bottom: 64,
      height: 64,
      left: 0,
      right: 160,
      top: 0,
      width: 160,
      x: 0,
      y: 0,
      toJSON: () => ({})
    });
    fireEvent.mouseDown(slot, { button: 0, buttons: 1, clientY: 0 });
    fireEvent.mouseMove(slot, { buttons: 1, clientY: 20 });
    fireEvent.mouseUp(slot, { button: 0, clientY: 20 });

    expect(screen.queryByRole("heading", { level: 2, name: "New event" })).not.toBeInTheDocument();
    expect(await screen.findByText("1 selected")).toBeInTheDocument();
  });

  it("runs calendar-focused command palette actions", async () => {
    installHcb(seededHcb());
    const user = userEvent.setup();
    render(<App />);

    await runPaletteCommand(user, "calendar week", /Calendar week view/);
    expect(await screen.findByRole("heading", { level: 1, name: "Calendar" })).toBeInTheDocument();
    expect(await screen.findByText("Week view")).toBeInTheDocument();

    await runPaletteCommand(user, "new event", /New event/);
    expect(await screen.findByRole("heading", { level: 2, name: "New event" })).toBeInTheDocument();
  });
});
