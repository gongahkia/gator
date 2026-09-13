import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { TaskListSummary, TaskSummary } from "@shared/ipc/contracts";
import { err, ok } from "@shared/ipc/result";
import App from "./App";
import {
  goToSection,
  installHcb,
  now,
  seededHcb
} from "./test/appTestHelpers";

const inboxTaskList: TaskListSummary = {
  id: "list-inbox",
  title: "Inbox",
  updatedAt: now,
  taskCount: 0,
  activeTaskCount: 0
};

function noteTask(overrides: Partial<TaskSummary> & Pick<TaskSummary, "id" | "title">): TaskSummary {
  return {
    listId: "list-inbox",
    status: "active",
    priority: "none",
    dueAt: null,
    updatedAt: now,
    notes: "",
    parentId: null,
    ...overrides
  };
}

function installTaskBackedNotes(
  api: ReturnType<typeof seededHcb>,
  items: TaskSummary[],
  lists: TaskListSummary[] = [inboxTaskList]
): void {
  api.tasks.listTaskLists = vi.fn(async () =>
    ok({ items: lists, page: { limit: 100, totalKnown: lists.length } })
  );
  api.tasks.list = vi.fn(async (request = {}) =>
    ok({
      items: request.status === "hidden" || request.status === "deleted" ? [] : items,
      page: { limit: 100, totalKnown: items.length }
    })
  );
}

describe("App notes", () => {
  it("creates, edits, and deletes notes through preload", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Notes");
    expect(screen.queryByRole("button", { name: "All notes" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Starred" })).not.toBeInTheDocument();
    expect(screen.getByRole("list", { name: "Inbox" })).toBeInTheDocument();
    expect(await screen.findByText("Startup data flow")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Create notes/ }));
    const titleInput = await screen.findByRole("textbox", { name: "Note title" });
    const bodyInput = screen.getByRole("textbox", { name: "Note body" });
    expect(screen.getByRole("combobox", { name: "Note template" })).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Preview" })).not.toBeInTheDocument();

    fireEvent.change(titleInput, { target: { value: "Release note draft" } });
    fireEvent.change(bodyInput, { target: { value: "Document planner flow." } });

    await waitFor(() => {
      expect(api.tasks.create).toHaveBeenCalledWith({
        title: "Untitled note",
        notes: "",
        listId: "list-inbox",
        dueDate: null,
        tags: []
      });
      expect(api.tasks.update).toHaveBeenCalledWith({
        id: "task-created-1",
        title: "Release note draft",
        notes: "Document planner flow.",
        dueDate: null,
        tags: []
      });
    });

    const deleteButton = screen.getByRole("button", { name: "Delete selected note" });
    expect(deleteButton.className).toContain("ring-danger");
    await user.click(deleteButton);
    expect(api.tasks.delete).toHaveBeenCalledWith({ id: "task-created-1" });

    await user.click(screen.getByRole("button", { name: "Delete selected note" }));
    expect(api.tasks.delete).toHaveBeenCalledWith({ id: "task-note-startup" });
    expect(screen.getAllByText("No notes in this list").length).toBeGreaterThan(0);
  });

  it("opens selected notes in the inspector and flushes pending edits on close", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Notes");
    await user.click(await screen.findByText("Startup data flow"));

    const inspector = await screen.findByTestId("inspector-shell");
    expect(inspector).toHaveAttribute("data-inspector-kind", "note");
    const inspectorBody = within(inspector).getByTestId("inspector-body");
    expect(within(inspectorBody).getByRole("heading", { name: "Startup data flow" })).toBeInTheDocument();
    expect(within(inspector).queryByText("Attachments")).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Note body" })).not.toBeInTheDocument();

    await user.click(
      within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Edit" })
    );
    expect(await within(inspector).findByText("Attachments")).toBeInTheDocument();
    const bodyInput = await screen.findByRole("textbox", { name: "Note body" });
    fireEvent.change(bodyInput, {
      target: { value: `${(bodyInput as HTMLTextAreaElement).value} Pending close flush.` }
    });
    await user.click(screen.getByTestId("inspector-close"));

    await waitFor(() => {
      expect(api.tasks.update).toHaveBeenCalledWith({
        id: "task-note-startup",
        title: "Startup data flow",
        notes: expect.stringContaining("Pending close flush."),
        dueDate: null,
        tags: []
      });
    });
    expect(screen.queryByTestId("inspector-shell")).not.toBeInTheDocument();
  });

  it("duplicates notes as unsaved drafts and persists them on Save", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Notes");
    await user.click(await screen.findByText("Startup data flow"));
    await user.click(within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Duplicate" }));

    expect(await screen.findByRole("textbox", { name: "Note title" })).toHaveValue("Startup data flow (copy)");
    expect(screen.getByRole("textbox", { name: "Note body" })).toHaveValue(
      "Renderer paints from SQLite before fresh sync completes."
    );
    expect(api.tasks.create).not.toHaveBeenCalled();

    await user.click(within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(api.tasks.create).toHaveBeenCalledWith({
        title: "Startup data flow (copy)",
        notes: "Renderer paints from SQLite before fresh sync completes.",
        listId: "list-inbox",
        dueDate: null,
        tags: []
      });
    });
  });

  it("keeps duplicate note drafts open and shows save errors", async () => {
    const api = seededHcb();
    api.tasks.create = vi.fn(async () =>
      err({
        code: "SERVICE_UNAVAILABLE",
        message: "Task queue is unavailable.",
        recoverable: true
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Notes");
    await user.click(await screen.findByText("Startup data flow"));
    await user.click(within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Duplicate" }));
    expect(await screen.findByRole("textbox", { name: "Note title" })).toHaveValue("Startup data flow (copy)");
    await user.click(within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(api.tasks.create).toHaveBeenCalledWith({
      title: "Startup data flow (copy)",
      notes: "Renderer paints from SQLite before fresh sync completes.",
      listId: "list-inbox",
      dueDate: null,
      tags: []
    }));
    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("Task queue is unavailable.");
      expect(screen.getByRole("textbox", { name: "Note title" })).toHaveValue("Startup data flow (copy)");
    }, { timeout: 10_000 });
  });

  it("flushes pending note edits before switching the selected note row", async () => {
    const api = seededHcb();
    installTaskBackedNotes(api, [
      noteTask({
        id: "task-note-startup",
        title: "Startup data flow",
        notes: "Renderer paints from SQLite before fresh sync completes."
      }),
      noteTask({ id: "task-note-daily", title: "Daily note", notes: "Review backlinks." })
    ]);
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Notes");
    await user.click(await screen.findByText("Startup data flow"));
    await user.click(
      within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Edit" })
    );
    const bodyInput = await screen.findByRole("textbox", { name: "Note body" });
    fireEvent.change(bodyInput, {
      target: { value: `${(bodyInput as HTMLTextAreaElement).value} Switch flush.` }
    });
    const notesList = screen.getByRole("list", { name: "Inbox" });
    await user.click(within(notesList).getByRole("button", { name: "Open note Daily note" }));

    await waitFor(() => {
      expect(api.tasks.update).toHaveBeenCalledWith({
        id: "task-note-startup",
        title: "Startup data flow",
        notes: expect.stringContaining("Switch flush."),
        dueDate: null,
        tags: []
      });
    });
    const inspector = await screen.findByTestId("inspector-shell");
    const inspectorBody = within(inspector).getByTestId("inspector-body");
    expect(within(inspectorBody).getByRole("heading", { name: "Daily note" })).toBeInTheDocument();
  });

  it("renders note markdown preview, outgoing links, and backlinks", async () => {
    const api = seededHcb();
    installTaskBackedNotes(api, [
      noteTask({ id: "task-note-project", title: "Project plan", notes: "# Plan\n- [x] Kickoff\nSee [[Daily note]]" }),
      noteTask({ id: "task-note-daily", title: "Daily note", notes: "Back to [[Project plan]]" })
    ]);
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Notes");
    await user.click(await screen.findByText("Project plan"));

    const preview = await screen.findByRole("region", { name: "Note preview" });
    expect(within(preview).getByText("Plan")).toBeInTheDocument();
    expect(within(preview).getByRole("checkbox")).toBeChecked();
    expect(preview).toHaveTextContent("Kickoff");
    expect(preview).toHaveTextContent("See Daily note");
    const dailyLink = within(preview).getByRole("button", { name: "Daily note" });

    dailyLink.focus();
    await user.keyboard("{Enter}");
    const inspector = await screen.findByTestId("inspector-shell");
    const inspectorBody = within(inspector).getByTestId("inspector-body");
    expect(within(inspectorBody).getByRole("heading", { name: "Daily note" })).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Open backlink Project plan" })).toBeInTheDocument();
  });

  it("inserts planner links and creates notes from the template field", async () => {
    const api = seededHcb();
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Notes");
    await user.click(await screen.findByText("Startup data flow"));
    await user.click(
      within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Edit" })
    );
    const bodyInput = await screen.findByRole("textbox", { name: "Note body" });

    await user.type(screen.getByRole("combobox", { name: "Planner link target" }), "triage");
    await user.click(await screen.findByRole("option", { name: /Draft inbox triage rules/ }));
    expect((bodyInput as HTMLTextAreaElement).value).toContain("[[task:Draft inbox triage rules]]");

    await user.click(screen.getByRole("tab", { name: "Preview" }));
    expect(screen.getAllByText("task: Draft inbox triage rules").length).toBeGreaterThan(0);

    expect(screen.queryByRole("button", { name: "Daily note" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Meeting note" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Create notes/ }));
    await user.selectOptions(await screen.findByRole("combobox", { name: "Note template" }), "daily");
    await waitFor(() => {
      expect(api.tasks.update).toHaveBeenCalledWith(
        expect.objectContaining({
          id: "task-created-1",
          title: expect.stringMatching(/^Daily \d{4}-\d{2}-\d{2}$/),
          notes: expect.stringContaining("tags: daily"),
          dueDate: null
        })
      );
    });
    expect((screen.getByRole("textbox", { name: "Note body" }) as HTMLTextAreaElement).value).toContain("tags: daily");
  });

  it("renames note lists from the column menu", async () => {
    const api = seededHcb();
    const promptSpy = vi.spyOn(window, "prompt").mockReturnValue("Renamed notes");
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Notes");
    await user.click(await screen.findByRole("button", { name: "More actions for Inbox" }));
    await user.click(await screen.findByRole("button", { name: "Rename list" }));

    expect(promptSpy).toHaveBeenCalledWith("Rename list", "Inbox");
    expect(api.tasks.renameTaskList).toHaveBeenCalledWith({
      id: "list-inbox",
      title: "Renamed notes"
    });
  });

  it("creates notes from each note list column", async () => {
    const api = seededHcb();
    installTaskBackedNotes(
      api,
      [],
      [
        { id: "list-inbox", title: "Inbox", taskCount: 0, activeTaskCount: 0, updatedAt: now },
        { id: "list-side", title: "Side notes", taskCount: 0, activeTaskCount: 0, updatedAt: now }
      ]
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Notes");
    await user.click((await screen.findAllByRole("button", { name: "Add a note" }))[1]);

    await waitFor(() => {
      expect(api.tasks.create).toHaveBeenCalledWith({
        title: "Untitled note",
        notes: "",
        listId: "list-side",
        dueDate: null,
        tags: []
      });
    });
  });

  it("deletes custom note lists through Google Tasks", async () => {
    const api = seededHcb();
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    installTaskBackedNotes(
      api,
      [
        noteTask({ id: "task-note-default", title: "Default note", notes: "Default" }),
        noteTask({ id: "task-note-side", listId: "list-side", title: "Side note", notes: "Side" })
      ],
      [
        { id: "list-inbox", title: "Inbox", taskCount: 1, activeTaskCount: 1, updatedAt: now },
        { id: "list-side", title: "Side notes", taskCount: 1, activeTaskCount: 1, updatedAt: now }
      ]
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Notes");
    await user.click(await screen.findByRole("button", { name: "More actions for Side notes" }));
    await user.click(await screen.findByRole("button", { name: "Delete list" }));

    expect(confirmSpy).toHaveBeenCalledWith("Delete Side notes? Notes in this list will be deleted in Google Tasks.");
    expect(api.tasks.deleteTaskList).toHaveBeenCalledWith({ id: "list-side" });
    expect(screen.queryByRole("heading", { name: "Side notes" })).not.toBeInTheDocument();
    expect(screen.queryByText("Side note")).not.toBeInTheDocument();
  });

  it("supports keyboard selection in the note link autocomplete", async () => {
    const api = seededHcb();
    api.notes.linkSuggest = vi.fn(async () =>
      ok({
        items: [
          { kind: "note" as const, id: "note-project", label: "Project plan" },
          { kind: "task" as const, id: "task-plan", label: "Project plan task" },
          { kind: "event" as const, id: "event-plan", label: "Project plan review" }
        ]
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Notes");
    await user.click(await screen.findByText("Startup data flow"));
    await user.click(
      within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Edit" })
    );
    const bodyInput = await screen.findByRole("textbox", { name: "Note body" });
    const linkInput = screen.getByRole("combobox", { name: "Planner link target" });

    await user.type(linkInput, "plan");
    expect(await screen.findByRole("option", { name: /Project plan task/ })).toBeInTheDocument();
    await user.keyboard("{ArrowDown}{Enter}");

    expect(api.notes.linkSuggest).toHaveBeenCalledWith({ query: "plan", limit: 8 });
    expect((bodyInput as HTMLTextAreaElement).value).toContain("[[task:Project plan task]]");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("repairs broken note links from the note inspector", async () => {
    const api = seededHcb();
    installTaskBackedNotes(api, [
      noteTask({ id: "task-note-startup", title: "Startup data flow", notes: "See [[Missing note]]" })
    ]);
    api.notes.listBrokenLinks = vi.fn(async () => ok({ items: [{ linkText: "Missing note" }] }));
    api.notes.linkSuggest = vi.fn(async (request) =>
      ok({
        items: request.query.toLowerCase().includes("replacement")
          ? [{ kind: "note" as const, id: "note-replacement", label: "Replacement note" }]
          : []
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    await goToSection("Notes");
    await user.click(await screen.findByText("Startup data flow"));
    await user.click(
      within(screen.getByTestId("inspector-actions")).getByRole("button", { name: "Edit" })
    );
    const bodyInput = await screen.findByRole("textbox", { name: "Note body" });

    await user.click(await screen.findByRole("button", { name: "Fix link Missing note" }));
    const linkInput = screen.getByRole("combobox", { name: "Planner link target" });
    await user.clear(linkInput);
    await user.type(linkInput, "replacement");
    await user.click(await screen.findByRole("option", { name: /Replacement note/ }));

    expect((bodyInput as HTMLTextAreaElement).value).toContain("[[note:Replacement note]]");
    expect((bodyInput as HTMLTextAreaElement).value).not.toContain("[[Missing note]]");
  });
});
