import { EventEmitter } from "node:events"
import type { Application } from "./application.mjs"
import type { VoiceStatus } from "../contracts/voice"
export type { VoiceStatus } from "../contracts/voice"
export class VoiceSession extends EventEmitter {
  constructor(application: Application)
  phase: string; playing: boolean; context?: string; issue?: string
  start(): Promise<void>
  stop(): Promise<{ context: string; text: string }>
  status(): Promise<VoiceStatus>
  configure(preferences: Partial<VoiceStatus["preferences"]>): Promise<VoiceStatus>
  cancel(): void
  stopPlayback(): void
  close(): void
}
