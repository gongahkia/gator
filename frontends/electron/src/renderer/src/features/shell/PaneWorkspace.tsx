import { useEffect, useRef, useState } from "react";
import type { DragEvent, FormEvent, PointerEvent as ReactPointerEvent, ReactNode } from "react";
import { ExternalLink, GripVertical, Plus, X } from "lucide-react";
import { Button, cx } from "../../components/primitives";
import { getPlannerSection, type SectionId } from "../../data/mockPlanner";
import { SectionContent, type TaskSurfaceCommand } from "../core/CoreScreens";
import {
  activeSplitPaneWebTab,
  clampPaneRatio,
  createSplitPaneWebTab,
  maxPaneLeaves,
  paneSectionIds,
  splitPaneUrlLabel,
  splitPaneWebContentTitle,
  splitPaneWebUrl,
  type PaneContent,
  type PaneDropZone,
  type PaneLeafNode,
  type PaneNode,
  type PaneSplitDirection
} from "./paneWorkspaceModel";

const paneDragDataType = "application/x-hcb-pane-id";

export function PaneWorkspace({
  activeSectionId,
  canSplit,
  focusedPaneId,
  onClosePane,
  onFocusPane,
  onMovePane,
  onOpenWebPage,
  onReplacePane,
  onSetSplitRatio,
  onSplitPane,
  root,
  taskCommand,
  visibleCalendarIds,
  visibleSectionIds
}: {
  activeSectionId: SectionId;
  canSplit: boolean;
  focusedPaneId: string;
  onClosePane: (paneId: string) => void;
  onFocusPane: (paneId: string) => void;
  onMovePane: (sourcePaneId: string, targetPaneId: string, dropZone: PaneDropZone) => void;
  onOpenWebPage: (rawUrl: string, label: string | null, paneId: string) => boolean;
  onReplacePane: (paneId: string, content: PaneContent) => void;
  onSetSplitRatio: (splitId: string, ratio: number) => void;
  onSplitPane: (paneId: string, direction: PaneSplitDirection, content?: PaneContent) => void;
  root: PaneNode;
  taskCommand?: TaskSurfaceCommand | null;
  visibleCalendarIds: ReadonlySet<string>;
  visibleSectionIds: SectionId[];
}): JSX.Element {
  const openSectionIds = new Set(paneSectionIds(root));

  return (
    <div className="flex min-h-0 min-w-0 flex-1 overflow-hidden" data-testid="pane-workspace">
      <PaneNodeView
        activeSectionId={activeSectionId}
        canSplit={canSplit}
        focusedPaneId={focusedPaneId}
        node={root}
        onClosePane={onClosePane}
        onFocusPane={onFocusPane}
        onMovePane={onMovePane}
        onOpenWebPage={onOpenWebPage}
        onReplacePane={onReplacePane}
        onSetSplitRatio={onSetSplitRatio}
        onSplitPane={onSplitPane}
        openSectionIds={openSectionIds}
        taskCommand={taskCommand}
        visibleCalendarIds={visibleCalendarIds}
        visibleSectionIds={visibleSectionIds}
      />
    </div>
  );
}

function PaneNodeView({
  activeSectionId,
  canSplit,
  focusedPaneId,
  node,
  onClosePane,
  onFocusPane,
  onMovePane,
  onOpenWebPage,
  onReplacePane,
  onSetSplitRatio,
  onSplitPane,
  openSectionIds,
  taskCommand,
  visibleCalendarIds,
  visibleSectionIds
}: {
  activeSectionId: SectionId;
  canSplit: boolean;
  focusedPaneId: string;
  node: PaneNode;
  onClosePane: (paneId: string) => void;
  onFocusPane: (paneId: string) => void;
  onMovePane: (sourcePaneId: string, targetPaneId: string, dropZone: PaneDropZone) => void;
  onOpenWebPage: (rawUrl: string, label: string | null, paneId: string) => boolean;
  onReplacePane: (paneId: string, content: PaneContent) => void;
  onSetSplitRatio: (splitId: string, ratio: number) => void;
  onSplitPane: (paneId: string, direction: PaneSplitDirection, content?: PaneContent) => void;
  openSectionIds: ReadonlySet<SectionId>;
  taskCommand?: TaskSurfaceCommand | null;
  visibleCalendarIds: ReadonlySet<string>;
  visibleSectionIds: SectionId[];
}): JSX.Element {
  if (node.kind === "leaf") {
    return (
      <PaneLeaf
        activeSectionId={activeSectionId}
        canSplit={canSplit}
        focused={focusedPaneId === node.id}
        leaf={node}
        onClosePane={onClosePane}
        onFocusPane={onFocusPane}
        onMovePane={onMovePane}
        onOpenWebPage={onOpenWebPage}
        onReplacePane={onReplacePane}
        onSplitPane={onSplitPane}
        openSectionIds={openSectionIds}
        taskCommand={taskCommand}
        visibleCalendarIds={visibleCalendarIds}
        visibleSectionIds={visibleSectionIds}
      />
    );
  }

  return (
    <div
      className={cx(
        "flex min-h-0 min-w-0 flex-1 overflow-hidden",
        node.direction === "row" ? "flex-row" : "flex-col"
      )}
      data-pane-direction={node.direction}
      data-testid="pane-split"
    >
      <div className="flex min-h-0 min-w-0" style={{ flex: `0 0 ${node.ratio * 100}%` }}>
        <PaneNodeView
          activeSectionId={activeSectionId}
          canSplit={canSplit}
          focusedPaneId={focusedPaneId}
          node={node.children[0]}
          onClosePane={onClosePane}
          onFocusPane={onFocusPane}
          onMovePane={onMovePane}
          onOpenWebPage={onOpenWebPage}
          onReplacePane={onReplacePane}
          onSetSplitRatio={onSetSplitRatio}
          onSplitPane={onSplitPane}
          openSectionIds={openSectionIds}
          taskCommand={taskCommand}
          visibleCalendarIds={visibleCalendarIds}
          visibleSectionIds={visibleSectionIds}
        />
      </div>
      <PaneDivider
        direction={node.direction}
        onResize={(ratio) => onSetSplitRatio(node.id, ratio)}
      />
      <div className="flex min-h-0 min-w-0 flex-1">
        <PaneNodeView
          activeSectionId={activeSectionId}
          canSplit={canSplit}
          focusedPaneId={focusedPaneId}
          node={node.children[1]}
          onClosePane={onClosePane}
          onFocusPane={onFocusPane}
          onMovePane={onMovePane}
          onOpenWebPage={onOpenWebPage}
          onReplacePane={onReplacePane}
          onSetSplitRatio={onSetSplitRatio}
          onSplitPane={onSplitPane}
          openSectionIds={openSectionIds}
          taskCommand={taskCommand}
          visibleCalendarIds={visibleCalendarIds}
          visibleSectionIds={visibleSectionIds}
        />
      </div>
    </div>
  );
}

function PaneLeaf({
  activeSectionId,
  canSplit,
  focused,
  leaf,
  onClosePane,
  onFocusPane,
  onMovePane,
  onOpenWebPage,
  onReplacePane,
  onSplitPane,
  openSectionIds,
  taskCommand,
  visibleCalendarIds,
  visibleSectionIds
}: {
  activeSectionId: SectionId;
  canSplit: boolean;
  focused: boolean;
  leaf: PaneLeafNode;
  onClosePane: (paneId: string) => void;
  onFocusPane: (paneId: string) => void;
  onMovePane: (sourcePaneId: string, targetPaneId: string, dropZone: PaneDropZone) => void;
  onOpenWebPage: (rawUrl: string, label: string | null, paneId: string) => boolean;
  onReplacePane: (paneId: string, content: PaneContent) => void;
  onSplitPane: (paneId: string, direction: PaneSplitDirection, content?: PaneContent) => void;
  openSectionIds: ReadonlySet<SectionId>;
  taskCommand?: TaskSurfaceCommand | null;
  visibleCalendarIds: ReadonlySet<string>;
  visibleSectionIds: SectionId[];
}): JSX.Element {
  const title = paneContentTitle(leaf.content);
  const [dropZone, setDropZone] = useState<PaneDropZone | null>(null);

  function handleDragOver(event: DragEvent<HTMLElement>): void {
    const dragTypes = Array.from(event.dataTransfer.types ?? []);

    if (dragTypes.length > 0 && !dragTypes.includes(paneDragDataType)) {
      return;
    }

    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
    setDropZone(paneDropZone(event));
  }

  function handleDrop(event: DragEvent<HTMLElement>): void {
    const sourcePaneId = event.dataTransfer.getData(paneDragDataType);

    if (!sourcePaneId || sourcePaneId === leaf.id) {
      setDropZone(null);
      return;
    }

    event.preventDefault();
    const nextDropZone = dropZone ?? paneDropZone(event);
    setDropZone(null);
    onMovePane(sourcePaneId, leaf.id, nextDropZone);
  }

  return (
    <section
      aria-label={`${title} pane`}
      className={cx(
        "relative flex min-h-[240px] min-w-[320px] flex-1 flex-col overflow-hidden border border-border bg-bg-primary",
        focused ? "ring-1 ring-inset ring-accent" : "ring-0"
      )}
      data-pane-id={leaf.id}
      data-testid="pane-leaf"
      onClick={() => onFocusPane(leaf.id)}
      onDragLeave={(event) => {
        const nextTarget = event.relatedTarget;

        if (nextTarget instanceof Node && event.currentTarget.contains(nextTarget)) {
          return;
        }

        setDropZone(null);
      }}
      onDragOver={handleDragOver}
      onDrop={handleDrop}
      role="region"
    >
      <div className="flex min-h-10 shrink-0 items-center justify-between gap-2 border-b border-border bg-bg-secondary px-2">
        <div
          aria-label={`Drag pane ${title}`}
          className="flex min-w-0 flex-1 cursor-grab items-center gap-2 text-left active:cursor-grabbing"
          draggable
          onDragStart={(event) => {
            event.dataTransfer.setData(paneDragDataType, leaf.id);
            event.dataTransfer.effectAllowed = "move";
          }}
          role="button"
          tabIndex={0}
        >
          <GripVertical aria-hidden="true" className="shrink-0 text-text-muted" size={15} />
          <h2 className="truncate text-[var(--text-sm)] font-semibold text-text-primary">{title}</h2>
        </div>
        <div className="flex shrink-0 items-center gap-1" role="toolbar" aria-label={`${title} pane actions`}>
          <Button
            aria-label={`Close ${title} pane`}
            className="min-h-8 gap-2 px-2"
            onClick={() => onClosePane(leaf.id)}
            title="Close pane"
            variant="ghost"
          >
            <X aria-hidden="true" size={18} />
          </Button>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-hidden">
        <PaneLeafContent
          activeSectionId={activeSectionId}
          leaf={leaf}
          openSectionIds={openSectionIds}
          onReplacePane={onReplacePane}
          taskCommand={taskCommand}
          onOpenWebPage={(rawUrl, label) => onOpenWebPage(rawUrl, label, leaf.id)}
          visibleCalendarIds={visibleCalendarIds}
          visibleSectionIds={visibleSectionIds}
        />
      </div>
      {dropZone ? (
        <div
          aria-hidden="true"
          className={paneDropPreviewClass(dropZone)}
          data-pane-drop-preview={dropZone}
        />
      ) : null}
    </section>
  );
}

function PaneLeafContent({
  activeSectionId,
  leaf,
  onOpenWebPage,
  openSectionIds,
  onReplacePane,
  taskCommand,
  visibleCalendarIds,
  visibleSectionIds
}: {
  activeSectionId: SectionId;
  leaf: PaneLeafNode;
  onOpenWebPage: (rawUrl: string, label: string | null) => boolean;
  openSectionIds: ReadonlySet<SectionId>;
  onReplacePane: (paneId: string, content: PaneContent) => void;
  taskCommand?: TaskSurfaceCommand | null;
  visibleCalendarIds: ReadonlySet<string>;
  visibleSectionIds: SectionId[];
}): JSX.Element {
  if (leaf.content.kind === "chooser") {
    return (
      <PaneChooser
        onOpenWebPage={onOpenWebPage}
        openSectionIds={openSectionIds}
        onSelectSection={(sectionId) => onReplacePane(leaf.id, { kind: "section", sectionId })}
        visibleSectionIds={visibleSectionIds}
      />
    );
  }

  if (leaf.content.kind === "web") {
    return (
      <WebPaneContent content={leaf.content} onReplacePane={onReplacePane} paneId={leaf.id} />
    );
  }

  return (
    <section className="h-full min-h-0 overflow-auto p-2 sm:p-3" aria-label={`${paneContentTitle(leaf.content)} split content`}>
      <SectionContent
        activeSectionId={leaf.content.sectionId}
        taskCommand={taskCommand?.paneId === leaf.id ? taskCommand : null}
        visibleCalendarIds={visibleCalendarIds}
      />
    </section>
  );
}

type WebviewElement = HTMLElement & {
  getTitle?: () => string;
};

function WebPaneContent({
  content,
  onReplacePane,
  paneId
}: {
  content: Extract<PaneContent, { kind: "web" }>;
  onReplacePane: (paneId: string, content: PaneContent) => void;
  paneId: string;
}): JSX.Element {
  const webviewRef = useRef<WebviewElement | null>(null);
  const activeTab = activeSplitPaneWebTab(content);
  const [webPageUrl, setWebPageUrl] = useState("");
  const [webPageError, setWebPageError] = useState<string | null>(null);

  function replaceWebContent(nextContent: Extract<PaneContent, { kind: "web" }>): void {
    onReplacePane(paneId, nextContent);
  }

  function selectTab(tabId: string): void {
    replaceWebContent({ ...content, activeTabId: tabId });
  }

  function closeTab(tabId: string): void {
    if (content.tabs.length <= 1) {
      onReplacePane(paneId, { kind: "chooser" });
      return;
    }

    const nextTabs = content.tabs.filter((tab) => tab.id !== tabId);
    const nextActiveTabId = content.activeTabId === tabId ? nextTabs.at(-1)?.id ?? nextTabs[0].id : content.activeTabId;
    replaceWebContent({ ...content, activeTabId: nextActiveTabId, tabs: nextTabs });
  }

  function newTab(): void {
    const tab = createSplitPaneWebTab("", "New tab");
    replaceWebContent({
      ...content,
      activeTabId: tab.id,
      tabs: [...content.tabs, tab].slice(-12)
    });
  }

  function openActiveTab(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    const url = splitPaneWebUrl(webPageUrl, window.location.href);

    if (!url) {
      setWebPageError("Enter an http or https URL.");
      return;
    }

    setWebPageError(null);
    setWebPageUrl("");
    replaceWebContent({
      ...content,
      tabs: content.tabs.map((tab) =>
        tab.id === activeTab.id ? { ...tab, title: splitPaneUrlLabel(url), url } : tab
      )
    });
  }

  useEffect(() => {
    const webview = webviewRef.current;

    if (!webview || !activeTab.url) {
      return;
    }

    function syncTitle(event?: Event): void {
      const eventTitle = event ? (event as Event & { title?: string }).title : undefined;
      const title = (eventTitle ?? webviewRef.current?.getTitle?.() ?? "").trim();

      if (!title || title === activeTab.title) {
        return;
      }

      replaceWebContent({
        ...content,
        tabs: content.tabs.map((tab) =>
          tab.id === activeTab.id ? { ...tab, title } : tab
        )
      });
    }

    const listener: EventListener = (event) => syncTitle(event);
    webview.addEventListener("page-title-updated", listener);
    webview.addEventListener("did-finish-load", listener);
    webview.addEventListener("dom-ready", listener);

    return () => {
      webview.removeEventListener("page-title-updated", listener);
      webview.removeEventListener("did-finish-load", listener);
      webview.removeEventListener("dom-ready", listener);
    };
  }, [activeTab.id, activeTab.title, activeTab.url, content, onReplacePane, paneId]);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex min-h-9 items-center gap-1 overflow-x-auto border-b border-border bg-bg-secondary px-2">
        {content.tabs.map((tab) => (
          <button
            aria-label={`Select ${tab.title}`}
            className={cx(
              "flex h-7 min-w-28 max-w-52 items-center gap-1 rounded-hcbSm px-2 text-left text-[var(--text-xs)] transition-colors duration-fast ease-hcb focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
              tab.id === activeTab.id ? "bg-surface-0 text-text-primary" : "text-text-muted hover:bg-surface-0"
            )}
            key={tab.id}
            onClick={() => selectTab(tab.id)}
            type="button"
          >
            <span className="min-w-0 flex-1 truncate">{tab.title}</span>
            <span
              aria-label={`Close ${tab.title}`}
              className="rounded-hcbSm p-0.5 hover:bg-bg-tertiary"
              onClick={(event) => {
                event.stopPropagation();
                closeTab(tab.id);
              }}
              role="button"
              tabIndex={-1}
            >
              <X aria-hidden="true" size={12} />
            </span>
          </button>
        ))}
        <Button
          aria-label="New web tab"
          className="size-7 shrink-0 px-0"
          onClick={newTab}
          title="New web tab"
          variant="ghost"
        >
          <Plus aria-hidden="true" size={14} />
        </Button>
      </div>
      {activeTab.url ? (
        <>
          <div className="flex min-h-8 items-center gap-2 border-b border-border px-3 text-[var(--text-xs)] text-text-muted">
            <ExternalLink aria-hidden="true" size={13} />
            <span className="truncate">{splitPaneUrlLabel(activeTab.url)}</span>
          </div>
          <webview
            className="min-h-0 flex-1 bg-bg-primary"
            data-testid="split-webview"
            key={`${paneId}:${activeTab.id}:${activeTab.url}`}
            partition={`persist:hcb-split-pane-${paneId}-${activeTab.id}`}
            ref={(node) => {
              webviewRef.current = node as WebviewElement | null;
            }}
            src={activeTab.url}
            webpreferences="contextIsolation=yes,nodeIntegration=no,sandbox=yes"
          />
        </>
      ) : (
        <div className="grid min-h-0 flex-1 place-items-center p-4">
          <form className="grid w-full max-w-lg gap-2" onSubmit={openActiveTab}>
            <div className="flex min-w-0 gap-2">
              <input
                aria-label="Webpage URL"
                autoFocus
                className="h-9 min-w-0 flex-1 rounded-hcbMd border border-border bg-surface-0 px-3 text-[var(--text-base)] text-text-primary placeholder:text-text-muted focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
                onChange={(event) => {
                  setWebPageUrl(event.target.value);
                  setWebPageError(null);
                }}
                placeholder="https://example.com"
                type="text"
                value={webPageUrl}
              />
              <Button disabled={webPageUrl.trim().length === 0} type="submit" variant="secondary">
                Open
              </Button>
            </div>
            {webPageError ? <p className="text-[var(--text-xs)] text-danger">{webPageError}</p> : null}
          </form>
        </div>
      )}
    </div>
  );
}

function PaneChooser({
  onOpenWebPage,
  openSectionIds,
  onSelectSection,
  visibleSectionIds
}: {
  onOpenWebPage: (rawUrl: string, label: string | null) => boolean;
  openSectionIds: ReadonlySet<SectionId>;
  onSelectSection: (sectionId: SectionId) => void;
  visibleSectionIds: SectionId[];
}): JSX.Element {
  const appSections = visibleSectionIds.filter((sectionId) => !openSectionIds.has(sectionId));
  const [webPageUrl, setWebPageUrl] = useState("");
  const [webPageError, setWebPageError] = useState<string | null>(null);

  function handleOpenWebPage(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    const accepted = onOpenWebPage(webPageUrl, null);

    if (!accepted) {
      setWebPageError("Enter an http or https URL.");
      return;
    }

    setWebPageError(null);
  }

  return (
    <div className="grid h-full min-h-0 place-items-center overflow-auto p-4">
      <div className="grid w-full max-w-3xl gap-5">
        <div className="grid gap-2">
          {appSections.length > 0 ? appSections.map((sectionId) => {
            const section = getPlannerSection(sectionId);
            const Icon = section.icon;

            return (
              <button
                className="flex min-h-12 min-w-0 items-center gap-3 rounded-hcbMd border border-border bg-bg-secondary px-3 py-2 text-left transition-colors duration-fast ease-hcb hover:bg-surface-0 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
                key={sectionId}
                onClick={() => onSelectSection(sectionId)}
                type="button"
              >
                <Icon aria-hidden="true" className="shrink-0 text-text-muted" size={17} />
                <span className="min-w-0">
                  <span className="block truncate text-[var(--text-base)] font-medium text-text-primary">{section.title}</span>
                  <span className="block truncate text-[var(--text-xs)] text-text-muted">{section.subtitle}</span>
                </span>
              </button>
            );
          }) : (
            <p className="rounded-hcbMd border border-dashed border-border px-3 py-4 text-[var(--text-sm)] text-text-muted">
              All app tabs are already open.
            </p>
          )}
          <div className="grid gap-2 rounded-hcbMd border border-border bg-bg-secondary px-3 py-2">
            <div className="flex min-h-12 min-w-0 items-center gap-3 text-left">
              <ExternalLink aria-hidden="true" className="shrink-0 text-text-muted" size={17} />
              <span className="min-w-0">
                <span className="block truncate text-[var(--text-base)] font-medium text-text-primary">Webpage</span>
                <span className="block truncate text-[var(--text-xs)] text-text-muted">Open a website in this pane</span>
              </span>
            </div>
            <form className="grid gap-2" onSubmit={handleOpenWebPage}>
              <div className="flex min-w-0 gap-2">
                <input
                  aria-label="Webpage URL"
                  className="h-9 min-w-0 flex-1 rounded-hcbMd border border-border bg-surface-0 px-3 text-[var(--text-base)] text-text-primary placeholder:text-text-muted focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
                  onChange={(event) => {
                    setWebPageUrl(event.target.value);
                    setWebPageError(null);
                  }}
                  placeholder="https://example.com"
                  type="text"
                  value={webPageUrl}
                />
                <Button
                  aria-label="Open webpage"
                  className="h-9 px-4"
                  disabled={webPageUrl.trim().length === 0}
                  title="Open webpage"
                  type="submit"
                  variant="secondary"
                >
                  Open
                </Button>
              </div>
              {webPageError ? <p className="text-[var(--text-xs)] text-danger">{webPageError}</p> : null}
            </form>
          </div>
        </div>
      </div>
    </div>
  );
}

function PaneDivider({
  direction,
  onResize
}: {
  direction: PaneSplitDirection;
  onResize: (ratio: number) => void;
}): JSX.Element {
  function handlePointerDown(event: ReactPointerEvent<HTMLDivElement>): void {
    const container = event.currentTarget.parentElement;

    if (!container) {
      return;
    }

    event.preventDefault();
    const rect = container.getBoundingClientRect();

    function handlePointerMove(moveEvent: PointerEvent): void {
      const ratio = direction === "row"
        ? (moveEvent.clientX - rect.left) / Math.max(rect.width, 1)
        : (moveEvent.clientY - rect.top) / Math.max(rect.height, 1);

      onResize(clampPaneRatio(ratio));
    }

    function handlePointerUp(): void {
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
    }

    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp);
  }

  return (
    <div
      aria-orientation={direction === "row" ? "vertical" : "horizontal"}
      className={cx(
        "shrink-0 bg-border transition-colors duration-fast ease-hcb hover:bg-accent",
        direction === "row" ? "w-1 cursor-col-resize" : "h-1 cursor-row-resize"
      )}
      data-testid="pane-divider"
      onPointerDown={handlePointerDown}
      role="separator"
      tabIndex={0}
    />
  );
}

function paneContentTitle(content: PaneContent): string {
  if (content.kind === "chooser") {
    return "Choose split view";
  }

  if (content.kind === "web") {
    return splitPaneWebContentTitle(content);
  }

  return getPlannerSection(content.sectionId).title;
}

function paneDropZone(event: DragEvent<HTMLElement>): PaneDropZone {
  const rect = event.currentTarget.getBoundingClientRect();
  const x = rect.width > 0 ? (event.clientX - rect.left) / rect.width : 0.5;
  const y = rect.height > 0 ? (event.clientY - rect.top) / rect.height : 0.5;

  if (x >= 0.25 && x <= 0.75 && y >= 0.25 && y <= 0.75) {
    return "center";
  }

  const distances: Array<[Exclude<PaneDropZone, "center">, number]> = [
    ["left", x],
    ["right", 1 - x],
    ["top", y],
    ["bottom", 1 - y]
  ];

  return distances.sort((first, second) => first[1] - second[1])[0][0];
}

function paneDropPreviewClass(dropZone: PaneDropZone): string {
  const base = "pointer-events-none absolute z-10 rounded-hcbMd border-2 border-accent bg-accent/20";

  if (dropZone === "left") {
    return cx(base, "inset-y-2 left-2 w-[calc(50%-8px)]");
  }

  if (dropZone === "right") {
    return cx(base, "inset-y-2 right-2 w-[calc(50%-8px)]");
  }

  if (dropZone === "top") {
    return cx(base, "inset-x-2 top-2 h-[calc(50%-8px)]");
  }

  if (dropZone === "bottom") {
    return cx(base, "inset-x-2 bottom-2 h-[calc(50%-8px)]");
  }

  return cx(base, "inset-2");
}
