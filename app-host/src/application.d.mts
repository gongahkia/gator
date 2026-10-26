import { EventEmitter } from "node:events"
import type { DraftRequest, DraftSnapshot, FrontendFrame, HostSnapshot, UploadedFile } from "../contracts/frontend"
import type { FileRequest, FileView, FileComparison } from "../contracts/fileProtocol"
export class Application extends EventEmitter {
  constructor(options: { binary: string; workspace: string; userData: string; env?: NodeJS.ProcessEnv })
  binary: string; workspace: string; userData: string; stateRoot: string; env: NodeJS.ProcessEnv; cleanupIssue?: string
  readonly context: string
  sessions: { busy: boolean; pending: boolean; restart(): Promise<void>; settled(): Promise<void>; open(request?: { new?: boolean; conversation_id?: string; view?: string; source?: string }): Promise<void> }
  drafts: { request(request: DraftRequest): Promise<DraftSnapshot>; flush(): Promise<void> }
  snapshot(): HostSnapshot
  start(): Promise<HostSnapshot>
  send(frame: FrontendFrame): Promise<void>
  importFiles(paths: string[], context: string): Promise<UploadedFile[]>
  file(request: FileRequest & { action: "attach" }): Promise<UploadedFile>
  file(request: FileRequest & { action: "compare" }): Promise<FileComparison>
  file(request: FileRequest): Promise<FileView>
  exportFile(request: FileRequest, destination: string): Promise<void>
  pdf(request: FileRequest, page: number, width?: number): Promise<{ pages: number; page: number; image: Uint8Array }>
  pasteImages(images: { type: string; data: Uint8Array }[], decode: (bytes: Uint8Array) => Uint8Array): Promise<UploadedFile[]>
  openSession(id: string): Promise<void>
  runExternal(): Promise<void>
  close(): Promise<void>
}
