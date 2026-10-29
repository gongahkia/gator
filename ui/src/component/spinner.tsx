// Frame data is independent of terminal IO; OpenTUI owns drawing and cleanup.
import { createEffect, createSignal, onCleanup, Show, type JSX } from "solid-js"
import type { RGBA } from "@opentui/core"
import { useTheme } from "../context/theme"
import { useConfig } from "../config"
import { SPINNER_FRAMES, SPINNER_INTERVAL } from "./spinner-frames"
export function Spinner(props: { children?: JSX.Element; color?: RGBA; id?: string; delay?: number }) {
  const theme = useTheme(), config = useConfig().data
  const [frame, setFrame] = createSignal(0), [ready, setReady] = createSignal(false)
  createEffect(() => {
    setReady(false); setFrame(0)
    let animation: ReturnType<typeof setInterval> | undefined
    const start = () => {
      setReady(true)
      if (!config.animations) return
      const started = performance.now()
      animation = setInterval(() => setFrame(Math.floor((performance.now() - started) / SPINNER_INTERVAL) % SPINNER_FRAMES.length), SPINNER_INTERVAL)
    }
    const delay = props.delay ? setTimeout(start, props.delay) : undefined
    if (!delay) start()
    onCleanup(() => { clearTimeout(delay); clearInterval(animation) })
  })
  return <Show when={ready()}><text id={props.id} fg={props.color ?? theme.text.muted}>{config.animations ? SPINNER_FRAMES[frame()] : "⋯"} {props.children}</text></Show>
}
