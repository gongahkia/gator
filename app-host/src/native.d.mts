export function helper(program: string, args: string[], options?: { input?: string | Uint8Array; maxBytes?: number; signal?: AbortSignal }): Promise<Buffer>
export function openLink(url: string): Promise<void>
export function clipboardRead(): Promise<string>
export function clipboardWrite(text: string): Promise<void>
export function clipboardImage(): Promise<Uint8Array>
export function recordingArguments(platform: string, device?: string): string[]
export function recordAudio(device?: string): { audio: Promise<Uint8Array>; stop(): void; cancel(): void }
export function speak(text: string, preferences?: { language?: string; rate?: number }, signal?: AbortSignal): Promise<void>
