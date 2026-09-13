import SwiftUI

// Single list row in HistoryWindow. Shows icon + summary + relative time
// on the left; right-side action is "Copy snapshot" when a priorSnapshotJSON
// is present (non-invertible ops) or nothing (a plain history record).
struct HistoryEntryRow: View {
    let entry: MutationAuditEntry
    let onSnapshotCopied: () -> Void

    var body: some View {
        HStack(alignment: .center, spacing: 12) {
            // Native macOS log rows (Mail activity, Messages receipts, Finder
            // inspector history) use plain SF Symbols without a circular
            // backdrop. Color conveys kind-at-a-glance; no extra geometry.
            Image(systemName: sfSymbol)
                .foregroundStyle(iconTint)
                .hcbFont(.title3)
                .hcbScaledFrame(width: 22, alignment: .center)
            VStack(alignment: .leading, spacing: 1) {
                Text(entry.summary)
                    .hcbFont(.body)
                    .lineLimit(2)
                HStack(spacing: 6) {
                    Text(entry.timestamp.formatted(.relative(presentation: .numeric)))
                        .foregroundStyle(.secondary)
                    if let listTitle = entry.metadata["toListTitle"] ?? entry.metadata["list"] {
                        Text("·")
                            .foregroundStyle(.secondary)
                        Text(listTitle)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                }
                .hcbFont(.caption)
            }
            Spacer(minLength: 0)
            if entry.priorSnapshotJSON != nil || entry.postSnapshotJSON != nil {
                Button {
                    onSnapshotCopied()
                } label: {
                    Label("Copy snapshot", systemImage: "doc.on.clipboard")
                }
                .buttonStyle(.bordered)
                .controlSize(.small)
                .help("Copies the recorded state as JSON so you can recreate it manually if needed.")
            }
        }
    }

    private var sfSymbol: String {
        let k = entry.kind
        if k.hasSuffix(".create") { return "plus.circle" }
        if k.hasSuffix(".edit") { return "square.and.pencil" }
        if k.hasSuffix(".delete") { return "trash" }
        if k == "task.complete" || k == "event.dismiss" { return "checkmark.circle.fill" }
        if k == "task.reopen" { return "arrow.uturn.backward.circle" }
        if k.hasPrefix("task.duplicate") { return "plus.square.on.square" }
        if k.hasPrefix("task.move") { return "arrow.right.square" }
        if k.hasPrefix("clipboard.") { return "doc.on.clipboard" }
        if k.hasSuffix(".restore") { return "arrow.uturn.backward.circle" }
        if k.hasPrefix("bulk.") { return "square.grid.2x2" }
        if k.hasPrefix("sync.") { return "arrow.triangle.2.circlepath" }
        return "circle"
    }

    private var iconTint: Color {
        let k = entry.kind
        if k.hasSuffix(".delete") { return AppColor.ember }
        if k == "task.complete" || k == "event.dismiss" { return AppColor.moss }
        if k.hasPrefix("sync.") { return AppColor.blue }
        if k.hasPrefix("bulk.") { return AppColor.blue }
        if k.hasPrefix("task.move") { return AppColor.blue }
        if k.hasSuffix(".create") || k.hasPrefix("task.duplicate") { return AppColor.moss }
        return .secondary
    }
}
