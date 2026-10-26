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
  openSession(id: string): Promise<void>
  close(): Promise<void>
}
