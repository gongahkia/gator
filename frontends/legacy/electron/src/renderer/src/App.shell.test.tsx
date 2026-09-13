import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { NativeAction } from "@shared/ipc/contracts";
import { err, ok } from "@shared/ipc/result";
import App from "./App";
import {
  goToSection,
  installHcb,
  loadingHcb,
  now,
  originalHcb,
  primaryNavigation,
  runPaletteCommand,
  seededHcb,
  settingsLoadingHcb,
  signedOutGoogleStatus,
  testDataTransfer,
  todayDate
} from "./test/appTestHelpers";

describe("App shell", () => {
  it("renders the loading state while preload reads are pending", () => {
    installHcb(loadingHcb());
    render(<App />);

    expect(screen.getByText("Reading planner data.")).toBeInTheDocument();
  });

  it("waits for settings before reporting the shell visible", async () => {
    const api = settingsLoadingHcb();

    installHcb(api);
    render(<App />);

    await waitFor(() => expect(api.tasks.listTaskLists).toHaveBeenCalled());
    await new Promise((resolve) => window.setTimeout(resolve, 25));

    expect(api.diagnostics.markShellVisible).not.toHaveBeenCalled();
  });

  it("reports the shell visible after settings load while other reads continue", async () => {
    const api = loadingHcb();
    (api as { bootstrap?: unknown }).bootstrap = undefined;

    installHcb(api);
    render(<App />);

    await waitFor(() => expect(api.diagnostics.markShellVisible).toHaveBeenCalledTimes(1));
  });

  it("reports the shell visible after the settings theme can be applied", async () => {
    const api = seededHcb();

    installHcb(api);
    render(<App />);

    await waitFor(() => expect(api.diagnostics.markShellVisible).toHaveBeenCalledTimes(1));
  });

  it("renders from empty planner data and invokes preload read APIs", async () => {
    render(<App />);

    expect(screen.getByTestId("app-shell")).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Calendar" })).toBeInTheDocument();

    for (const label of ["Calendar", "Tasks", "Notes"]) {
      expect(within(primaryNavigation()).getByRole("button", { name: new RegExp(label) })).toBeInTheDocument();
    }
    expect(screen.queryByText("Runtime")).not.toBeInTheDocument();
    expect(within(primaryNavigation()).queryByRole("button", { name: /Settings/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Settings" })).toBeInTheDocument();
    expect(within(primaryNavigation()).queryByRole("button", { name: /Notifications/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Notifications, \d+ active/ })).toBeInTheDocument();

    expect(await screen.findByText("No agenda items")).toBeInTheDocument();

    await waitFor(() => {
      expect(originalHcb?.tasks.listTaskLists).toHaveBeenCalled();
      expect(originalHcb?.tasks.list).toHaveBeenCalled();
      expect(originalHcb?.calendar.listCalendars).toHaveBeenCalled();
      expect(originalHcb?.calendar.listEvents).toHaveBeenCalled();
      expect(originalHcb?.notes.list).toHaveBeenCalled();
      expect(originalHcb?.sync.status).toHaveBeenCalled();
    });
  });

  it("navigates sections and opens the command palette", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(within(primaryNavigation()).getByRole("button", { name: /Tasks/ }));
    expect(screen.getByRole("heading", { level: 1, name: "Tasks" })).toBeInTheDocument();

    const tasksButton = within(primaryNavigation()).getByRole("button", { name: /Tasks/ });
    tasksButton.focus();
    await user.keyboard("{ArrowUp}");

    expect(screen.getByRole("heading", { level: 1, name: "Calendar" })).toBeInTheDocument();
    expect(within(primaryNavigation()).getByRole("button", { name: /Calendar/ })).toHaveFocus();

    await user.keyboard("{Meta>}p{/Meta}");
    const dialog = await screen.findByRole("dialog", { name: "Command palette" });

    await user.type(within(dialog).getByRole("searchbox", { name: "Filter commands" }), "note");
    expect(within(dialog).getByRole("option", { name: /New note/ })).toBeInTheDocument();

    await user.click(within(dialog).getByRole("option", { name: /Go to Notes/ }));
    expect(screen.queryByRole("dialog", { name: "Command palette" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Notes" })).toBeInTheDocument();
  });

  it("opens the command palette from the toolbar button", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "Command palette" }));

    const dialog = await screen.findByRole("dialog", { name: "Command palette" });
    expect(within(dialog).getByRole("searchbox", { name: "Filter commands" })).toHaveValue("");
  });

  it("opens quick add from the command palette and fills the event inspector", async () => {
    const user = userEvent.setup();
    installHcb(seededHcb());
    render(<App />);

    await screen.findByText("Planner shell standup");
    await user.keyboard("{Meta>}p{/Meta}");
    const palette = await screen.findByRole("dialog", { name: "Command palette" });

    await user.type(within(palette).getByRole("searchbox", { name: "Filter commands" }), "quick add");
    await user.click(within(palette).getByRole("option", { name: /Quick Add/ }));

    const quickAdd = await screen.findByRole("dialog", { name: "Quick Add" });
    await user.type(
      within(quickAdd).getByRole("textbox", { name: "Quick add text" }),
      "Lunch with Bob tomorrow 1pm at Philz #Product"
    );
    await user.click(within(quickAdd).getByRole("button", { name: "Add" }));

    expect(await screen.findByRole("textbox", { name: "Event title" })).toHaveValue("Lunch with Bob");
    expect(screen.getByRole("textbox", { name: "Event location" })).toHaveValue("Philz");
  });

  it("opens quick add from a native global shortcut action", async () => {
    const api = seededHcb();
    let nativeActionListener: ((action: NativeAction) => void) | undefined;
    api.native = {
      ...api.native,
      subscribeAction: vi.fn((listener) => {
        nativeActionListener = listener;
        return vi.fn();
      })
    };

    installHcb(api);
    render(<App />);

    await screen.findByText("Planner shell standup");
    nativeActionListener?.({ type: "openQuickAdd" });

    expect(await screen.findByRole("dialog", { name: "Quick Add" })).toBeInTheDocument();
  });

  it("opens a split view chooser for other app tabs", async () => {
    const user = userEvent.setup();
    installHcb(seededHcb());
    render(<App />);

    await user.click(screen.getByRole("button", { name: "Split view" }));

    let splitPane = await screen.findByRole("region", { name: "Choose split view pane" });
    expect(within(splitPane).getByRole("heading", { name: "Choose split view" })).toBeInTheDocument();
    expect(within(splitPane).queryByText("App tabs")).not.toBeInTheDocument();
    expect(within(splitPane).queryByText("Open webpage")).not.toBeInTheDocument();
    expect(within(splitPane).getByRole("button", { name: /Tasks/ })).toBeInTheDocument();
    expect(within(splitPane).getByRole("button", { name: /Notes/ })).toBeInTheDocument();
    expect(within(splitPane).getByText("Webpage")).toBeInTheDocument();
    expect(within(splitPane).queryByRole("button", { name: /Calendar/ })).not.toBeInTheDocument();

    await user.click(within(splitPane).getByRole("button", { name: /Tasks/ }));

    splitPane = await screen.findByRole("region", { name: "Tasks pane" });
    expect(within(splitPane).getByRole("heading", { name: "Tasks" })).toBeInTheDocument();
    expect(within(splitPane).getByRole("list", { name: "Task lists" })).toBeInTheDocument();

    expect(within(splitPane).queryByRole("button", { name: /Split Tasks/ })).not.toBeInTheDocument();
    expect(within(splitPane).queryByRole("button", { name: "Choose content for Tasks" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Split view" }));
    splitPane = await screen.findByRole("region", { name: "Choose split view pane" });
    expect(within(splitPane).queryByText("Recent webpages")).not.toBeInTheDocument();
    expect(within(splitPane).queryByRole("button", { name: /Tasks/ })).not.toBeInTheDocument();
    expect(within(splitPane).queryByRole("button", { name: /Calendar/ })).not.toBeInTheDocument();
    expect(within(splitPane).getByRole("button", { name: /Notes/ })).toBeInTheDocument();
  });

  it("opens typed webpages from the split view chooser", async () => {
    const user = userEvent.setup();
    window.localStorage.clear();
    installHcb(seededHcb());
    render(<App />);

    await user.click(screen.getByRole("button", { name: "Split view" }));
    const chooserPane = await screen.findByRole("region", { name: "Choose split view pane" });

    await user.type(within(chooserPane).getByLabelText("Webpage URL"), "example.com");
    await user.click(within(chooserPane).getByRole("button", { name: "Open webpage" }));

    const webPane = await screen.findByRole("region", { name: "example.com pane" });
    expect(within(webPane).getByTestId("split-webview")).toHaveAttribute("src", "https://example.com/");

    fireEvent.keyDown(window, { key: "t", metaKey: true });
    expect(screen.getAllByTestId("pane-leaf")).toHaveLength(2);
    expect(within(webPane).getByRole("button", { name: "Select New tab" })).toBeInTheDocument();
    expect(within(webPane).getByLabelText("Webpage URL")).toBeInTheDocument();
  });

  it("opens typed webpages from the split view chooser on enter", async () => {
    const user = userEvent.setup();
    window.localStorage.clear();
    installHcb(seededHcb());
    render(<App />);

    await user.click(screen.getByRole("button", { name: "Split view" }));
    const chooserPane = await screen.findByRole("region", { name: "Choose split view pane" });

    await user.type(within(chooserPane).getByLabelText("Webpage URL"), "example.org{Enter}");

    const webPane = await screen.findByRole("region", { name: "example.org pane" });
    expect(within(webPane).getByTestId("split-webview")).toHaveAttribute("src", "https://example.org/");
  });

  it("uses pane and diagnostics hotkeys", async () => {
    window.localStorage.clear();
    installHcb(seededHcb());
    render(<App />);

    fireEvent.keyDown(window, { key: ".", metaKey: true });
    expect(await screen.findByRole("dialog", { name: "Diagnostics" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Close diagnostics" }));

    fireEvent.keyDown(window, { key: "d", metaKey: true });
    await waitFor(() => expect(screen.getAllByTestId("pane-leaf")).toHaveLength(2));
    expect(screen.getByTestId("pane-split")).toHaveAttribute("data-pane-direction", "row");

    fireEvent.keyDown(window, { key: "d", metaKey: true, shiftKey: true });
    await waitFor(() => expect(screen.getAllByTestId("pane-leaf")).toHaveLength(3));
    expect(screen.getAllByTestId("pane-split").some((split) => split.getAttribute("data-pane-direction") === "column")).toBe(true);

    fireEvent.keyDown(window, { key: "w", metaKey: true });
    await waitFor(() => expect(screen.getAllByTestId("pane-leaf")).toHaveLength(2));

    fireEvent.keyDown(window, { key: "t", metaKey: true });
    await waitFor(() => expect(screen.getAllByTestId("pane-leaf")).toHaveLength(2));
  });

  it("closes a single pane to the chooser before closing the window", async () => {
    window.localStorage.clear();
    const closeSpy = vi.spyOn(window, "close").mockImplementation(() => undefined);
    installHcb(seededHcb());
    render(<App />);

    const calendarPane = await screen.findByRole("region", { name: "Calendar pane" });
    expect(within(calendarPane).getByRole("button", { name: "Close Calendar pane" })).toBeEnabled();

    fireEvent.keyDown(window, { key: "w", metaKey: true });
    const chooserPane = await screen.findByRole("region", { name: "Choose split view pane" });
    expect(within(chooserPane).getByRole("heading", { name: "Choose split view" })).toBeInTheDocument();
    expect(closeSpy).not.toHaveBeenCalled();

    fireEvent.keyDown(window, { key: "w", metaKey: true });
    expect(closeSpy).toHaveBeenCalledTimes(1);
    closeSpy.mockRestore();
  });

  it("splits, closes, drags, and restores pane layouts", async () => {
    const user = userEvent.setup();
    window.localStorage.clear();
    installHcb(seededHcb());
    render(<App />);

    await user.click(screen.getByRole("button", { name: "Split view" }));
    let chooserPane = await screen.findByRole("region", { name: "Choose split view pane" });
    await user.click(within(chooserPane).getByRole("button", { name: /Tasks/ }));
    const tasksPane = await screen.findByRole("region", { name: "Tasks pane" });

    fireEvent.keyDown(window, { key: "d", metaKey: true, shiftKey: true });
    expect(await screen.findAllByTestId("pane-leaf")).toHaveLength(3);

    chooserPane = await screen.findByRole("region", { name: "Choose split view pane" });
    await user.click(within(chooserPane).getByRole("button", { name: "Close Choose split view pane" }));
    expect(await screen.findAllByTestId("pane-leaf")).toHaveLength(2);

    const calendarPane = screen.getByRole("region", { name: "Calendar pane" });
    calendarPane.getBoundingClientRect = () => ({
      bottom: 500,
      height: 500,
      left: 0,
      right: 500,
      top: 0,
      width: 500,
      x: 0,
      y: 0,
      toJSON: () => ({})
    });
    const transfer = testDataTransfer();
    fireEvent.dragStart(within(tasksPane).getByRole("button", { name: "Drag pane Tasks" }), {
      dataTransfer: transfer
    });
    fireEvent.dragOver(calendarPane, { clientX: 250, clientY: 10, dataTransfer: transfer });

    await waitFor(() => {
      expect(calendarPane.querySelector("[data-pane-drop-preview]")).toBeInTheDocument();
    });

    fireEvent.drop(calendarPane, { clientX: 250, clientY: 10, dataTransfer: transfer });

    expect(screen.getAllByTestId("pane-leaf")).toHaveLength(2);
    expect(window.localStorage.getItem("hcb.paneWorkspace.v1")).toContain("tasks");
  });

  it("restores a persisted pane layout", async () => {
    window.localStorage.setItem(
      "hcb.paneWorkspace.v1",
      JSON.stringify({
        focusedPaneId: "pane-notes",
        root: {
          id: "split-root",
          kind: "split",
          direction: "row",
          ratio: 0.42,
          children: [
            { id: "pane-calendar", kind: "leaf", content: { kind: "section", sectionId: "calendar" } },
            { id: "pane-notes", kind: "leaf", content: { kind: "section", sectionId: "notes" } }
          ]
        }
      })
    );
    installHcb(seededHcb());
    render(<App />);

    expect(await screen.findByRole("heading", { level: 1, name: "Notes" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Calendar pane" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Notes pane" })).toBeInTheDocument();
    expect(screen.getAllByTestId("pane-leaf")).toHaveLength(2);
  });

  it("supports command-number shortcuts for primary sidebar sections", async () => {
    const user = userEvent.setup();
    render(<App />);

    const navButtons = within(primaryNavigation()).getAllByRole("button");
    expect(navButtons.slice(0, 3).map((button) => button.getAttribute("aria-label"))).toEqual([
      "Calendar",
      "Tasks",
      "Notes"
    ]);

    const tasksButton = within(primaryNavigation()).getByRole("button", { name: "Tasks" });
    const calendarButton = within(primaryNavigation()).getByRole("button", { name: "Calendar" });
    const notesButton = within(primaryNavigation()).getByRole("button", { name: "Notes" });

    expect(tasksButton).toHaveAttribute(
      "aria-keyshortcuts",
      "Meta+1 Control+1"
    );
    expect(calendarButton).toHaveAttribute(
      "aria-keyshortcuts",
      "Meta+2 Control+2"
    );
    expect(notesButton).toHaveAttribute(
      "aria-keyshortcuts",
      "Meta+3 Control+3"
    );
    expect(within(tasksButton).queryByText("Cmd 1")).not.toBeInTheDocument();
    expect(within(calendarButton).queryByText("Cmd 2")).not.toBeInTheDocument();
    expect(within(notesButton).queryByText("Cmd 3")).not.toBeInTheDocument();

    await user.keyboard("{Meta>}1{/Meta}");
    expect(screen.getByRole("heading", { level: 1, name: "Tasks" })).toBeInTheDocument();

    await user.keyboard("{Meta>}2{/Meta}");
    expect(screen.getByRole("heading", { level: 1, name: "Calendar" })).toBeInTheDocument();

    await user.keyboard("{Meta>}3{/Meta}");
    expect(screen.getByRole("heading", { level: 1, name: "Notes" })).toBeInTheDocument();
  });

  it("uses HCB section counts in primary navigation badges", async () => {
    const api = seededHcb();

    api.tasks.listTaskLists = async () =>
      ok({
        items: [
          {
            id: "list-inbox",
            title: "Inbox",
            updatedAt: now,
            taskCount: 1200,
            activeTaskCount: 900
          }
        ],
        page: { limit: 100, totalKnown: 1 }
      });
    api.calendar.listCalendars = async () =>
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
            eventCount: 18000
          }
        ],
        page: { limit: 100, totalKnown: 1 }
      });
    api.calendar.listEvents = async () =>
      ok({
        items: [
          {
            id: "event-visible",
            calendarId: "cal-product",
            title: "Visible window event",
            startsAt: `${todayDate}T09:00:00.000Z`,
            endsAt: `${todayDate}T09:30:00.000Z`,
            allDay: false,
            updatedAt: now
          }
        ],
        page: { limit: 250, totalKnown: 250 }
      });
    api.notes.list = async () =>
      ok({
        items: [
          {
            id: "note-cache-first",
            listId: "list-inbox",
            listTitle: "Notes",
            title: "Startup data flow",
            preview: "Renderer paints from SQLite.",
            updatedAt: now
          }
        ],
        lists: [{ id: "list-inbox", title: "Notes", noteCount: 1, updatedAt: now }],
        page: { limit: 50, totalKnown: 321 }
      });
    installHcb(api);
    render(<App />);

    const navigation = primaryNavigation();

    await waitFor(
      () => {
        expect(within(within(navigation).getByRole("button", { name: "Tasks" })).getByText("2")).toBeInTheDocument();
        expect(within(within(navigation).getByRole("button", { name: "Calendar" })).getByText("18000")).toBeInTheDocument();
        expect(within(within(navigation).getByRole("button", { name: "Notes" })).getByText("1")).toBeInTheDocument();
      },
      { timeout: 2500 }
    );
  });

  it("renders deferred task and note counts as loading placeholders before hydration", async () => {
    const api = seededHcb();
    const bootstrapResult = await api.bootstrap.get({
      mode: "light",
      calendarRange: {
        start: `${todayDate}T00:00:00.000Z`,
        end: `${todayDate}T23:59:59.999Z`,
        limit: 500
      }
    });

    if (!bootstrapResult.ok) {
      throw new Error("Expected seeded bootstrap fixture.");
    }

    api.bootstrap.get = vi.fn(async () =>
      ok({
        ...bootstrapResult.data,
        tasks: { items: [], page: { limit: 1, totalKnown: 0 } },
        hiddenTasks: { items: [], page: { limit: 1, totalKnown: 0 } },
        deletedTasks: { items: [], page: { limit: 1, totalKnown: 0 } },
        notes: { items: [], lists: [], page: { limit: 1, totalKnown: 0 } },
        resourceCounts: {
          ...bootstrapResult.data.resourceCounts,
          tasks: 0,
          notes: 0
        }
      })
    );

    installHcb(api);
    render(<App />);

    await screen.findByRole("heading", { level: 1, name: "Calendar" });
    const navigation = primaryNavigation();
    const tasksButton = within(navigation).getByRole("button", { name: "Tasks" });
    const notesButton = within(navigation).getByRole("button", { name: "Notes" });

    expect(within(tasksButton).getByText("...")).toBeInTheDocument();
    expect(within(notesButton).getByText("...")).toBeInTheDocument();
    expect(within(tasksButton).queryByText("0")).not.toBeInTheDocument();
    expect(within(notesButton).queryByText("0")).not.toBeInTheDocument();

    await waitFor(
      () => {
        expect(within(tasksButton).getByText("2")).toBeInTheDocument();
        expect(within(notesButton).getByText("1")).toBeInTheDocument();
      },
      { timeout: 2500 }
    );
  });

  it("flags failed deferred hydration and retries counts on reload", async () => {
    const user = userEvent.setup();
    const api = seededHcb();
    const bootstrapResult = await api.bootstrap.get({
      mode: "light",
      calendarRange: {
        start: `${todayDate}T00:00:00.000Z`,
        end: `${todayDate}T23:59:59.999Z`,
        limit: 500
      }
    });

    if (!bootstrapResult.ok) {
      throw new Error("Expected seeded bootstrap fixture.");
    }

    const listTasks = api.tasks.list;
    const listNotes = api.notes.list;
    let failHydration = true;

    api.bootstrap.get = vi.fn(async () =>
      ok({
        ...bootstrapResult.data,
        tasks: { items: [], page: { limit: 1, totalKnown: 0 } },
        hiddenTasks: { items: [], page: { limit: 1, totalKnown: 0 } },
        deletedTasks: { items: [], page: { limit: 1, totalKnown: 0 } },
        notes: { items: [], lists: [], page: { limit: 1, totalKnown: 0 } },
        resourceCounts: {
          ...bootstrapResult.data.resourceCounts,
          tasks: 0,
          notes: 0
        }
      })
    );
    api.tasks.list = vi.fn(async (request) => {
      if (failHydration) {
        throw new Error("access_token=fake-access-token /Users/tester");
      }

      return listTasks(request);
    });
    api.notes.list = vi.fn(async (request) => {
      if (failHydration) {
        throw new Error("notes unavailable");
      }

      return listNotes(request);
    });

    installHcb(api);
    render(<App />);

    await screen.findByRole("heading", { level: 1, name: "Calendar" });
    const navigation = primaryNavigation();
    const tasksButton = within(navigation).getByRole("button", { name: "Tasks" });
    const notesButton = within(navigation).getByRole("button", { name: "Notes" });

    expect(within(tasksButton).getByText("...")).toBeInTheDocument();
    expect(within(notesButton).getByText("...")).toBeInTheDocument();

    await waitFor(
      () => {
        expect(within(tasksButton).getByText("!")).toBeInTheDocument();
        expect(within(notesButton).getByText("!")).toBeInTheDocument();
      },
      { timeout: 2500 }
    );
    await waitFor(() => {
      expect(api.diagnostics.recordTiming).toHaveBeenCalledWith(
        expect.objectContaining({
          name: "startup.hydration.merge",
          metadata: expect.objectContaining({
            errorMessage: expect.not.stringContaining("fake-access-token"),
            outcome: "failed"
          })
        })
      );
    });

    await user.click(screen.getByRole("button", { name: /Notifications,/ }));
    const dialog = await screen.findByRole("dialog", { name: "Notifications" });
    expect(within(dialog).getByText("Some counts could not refresh")).toBeInTheDocument();
    expect(within(dialog).getByText("Tasks and notes are still usable, but some sidebar counts could not be updated. Use Reload to retry.")).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Close notifications" }));

    failHydration = false;
    await user.click(screen.getByRole("button", { name: "Reload" }));

    await waitFor(
      () => {
        expect(within(tasksButton).getByText("2")).toBeInTheDocument();
        expect(within(notesButton).getByText("1")).toBeInTheDocument();
      },
      { timeout: 3000 }
    );
  });

  it("opens notifications as a toolbar overlay instead of a primary navigation section", async () => {
    const user = userEvent.setup();
    installHcb(seededHcb());
    render(<App />);

    expect(within(primaryNavigation()).queryByRole("button", { name: /Notifications/ })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Notifications, \d+ active/ }));

    const dialog = await screen.findByRole("dialog", { name: "Notifications" });
    expect(dialog.parentElement).toHaveClass("bg-bg-tertiary/45", "backdrop-blur-sm");
    expect(dialog).toHaveClass("bg-bg-primary");
    expect(dialog).not.toHaveClass("bg-bg-primary/95");
    expect(within(dialog).getByRole("heading", { level: 3, name: "App notices" })).toBeInTheDocument();
    expect(within(dialog).queryByText("Local reminders")).not.toBeInTheDocument();
    expect(within(dialog).queryByLabelText("Notification lead minutes")).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Request permission" })).not.toBeInTheDocument();
    expect(within(dialog).queryByText("Delivery status")).not.toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Dismiss all" }));
    expect(within(dialog).getByText("No app notices.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Notifications, 0 active" })).toBeInTheDocument();

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "Notifications" })).not.toBeInTheDocument();
  });

  it("opens settings as a centered opaque overlay with a blurred backdrop", async () => {
    const user = userEvent.setup();
    installHcb(seededHcb());
    render(<App />);

    expect(within(primaryNavigation()).queryByRole("button", { name: /Settings/ })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Calendar" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Settings" }));

    const dialog = await screen.findByRole("dialog", { name: "Settings" });
    expect(dialog.parentElement).toHaveClass("place-items-center", "bg-bg-tertiary/45", "backdrop-blur-sm");
    expect(dialog).toHaveClass("max-w-[1120px]", "bg-bg-primary");
    expect(dialog).not.toHaveClass("bg-bg-primary/90", "backdrop-blur-xl");
    expect(within(dialog).getByRole("button", { name: "General" })).toHaveAttribute("aria-pressed", "true");
    expect(within(dialog).getByRole("button", { name: "Profile" })).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Appearance" })).toBeInTheDocument();
    expect(within(dialog).getByRole("heading", { level: 2, name: "Language" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Calendar" })).toBeInTheDocument();

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "Settings" })).not.toBeInTheDocument();
  });

  it("routes palette action command shells without waiting on sync or search", async () => {
    const user = userEvent.setup();
    installHcb(seededHcb());
    render(<App />);

    await runPaletteCommand(user, "new task", /New task/);
    expect(screen.getByRole("heading", { level: 1, name: "Tasks" })).toBeInTheDocument();

    await runPaletteCommand(user, "new event", /New event/);
    expect(screen.getByRole("heading", { level: 1, name: "Calendar" })).toBeInTheDocument();

    await runPaletteCommand(user, "quick add", /Quick Add/);
    expect(await screen.findByRole("dialog", { name: "Quick Add" })).toBeInTheDocument();
    await user.keyboard("{Escape}");

    await runPaletteCommand(user, "refresh", /Refresh/);
    expect(screen.getByRole("heading", { level: 1, name: "Calendar" })).toBeInTheDocument();

    await runPaletteCommand(user, "force", /Force full resync/);
    expect(await screen.findByRole("dialog", { name: "Settings" })).toBeInTheDocument();

    await runPaletteCommand(user, "mcp", /Toggle MCP server/);
    expect(await screen.findByRole("dialog", { name: "Settings" })).toBeInTheDocument();

    await runPaletteCommand(user, "diagnostics", /Copy diagnostics summary/);
    expect(await screen.findByRole("dialog", { name: "Diagnostics" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Close diagnostics" }));

    await runPaletteCommand(user, "diagnostics history", /Open Diagnostics History/);
    let diagnostics = await screen.findByRole("dialog", { name: "Diagnostics" });
    expect(within(diagnostics).getByRole("button", { name: "History" })).toHaveAttribute("aria-pressed", "true");
    await user.click(screen.getByRole("button", { name: "Close diagnostics" }));

    await runPaletteCommand(user, "sync issues", /Open Sync Issues/);
    diagnostics = await screen.findByRole("dialog", { name: "Diagnostics" });
    expect(within(diagnostics).getByRole("button", { name: "Sync" })).toHaveAttribute("aria-pressed", "true");
  });

  it("reuses diagnostics overlay when direct navigation changes tabs", async () => {
    const user = userEvent.setup();
    installHcb(seededHcb());
    render(<App />);

    await runPaletteCommand(user, "diagnostics history", /Open Diagnostics History/);
    let diagnostics = await screen.findByRole("dialog", { name: "Diagnostics" });
    expect(within(diagnostics).getByRole("button", { name: "History" })).toHaveAttribute("aria-pressed", "true");

    window.dispatchEvent(new CustomEvent("hcb:open-diagnostics", { detail: { tab: "sync" } }));

    await waitFor(() => expect(screen.getAllByRole("dialog", { name: "Diagnostics" })).toHaveLength(1));
    diagnostics = screen.getByRole("dialog", { name: "Diagnostics" });
    expect(within(diagnostics).getByRole("button", { name: "Sync" })).toHaveAttribute("aria-pressed", "true");
  });

  it("groups sync issues and copies visible sync issue text", async () => {
    const user = userEvent.setup();
    const api = seededHcb();
    const baseSummary = await api.diagnostics.summary();
    const writeText = vi.fn(async () => undefined);

    if (!baseSummary.ok) {
      throw new Error("Missing diagnostics fixture");
    }

    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText }
    });
    api.diagnostics.summary = vi.fn(async () =>
      ok({
        ...baseSummary.data,
        sync: {
          ...baseSummary.data.sync,
          state: "error" as const,
          offline: true,
          stale: true,
          lastErrorCode: "SERVICE_UNAVAILABLE" as const
        },
        pendingMutations: {
          ...baseSummary.data.pendingMutations,
          totalCount: 4,
          pendingCount: 1,
          applyingCount: 1,
          failedCount: 2,
          retryableCount: 1,
          authPausedCount: 1,
          nextRetryAt: `${todayDate}T00:10:00.000Z`,
          lastErrorCode: "UNAUTHORIZED" as const
        }
      })
    );
    api.diagnostics.pendingMutations = vi.fn(async () =>
      ok({
        mutations: [
          {
            id: "mutation-failed",
            accountId: "acct-1",
            resourceType: "task" as const,
            resourceId: "task-failed",
            operation: "task.update",
            status: "failed" as const,
            attemptCount: 1,
            nextRetryAt: null,
            lastErrorCode: "CONFLICT" as const,
            lastErrorMessage: "Server conflict",
            createdAt: now,
            updatedAt: now
          },
          {
            id: "mutation-retryable",
            accountId: "acct-1",
            resourceType: "event" as const,
            resourceId: "event-retry",
            operation: "event.create",
            status: "failed" as const,
            attemptCount: 2,
            nextRetryAt: `${todayDate}T00:10:00.000Z`,
            lastErrorCode: "RATE_LIMITED" as const,
            lastErrorMessage: "Rate limited",
            createdAt: now,
            updatedAt: now
          },
          {
            id: "mutation-auth",
            accountId: "acct-1",
            resourceType: "task" as const,
            resourceId: "task-auth",
            operation: "task.delete",
            status: "failed" as const,
            attemptCount: 1,
            nextRetryAt: null,
            lastErrorCode: "UNAUTHORIZED" as const,
            lastErrorMessage: "Google account reauthentication is required",
            createdAt: now,
            updatedAt: now
          },
          {
            id: "mutation-applying",
            accountId: "acct-1",
            resourceType: "task_list" as const,
            resourceId: "list-applying",
            operation: "taskList.rename",
            status: "applying" as const,
            attemptCount: 0,
            nextRetryAt: null,
            lastErrorCode: null,
            lastErrorMessage: null,
            createdAt: now,
            updatedAt: now
          }
        ]
      })
    );
    installHcb(api);
    render(<App />);

    await runPaletteCommand(user, "sync issues", /Open Sync Issues/);
    const diagnostics = await screen.findByRole("dialog", { name: "Diagnostics" });

    expect(await within(diagnostics).findByText("Sync offline")).toBeInTheDocument();
    expect(within(diagnostics).getByText("Cache is stale")).toBeInTheDocument();
    expect(within(diagnostics).getByText("Sync error")).toBeInTheDocument();
    expect(within(diagnostics).getByText("Failed mutations")).toBeInTheDocument();
    expect(within(diagnostics).getByText("Retryable/auth-paused mutations")).toBeInTheDocument();
    expect(within(diagnostics).getByText("Queued/applying mutations")).toBeInTheDocument();

    await user.click(within(diagnostics).getByRole("button", { name: "Copy visible sync issues" }));
    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith(expect.stringContaining("[state:offline] Sync offline"))
    );
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining("task-failed"));
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining("task-auth"));
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining("list-applying"));
  });

  it("shows distinct diagnostics empty and error states", async () => {
    const user = userEvent.setup();
    const api = seededHcb();

    api.diagnostics.summary = vi.fn(async () =>
      err({
        code: "SERVICE_UNAVAILABLE",
        message: "Summary unavailable",
        recoverable: true
      })
    );
    api.diagnostics.pendingMutations = vi.fn(async () =>
      err({
        code: "SERVICE_UNAVAILABLE",
        message: "Pending mutations unavailable",
        recoverable: true
      })
    );
    api.diagnostics.history = vi.fn(async () =>
      err({
        code: "SERVICE_UNAVAILABLE",
        message: "History unavailable",
        recoverable: true
      })
    );
    installHcb(api);
    render(<App />);

    await runPaletteCommand(user, "sync issues", /Open Sync Issues/);
    let diagnostics = await screen.findByRole("dialog", { name: "Diagnostics" });
    expect(await within(diagnostics).findByText("Sync diagnostics could not load.")).toBeInTheDocument();
    expect(within(diagnostics).queryByText("No sync issues found.")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Close diagnostics" }));

    await runPaletteCommand(user, "diagnostics history", /Open Diagnostics History/);
    diagnostics = await screen.findByRole("dialog", { name: "Diagnostics" });
    expect(await within(diagnostics).findByText("History could not load.")).toBeInTheDocument();
    expect(within(diagnostics).queryByText("No history entries recorded.")).not.toBeInTheDocument();
  });

  it("exposes task command action IDs through the command palette", async () => {
    const user = userEvent.setup();
    installHcb(seededHcb());
    render(<App />);

    await goToSection("Tasks");
    expect(await screen.findByRole("heading", { name: "Inbox" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create tasks" })).toBeInTheDocument();

    await user.keyboard("{Meta>}p{/Meta}");
    const dialog = await screen.findByRole("dialog", { name: "Command palette" });
    const input = within(dialog).getByRole("searchbox", { name: "Filter commands" });

    await user.type(input, "new task");
    expect(within(dialog).getByRole("option", { name: /New task/ })).toHaveAttribute(
      "data-action-id",
      "task.create"
    );

    await user.clear(input);
    await user.type(input, "complete selected");
    const completeCommand = within(dialog).getByRole("option", { name: /Complete selected task/ });

    expect(completeCommand).toHaveAttribute("data-action-id", "task.completeSelected");
    expect(completeCommand).toBeDisabled();
    expect(completeCommand).toHaveTextContent("No selected task");
  });

  it("renders seeded planner data and uses search", async () => {
    const api = seededHcb();
    installHcb(api);
    render(<App />);

    expect(await screen.findByText("Planner shell standup")).toBeInTheDocument();

    await goToSection("Tasks");
    expect(screen.getByRole("heading", { name: "Inbox" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Planning" })).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Inbox" })).toHaveAttribute("aria-checked", "true");

    await goToSection("Calendar");
    expect(screen.getByText("Agenda view")).toBeInTheDocument();
    expect(screen.getAllByText("Product").length).toBeGreaterThan(0);

    const user = userEvent.setup();
    await user.keyboard("{Meta>}p{/Meta}");
    const dialog = await screen.findByRole("dialog", { name: "Command palette" });
    await user.type(within(dialog).getByRole("searchbox", { name: "Filter commands" }), "triage");

    expect(await within(dialog).findByText(/Task in Inbox/)).toBeInTheDocument();
    expect(api.search.query).toHaveBeenCalledWith({ query: "triage", limit: 30 });
  });

  it("flags disconnected Google while cached planner data renders", async () => {
    const api = seededHcb();
    api.google.status = vi.fn(async () => ok(signedOutGoogleStatus()));
    api.sync.status = vi.fn(async () =>
      ok({
        state: "idle" as const,
        pendingMutationCount: 0,
        lastCompletedAt: now,
        offline: true,
        stale: false
      })
    );
    installHcb(api);
    const user = userEvent.setup();
    render(<App />);

    expect(await screen.findByText("Planner shell standup")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Notifications,/ }));
    expect(await screen.findByText("Google account not connected")).toBeInTheDocument();
    expect(screen.getByText("Showing local planner data. Connect Google to sync changes.")).toBeInTheDocument();
  });

  it("handles an unavailable preload bridge as an offline state", async () => {
    installHcb(undefined);
    render(<App />);

    expect((await screen.findAllByText("Offline"))[0]).toBeInTheDocument();
    expect(screen.getByText("The preload bridge is unavailable in this renderer context.")).toBeInTheDocument();
  });
});
