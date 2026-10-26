import { createSignal, onCleanup } from "solid-js"
import { useClient } from "./client"
export function useData() {
  const client = useClient(), [revision, setRevision] = createSignal(0)
  const changed = () => setRevision(v => v + 1)
  client.on("change", changed); onCleanup(() => client.off("change", changed))
  return { get state() { revision(); return client.state }, get snapshot() { revision(); return client.snapshot },
    get draft() { revision(); return client.draft }, get updateFeedback() { revision(); return client.updateFeedback.get(client.context) }, get route() { revision(); return client.route }, get voicePhase() { revision(); return client.voicePhase }, get issue() { revision(); return client.issue } }
}
