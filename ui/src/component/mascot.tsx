import { createEffect, createSignal, onCleanup, Show } from "solid-js"
import { useTerminalDimensions } from "@opentui/solid"
import { useData } from "../context/data"
import { useTheme } from "../context/theme"

type Phase = "idle" | "working" | "attention"

// Four fixed-width rows keep the composer still while the eyes, jaw and tail
// change. Plain terminal text works without an image protocol or extra assets.
export function mascotFrame(phase: Phase, tick: number) {
  const eye = phase === "attention" ? "?" : phase === "idle" && tick ? "-" : phase === "working" && tick % 2 ? "O" : "o"
  const jaw = phase === "working" && tick % 2 ? "/" : ","
  const tail = phase === "working" && tick % 2 ? ")" : "/"
  return ["     .--.    __", ` ___/ ${eye}  \\__/ ${tail}`, `<___${jaw}       _/`, "    \\__/\\__/"].map(line => line.padEnd(18)).join("\n")
}

export function Mascot() {
  const data = useData(), theme = useTheme(), dimensions = useTerminalDimensions()
  const [tick, setTick] = createSignal(0)
  const visible = () => data.preferences.mascot !== "hidden" && dimensions().width >= 70
  const phase = (): Phase => data.state?.pending_approval || data.state?.pending_connection || data.state?.external || data.snapshot.connection === "disconnected" ? "attention"
    : data.state?.running || data.snapshot.sessionSwitching || data.snapshot.connection === "starting" || data.progress || ["transcribing", "downloading"].includes(data.voicePhase) ? "working" : "idle"
  createEffect(() => {
    const current = phase(), animated = visible() && data.preferences.mascot === "animated" && !process.env.GATOR_REDUCED_MOTION
    setTick(0)
    if (!animated || current === "attention") return
    if (current === "working") {
      const timer = setInterval(() => setTick(value => value + 1), 360)
      onCleanup(() => clearInterval(timer))
      return
    }
    // Idle only wakes for a brief blink; it does not repaint on a busy loop.
    let timer: ReturnType<typeof setTimeout>
    const blink = () => {
      setTick(1)
      timer = setTimeout(() => { setTick(0); timer = setTimeout(blink, 5000) }, 120)
    }
    timer = setTimeout(blink, 5000)
    onCleanup(() => clearTimeout(timer))
  })
  return <Show when={visible()}><box id="gator-mascot" width={18} height={4} flexShrink={0}>
    <text fg={theme.text.feedback.success.base} wrapMode="none">{mascotFrame(phase(), tick())}</text>
  </box></Show>
}
