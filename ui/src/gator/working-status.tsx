import { Show } from "solid-js"
import { useTheme } from "../context/theme"
import { Spinner } from "../component/spinner"
import { literal } from "./text"

export function WorkingStatus(props: { status?: string; elapsed?: string; attention: boolean; cancelling: boolean }) {
  const theme = useTheme()
  const label = () => {
    const status = literal(props.status)
    return !status || /^Working from an immutable snapshot/.test(status) ? "Thinking…" : status
  }
  return <Show when={!props.attention && !props.cancelling} fallback={
    <text id="working-status" fg={theme.text.feedback.warning.base}>{props.cancelling ? "Stopping…" : "Waiting for approval"}</text>
  }>
    <Spinner id="working-status" color={theme.text.base}>{label()}<span style={{ fg: theme.text.muted }}> {literal(props.elapsed)}</span></Spinner>
  </Show>
}
