import { createSignal, onCleanup } from "solid-js"
import { useClient } from "./client"
export function useData() {
  const client = useClient(), [revision, setRevision] = createSignal(0)
  const changed = () => setRevision(v => v + 1)
  client.on("change", changed); onCleanup(() => client.off("change", changed))
  return { get progress() { revision(); return [...client.operations.values()].reverse().find(operation => operation.context === client.context)?.label },
    get admitting() { revision(); return client.pending.has(client.context) },
    get state() { revision(); return client.state }, get snapshot() { revision(); return client.snapshot },
    get activityExpanded() { revision(); return client.activityExpanded },
    get draft() { revision(); return client.draft }, get preferences() { revision(); return client.preferences }, get updateFeedback() { revision(); return client.updateFeedback.get(client.context) }, get route() { revision(); return client.route }, get voicePhase() { revision(); return client.voicePhase }, get issue() { revision(); return client.issue } }
}
