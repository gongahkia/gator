import SwiftUI

enum TaskSnoozeSupport {
    static func targetDate(daysFromToday days: Int, calendar: Calendar = .current, now: Date = Date()) -> Date {
        let startOfToday = calendar.startOfDay(for: now)
        return calendar.date(byAdding: .day, value: days, to: startOfToday) ?? startOfToday
    }
}

struct TaskSnoozeContextMenu: View {
    let onSnoozeTo: (Date) -> Void
    let onPickCustomDate: () -> Void

    var body: some View {
        Menu {
            Button("Tomorrow") {
                onSnoozeTo(TaskSnoozeSupport.targetDate(daysFromToday: 1))
            }
            Button("In 2 days") {
                onSnoozeTo(TaskSnoozeSupport.targetDate(daysFromToday: 2))
            }
            Button("Next week") {
                onSnoozeTo(TaskSnoozeSupport.targetDate(daysFromToday: 7))
            }
            Divider()
            Button("Pick date…", action: onPickCustomDate)
        } label: {
            Label("Snooze", systemImage: "moon.zzz")
        }
    }
}

struct SnoozePickerSheet: View {
    @Environment(\.dismiss) private var dismiss
    let task: TaskMirror
    let onSelect: (Date) -> Void

    @State private var pickedDate: Date = Date()

    var body: some View {
        NavigationStack {
            Form {
                Section("Snooze \(task.title) until") {
                    DatePicker("Date", selection: $pickedDate, in: Calendar.current.startOfDay(for: Date())..., displayedComponents: [.date])
                        .datePickerStyle(.graphical)
                }
            }
            .navigationTitle("Snooze Task")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                        .keyboardShortcut(.cancelAction)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Snooze") {
                        onSelect(Calendar.current.startOfDay(for: pickedDate))
                        dismiss()
                    }
                    .keyboardShortcut(.defaultAction)
                    .buttonStyle(.borderedProminent)
                }
            }
        }
        .hcbScaledFrame(minWidth: 360, minHeight: 400)
    }
}
