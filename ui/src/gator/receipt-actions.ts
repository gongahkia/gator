import { randomUUID } from "node:crypto"
import type { ActionReceipt, BackendBundle } from "@gator/application-host/contracts/protocol"
import { useClient } from "../context/client"
import type { useActions } from "./actions"
import { conversationBundles } from "./outputs"
import { useReadView } from "./read-view"

export function useReceiptActions(controls: ReturnType<typeof useActions>) {
  const client = useClient(), readView = useReadView()
  const { menu, details, actions } = controls
  function receiptMenu(bundle: BackendBundle, receipt: ActionReceipt, context: string, back?: () => void) {
    const check = () => {
      if (client.context !== context) throw new Error("Return to the original conversation and select this request again.")
      const retained = bundle.Path
        ? conversationBundles(client.state).find(current => current.Path === bundle.Path && current.RunID === bundle.RunID && current.ManifestSHA256 === bundle.ManifestSHA256)?.Actions
        : client.state?.action_recoveries?.filter(current => current.run_id === bundle.RunID).map(current => current.receipt)
      if (!retained?.some(current => JSON.stringify(current) === JSON.stringify(receipt))) throw new Error("This request changed or is unavailable. Reopen its retained result.")
    }
    check()
    const again = () => { try { receiptMenu(bundle, receipt, context, back) } catch (error) { client.fail(error) } }
    const inspect = (action: "inspect" | "check_sent") => {
      check()
      return readView({ title: action === "inspect" ? "Saved receipt check" : "Check Sent", context,
        description: action === "inspect" ? "Reading the saved check on this device." : `Read-only Sent check in original account ${receipt.ConnectorID}. Closing this view does not recall an already started check.`, back: again,
        read: async () => {
          check()
          const request_id = randomUUID()
          return client.result({ type: "receipt", receipt: { conversation_id: client.state!.conversation_id!, run_id: bundle.RunID!, action_id: receipt.ID, correlation_id: receipt.CorrelationID!, manifest_sha256: bundle.ManifestSHA256!, action, request_id } }, request_id, state => state.receipt_result)
        }, show: result => {
          check()
          const meaning = result.check?.status === "found" ? "Found in Sent. This confirms a matching Sent message, not recipient delivery." : result.check?.status === "not_checked" ? "No saved Sent check. Inspecting this record does not contact the provider." : "Sent outcome remains unconfirmed. Verify the connected service before considering a new send."
          details("Saved receipt check", [`Original account: ${receipt.ConnectorID}`, `Destination: ${receipt.Recipient || receipt.Target}`, meaning, JSON.stringify(result.check, null, 2)].join("\n\n"), again)
        } })
    }
    const canCheck = receipt.Operation === "gmail_send" && receipt.Status === "unknown" && !!bundle.RunID && !!bundle.ManifestSHA256 && !!receipt.CorrelationID
    menu("Receipt: " + receipt.Status, [
      { title: "Inspect recorded request", description: `${receipt.ConnectorID} · ${receipt.Recipient || receipt.Target}`, run: () => { check(); details("Request receipt", [`Original account: ${receipt.ConnectorID}`, `Destination: ${receipt.Recipient || receipt.Target}`, `Original conversation: ${context}`, `Run: ${bundle.RunID}`, JSON.stringify(receipt, null, 2)].join("\n\n"), again) } },
      { title: "Inspect saved Sent check", disabled: !canCheck, run: () => inspect("inspect") },
      { title: "Check Sent (read only)", disabled: !canCheck, run: () => inspect("check_sent") },
    ], false, back)
  }
  actions.receipts = () => {
    const context = client.context, rows = new Map<string, { bundle: BackendBundle; receipt: ActionReceipt }>()
    for (const bundle of conversationBundles(client.state)) for (const receipt of bundle.Actions ?? []) rows.set(`${bundle.RunID}:${receipt.ID}:${receipt.CorrelationID}`, { bundle, receipt })
    for (const item of client.state?.action_recoveries ?? []) {
      const key = `${item.run_id}:${item.receipt.ID}:${item.receipt.CorrelationID}`
      if (!rows.has(key)) rows.set(key, { bundle: { Path: "", Status: item.receipt.Status, Artifacts: null, Candidates: null, RunID: item.run_id }, receipt: item.receipt })
    }
    menu("Connected request receipts", [...rows.values()].map(({ bundle, receipt }) => ({ title: `${receipt.Operation} · ${receipt.Status}`, description: `${receipt.ConnectorID} · ${receipt.Recipient || receipt.Target}`, run: () => receiptMenu(bundle, receipt, context, actions.receipts) })))
  }
  return receiptMenu
}
