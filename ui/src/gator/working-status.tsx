import { createEffect, createSignal, onCleanup } from "solid-js"
import { useConfig } from "../config"
import { useTheme } from "../context/theme"
import { tint } from "../theme/color"
import { literal } from "./text"

export function WorkingStatus(props: { status?: string; elapsed?: string; attention: boolean; cancelling: boolean }) {
  const theme = useTheme(), config = useConfig().data
  const [fade, setFade] = createSignal(0)
  createEffect(() => {
    setFade(0)
    if (!config.animations || props.attention || props.cancelling) return
    const started = performance.now()
    // Blend readable theme colors, without changing the text or its layout.
    const timer = setInterval(() => setFade((1 - Math.cos((performance.now() - started) * 2 * Math.PI / 2600)) * 0.425), 50)
    onCleanup(() => clearInterval(timer))
  })
  return <text id="working-status" fg={tint(theme.accent, theme.text.muted, fade())}>
    {props.cancelling ? "Stopping…" : literal(props.status) || "Gator is working…"}
    <span style={{ fg: theme.text.muted }}> {literal(props.elapsed)}</span>
  </text>
}
