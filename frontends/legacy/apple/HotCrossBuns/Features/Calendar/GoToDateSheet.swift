import SwiftUI

struct GoToDateSheet: View {
    @Environment(\.dismiss) private var dismiss
    let initialDate: Date
    let onPick: (Date) -> Void

    @State private var pickedDate: Date

    init(initialDate: Date, onPick: @escaping (Date) -> Void) {
        self.initialDate = initialDate
        self.onPick = onPick
        _pickedDate = State(initialValue: initialDate)
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    DatePicker("Jump to", selection: $pickedDate, displayedComponents: [.date])
                        .datePickerStyle(.graphical)
                }
                Section {
                    HStack(spacing: 8) {
                        Button("Today") { pickedDate = Calendar.current.startOfDay(for: Date()) }
                        Button("Tomorrow") { pickedDate = shift(days: 1) }
                        Button("Next week") { pickedDate = shift(days: 7) }
                        Button("In 1 month") { pickedDate = shift(months: 1) }
                    }
                    .buttonStyle(.bordered)
                } header: {
                    Text("Jump to…")
                }
            }
            .navigationTitle("Go to Date")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                        .keyboardShortcut(.cancelAction)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Go") {
                        onPick(pickedDate)
                        dismiss()
                    }
                    .keyboardShortcut(.defaultAction)
                }
            }
        }
        .hcbScaledFrame(minWidth: 380, minHeight: 420)
    }

    private func shift(days: Int = 0, months: Int = 0) -> Date {
        let cal = Calendar.current
        let today = cal.startOfDay(for: Date())
        var date = cal.date(byAdding: .day, value: days, to: today) ?? today
        if months != 0 {
            date = cal.date(byAdding: .month, value: months, to: date) ?? date
        }
        return date
    }
}
