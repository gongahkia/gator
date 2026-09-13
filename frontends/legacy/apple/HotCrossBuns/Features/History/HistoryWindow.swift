import AppKit
import SwiftUI

// Floating window ledger of all recorded mutations. Reads from MutationAuditLog
// (actor, JSON-persisted at ~/Library/Application Support/<bundleID>/audit.log).
// The undo toast still handles the recent-action convenience; this window is
// the long-memory "did sync drop something? when did I mark that done?" view.
//
// Category filters persist in AppSettings so the user's choice survives
// relaunch. Sync-pulled diffs are excluded from the default filter set
// (too chatty) but visible if the user opts in.
struct HistoryWindow: View {
    @Environment(AppModel.self) private var model
    @State private var entries: [MutationAuditEntry] = []
    @State private var isLoading = true
    @State private var refreshTrigger = UUID()
    @State private var isConfirmingClear = false

    var body: some View {
        NavigationStack {
            contentBody
        }
    }

    @ViewBuilder
    private var contentBody: some View {
        VStack(spacing: 0) {
            if isLoading {
                VStack { ProgressView().controlSize(.regular) }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if filteredEntries.isEmpty {
                ContentUnavailableView(
                    "No history yet",
                    systemImage: "clock.arrow.circlepath",
                    description: Text(entries.isEmpty
                        ? "Your edits, moves, and syncs will appear here."
                        : "No entries match the current filters.")
                )
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                List {
                    ForEach(filteredEntries, id: \.id) { entry in
                        // NavigationLink wraps the row so click → push to
                        // HistoryEntryDetailView. The default disclosure chevron
                        // is the native macOS affordance for "this row drills in".
                        NavigationLink(value: entry) {
                            HistoryEntryRow(entry: entry, onSnapshotCopied: {
                                NSPasteboard.general.clearContents()
                                NSPasteboard.general.setString(entry.priorSnapshotJSON ?? entry.postSnapshotJSON ?? "", forType: .string)
                            })
                        }
                    }
                }
                .listStyle(.inset)
            }

            Divider()

            HStack(spacing: 10) {
                Text("\(filteredEntries.count) of \(entries.count) entries")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                Spacer(minLength: 0)
                Button {
                    refreshTrigger = UUID()
                } label: {
                    Label("Refresh", systemImage: "arrow.clockwise")
                }
                .buttonStyle(.bordered)
                .controlSize(.small)
                Button(role: .destructive) {
                    isConfirmingClear = true
                } label: {
                    Label("Clear All…", systemImage: "trash")
                }
                .buttonStyle(.bordered)
                .controlSize(.small)
            }
            .hcbScaledPadding(.horizontal, 14)
            .hcbScaledPadding(.vertical, 10)
        }
        .hcbSurface(.inspector)
        .frame(minWidth: 600, minHeight: 420)
        // The History window is a detached Scene — environment set in
        // MacSidebarShell doesn't reach here. Re-apply the appearance chain
        // (id + withHCBAppearance + preferredColorScheme + appBackground)
        // so the window follows the user's color scheme instead of the
        // default system light theme. Mirrors HCBSettingsWindow.
        .id(model.settings.colorSchemeID)
        .withHCBAppearance(model.settings)
        .environment(\.hcbShortcutOverrides, model.settings.shortcutOverrides)
        .hcbPreferredColorScheme(model.settings)
        .appBackground()
        .navigationTitle("History")
        .toolbar {
            ToolbarItemGroup(placement: .primaryAction) {
                // Single native Filter menu in the title bar — replaces the
                // inline chip row. Matches Mail / Messages / Finder toolbar
                // conventions: toolbar surfaces filters via a dropdown, the
                // content area stays clean for its primary data.
                Menu {
                    ForEach(Self.filterCategories, id: \.0) { (key, title) in
                        Toggle(title, isOn: Binding(
                            get: { model.settings.historyCategoryFilters.contains(key) },
                            set: { model.setHistoryCategoryEnabled(key, enabled: $0) }
                        ))
                    }
                } label: {
                    Label("Filter", systemImage: activeFilterCount < Self.filterCategories.count ? "line.3.horizontal.decrease.circle.fill" : "line.3.horizontal.decrease.circle")
                }
                .help("Choose which history entry types to show")
            }
        }
        .navigationDestination(for: MutationAuditEntry.self) { entry in
            HistoryEntryDetailView(entry: entry)
        }
        .task(id: refreshTrigger) { await reload() }
        .confirmationDialog(
            "Clear all history?",
            isPresented: $isConfirmingClear,
            titleVisibility: .visible
        ) {
            Button("Clear", role: .destructive) {
                Task {
                    await MutationAuditLog.shared.clear()
                    await reload()
                }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("This erases the local audit log. It does not touch Google — your tasks and events are unaffected.")
        }
    }

    private var filteredEntries: [MutationAuditEntry] {
        let limit = model.settings.historyVisibleLimit
        let enabled = model.settings.historyCategoryFilters
        return entries
            .filter { enabled.contains(Self.category(for: $0.kind)) }
            .prefix(limit)
            .map { $0 }
    }

    private func reload() async {
        isLoading = true
        let all = await MutationAuditLog.shared.allEntries()
        entries = all
        isLoading = false
    }

    // Source of truth for the filter menu. Order defines the menu order.
    static let filterCategories: [(String, String)] = [
        ("create",    "Created"),
        ("edit",      "Edited"),
        ("delete",    "Deleted"),
        ("complete",  "Completed"),
        ("duplicate", "Duplicated"),
        ("move",      "Moved"),
        ("clipboard", "Clipboard"),
        ("restore",   "Restored"),
        ("bulk",      "Bulk"),
        ("sync",      "Sync"),
        ("other",     "Other"),
    ]

    private var activeFilterCount: Int {
        let all = Set(Self.filterCategories.map(\.0))
        return model.settings.historyCategoryFilters.intersection(all).count
    }

    // maps audit-entry kind strings to the filter category bucket.
    static func category(for kind: String) -> String {
        if kind.hasPrefix("task.create") || kind.hasPrefix("event.create") { return "create" }
        if kind.hasPrefix("task.edit") || kind.hasPrefix("event.edit") { return "edit" }
        if kind.hasPrefix("task.delete") || kind.hasPrefix("event.delete") { return "delete" }
        if kind == "task.complete" || kind == "task.reopen" || kind == "event.dismiss" { return "complete" }
        if kind.hasPrefix("task.duplicate") { return "duplicate" }
        if kind.hasPrefix("task.move") { return "move" }
        if kind.hasPrefix("clipboard.") { return "clipboard" }
        if kind.hasPrefix("task.restore") || kind.hasPrefix("event.restore") { return "restore" }
        if kind.hasPrefix("bulk.") { return "bulk" }
        if kind.hasPrefix("sync.") { return "sync" }
        return "other"
    }
}
