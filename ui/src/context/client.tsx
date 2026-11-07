import { createContext, useContext, type ParentProps } from "solid-js"
import type { GatorClient } from "../gator/client"
const Client = createContext<GatorClient>()
export function ClientProvider(props: ParentProps<{ client: GatorClient }>) { return <Client.Provider value={props.client}>{props.children}</Client.Provider> }
export function useClient() { const value = useContext(Client); if (!value) throw new Error("Missing Gator client"); return value }
