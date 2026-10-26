import { stripAnsiSequences } from "@opentui/core"
// Content is data, including OSC hyperlinks, cursor controls and pasted escape
// sequences. Only the renderer may emit terminal control bytes.
export function literal(value: string | undefined) { return stripAnsiSequences(value ?? "").replace(/[\x00-\x08\x0b-\x1f\x7f-\x9f]/g, "") }
