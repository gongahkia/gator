import { EventEmitter } from "node:events"
import type { Application } from "./application.mjs"
import type { VoiceStatus, VoicePreferences } from "../contracts/voice"
export type { VoiceStatus } from "../contracts/voice"
export class VoiceSession extends EventEmitter {
  constructor(application: Application, services?: {
    capture?(): { audio: Promise<Uint8Array>; stop(): void; cancel(): void }
    call?(binary: string, workspace: string, env: NodeJS.ProcessEnv, request: { action: string; [key: string]: unknown }, signal?: AbortSignal): Promise<VoiceStatus>
    playback?(text: string, preferences: VoicePreferences, signal: AbortSignal): Promise<void>
  })
  readonly application: Application
  phase: string; playing: boolean; context?: string; issue?: string
  read(text: string): Promise<void>
  download(model: string): Promise<VoiceStatus>
  start(expectedRoute?: string): Promise<void>
  stop(): Promise<{ context: string; text: string }>
  status(): Promise<VoiceStatus>
  configure(preferences: Partial<VoiceStatus["preferences"]>): Promise<VoiceStatus>
  cancel(): void
  stopPlayback(): void
  close(): void
}
