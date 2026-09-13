import { describe, expect, it } from "vitest";
import {
  parseQuickAddEvent,
  parseQuickAddTask,
  stripHashToken
} from "./naturalLanguage";

describe("quick add natural language parsing", () => {
  const now = new Date(2026, 3, 20, 9, 0, 0, 0);

  it("parses task dates and list hints", () => {
    const parsed = parseQuickAddTask("email rent receipt tmr #Personal", now);

    expect(parsed).toMatchObject({
      title: "email rent receipt",
      dueDate: "2026-04-21",
      taskListHint: "Personal"
    });
  });

  it("parses task relative and absolute dates", () => {
    expect(parseQuickAddTask("submit report in 3 weeks", now).dueDate).toBe("2026-05-11");
    expect(parseQuickAddTask("renew passport Apr 25", now).dueDate).toBe("2026-04-25");
    expect(parseQuickAddTask("book dentist eom", now).dueDate).toBe("2026-04-30");
  });

  it("parses event time, date, duration, and location", () => {
    const parsed = parseQuickAddEvent("Lunch with Bob tomorrow 1pm for 45m at Philz", now);

    expect(parsed.summary).toBe("Lunch with Bob");
    expect(parsed.location).toBe("Philz");
    expect(parsed.isAllDay).toBe(false);
    expect(parsed.startDate?.getFullYear()).toBe(2026);
    expect(parsed.startDate?.getMonth()).toBe(3);
    expect(parsed.startDate?.getDate()).toBe(21);
    expect(parsed.startDate?.getHours()).toBe(13);
    expect(parsed.endDate?.getHours()).toBe(13);
    expect(parsed.endDate?.getMinutes()).toBe(45);
  });

  it("parses event ranges and all-day anchors", () => {
    const timed = parseQuickAddEvent("Planning next fri from 2 to 4pm", now);
    const allDay = parseQuickAddEvent("Maya Apr 25", now);

    expect(timed.summary).toBe("Planning");
    expect(timed.startDate?.getHours()).toBe(14);
    expect(timed.endDate?.getHours()).toBe(16);
    expect(allDay.summary).toBe("Maya");
    expect(allDay.isAllDay).toBe(true);
    expect(allDay.startDate?.getDate()).toBe(25);
  });

  it("parses recurring event cadence, ending, and locations", () => {
    const weekly = parseQuickAddEvent("Run club every Mon/Wed at 6:30pm until Jun 1 at \"East Coast Park\"", now);

    expect(weekly.summary).toBe("Run club");
    expect(weekly.location).toBe("East Coast Park");
    expect(weekly.startDate?.getHours()).toBe(18);
    expect(weekly.startDate?.getMinutes()).toBe(30);
    expect(weekly.recurrence).toMatchObject({
      frequency: "weekly",
      interval: 1,
      endsOn: "2026-06-01",
      byDay: ["MO", "WE"]
    });

    const monthly = parseQuickAddEvent("Pay rent every 2 months for 5 times", now);

    expect(monthly.summary).toBe("Pay rent");
    expect(monthly.recurrence).toMatchObject({
      frequency: "monthly",
      interval: 2,
      count: 5
    });
  });

  it("parses alternate weekday recurrence", () => {
    const parsed = parseQuickAddEvent("Sprint review every other Tue at 9am", now);

    expect(parsed.summary).toBe("Sprint review");
    expect(parsed.startDate?.getHours()).toBe(9);
    expect(parsed.recurrence).toMatchObject({
      frequency: "weekly",
      interval: 2,
      byDay: ["TU"]
    });
  });

  it("parses simple cadence with an until date", () => {
    const parsed = parseQuickAddEvent("Office hours weekly until Jul 30", now);

    expect(parsed.summary).toBe("Office hours");
    expect(parsed.recurrence).toMatchObject({
      frequency: "weekly",
      interval: 1,
      endsOn: "2026-07-30"
    });
  });

  it("strips matched routing tags", () => {
    expect(stripHashToken("Lunch #Product", "Product")).toBe("Lunch");
  });
});
