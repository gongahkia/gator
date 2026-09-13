import { useCallback, useEffect, useRef } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { X } from "lucide-react";
import { Badge, IconButton, cx } from "../primitives";
import { useInspector } from "./InspectorContext";

const titleId = "inspector-title";

export function InspectorShell(): JSX.Element | null {
  const { current, close, isOpen } = useInspector();
  const containerRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!isOpen) {
      return;
    }

    const node = containerRef.current;
    if (!node) {
      return;
    }

    const focusable = node.querySelector<HTMLElement>(
      'input,select,textarea,button,[tabindex]:not([tabindex="-1"]),a[href]'
    );
    queueMicrotask(() => focusable?.focus());
  }, [isOpen, current?.kind, current?.id]);

  const requestClose = useCallback(() => {
    void close();
  }, [close]);

  function handleKeyDown(event: ReactKeyboardEvent<HTMLDivElement>): void {
    if (event.key === "Escape") {
      event.preventDefault();
      requestClose();
    }
  }

  if (!current) {
    return null;
  }

  const showHeader = !current.hideHeader;

  return (
    <>
      <div
        aria-hidden="true"
        className="fixed inset-0 z-40 bg-bg-primary/70 backdrop-blur-sm"
        onClick={requestClose}
      />
      <section
        aria-label={showHeader ? undefined : current.title}
        aria-labelledby={showHeader ? titleId : undefined}
        aria-modal="true"
        className={cx(
          "fixed left-1/2 top-1/2 z-50 flex max-h-[min(820px,calc(100dvh-32px))] w-[min(760px,calc(100vw-32px))] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-hcbLg border border-border bg-bg-secondary shadow-hcbLg"
        )}
        data-inspector-kind={current.kind}
        data-inspector-id={current.id}
        data-testid="inspector-shell"
        onKeyDown={handleKeyDown}
        ref={containerRef}
        role="dialog"
        tabIndex={-1}
      >
        {showHeader ? (
          <header className="flex min-h-14 items-start justify-between gap-3 border-b border-border px-4 py-2">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <h2
                  className="min-w-0 whitespace-normal break-words text-[var(--text-lg)] font-semibold leading-snug text-text-primary"
                  id={titleId}
                >
                  {current.title}
                </h2>
                {current.dirty ? <Badge tone="warning">Unsaved</Badge> : null}
              </div>
              {current.subtitle ? (
                <p className="mt-0.5 whitespace-normal break-words text-[var(--text-xs)] leading-snug text-text-muted">{current.subtitle}</p>
              ) : null}
            </div>
            <IconButton
              data-testid="inspector-close"
              icon={X}
              label="Close inspector"
              onClick={requestClose}
              variant="ghost"
            />
          </header>
        ) : null}

        <div className="min-w-0 flex-1 overflow-x-hidden overflow-y-auto px-4 py-3" data-testid="inspector-body">
          {current.body}
        </div>

        {current.actions ? (
          <footer
            className="flex min-h-14 items-center justify-end gap-2 border-t border-border px-4"
            data-testid="inspector-actions"
          >
            {current.actions}
          </footer>
        ) : null}
      </section>
    </>
  );
}
