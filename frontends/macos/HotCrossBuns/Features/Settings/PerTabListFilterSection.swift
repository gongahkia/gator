import SwiftUI

// Per-tab list visibility filters for the Tasks and Notes tabs. These
// override the global "Task lists" visibility for the specific tab when
// configured. Stays local — Google Tasks isn't aware of per-tab views.
struct PerTabListFilterSection: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        Section("Per-tab list filters") {
            explainer

            tabRows(
                title: "Tasks tab",
                systemImage: "checklist",
                hasConfigured: model.settings.hasConfiguredTasksTabSelection,
                selected: model.settings.tasksTabSelectedListIDs,
                onToggleOverride: { model.setTasksTabListFilter($0 ? Set(model.taskLists.map(\.id)) : nil) },
                onToggleList: { id in toggleTasksTabList(id) }
            )

            tabRows(
                title: "Notes tab",
                systemImage: "note.text",
                hasConfigured: model.settings.hasConfiguredNotesTabSelection,
                selected: model.settings.notesTabSelectedListIDs,
                onToggleOverride: { model.setNotesTabListFilter($0 ? Set(model.taskLists.map(\.id)) : nil) },
                onToggleList: { id in toggleNotesTabList(id) }
            )
        }
    }

    private var explainer: some View {
        Text("Each tab can hide lists independently of the global Task Lists selection. Turn off \"Use custom filter\" to fall back to the global set.")
            .hcbFont(.caption)
            .foregroundStyle(.secondary)
    }

    @ViewBuilder
    private func tabRows(
        title: String,
        systemImage: String,
        hasConfigured: Bool,
        selected: Set<TaskListMirror.ID>,
        onToggleOverride: @escaping (Bool) -> Void,
        onToggleList: @escaping (TaskListMirror.ID) -> Void
    ) -> some View {
        DisclosureGroup {
            Toggle("Use custom filter", isOn: Binding(
                get: { hasConfigured },
                set: { onToggleOverride($0) }
            ))
            if hasConfigured {
                if model.taskLists.isEmpty {
                    Text("No task lists loaded yet.")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                } else {
                    ForEach(model.taskLists) { list in
                        Toggle(isOn: Binding(
                            get: { selected.contains(list.id) },
                            set: { _ in onToggleList(list.id) }
                        )) {
                            Label(list.title, systemImage: "checklist")
                        }
                        .hcbScaledPadding(.leading, 12)
                    }
                }
            } else {
                Text("Inheriting global Task Lists visibility.")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .hcbScaledPadding(.leading, 4)
            }
        } label: {
            Label(title, systemImage: systemImage)
        }
    }

    private func toggleTasksTabList(_ id: TaskListMirror.ID) {
        var next = model.settings.tasksTabSelectedListIDs
        if next.contains(id) { next.remove(id) } else { next.insert(id) }
        model.setTasksTabListFilter(next)
    }

    private func toggleNotesTabList(_ id: TaskListMirror.ID) {
        var next = model.settings.notesTabSelectedListIDs
        if next.contains(id) { next.remove(id) } else { next.insert(id) }
        model.setNotesTabListFilter(next)
    }
}
