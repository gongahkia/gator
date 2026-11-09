import { Show, createMemo } from "solid-js"
import type { DialogueMessage, DialoguePeer } from "@gator/application-host/contracts/protocol"
import { useClient } from "../context/client"
import { useTheme } from "../context/theme"
import { Markdown } from "./markdown"
import { literal } from "./text"
import { displayDate } from "./display"

export const dialogueStatus = (state: string) => ({ running: "Working", review: "Needs your review", queued: "Waiting", completed: "Finished", failed: "Needs attention", stopped: "Stopped", interrupted: "Interrupted", unconfirmed: "Outcome unconfirmed", discarded: "Stopped" })[state] || state

export function ConversationTurn(props: { turn: DialogueMessage; task?:DialoguePeer; busy?:boolean }) {
  const client = useClient(), theme = useTheme()
  const task = createMemo(() => props.task)
  return <box id={`dialogue-${props.turn.id}`} marginY={1} gap={1}>
    <box paddingY={1} paddingX={2} border={["left"]} borderColor={theme.accent} backgroundColor={theme.background.raised.base}>
      <text fg={theme.accent}><b>You</b>  {displayDate(props.turn.created_at)}</text>
      <text fg={theme.text.base}>{literal(props.turn.text)}</text>
      <Show when={props.turn.input_ids?.length}><text fg={theme.text.muted}>Inputs: {props.turn.input_ids?.map(id => literal(id.split("/").at(-1))).join(" · ")}</text></Show>
    </box>
    <box paddingLeft={3} paddingRight={2} gap={1}>
      <Show when={props.turn.response}><Markdown text={props.turn.response!} /></Show>
      <Show when={props.turn.task_chat}>
        <text fg={theme.accent} onMouseUp={() => void client.openSession(props.turn.task_chat!).catch(error => client.fail(error))}>{dialogueStatus(task()?.state || "unconfirmed")} · /tasks opens the original task, reviews and outputs</text>
        <Show when={task()?.summary}><Markdown text={task()!.summary!} /></Show>
      </Show>
      <Show when={props.turn.issue}><text fg={theme.text.feedback.warning.base}>{literal(props.turn.issue)}</text></Show>
      <Show when={["received", "routing", "ready", "dispatching"].includes(props.turn.state)}><text fg={theme.text.muted}>{props.busy ? "Considering this message…" : "Response unconfirmed. Your message and inputs are kept; reopening does not replay it."}</text></Show>
    </box>
  </box>
}
