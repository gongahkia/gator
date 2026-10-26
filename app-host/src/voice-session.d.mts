import { EventEmitter } from "node:events"
import type { Application } from "./application.mjs"
export type VoiceStatus = { ready: boolean; issue?: string; provider: string; model: string; route: string; preferences: { spoken_replies: boolean; provider?: string; model?: string; language?: string; rate?: number }; max_seconds: number; max_audio_bytes: number }
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
