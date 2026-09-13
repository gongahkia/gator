import SwiftUI

// Settings block for the history window: visible limit, storage cap, and
// per-category show/hide toggles. Also exposes an "Open History" button
// that opens the floating HistoryWindow scene via openWindow.
struct HistorySection: View {
    @Environment(AppModel.self) private var model
    @Environment(\.openWindow) private var openWindow
    @State private var isConfirmingClearDismissals = false

    var body: some View {
        Section("History") {
            Button {
                openWindow(id: "history")
            } label: {
                Label("Open history…", systemImage: "clock.arrow.circlepath")
            }
            .buttonStyle(.borderless)

            LabeledContent("Visible entries") {
                HStack(spacing: 8) {
                    // Ceiling matches historyStorageCap's maximum so users who
                    // raise storage to the hard ceiling can actually display
                    // every entry at once. Step is 10 for the common range,
                    // accepting coarser snapping at the high end as the trade-
                    // off for a single-slider UI.
                    Slider(
                        value: Binding(
                            get: { Double(model.settings.historyVisibleLimit) },
                            set: { model.setHistoryVisibleLimit(Int($0)) }
                        ),
                        in: 10...Double(MutationAuditLog.absoluteCeiling),
                        step: 10
                    )
                    Text("\(model.settings.historyVisibleLimit)")
                        .hcbFont(.caption, weight: .medium)
                        .monospacedDigit()
                        .foregroundStyle(.secondary)
                        .frame(minWidth: 48, alignment: .trailing)
                }
            }

            LabeledContent("Storage cap") {
                HStack(spacing: 8) {
                    Slider(
                        value: Binding(
                            get: { Double(model.settings.historyStorageCap) },
                            set: { model.setHistoryStorageCap(Int($0)) }
                        ),
                        in: 500...Double(MutationAuditLog.absoluteCeiling),
                        step: 500
                    )
                    Text("\(model.settings.historyStorageCap)")
                        .hcbFont(.caption, weight: .medium)
                        .monospacedDigit()
                        .foregroundStyle(.secondary)
                        .frame(minWidth: 56, alignment: .trailing)
                }
            }

            Text(storageFootnote)
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }

        Section("History categories") {
            Text("Uncheck to hide a category from the history window. Sync events are off by default because every successful refresh emits one entry.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
            ForEach(Self.categories, id: \.0) { (key, title) in
                Toggle(isOn: Binding(
                    get: { model.settings.historyCategoryFilters.contains(key) },
                    set: { model.setHistoryCategoryEnabled(key, enabled: $0) }
                )) {
                    Text(title)
                }
            }
        }

        Section("Duplicate detection") {
            let count = model.settings.dismissedDuplicateGroups.count
            Text(count == 0
                 ? "No duplicate groups have been dismissed."
                 : "\(count) duplicate group\(count == 1 ? "" : "s") dismissed as false positives.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
            Button {
                openWindow(id: "duplicate-review")
            } label: {
                Label("Review duplicates…", systemImage: "square.stack.3d.up")
            }
            .buttonStyle(.borderless)
            Button(role: .destructive) {
                isConfirmingClearDismissals = true
            } label: {
                Label("Reset dismissals", systemImage: "arrow.counterclockwise")
            }
            .buttonStyle(.borderless)
            .disabled(count == 0)
        }
        .confirmationDialog(
            "Clear all duplicate dismissals?",
            isPresented: $isConfirmingClearDismissals,
            titleVisibility: .visible
        ) {
            Button("Clear", role: .destructive) {
                model.clearAllDuplicateDismissals()
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Dismissed duplicate groups will reappear with the !! badge if the current content still matches.")
        }
    }

    private static let categories: [(String, String)] = [
        ("create",    "Created"),
        ("edit",      "Edited"),
        ("delete",    "Deleted"),
        ("complete",  "Completed / reopened"),
        ("duplicate", "Duplicated"),
        ("move",      "Moved between lists"),
        ("clipboard", "Clipboard (copy / paste / cut)"),
        ("restore",   "Restored"),
        ("bulk",      "Bulk actions"),
        ("sync",      "Sync diffs"),
        ("other",     "Other"),
    ]

    // ~75 bytes per entry is a reasonable napkin estimate (timestamp + short
    // summary + metadata keys). Snapshot-bearing entries can be ~1-4 KB each
    // but only a minority of entries carry snapshots, so the blended number
    // holds. The note is deliberately fuzzy — we don't want users to
    // micromanage bytes.
    private var storageFootnote: String {
        let cap = model.settings.historyStorageCap
        let approxKB = cap / 12
        return "The on-disk log caps at \(cap) entries (~\(approxKB) KB typical; more if entries carry snapshots). Hard ceiling: \(MutationAuditLog.absoluteCeiling)."
    }
}
