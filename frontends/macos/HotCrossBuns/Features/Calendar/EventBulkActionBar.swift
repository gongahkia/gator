import SwiftUI

struct EventBulkActionBar: View {
    @Environment(AppModel.self) private var model
    @Binding var selection: Set<String>
    let events: [CalendarEventMirror]

    @State private var isConfirmingDelete = false
    @State private var isMutating = false

    var body: some View {
        HStack(spacing: 14) {
            Text("\(events.count) event\(events.count == 1 ? "" : "s")")
                .hcbFont(.subheadline, weight: .semibold)
            Divider().hcbScaledFrame(height: 20)
            Menu {
                Button("+ 15 minutes") { Task { await shift(by: 15) } }
                Button("+ 1 hour") { Task { await shift(by: 60) } }
                Button("+ 1 day") { Task { await shift(by: 60 * 24) } }
                Divider()
                Button("− 15 minutes") { Task { await shift(by: -15) } }
                Button("− 1 hour") { Task { await shift(by: -60) } }
                Button("− 1 day") { Task { await shift(by: -60 * 24) } }
            } label: {
                Label("Shift", systemImage: "clock.arrow.2.circlepath")
            }
            .disabled(isMutating || events.allSatisfy(\.isAllDay))
            Button(role: .destructive) {
                isConfirmingDelete = true
            } label: {
                Label("Delete", systemImage: "trash")
            }
            .disabled(isMutating)
            Button {
                selection.removeAll()
            } label: {
                Label("Clear", systemImage: "xmark.circle")
            }
        }
        .hcbScaledPadding(.horizontal, 16)
        .hcbScaledPadding(.vertical, 10)
        .background(.thinMaterial, in: Capsule())
        .overlay(Capsule().strokeBorder(AppColor.cardStroke, lineWidth: 0.6))
        .shadow(radius: 6, y: 2)
        .confirmationDialog(
            "Delete \(events.count) events?",
            isPresented: $isConfirmingDelete,
            titleVisibility: .visible
        ) {
            Button("Delete \(events.count)", role: .destructive) {
                Task { await delete() }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Removes these events from Google Calendar without sending guest updates. Recurring events are deleted as single occurrences.")
        }
    }

    private func shift(by minutes: Int) async {
        isMutating = true
        defer { isMutating = false }
        let moved = await model.bulkShiftEvents(events, byMinutes: minutes)
        if moved == events.count {
            selection.removeAll()
        }
    }

    private func delete() async {
        isMutating = true
        defer { isMutating = false }
        let deleted = await model.bulkDeleteEvents(events)
        if deleted > 0 {
            selection.removeAll()
        }
    }
}
