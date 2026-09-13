import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen, fireEvent, act, waitFor } from "@testing-library/react";
import { useRef } from "react";
import { InspectorProvider, InspectorShell, useInspector, useDirtyState } from "./index";

afterEach(() => {
  cleanup();
});

function TriggerWithBody({ initialTitle = "Untitled" }: { initialTitle?: string }): JSX.Element {
  const { open, update } = useInspector();
  const buttonRef = useRef<HTMLButtonElement | null>(null);
  const dirty = useDirtyState({ title: initialTitle });

  function handleOpen(): void {
    open({
      kind: "task",
      id: "task-1",
      title: dirty.value.title,
      returnFocus: buttonRef,
      body: (
        <input
          aria-label="Task title"
          onChange={(event) => dirty.patch({ title: event.target.value })}
          value={dirty.value.title}
        />
      ),
      dirty: dirty.isDirty
    });
  }

  // keep inspector in sync as body edits (consumer responsibility)
  if (dirty.isDirty) {
    update({ title: dirty.value.title, dirty: true });
  }

  return (
    <button data-testid="trigger" onClick={handleOpen} ref={buttonRef} type="button">
      Open inspector
    </button>
  );
}

function renderWithProvider(ui: JSX.Element): ReturnType<typeof render> {
  return render(
    <InspectorProvider>
      {ui}
      <InspectorShell />
    </InspectorProvider>
  );
}

describe("Inspector", () => {
  it("opens with title and renders body", () => {
    renderWithProvider(<TriggerWithBody />);
    fireEvent.click(screen.getByTestId("trigger"));
    expect(screen.getByTestId("inspector-shell")).toBeInTheDocument();
    expect(screen.getByLabelText("Task title")).toBeInTheDocument();
  });

  it("closes on Escape and returns focus to invoker", async () => {
    renderWithProvider(<TriggerWithBody />);
    const trigger = screen.getByTestId("trigger");
    fireEvent.click(trigger);
    const shell = screen.getByTestId("inspector-shell");
    fireEvent.keyDown(shell, { key: "Escape" });
    await waitFor(() => expect(screen.queryByTestId("inspector-shell")).not.toBeInTheDocument());
  });

  it("blocks close when onConfirmClose returns false", () => {
    function GatedTrigger(): JSX.Element {
      const { open } = useInspector();
      return (
        <button
          data-testid="gated"
          onClick={() =>
            open({
              kind: "note",
              id: "note-1",
              title: "Gated",
              dirty: true,
              onConfirmClose: () => false,
              body: <div>note body</div>
            })
          }
          type="button"
        >
          gated
        </button>
      );
    }
    renderWithProvider(<GatedTrigger />);
    fireEvent.click(screen.getByTestId("gated"));
    fireEvent.click(screen.getByTestId("inspector-close"));
    expect(screen.getByTestId("inspector-shell")).toBeInTheDocument();
  });

  it("shows Unsaved badge when dirty", () => {
    function DirtyTrigger(): JSX.Element {
      const { open } = useInspector();
      return (
        <button
          data-testid="dirty"
          onClick={() =>
            open({ kind: "task", id: "task-x", title: "Dirty", dirty: true, body: <div>x</div> })
          }
          type="button"
        >
          dirty
        </button>
      );
    }
    renderWithProvider(<DirtyTrigger />);
    fireEvent.click(screen.getByTestId("dirty"));
    expect(screen.getByText("Unsaved")).toBeInTheDocument();
  });
});

describe("useDirtyState", () => {
  function Probe(): JSX.Element {
    const dirty = useDirtyState({ title: "a", count: 0 });
    return (
      <div>
        <span data-testid="dirty-flag">{String(dirty.isDirty)}</span>
        <span data-testid="title">{dirty.value.title}</span>
        <button data-testid="patch" onClick={() => dirty.patch({ title: "b" })} type="button">
          patch
        </button>
        <button data-testid="reset" onClick={() => dirty.reset()} type="button">
          reset
        </button>
        <button data-testid="markClean" onClick={() => dirty.markClean()} type="button">
          mark clean
        </button>
      </div>
    );
  }

  it("flags dirty after patch and clean after markClean", () => {
    render(<Probe />);
    expect(screen.getByTestId("dirty-flag")).toHaveTextContent("false");
    fireEvent.click(screen.getByTestId("patch"));
    expect(screen.getByTestId("dirty-flag")).toHaveTextContent("true");
    fireEvent.click(screen.getByTestId("markClean"));
    expect(screen.getByTestId("dirty-flag")).toHaveTextContent("false");
  });

  it("reset restores baseline", () => {
    render(<Probe />);
    fireEvent.click(screen.getByTestId("patch"));
    fireEvent.click(screen.getByTestId("reset"));
    expect(screen.getByTestId("title")).toHaveTextContent("a");
    expect(screen.getByTestId("dirty-flag")).toHaveTextContent("false");
  });
});
