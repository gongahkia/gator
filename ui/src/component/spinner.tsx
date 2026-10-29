// Frame data is independent of terminal IO; OpenTUI owns drawing and cleanup.
import { createEffect, createSignal, onCleanup, type JSX } from "solid-js"
import type { RGBA } from "@opentui/core"
import { useTheme } from "../context/theme"
import { useConfig } from "../config"
import { SPINNER_FRAMES, SPINNER_INTERVAL } from "./spinner-frames"
export function Spinner(props: { children?: JSX.Element; color?: RGBA; id?: string }) {
  const theme = useTheme(), config = useConfig().data
  const [frame, setFrame] = createSignal(0)
  createEffect(() => { if (!config.animations) return; const started = performance.now(); const timer = setInterval(() => setFrame(Math.floor((performance.now() - started) / SPINNER_INTERVAL) % SPINNER_FRAMES.length), SPINNER_INTERVAL); onCleanup(() => clearInterval(timer)) })
  return <text id={props.id} fg={props.color ?? theme.text.muted}>{config.animations ? SPINNER_FRAMES[frame()] : "⋯"} {props.children}</text>
}
