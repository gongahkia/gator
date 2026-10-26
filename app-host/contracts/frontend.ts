import type { BackendState, Frame } from "./protocol"
export type UploadedFile = { id: string; name: string; size: number }
export type Draft = { context: string; title: string; source: string; text: string; uploads?: UploadedFile[]; reply_to_update_id?: string; updated?: number }
export type DraftSnapshot = { enabled: boolean; drafts: (Draft & { updated: number })[] }
export type DraftRequest = { action: "list" | "clear" } | { action: "remove"; context: string } | { action: "configure"; enabled: boolean } | { action: "save"; draft: Draft }
export type HostSnapshot = { state?: BackendState; error?: string; connection: string; sessionSwitching: boolean; sessionIssue: string; chatActivity: { id: string; title: string; status: string; running: boolean }[]; allChatsBusy: boolean; allChatsQueued: boolean; terminalBusy: boolean }
export type FrontendFrame = Omit<Frame, "version">
