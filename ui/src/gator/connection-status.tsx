import { useTheme } from "../context/theme"

export function ConnectionStatus(props: { connection: string; error?: boolean }) {
  const theme = useTheme()
  const state = () => props.error || props.connection === "disconnected" ? "disconnected" : props.connection === "connected" ? "connected" : "connecting"
  const color = () => state() === "connected" ? theme.text.feedback.success.base : state() === "disconnected" ? theme.text.feedback.error.base : theme.text.feedback.warning.base
  // Keep a label as well as color for NO_COLOR and color vision differences.
  return <text fg={theme.text.base} id="connection-status" flexShrink={0}><span style={{ fg: color() }}>●</span> {state()}</text>
}
