import { useCallback, useEffect, useMemo, useState } from "react";
import type { SectionId } from "../../data/mockPlanner";
import { writeLocalStorageJSON } from "../core/localStorageHelpers";
import {
  closePaneLeaf,
  createDefaultPaneTree,
  createSplitPaneWebContent,
  createSplitPaneWebTab,
  findPaneLeaf,
  firstPaneLeaf,
  firstPaneSectionId,
  maxPaneLeaves,
  movePaneToEdge,
  paneLeafCount,
  paneLeafIds,
  paneWorkspaceStorageKey,
  replacePaneContent,
  sanitizeStoredPaneWorkspace,
  setPaneSplitRatio,
  splitPaneLeaf,
  splitPaneWebContentTitle,
  splitPaneWebUrl,
  swapPaneContents,
  type PaneContent,
  type PaneDropZone,
  type PaneNode,
  type PaneSplitDirection
} from "./paneWorkspaceModel";

interface PaneWorkspaceState {
  focusedPaneId: string;
  root: PaneNode;
}

function readPaneWorkspaceState(): PaneWorkspaceState {
  try {
    const parsed = JSON.parse(window.localStorage?.getItem?.(paneWorkspaceStorageKey) ?? "null");
    const stored = sanitizeStoredPaneWorkspace(parsed);

    if (stored) {
      return stored;
    }
  } catch {
    // best-effort persisted UI only
  }

  const root = createDefaultPaneTree("calendar");
  return { focusedPaneId: root.id, root };
}

export function usePaneWorkspace(): {
  activeSectionId: SectionId;
  canSplit: boolean;
  closePane: (paneId: string) => void;
  focusedPaneId: string;
  focusedSectionId: SectionId | null;
  focusedTitle: string;
  focusPane: (paneId: string) => void;
  focusPaneByDirection: (direction: "left" | "right" | "up" | "down") => void;
  newWebTab: (paneId: string) => void;
  movePane: (sourcePaneId: string, targetPaneId: string, dropZone: PaneDropZone) => void;
  openChooser: () => void;
  openUrl: (url: string, label: string | null) => void;
  openWebPageInPane: (rawUrl: string, label: string | null, paneId: string) => boolean;
  replaceFocusedWithSection: (sectionId: SectionId) => void;
  replacePane: (paneId: string, content: PaneContent) => void;
  root: PaneNode;
  setSplitRatio: (splitId: string, ratio: number) => void;
  splitPane: (paneId: string, direction: PaneSplitDirection, content?: PaneContent) => void;
} {
  const [state, setState] = useState<PaneWorkspaceState>(() => readPaneWorkspaceState());
  const focusedLeaf = findPaneLeaf(state.root, state.focusedPaneId) ?? firstPaneLeaf(state.root);
  const focusedSectionId = focusedLeaf.content.kind === "section" ? focusedLeaf.content.sectionId : null;
  const activeSectionId = focusedSectionId ?? firstPaneSectionId(state.root);
  const focusedTitle = paneContentTitle(focusedLeaf.content);
  const canSplit = paneLeafCount(state.root) < maxPaneLeaves;

  useEffect(() => {
    writeLocalStorageJSON(paneWorkspaceStorageKey, state);
  }, [state]);

  const focusPane = useCallback((paneId: string): void => {
    setState((current) =>
      findPaneLeaf(current.root, paneId)
        ? { ...current, focusedPaneId: paneId }
        : current
    );
  }, []);

  const focusPaneByDirection = useCallback((direction: "left" | "right" | "up" | "down"): void => {
    setState((current) => {
      const currentElement = document.querySelector<HTMLElement>(`[data-pane-id="${current.focusedPaneId}"]`);

      if (!currentElement) {
        return current;
      }

      const currentRect = currentElement.getBoundingClientRect();
      const currentCenter = {
        x: currentRect.left + currentRect.width / 2,
        y: currentRect.top + currentRect.height / 2
      };
      const candidates = paneLeafIds(current.root)
        .filter((paneId) => paneId !== current.focusedPaneId)
        .flatMap((paneId) => {
          const element = document.querySelector<HTMLElement>(`[data-pane-id="${paneId}"]`);

          if (!element) {
            return [];
          }

          const rect = element.getBoundingClientRect();
          const center = {
            x: rect.left + rect.width / 2,
            y: rect.top + rect.height / 2
          };
          const dx = center.x - currentCenter.x;
          const dy = center.y - currentCenter.y;

          if (
            (direction === "left" && dx >= 0) ||
            (direction === "right" && dx <= 0) ||
            (direction === "up" && dy >= 0) ||
            (direction === "down" && dy <= 0)
          ) {
            return [];
          }

          const primary = direction === "left" || direction === "right" ? Math.abs(dx) : Math.abs(dy);
          const secondary = direction === "left" || direction === "right" ? Math.abs(dy) : Math.abs(dx);

          return [{ paneId, score: primary * 1_000 + secondary }];
        })
        .sort((left, right) => left.score - right.score);
      const nextPaneId = candidates[0]?.paneId;

      return nextPaneId ? { ...current, focusedPaneId: nextPaneId } : current;
    });
  }, []);

  const replacePane = useCallback((paneId: string, content: PaneContent): void => {
    setState((current) =>
      findPaneLeaf(current.root, paneId)
        ? {
            ...current,
            focusedPaneId: paneId,
            root: replacePaneContent(current.root, paneId, content)
          }
        : current
    );
  }, []);

  const replaceFocusedWithSection = useCallback((sectionId: SectionId): void => {
    setState((current) => ({
      ...current,
      root: replacePaneContent(current.root, current.focusedPaneId, {
        kind: "section",
        sectionId
      })
    }));
  }, []);

  const splitPane = useCallback((
    paneId: string,
    direction: PaneSplitDirection,
    content: PaneContent = { kind: "chooser" }
  ): void => {
    setState((current) => {
      if (!findPaneLeaf(current.root, paneId) || paneLeafCount(current.root) >= maxPaneLeaves) {
        return current;
      }

      const result = splitPaneLeaf(current.root, paneId, direction, content);
      return result.newPaneId
        ? { ...current, focusedPaneId: result.newPaneId, root: result.node }
        : current;
    });
  }, []);

  const openChooser = useCallback((): void => {
    setState((current) => {
      if (paneLeafCount(current.root) >= maxPaneLeaves) {
        return {
          ...current,
          root: replacePaneContent(current.root, current.focusedPaneId, { kind: "chooser" })
        };
      }

      const result = splitPaneLeaf(current.root, current.focusedPaneId, "row", { kind: "chooser" });
      return result.newPaneId
        ? { ...current, focusedPaneId: result.newPaneId, root: result.node }
        : current;
    });
  }, []);

  const closePane = useCallback((paneId: string): void => {
    const currentLeaf = findPaneLeaf(state.root, paneId);

    if (paneLeafCount(state.root) <= 1 && currentLeaf?.content.kind === "chooser") {
      window.close();
      return;
    }

    setState((current) => {
      if (paneLeafCount(current.root) <= 1) {
        const leaf = findPaneLeaf(current.root, paneId);

        return leaf
          ? {
              ...current,
              focusedPaneId: leaf.id,
              root: replacePaneContent(current.root, leaf.id, { kind: "chooser" })
            }
          : current;
      }

      const nextRoot = closePaneLeaf(current.root, paneId);

      if (!nextRoot) {
        return current;
      }

      const nextFocusedPaneId = findPaneLeaf(nextRoot, current.focusedPaneId)
        ? current.focusedPaneId
        : firstPaneLeaf(nextRoot).id;

      return { ...current, focusedPaneId: nextFocusedPaneId, root: nextRoot };
    });
  }, [state.root]);

  const setSplitRatio = useCallback((splitId: string, ratio: number): void => {
    setState((current) => ({
      ...current,
      root: setPaneSplitRatio(current.root, splitId, ratio)
    }));
  }, []);

  const movePane = useCallback((sourcePaneId: string, targetPaneId: string, dropZone: PaneDropZone): void => {
    setState((current) => {
      if (!findPaneLeaf(current.root, sourcePaneId) || !findPaneLeaf(current.root, targetPaneId)) {
        return current;
      }

      if (dropZone === "center") {
        return {
          ...current,
          focusedPaneId: targetPaneId,
          root: swapPaneContents(current.root, sourcePaneId, targetPaneId)
        };
      }

      const result = movePaneToEdge(current.root, sourcePaneId, targetPaneId, dropZone);
      return result.movedPaneId
        ? { ...current, focusedPaneId: result.movedPaneId, root: result.node }
        : current;
    });
  }, []);

  const openUrl = useCallback((rawUrl: string, label: string | null): void => {
    const url = splitPaneWebUrl(rawUrl, window.location.href);

    if (!url) {
      return;
    }

    setState((current) => {
      const leafIds = paneLeafIds(current.root);
      const secondaryPaneId = leafIds.find((paneId) => paneId !== current.focusedPaneId);
      const content = createSplitPaneWebContent(url, label);

      if (secondaryPaneId) {
        return {
          ...current,
          focusedPaneId: secondaryPaneId,
          root: replacePaneContent(current.root, secondaryPaneId, content)
        };
      }

      const result = splitPaneLeaf(current.root, current.focusedPaneId, "row", content);
      return result.newPaneId
        ? { ...current, focusedPaneId: result.newPaneId, root: result.node }
        : current;
    });
  }, []);

  const openWebPageInPane = useCallback((rawUrl: string, label: string | null, paneId: string): boolean => {
    const url = splitPaneWebUrl(rawUrl, window.location.href);

    if (!url) {
      return false;
    }

    setState((current) =>
      findPaneLeaf(current.root, paneId)
        ? {
            ...current,
            focusedPaneId: paneId,
            root: replacePaneContent(current.root, paneId, createSplitPaneWebContent(url, label))
          }
        : current
    );

    return true;
  }, []);

  const newWebTab = useCallback((paneId: string): void => {
    setState((current) => {
      const leaf = findPaneLeaf(current.root, paneId);

      if (leaf?.content.kind !== "web") {
        return current;
      }

      const tab = createSplitPaneWebTab("", "New tab");
      return {
        ...current,
        focusedPaneId: paneId,
        root: replacePaneContent(current.root, paneId, {
          ...leaf.content,
          activeTabId: tab.id,
          tabs: [...leaf.content.tabs, tab].slice(-12)
        })
      };
    });
  }, []);

  return useMemo(
    () => ({
      activeSectionId,
      canSplit,
      closePane,
      focusedPaneId: state.focusedPaneId,
      focusedSectionId,
      focusedTitle,
      focusPane,
      focusPaneByDirection,
      newWebTab,
      movePane,
      openChooser,
      openUrl,
      openWebPageInPane,
      replaceFocusedWithSection,
      replacePane,
      root: state.root,
      setSplitRatio,
      splitPane
    }),
    [
      activeSectionId,
      canSplit,
      closePane,
      focusPane,
      focusPaneByDirection,
      focusedSectionId,
      focusedTitle,
      movePane,
      newWebTab,
      openChooser,
      openUrl,
      openWebPageInPane,
      replaceFocusedWithSection,
      replacePane,
      setSplitRatio,
      splitPane,
      state.focusedPaneId,
      state.root
    ]
  );
}

function paneContentTitle(content: PaneContent): string {
  if (content.kind === "chooser") {
    return "Choose split view";
  }

  if (content.kind === "web") {
    return splitPaneWebContentTitle(content);
  }

  return content.sectionId[0].toUpperCase() + content.sectionId.slice(1);
}
