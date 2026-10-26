// Adapted from upstream component/spinner.tsx; Gator uses the same frame set
// without the custom shimmer renderer and its unrelated dependencies.
import { createEffect, createSignal, onCleanup, Show, type JSX } from "solid-js"
import type { RGBA } from "@opentui/core"
import { useTheme } from "../context/theme"
import { useConfig } from "../config"
import { SPINNER_FRAMES } from "./spinner-frames"
export function Spinner(props: { children?: JSX.Element; color?: RGBA }) {
  const theme = useTheme(), config = useConfig().data
  const [frame, setFrame] = createSignal(0)
  createEffect(() => { if (!config.animations) return; const timer = setInterval(() => setFrame(v => (v + 1) % SPINNER_FRAMES.length), 80); onCleanup(() => clearInterval(timer)) })
  return <text fg={props.color ?? theme.text.muted}>{config.animations ? SPINNER_FRAMES[frame()] : "⋯"} {props.children}</text>
}
