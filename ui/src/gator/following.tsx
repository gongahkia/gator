import { For, Show, createMemo, createSignal, createEffect, onCleanup } from "solid-js"
import { useKeyboard, useRenderer, useTerminalDimensions } from "@opentui/solid"
import { TextAttributes, type ScrollBoxRenderable } from "@opentui/core"
import type { FollowingCheck, FollowingGoal } from "@gator/application-host/contracts/protocol"
import { useClient } from "../context/client"
import { useData } from "../context/data"
import { useTheme } from "../context/theme"
import { useDialog } from "../ui/dialog"
import type { useActions } from "./actions"
import { literal } from "./text"

export const checkTime = (value?: string, timezone?: string) => {
  if (!value || !Number.isFinite(Date.parse(value))) return "Not available"
  try { return new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit", timeZone: timezone || undefined }).format(new Date(value)) }
  catch { return value }
}

export function Following(props: { inspect(check: FollowingCheck): void; goal(goal: FollowingGoal): void; refresh(): Promise<void>; controls: Pick<ReturnType<typeof useActions>, "menu" | "details" | "resetMenus"> }) {
  const client = useClient(), data = useData(), theme = useTheme(), dialog = useDialog(), controls = props.controls
  const renderer = useRenderer(), dimensions = useTerminalDimensions()
  const [selected, setSelected] = createSignal(0), [refreshing, setRefreshing] = createSignal(false)
  let previousKeys: string[] = []
  let scroll: ScrollBoxRenderable | undefined, reveal: (() => void) | undefined
  const cards = createMemo(() => {
    const checks = data.state?.checks ?? [], goals = data.state?.following_goals ?? [], known = new Set(goals.map(goal => goal.id))
    return [
      ...goals.map(goal => ({ key: `goal-${goal.id}`, kind: "goal", title: goal.title, outcome: goal.outcome || checks.find(check => check.goal_id === goal.id)?.outcome, source: goal.source, status: goal.status, goal: goal.id, checks: checks.filter(check => check.goal_id === goal.id) })),
      ...checks.filter(check => !check.goal_id || !known.has(check.goal_id)).map(check => ({ key: `check-${check.id}`, kind: "check", title: check.title, outcome: check.outcome, source: check.source, status: check.state, goal: check.goal_id, checks: [check] })),
    ]
  })
  const utilities = () => [
    ["Goals and plans", "/goals"], ["Manage recurring checks", "/followup"], ["Inbox", "/inbox"],
    ["Activity and attention", "/overview"], ["This chat's updates", "/updates"],
  ]
  const count = () => cards().length + utilities().length + 1
  createEffect(() => {
    dimensions()
    const keys = [...cards().map(card => card.key), ...utilities().map(item => `utility:${item[1]}`), "how-following-runs"]
    const retained = previousKeys[selected()], restored = retained ? keys.indexOf(retained) : -1
    const index = restored >= 0 ? restored : Math.min(selected(), count() - 1)
    previousKeys = keys
    if (selected() !== index) setSelected(index)
    if (reveal) renderer.off("frame", reveal)
    reveal = () => {
      const node = scroll?.content.getChildren().find(node => node.id === `following-item-${index}`)
      if (scroll && !scroll.isDestroyed && node) {
        const top = scroll.viewport.screenY, bottom = top + scroll.viewport.height
        if (node.height >= scroll.viewport.height || node.screenY < top) scroll.scrollBy(node.screenY - top)
        else if (node.screenY + node.height > bottom) scroll.scrollBy(node.screenY + node.height - bottom)
      }
      reveal = undefined
    }
    renderer.once("frame", reveal)
  })
  onCleanup(() => { if (reveal) renderer.off("frame", reveal) })
  const attempt = (promise: Promise<unknown>) => { void promise.catch(error => client.fail(error)) }
  function open(index: number) {
    controls.resetMenus()
    const card = cards()[index]
    if (card) {
      if (card.checks.length === 1) props.inspect(card.checks[0])
      else if (card.kind === "goal" && client.state?.following_goals?.find(goal => goal.id === card.goal)?.controllable) props.goal(client.state!.following_goals!.find(goal => goal.id === card.goal)!)
      else controls.menu(card.title, [
        ...card.checks.map(check => ({ title: check.title, description: `${check.state} · ${check.next_action}`, run: () => props.inspect(check) })),
        { title: "Goal and plan", description: card.checks.length ? "Intended work and retained progress" : "Not monitored automatically", run: () => client.openGoal(card.goal!) },
        ...(!card.checks.length ? [{ title: "Attach a recurring check", description: "Review sources and timing for this goal", run: () => client.prepareGoalCheck(card.goal!) }] : []),
      ], true)
      return
    }
    const utility = utilities()[index - cards().length]
    if (utility) { attempt(client.navigate(utility[1])); return }
    controls.details("How Following runs", [
      data.state?.checks_issue ? "Service status could not be refreshed. Inspect check health before relying on the last observed state." : data.state?.supervisor_running === true ? "The local checking service is reachable." : data.state?.supervisor_running === false ? "The local checking service is offline." : "Service status has not been verified.",
      "A saved goal describes an intention. Only an enabled recurring check monitors it automatically.",
      "Configured checks run through the local service while this computer is awake and their sources are accessible. Sleep, shutdown, an offline service, or lost source access can leave gaps. A next check time is an estimate, not a delivery promise.",
      "Closing Gator stops foreground conversation tasks. A separately running local checking service can continue. Starting or installing that service requires its own review in /recovery.",
      "Pause stops future checks. A check already running may finish. Previous check results and evidence remain available in Inbox.",
    ].join("\n\n"))
  }
  async function refresh() { if (refreshing()) return; setRefreshing(true); try { await props.refresh() } catch (error) { client.fail(error) } finally { setRefreshing(false) } }
  useKeyboard(event => {
    if (dialog.stack.length || event.meta) return
    if (event.ctrl) { if (event.name === "r") { event.preventDefault(); void refresh() }; return }
    if (event.name === "up" || event.name === "down") { event.preventDefault(); setSelected(value => (value + (event.name === "up" ? -1 : 1) + count()) % count()) }
    if (event.name === "return") { event.preventDefault(); open(selected()) }
    if (event.name === "pageup" || event.name === "pagedown") { event.preventDefault(); scroll?.scrollBy((event.name === "pageup" ? -1 : 1) * Math.max(1, (scroll?.height ?? 8) - 2)) }
    if (event.name === "escape") { event.preventDefault(); attempt(client.navigate("/chat")) }
  })
  const selectedColor = (index: number) => selected() === index ? theme.background.raised.high : undefined
  return <box flexGrow={1} minHeight={0} paddingX={dimensions().width < 60 ? 1 : 2}>
    <text height={1} fg={theme.text.base}><b>Following</b> · {refreshing() ? "Refreshing…" : data.state?.checks_issue ? "Service unverified" : data.state?.supervisor_running === true ? "Service online" : data.state?.supervisor_running === false ? "Service offline" : "Service unverified"}</text>
    <Show when={data.state?.following_issue}><text maxHeight={2} fg={theme.text.feedback.warning.base}>{literal(data.state?.following_issue)}</text></Show>
    <Show when={data.state?.checks_issue}><text maxHeight={2} fg={theme.text.feedback.warning.base}>{literal(data.state?.checks_issue)} · Refresh before making changes.</text></Show>
    <scrollbox id="following-items" ref={value => { scroll = value }} flexGrow={1} minHeight={0}>
      <Show when={!cards().length}><text fg={theme.text.muted}>{data.state?.checks_issue || data.state?.following_issue ? "Commitments could not be verified." : "No commitments yet. Ask Gator to follow something, then review its sources and timing."}</text></Show>
      <Show when={!data.state?.checks_issue && !data.state?.checks?.length}><text fg={theme.text.muted}>No recurring checks configured.</text></Show>
      <For each={cards()}>{(card, index) => <box id={`following-item-${index()}`} paddingX={1} paddingBottom={1} backgroundColor={selectedColor(index())} onMouseUp={() => { setSelected(index()); open(index()) }}>
        <text height={1} wrapMode="none" truncate fg={selected() === index() ? theme.accent : theme.text.base}><b>{selected() === index() ? "› " : "  "}{literal(card.title)}</b></text>
        <Show when={card.outcome}><text fg={theme.text.base} maxHeight={2}>Outcome: {literal(card.outcome)}</text></Show>
        <Show when={card.kind === "goal" && card.checks.length}><text fg={theme.text.muted}>Goal: {literal(card.status)}</text></Show>
        <Show when={!card.checks.length}>
          <text fg={theme.text.muted}>{literal(card.status)} · {data.state?.checks_issue ? "Check status unavailable" : "No recurring check attached"}</text>
          <text fg={theme.text.base} maxHeight={2}>Next: {data.state?.checks_issue ? "Refresh to verify check coverage." : card.status === "paused" ? "Inspect the retained plan or resume this goal." : "Review the plan or attach a recurring check."}</text>
          <text fg={theme.text.muted} maxHeight={2}>Workspace: {literal(card.source || "Unavailable; inspect this goal before starting work")}</text>
        </Show>
        <For each={card.checks}>{check => <box>
          <Show when={check.outcome && check.outcome !== card.outcome}><text fg={theme.text.base} maxHeight={2}>Checks for: {literal(check.outcome)}</text></Show>
          <text fg={check.tone === "error" ? theme.text.feedback.error.base : check.tone === "warning" ? theme.text.feedback.warning.base : theme.text.muted}>{card.checks.length > 1 ? `${literal(check.title)} · ` : ""}{literal(check.state)}{data.state?.checks_issue ? " · Last observed" : ""}</text>
          <text fg={theme.text.base} maxHeight={2}>Next: {literal(check.next_action) || "Inspect check health"}</text>
          <text fg={theme.text.muted}>Next check: {check.next_due ? checkTime(check.next_due, check.timezone) : check.enabled ? "Not scheduled or unavailable" : "Paused"} · Last success: {check.last_success ? checkTime(check.last_success, check.timezone) : "None recorded"}</text>
          <text fg={theme.text.muted} maxHeight={2}>Sources: {literal([check.source, ...(check.connectors ?? [])].filter(Boolean).join(", ") || "None")} · {literal(check.timezone || "Timezone unavailable")}</text>
          <Show when={check.issue}><text fg={theme.text.feedback.warning.base} maxHeight={2}>{literal(check.issue)}</text></Show>
        </box>}</For>
      </box>}</For>
      <For each={utilities()}>{(item, index) => <box id={`following-item-${cards().length + index()}`} paddingX={1} backgroundColor={selectedColor(cards().length + index())} onMouseUp={() => open(cards().length + index())}><text fg={selected() === cards().length + index() ? theme.accent : theme.text.muted}>{selected() === cards().length + index() ? "› " : "  "}{item[0]}</text></box>}</For>
      <box id={`following-item-${count() - 1}`} paddingX={1} backgroundColor={selectedColor(count() - 1)} onMouseUp={() => open(count() - 1)}><text fg={selected() === count() - 1 ? theme.accent : theme.text.muted}>{selected() === count() - 1 ? "› " : "  "}How Following runs</text></box>
    </scrollbox>
    <text height={1} attributes={TextAttributes.DIM} fg={theme.text.muted}>{dimensions().width < 60 ? "Enter actions · Ctrl+R refresh · Esc chat" : "↑/↓ select · Enter actions · Ctrl+R refresh · Esc chat"}</text>
  </box>
}
