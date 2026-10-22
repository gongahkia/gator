import type { BackendState, Frame } from "./protocol";

export type UpdateState = { phase: "manual" | "idle" | "checking" | "current" | "available" | "downloading" | "ready" | "installing" | "error"; installed: string; version?: string; reason?: string; percent?: number; checkedAt?: string };
export type UpdateRequest = { action: "open" | "close" | "check" | "download" | "install" | "releases" };
export type DesktopSnapshot = { state?: BackendState; error?: string; connection?: "starting" | "connected" | "disconnected"; sessionID?: string; terminalBusy?: boolean; fullScreen?: boolean; updates?: UpdateState; updatesOpen?: boolean; updatePreparing?: boolean };
export type DesktopFrame = Omit<Frame, "version">;
export type SavedDraft = { context: string; text: string; title: string; source: string; updated: number };
export type DraftSnapshot = { enabled: boolean; drafts: SavedDraft[] };
export type DraftRequest = { action: "list" | "clear" } | { action: "remove"; context: string } | { action: "configure"; enabled: boolean } | { action: "save"; draft: Omit<SavedDraft, "updated"> };

declare global {
  interface Window {
    gator?: {
      platform: string;
      snapshot(): Promise<DesktopSnapshot>;
      subscribe(callback: (snapshot: DesktopSnapshot) => void): () => void;
      send(frame: DesktopFrame): Promise<void>;
      chooseFolder(): Promise<string | null>;
      choosePath(request: { panel_id: string; kind: string }): Promise<string | null>;
      openLink(url: string): Promise<void>;
      continueInTerminal(): Promise<void>;
      restart(): Promise<void>;
      drafts(request: DraftRequest): Promise<DraftSnapshot>;
      updates(request: UpdateRequest): Promise<UpdateState>;
    };
  }
}
