import SwiftUI
import GatorCore

/// Embedded in the existing task inspector, shared by both application surfaces.
struct RecoveryInspectorContent: View {
    @Environment(ManagedSessionCoordinator.self) private var sessions
    let owner: GatorApplicationState
    let generation: UUID
    var body: some View {
        if let recovery = owner.recovery, recovery.relevant || owner.receiptOperation != nil {
            VStack(alignment: .leading, spacing: 14) {
                Label("Recovery and retained evidence", systemImage: "clock.arrow.circlepath").font(.headline).accessibilityAddTraits(.isHeader)
                Text("Inspection never resumes work, grants access, approves an action or resends anything.").font(.callout)
                if let pending = owner.receiptOperation, !recovery.receipts.contains(where: { $0.identity == pending.identity }) {
                    Text(verbatim: pending.summary).textSelection(.enabled)
                    DisclosureGroup("Retained check identity · no replay") {
                        Text(verbatim: "Conversation: " + pending.identity.conversationID + "\nRun: " + pending.identity.runID + "\nAction: " + pending.identity.actionID + "\nRequest: " + pending.id).textSelection(.enabled)
                    }.font(.caption)
                }
                if !owner.isConnected { Label("Owner disconnected · retained snapshot may be out of date", systemImage: "exclamationmark.triangle") }
                if recovery.unresolved { Text("This owner retains unresolved evidence. It may keep a cached slot until the original work can be safely inspected; work is never cancelled to free a slot.").font(.caption).foregroundStyle(.secondary) }
                ForEach(Array(recovery.notices.enumerated()), id: \.offset) { _, notice in
                    Label { Text(verbatim: notice).textSelection(.enabled) } icon: { Image(systemName: "exclamationmark.triangle") }
                }
                ForEach(recovery.records) { record in
                    VStack(alignment: .leading, spacing: 6) {
                        Text(verbatim: record.title).font(.body.weight(.medium)).textSelection(.enabled)
                        Label(record.status, systemImage: "doc.text.magnifyingglass").accessibilityLabel("Retained work status: " + record.status)
                        Text(verbatim: record.detail).font(.callout).textSelection(.enabled)
                        DisclosureGroup("Evidence for " + record.status) {
                            ForEach(record.fields) { field in Text(verbatim: field.text).frame(maxWidth: .infinity, alignment: .leading).textSelection(.enabled) }
                        }.font(.caption)
                    }.padding(10).background(.background, in: RoundedRectangle(cornerRadius: 10))
                }
                ForEach(recovery.receipts) { receipt in
                    VStack(alignment: .leading, spacing: 8) {
                        Text(verbatim: receipt.title).font(.body.weight(.medium)).accessibilityAddTraits(.isHeader)
                        Text(verbatim: "Run: " + receipt.runID + (receipt.at.isEmpty ? "" : " · " + receipt.at)).font(.caption).textSelection(.enabled)
                        if receipt.live { Text("Go reports this attempt as live; a lost connection does not establish failure.").font(.caption) }
                        ForEach(receipt.fields.filter { !["ID", "CorrelationID"].contains($0.id) }) { field in Text(verbatim: field.text).textSelection(.enabled) }
                        DisclosureGroup("Exact receipt identity") {
                            ForEach(receipt.fields.filter { ["ID", "CorrelationID"].contains($0.id) }) { field in Text(verbatim: field.text).textSelection(.enabled) }
                            if let identity = receipt.identity { Text(verbatim: "Manifest: " + identity.manifestSHA256).textSelection(.enabled) }
                        }.font(.caption)
                        if receipt.value["Status"].string == "unknown" { Text("External outcome uncertain. Neither a disconnect nor an unconfirmed check proves this action failed. No Resend is available.").font(.callout) }
                        if let identity = receipt.identity {
                            if let pending = owner.receiptOperation, pending.identity == identity {
                                Label(pending.summary, systemImage: "info.circle").textSelection(.enabled)
                                if !pending.result["check"]["provider_id"].string.isEmpty { Text(verbatim: "Matching record: " + pending.result["check"]["provider_id"].string).textSelection(.enabled) }
                                if !pending.result["check"]["checked_at"].string.isEmpty { Text(verbatim: "Checked at: " + pending.result["check"]["checked_at"].string).font(.caption).textSelection(.enabled) }
                            }
                            let reason = owner.receiptUnavailable(receipt, generation: generation)
                            HStack {
                                Button("Inspect Saved Check") { check(receipt, .inspect) }.focusable()
                                    .help("Read a retained check. Does not query Sent or send anything.")
                                Button("Check Sent Read-only") { check(receipt, .checkSent) }.focusable()
                                    .help("Explicitly query Sent for this exact unknown Gmail send. Never resends.")
                            }.disabled(reason != nil || owner !== sessions.current || sessions.switching || !sessions.isInspectionCurrent)
                            if let reason { Text(verbatim: reason).font(.caption).foregroundStyle(.secondary) }
                            if owner !== sessions.current { Text("Open the original owning chat and inspect its current receipt before requesting a check.").font(.caption) }
                        } else {
                            Text("No eligible exact sealed-bundle identity is available for a receipt check. This evidence cannot authorize an external lookup or retry.").font(.caption).foregroundStyle(.secondary)
                        }
                    }.padding(10).background(.background, in: RoundedRectangle(cornerRadius: 10))
                }
            }.accessibilityIdentifier("gator.recovery." + recovery.conversationID)
        }
    }
    private func check(_ receipt: NativeRecoveryReceipt, _ action: NativeReceiptAction) {
        Task { await sessions.checkReceipt(owner: owner, receipt: receipt, generation: generation, action: action) }
    }
}
