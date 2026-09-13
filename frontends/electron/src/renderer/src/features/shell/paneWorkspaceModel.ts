import type { SectionId } from "../../data/mockPlanner";

export const paneWorkspaceStorageKey = "hcb.paneWorkspace.v1";
export const maxPaneLeaves = 4;

export type PaneSplitDirection = "row" | "column";
export type PaneDropZone = "left" | "right" | "top" | "bottom" | "center";

export interface SplitPaneWebTab {
  id: string;
  title: string;
  url: string;
}

export type PaneContent =
  | { kind: "section"; sectionId: SectionId }
  | { kind: "web"; tabs: SplitPaneWebTab[]; activeTabId: string }
  | { kind: "chooser" };

export interface PaneLeafNode {
  id: string;
  kind: "leaf";
  content: PaneContent;
}

export interface PaneSplitNode {
  id: string;
  kind: "split";
  direction: PaneSplitDirection;
  ratio: number;
  children: [PaneNode, PaneNode];
}

export type PaneNode = PaneLeafNode | PaneSplitNode;

export interface StoredPaneWorkspace {
  focusedPaneId: string;
  root: PaneNode;
}

const validSectionIds = new Set(["tasks", "calendar", "notes", "settings"]);
let paneIdCounter = 0;

export function createPaneId(): string {
  paneIdCounter += 1;
  return `pane-${Date.now().toString(36)}-${paneIdCounter}`;
}

export function createDefaultPaneTree(sectionId: SectionId = "calendar"): PaneLeafNode {
  return {
    id: createPaneId(),
    kind: "leaf",
    content: { kind: "section", sectionId }
  };
}

export function paneLeafIds(node: PaneNode): string[] {
  if (node.kind === "leaf") {
    return [node.id];
  }

  return node.children.flatMap(paneLeafIds);
}

export function paneLeafCount(node: PaneNode): number {
  return paneLeafIds(node).length;
}

export function findPaneLeaf(node: PaneNode, paneId: string): PaneLeafNode | null {
  if (node.kind === "leaf") {
    return node.id === paneId ? node : null;
  }

  return findPaneLeaf(node.children[0], paneId) ?? findPaneLeaf(node.children[1], paneId);
}

export function firstPaneLeaf(node: PaneNode): PaneLeafNode {
  return node.kind === "leaf" ? node : firstPaneLeaf(node.children[0]);
}

export function firstPaneSectionId(node: PaneNode): SectionId {
  const leaves = paneLeafIds(node)
    .map((leafId) => findPaneLeaf(node, leafId))
    .filter((leaf): leaf is PaneLeafNode => Boolean(leaf));
  const sectionLeaf = leaves.find((leaf) => leaf.content.kind === "section");

  return sectionLeaf?.content.kind === "section" ? sectionLeaf.content.sectionId : "calendar";
}

export function paneSectionIds(node: PaneNode): SectionId[] {
  return paneLeafIds(node).flatMap((leafId) => {
    const leaf = findPaneLeaf(node, leafId);
    return leaf?.content.kind === "section" ? [leaf.content.sectionId] : [];
  });
}

export function replacePaneContent(node: PaneNode, paneId: string, content: PaneContent): PaneNode {
  if (node.kind === "leaf") {
    return node.id === paneId ? { ...node, content } : node;
  }

  return {
    ...node,
    children: [
      replacePaneContent(node.children[0], paneId, content),
      replacePaneContent(node.children[1], paneId, content)
    ]
  };
}

export function splitPaneLeaf(
  node: PaneNode,
  paneId: string,
  direction: PaneSplitDirection,
  content: PaneContent,
  placement: "before" | "after" = "after"
): { node: PaneNode; newPaneId: string | null } {
  if (node.kind === "leaf") {
    if (node.id !== paneId) {
      return { node, newPaneId: null };
    }

    const newLeaf: PaneLeafNode = { id: createPaneId(), kind: "leaf", content };
    const children: [PaneNode, PaneNode] = placement === "before" ? [newLeaf, node] : [node, newLeaf];

    return {
      node: {
        id: createPaneId(),
        kind: "split",
        direction,
        ratio: 0.5,
        children
      },
      newPaneId: newLeaf.id
    };
  }

  const left = splitPaneLeaf(node.children[0], paneId, direction, content, placement);
  if (left.newPaneId) {
    return { node: { ...node, children: [left.node, node.children[1]] }, newPaneId: left.newPaneId };
  }

  const right = splitPaneLeaf(node.children[1], paneId, direction, content, placement);
  return {
    node: right.newPaneId ? { ...node, children: [node.children[0], right.node] } : node,
    newPaneId: right.newPaneId
  };
}

export function closePaneLeaf(node: PaneNode, paneId: string): PaneNode | null {
  if (node.kind === "leaf") {
    return node.id === paneId ? null : node;
  }

  const first = closePaneLeaf(node.children[0], paneId);
  const second = closePaneLeaf(node.children[1], paneId);

  if (!first && !second) {
    return null;
  }

  if (!first) {
    return second;
  }

  if (!second) {
    return first;
  }

  return { ...node, children: [first, second] };
}

export function setPaneSplitRatio(node: PaneNode, splitId: string, ratio: number): PaneNode {
  if (node.kind === "leaf") {
    return node;
  }

  if (node.id === splitId) {
    return { ...node, ratio: clampPaneRatio(ratio) };
  }

  return {
    ...node,
    children: [
      setPaneSplitRatio(node.children[0], splitId, ratio),
      setPaneSplitRatio(node.children[1], splitId, ratio)
    ]
  };
}

export function swapPaneContents(node: PaneNode, sourcePaneId: string, targetPaneId: string): PaneNode {
  const source = findPaneLeaf(node, sourcePaneId);
  const target = findPaneLeaf(node, targetPaneId);

  if (!source || !target || source.id === target.id) {
    return node;
  }

  return replacePaneContent(
    replacePaneContent(node, source.id, target.content),
    target.id,
    source.content
  );
}

export function movePaneToEdge(
  node: PaneNode,
  sourcePaneId: string,
  targetPaneId: string,
  edge: Exclude<PaneDropZone, "center">
): { node: PaneNode; movedPaneId: string | null } {
  const source = findPaneLeaf(node, sourcePaneId);

  if (!source || sourcePaneId === targetPaneId || paneLeafCount(node) < 2) {
    return { node, movedPaneId: null };
  }

  const withoutSource = closePaneLeaf(node, sourcePaneId);

  if (!withoutSource || !findPaneLeaf(withoutSource, targetPaneId)) {
    return { node, movedPaneId: null };
  }

  const direction: PaneSplitDirection = edge === "left" || edge === "right" ? "row" : "column";
  const placement = edge === "left" || edge === "top" ? "before" : "after";
  const result = splitPaneLeaf(withoutSource, targetPaneId, direction, source.content, placement);
  return { node: result.node, movedPaneId: result.newPaneId };
}

export function clampPaneRatio(value: number): number {
  if (!Number.isFinite(value)) {
    return 0.5;
  }

  return Math.min(0.8, Math.max(0.2, value));
}

export function splitPaneWebUrl(value: string | null, baseUrl: string): string | null {
  if (!value) {
    return null;
  }

  const trimmed = value.trim();
  const candidate = /^[a-z][a-z0-9+.-]*:/i.test(trimmed) || trimmed.startsWith("/")
    ? trimmed
    : `https://${trimmed}`;

  try {
    const url = new URL(candidate, baseUrl);
    return url.protocol === "http:" || url.protocol === "https:" ? url.toString() : null;
  } catch {
    return null;
  }
}

export function splitPaneWebTitle(url: string, label: string | null): string {
  const trimmedLabel = label?.replace(/\s+/g, " ").trim();

  if (trimmedLabel) {
    return trimmedLabel.slice(0, 120);
  }

  try {
    return new URL(url).hostname;
  } catch {
    return url;
  }
}

export function createSplitPaneWebTab(url: string, label: string | null): SplitPaneWebTab {
  return {
    id: createPaneId(),
    title: splitPaneWebTitle(url, label),
    url
  };
}

export function createSplitPaneWebContent(url: string, label: string | null): Extract<PaneContent, { kind: "web" }> {
  const tab = createSplitPaneWebTab(url, label);
  return {
    kind: "web",
    activeTabId: tab.id,
    tabs: [tab]
  };
}

export function activeSplitPaneWebTab(content: Extract<PaneContent, { kind: "web" }>): SplitPaneWebTab {
  return content.tabs.find((tab) => tab.id === content.activeTabId) ?? content.tabs[0] ?? {
    id: createPaneId(),
    title: "New tab",
    url: ""
  };
}

export function splitPaneWebContentTitle(content: Extract<PaneContent, { kind: "web" }>): string {
  return activeSplitPaneWebTab(content).title;
}

export function splitPaneUrlLabel(url: string): string {
  try {
    const parsed = new URL(url);
    return `${parsed.hostname}${parsed.pathname === "/" ? "" : parsed.pathname}`;
  } catch {
    return url;
  }
}

export function sanitizeStoredPaneWorkspace(value: unknown): StoredPaneWorkspace | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }

  const record = value as Record<string, unknown>;
  const root = sanitizePaneNode(record.root, 0);

  if (!root || paneLeafCount(root) > maxPaneLeaves) {
    return null;
  }

  const focusedPaneId = typeof record.focusedPaneId === "string" && findPaneLeaf(root, record.focusedPaneId)
    ? record.focusedPaneId
    : firstPaneLeaf(root).id;
  return { root, focusedPaneId };
}

function sanitizePaneNode(value: unknown, depth: number): PaneNode | null {
  if (!value || typeof value !== "object" || Array.isArray(value) || depth > 4) {
    return null;
  }

  const record = value as Record<string, unknown>;
  const id = typeof record.id === "string" && record.id.trim() ? record.id : createPaneId();

  if (record.kind === "leaf") {
    const content = sanitizePaneContent(record.content);
    return content ? { id, kind: "leaf", content } : null;
  }

  if (record.kind === "split" && Array.isArray(record.children) && record.children.length === 2) {
    const first = sanitizePaneNode(record.children[0], depth + 1);
    const second = sanitizePaneNode(record.children[1], depth + 1);
    const direction = record.direction === "column" ? "column" : "row";

    return first && second
      ? {
          id,
          kind: "split",
          direction,
          ratio: clampPaneRatio(typeof record.ratio === "number" ? record.ratio : 0.5),
          children: [first, second]
        }
      : null;
  }

  return null;
}

function sanitizePaneContent(value: unknown): PaneContent | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }

  const record = value as Record<string, unknown>;

  if (record.kind === "chooser") {
    return { kind: "chooser" };
  }

  if (record.kind === "section" && record.sectionId === "today") {
    return { kind: "section", sectionId: "calendar" };
  }

  if (record.kind === "section" && typeof record.sectionId === "string" && validSectionIds.has(record.sectionId)) {
    return { kind: "section", sectionId: record.sectionId as SectionId };
  }

  if (record.kind === "web" && typeof record.url === "string") {
    const url = splitPaneWebUrl(record.url, window.location.href);
    const title = typeof record.title === "string" && record.title.trim()
      ? record.title.trim().slice(0, 120)
      : url
        ? splitPaneWebTitle(url, null)
        : "";

    return url ? createSplitPaneWebContent(url, title) : null;
  }

  if (record.kind === "web" && Array.isArray(record.tabs)) {
    const tabs = record.tabs.flatMap((item) => {
      if (!item || typeof item !== "object" || Array.isArray(item)) {
        return [];
      }

      const tab = item as Record<string, unknown>;
      const url = typeof tab.url === "string"
        ? splitPaneWebUrl(tab.url, window.location.href)
        : null;

      if (!url) {
        return [];
      }

      return [{
        id: typeof tab.id === "string" && tab.id.trim() ? tab.id : createPaneId(),
        title: splitPaneWebTitle(url, typeof tab.title === "string" ? tab.title : null),
        url
      }];
    }).slice(0, 12);

    if (tabs.length === 0) {
      return null;
    }

    const activeTabId = typeof record.activeTabId === "string" && tabs.some((tab) => tab.id === record.activeTabId)
      ? record.activeTabId
      : tabs[0].id;

    return { kind: "web", tabs, activeTabId };
  }

  return null;
}
