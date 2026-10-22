import type { BackendState, Frame } from "./protocol";
import type { FileRequest, FileView } from "./fileProtocol";

export type UpdateState = { phase: "manual" | "idle" | "checking" | "current" | "available" | "downloading" | "ready" | "installing" | "error"; installed: string; version?: string; reason?: string; percent?: number; checkedAt?: string };
export type UpdateRequest = { action: "open" | "close" | "check" | "download" | "install" | "releases" };
export type ChatActivity = { id: string; title: string; status: string; running: boolean };
export type DesktopSnapshot = { state?: BackendState; error?: string; connection?: "starting" | "connected" | "disconnected"; sessionID?: string; sessionSwitching?: boolean; sessionIssue?: string; chatActivity?: ChatActivity[]; allChatsBusy?: boolean; allChatsQueued?: boolean; terminalBusy?: boolean; fullScreen?: boolean; updates?: UpdateState; updatesOpen?: boolean; updatePreparing?: boolean };
export type DesktopFrame = Omit<Frame, "version">;
export type UploadedFile = { id: string; name: string; size: number };
export type SavedDraft = { context: string; text: string; title: string; source: string; updated: number; uploads?: UploadedFile[]; reply_to_update_id?: string };
export type DraftSnapshot = { enabled: boolean; drafts: SavedDraft[] };
export type VoicePreferences = { spoken_replies: boolean; provider?: string; model?: string; language?: string; rate?: number };
export type VoiceStatus = { ready: boolean; issue?: string; provider: string; model: string; preferences: VoicePreferences; max_seconds: number; max_audio_bytes: number; allowed?: boolean; text?: string };
export type VoiceRequest = { action: "status" | "release" | "cancel" } | { action: "authorize"; context: string } | { action: "configure"; preferences: Partial<VoicePreferences> } | { action: "transcribe"; context: string; media_type: string; data: Uint8Array };
export type DraftRequest = { action: "list" | "clear" } | { action: "remove"; context: string } | { action: "configure"; enabled: boolean } | { action: "save"; draft: Omit<SavedDraft, "updated"> };

declare global {
  interface Window {
    gator?: {
      platform: string;
      snapshot(): Promise<DesktopSnapshot>;
      openSession(id: string): Promise<void>;
      subscribe(callback: (snapshot: DesktopSnapshot) => void): () => void;
      send(frame: DesktopFrame): Promise<void>;
      chooseFolder(): Promise<string | null>;
      chooseFiles(context: string): Promise<UploadedFile[]>;
      dropFiles(files: File[], context: string): Promise<UploadedFile[]>;
      pasteImages(images: { type: string; data: Uint8Array }[], context: string): Promise<UploadedFile[]>;
      files<T = FileView>(request: FileRequest): Promise<T>;
      choosePath(request: { panel_id: string; kind: string }): Promise<string | null>;
      openLink(url: string): Promise<void>;
      continueInTerminal(): Promise<void>;
      restart(): Promise<void>;
      drafts(request: DraftRequest): Promise<DraftSnapshot>;
      updates(request: UpdateRequest): Promise<UpdateState>;
			voice(request: VoiceRequest): Promise<VoiceStatus>;
    };
  }
}
