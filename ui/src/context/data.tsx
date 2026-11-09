import { createContext, createMemo, createSignal, getOwner, onCleanup, runWithOwner, untrack, useContext, type ParentProps } from "solid-js"
import type { BackendState } from "@gator/application-host/contracts/protocol"
import { useClient } from "./client"

// Keep the last immutable value when a complete native snapshot repeats a
// field. Never reconcile into captured review objects: old approval identities
// and displayed action content must remain frozen when a new snapshot arrives.
function sameField(before: unknown, after: unknown) {
  return before === after || before !== null && after !== null && typeof before === "object" && typeof after === "object" && JSON.stringify(before) === JSON.stringify(after)
}
function createData() {
  const client = useClient(), [revision, setRevision] = createSignal(0)
  const changed = () => setRevision(v => v + 1)
  client.on("change", changed); onCleanup(() => client.off("change", changed))
  const owner = getOwner()
  function fields<T extends object>(read: () => T | undefined): T {
    const selected = new Map<PropertyKey, () => unknown>()
    return new Proxy({} as T, {
      get(_, key) {
        if (!selected.has(key)) {
          // A field first read by a transient row still belongs to the shared
          // provider, rather than to a row that can disappear on the next tick.
          const select = untrack(() => runWithOwner(owner, () => createMemo(() => { revision(); return Reflect.get(read() ?? {}, key) }, undefined, { equals: sameField })))!
          selected.set(key, select)
        }
        return selected.get(key)!()
      },
      ownKeys() { revision(); return Reflect.ownKeys(read() ?? {}) },
      getOwnPropertyDescriptor() { return { enumerable: true, configurable: true } },
      has(_, key) { revision(); return Reflect.has(read() ?? {}, key) },
    })
  }
  const state = fields<BackendState>(() => client.state), snapshot = fields(() => client.snapshot)
  const present = createMemo(() => { revision(); return client.state !== undefined })
  return { get progress() { revision(); return [...client.operations.values()].reverse().find(operation => operation.context === client.context)?.label },
    get admitting() { revision(); return client.pending.has(client.context) },
    get state() { return present() ? state : undefined }, get snapshot() { return snapshot },
    get activityExpanded() { revision(); return client.activityExpanded }, get voiceDestination() { revision(); return client.voiceDestination },
    get draft() { revision(); return client.draft }, get preferences() { revision(); return client.preferences }, get updateFeedback() { revision(); return client.updateFeedback.get(client.context) }, get route() { revision(); return client.route }, get voicePhase() { revision(); return client.voicePhase }, get issue() { revision(); return client.issue } }
}

const Context = createContext<ReturnType<typeof createData>>()
export function DataProvider(props: ParentProps) {
  const data = createData()
  return <Context.Provider value={data}>{props.children}</Context.Provider>
}
export function useData() {
  const data = useContext(Context)
  if (!data) throw new Error("Missing Gator data provider")
  return data
}
