import SwiftUI

struct RecurrenceEditor: View {
    @Binding var rule: RecurrenceRule?
    let startDate: Date

    @State private var endKind: RecurrenceEndKind = .never
    @State private var endCount: Int = 5
    @State private var endDate: Date

    private enum RecurrenceEndKind: Hashable {
        case never
        case after
        case onDate
    }

    init(rule: Binding<RecurrenceRule?>, startDate: Date = Date()) {
        _rule = rule
        self.startDate = startDate
        _endDate = State(initialValue: Calendar.current.date(byAdding: .month, value: 1, to: startDate) ?? startDate)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Picker("Repeat", selection: frequencyBinding) {
                Text("Does not repeat").tag(RecurrenceFrequency?.none)
                ForEach(RecurrenceFrequency.allCases, id: \.self) { f in
                    Text(f.title).tag(Optional(f))
                }
            }
            .pickerStyle(.menu)

            if let rule {
                Label(rule.summary, systemImage: "repeat")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)

                VStack(alignment: .leading, spacing: 8) {
                    intervalStepper
                    endControls
                }
                .hcbScaledPadding(.leading, 4)
            }
        }
        .onAppear { syncEndControls(from: rule) }
        .onChange(of: rule) { _, newValue in
            syncEndControls(from: newValue)
        }
        .onChange(of: startDate) { _, newValue in
            if endDate < newValue {
                endDate = newValue
                applyEndToRule()
            }
        }
    }

    private var frequencyBinding: Binding<RecurrenceFrequency?> {
        Binding(
            get: { rule?.frequency },
            set: { newValue in
                if let newValue {
                    rule = RecurrenceRule(
                        frequency: newValue,
                        interval: rule?.interval ?? 1,
                        end: rule?.end ?? .never
                    )
                } else {
                    rule = nil
                }
            }
        )
    }

    private var intervalStepper: some View {
        HStack {
            Text("Every")
            Stepper(value: intervalBinding, in: 1...99) {
                Text("\(rule?.interval ?? 1) \(frequencyLabelPlural)")
                    .monospacedDigit()
            }
        }
        .hcbScaledPadding(.vertical, 6)
    }

    private var endControls: some View {
        VStack(alignment: .leading, spacing: 8) {
            Picker("Ends", selection: endKindBinding) {
                Text("Never").tag(RecurrenceEndKind.never)
                Text("After").tag(RecurrenceEndKind.after)
                Text("On date").tag(RecurrenceEndKind.onDate)
            }
            .pickerStyle(.menu)

            switch endKind {
            case .never:
                EmptyView()
            case .after:
                Stepper(value: endCountBinding, in: 1...999) {
                    Text("\(endCount) occurrence\(endCount == 1 ? "" : "s")")
                        .monospacedDigit()
                }
            case .onDate:
                DatePicker("On", selection: endDateBinding, in: startDate..., displayedComponents: [.date])
            }
        }
    }

    private var intervalBinding: Binding<Int> {
        Binding(
            get: { rule?.interval ?? 1 },
            set: { newValue in
                guard let existing = rule else { return }
                rule = RecurrenceRule(frequency: existing.frequency, interval: newValue, end: existing.end)
            }
        )
    }

    private var endKindBinding: Binding<RecurrenceEndKind> {
        Binding(
            get: { endKind },
            set: { newValue in
                endKind = newValue
                applyEndToRule()
            }
        )
    }

    private var endCountBinding: Binding<Int> {
        Binding(
            get: { endCount },
            set: { newValue in
                endCount = max(1, newValue)
                applyEndToRule()
            }
        )
    }

    private var endDateBinding: Binding<Date> {
        Binding(
            get: { endDate },
            set: { newValue in
                endDate = newValue
                applyEndToRule()
            }
        )
    }

    private func applyEndToRule() {
        guard var existing = rule else { return }
        switch endKind {
        case .never:
            existing.end = .never
        case .after:
            existing.end = .after(max(1, endCount))
        case .onDate:
            existing.end = .until(max(endDate, startDate))
        }
        rule = existing
    }

    private func syncEndControls(from rule: RecurrenceRule?) {
        guard let rule else {
            endKind = .never
            return
        }
        switch rule.end {
        case .never:
            endKind = .never
        case .after(let count):
            endKind = .after
            endCount = max(1, count)
        case .until(let date):
            endKind = .onDate
            endDate = max(date, startDate)
        }
    }

    private var frequencyLabelPlural: String {
        guard let frequency = rule?.frequency else { return "days" }
        let interval = rule?.interval ?? 1
        let singular: String = switch frequency {
        case .daily: "day"
        case .weekly: "week"
        case .monthly: "month"
        case .yearly: "year"
        }
        return interval == 1 ? singular : singular + "s"
    }
}
