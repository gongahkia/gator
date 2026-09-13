import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { err } from "@shared/ipc/result";
import App from "./App";
import {
  goToSection,
  installHcb,
  seededHcb,
  todayDate
} from "./test/appTestHelpers";
import type { ConvertCommandDetail } from "./features/core/conversionEvents";

function dispatchConvert(detail: ConvertCommandDetail): void {
  fireEvent(window, new CustomEvent("hcb:convert-command", { detail }));
}

function dispatchNoteConvert(detail: NonNullable<ConvertCommandDetail["noteDraft"]>): void {
  fireEvent(window, new CustomEvent("hcb:note-command", {
    detail: {
      action: "convert-to-note",
      draft: detail
    }
  }));
}

async function saveInspector(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Save" }));
}

describe("App conversion flows", () => {
  it("converts events to tasks and removes the source only after target save", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Tasks");
    dispatchConvert({
      target: "task",
      taskDraft: {
        title: "Converted event",
        notes: "Event details",
        dueDate: todayDate,
        listId: "list-inbox",
        priority: "none"
      }
    });
    expect(await screen.findByRole("heading", { level: 2, name: "New task" })).toBeInTheDocument();
    await saveInspector(user);
    await waitFor(() => {
      expect(api.tasks.create).toHaveBeenCalledWith(expect.objectContaining({
        title: "Converted event",
        notes: "Event details",
        dueDate: todayDate,
        listId: "list-inbox"
      }));
    });
    expect(api.calendar.delete).not.toHaveBeenCalled();

    dispatchConvert({
      cleanup: { kind: "event", id: "event-source" },
      target: "task",
      taskDraft: {
        title: "Converted event replace",
        notes: "Event details",
        dueDate: todayDate,
        listId: "list-inbox",
        priority: "none"
      }
    });
    expect(await screen.findByRole("heading", { level: 2, name: "New task" })).toBeInTheDocument();
    expect(api.calendar.delete).not.toHaveBeenCalled();
    await saveInspector(user);
    await waitFor(() => expect(api.calendar.delete).toHaveBeenCalledWith({ id: "event-source" }));
    expect(vi.mocked(api.tasks.create).mock.invocationCallOrder.at(-1)!).toBeLessThan(
      vi.mocked(api.calendar.delete).mock.invocationCallOrder.at(-1)!
    );
  });

  it("converts tasks to events and removes the source only after target save", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Calendar");
    dispatchConvert({
      target: "event",
      eventDraft: {
        title: "Converted task",
        notes: "Task notes",
        calendarId: "cal-product",
        startsAt: `${todayDate}T10:00:00.000Z`,
        endsAt: `${todayDate}T10:30:00.000Z`,
        allDay: false
      }
    });
    expect(await screen.findByRole("heading", { level: 2, name: "New event" })).toBeInTheDocument();
    await saveInspector(user);
    await waitFor(() => {
      expect(api.calendar.create).toHaveBeenCalledWith(expect.objectContaining({
        title: "Converted task",
        notes: "Task notes",
        calendarId: "cal-product"
      }));
    });
    expect(api.tasks.delete).not.toHaveBeenCalled();

    dispatchConvert({
      cleanup: { kind: "task", id: "task-source" },
      target: "event",
      eventDraft: {
        title: "Converted task replace",
        notes: "Task notes",
        calendarId: "cal-product",
        startsAt: `${todayDate}T11:00:00.000Z`,
        endsAt: `${todayDate}T11:30:00.000Z`,
        allDay: false
      }
    });
    expect(await screen.findByRole("heading", { level: 2, name: "New event" })).toBeInTheDocument();
    expect(api.tasks.delete).not.toHaveBeenCalled();
    await saveInspector(user);
    await waitFor(() => expect(api.tasks.delete).toHaveBeenCalledWith({ id: "task-source" }));
    expect(vi.mocked(api.calendar.create).mock.invocationCallOrder.at(-1)!).toBeLessThan(
      vi.mocked(api.tasks.delete).mock.invocationCallOrder.at(-1)!
    );
  });

  it("converts notes to tasks and removes the source only after target save", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Tasks");
    dispatchConvert({
      target: "task",
      taskDraft: {
        title: "Converted note",
        notes: "Note body",
        dueDate: todayDate,
        listId: "list-inbox",
        priority: "none"
      }
    });
    expect(await screen.findByRole("heading", { level: 2, name: "New task" })).toBeInTheDocument();
    await saveInspector(user);
    await waitFor(() => expect(api.tasks.create).toHaveBeenCalledWith(expect.objectContaining({
      title: "Converted note",
      notes: "Note body",
      dueDate: todayDate
    })));
    expect(api.tasks.delete).not.toHaveBeenCalled();

    dispatchConvert({
      cleanup: { kind: "note", id: "task-note-source" },
      target: "task",
      taskDraft: {
        title: "Converted note replace",
        notes: "Note body",
        dueDate: todayDate,
        listId: "list-inbox",
        priority: "none"
      }
    });
    expect(await screen.findByRole("heading", { level: 2, name: "New task" })).toBeInTheDocument();
    expect(api.tasks.delete).not.toHaveBeenCalled();
    await saveInspector(user);
    await waitFor(() => expect(api.tasks.delete).toHaveBeenCalledWith({ id: "task-note-source" }));
    expect(vi.mocked(api.tasks.create).mock.invocationCallOrder.at(-1)!).toBeLessThan(
      vi.mocked(api.tasks.delete).mock.invocationCallOrder.at(-1)!
    );
  });

  it("converts tasks to notes as keep-copy drafts", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Notes");
    dispatchNoteConvert({
      title: "Converted task",
      body: "Task notes",
      listId: "list-inbox",
      listTitle: "Inbox"
    });
    expect(await screen.findByRole("textbox", { name: "Note title" })).toHaveValue("Converted task");
    await saveInspector(user);
    await waitFor(() => expect(api.tasks.create).toHaveBeenCalledWith(expect.objectContaining({
      title: "Converted task",
      notes: "Task notes",
      dueDate: null,
      listId: "list-inbox"
    })));
    expect(api.tasks.delete).not.toHaveBeenCalled();
  });

  it("converts tasks to notes in place when replacing the source", async () => {
    const api = seededHcb();
    installHcb(api);
    render(<App />);

    await goToSection("Notes");
    dispatchNoteConvert({
      title: "Converted task replace",
      body: "Task notes",
      id: "task-source",
      listId: "list-inbox",
      listTitle: "Inbox",
      replaceSource: true
    });
    await waitFor(() => {
      expect(screen.getByRole("textbox", { name: "Note title" })).toHaveValue("Converted task replace");
    });
    await waitFor(() => expect(api.tasks.update).toHaveBeenCalledWith(expect.objectContaining({
      id: "task-source",
      title: "Converted task replace",
      notes: "Task notes",
      dueDate: null
    })));
    expect(api.tasks.delete).not.toHaveBeenCalled();
  });

  it("alerts when converted target save succeeds but source cleanup fails", async () => {
    const api = seededHcb();
    api.tasks.delete = vi.fn(async () =>
      err({
        code: "SERVICE_UNAVAILABLE",
        message: "Original note removal failed.",
        recoverable: true
      })
    );
    const alertSpy = vi.spyOn(window, "alert").mockImplementation(() => undefined);
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Tasks");
    dispatchConvert({
      cleanup: { kind: "note", id: "task-note-source" },
      target: "task",
      taskDraft: {
        title: "Converted note replace",
        notes: "Note body",
        dueDate: todayDate,
        listId: "list-inbox",
        priority: "none"
      }
    });
    await screen.findByRole("heading", { level: 2, name: "New task" });
    await saveInspector(user);

    await waitFor(() => {
      expect(alertSpy).toHaveBeenCalledWith("Converted item was saved, but Original note removal failed.");
    });
  });
});
