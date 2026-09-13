import SwiftUI

// Color-tag bindings for events. When enabled, typing `#<tag>` in an
// event title during Quick Create auto-applies the bound Google Calendar
// color. Event-only — tasks and notes have no color field in Google.
struct ColorTagBindingsSection: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        Section("Event color tags") {
            Toggle("Auto-color events by tag", isOn: Binding(
                get: { model.settings.colorTagAutoApplyEnabled },
                set: { model.setColorTagAutoApplyEnabled($0) }
            ))
            Text("In Quick Create (events only), typing a hashtag you've bound below auto-applies the matching Google Calendar color. The hashtag is stripped from the title before it's sent to Google.")
                .hcbFont(.caption)
                .foregroundStyle(.secondary)

            if model.settings.colorTagAutoApplyEnabled {
                Picker("Conflict resolution", selection: Binding(
                    get: { model.settings.colorTagMatchPolicy },
                    set: { model.setColorTagMatchPolicy($0) }
                )) {
                    ForEach(ColorTagMatchPolicy.allCases, id: \.self) { policy in
                        Text(policy.title).tag(policy)
                    }
                }
                .pickerStyle(.menu)
                Text(model.settings.colorTagMatchPolicy.subtitle)
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)

                DisclosureGroup("Bindings") {
                    ForEach(CalendarEventColor.allCases.filter { $0 != .defaultColor }) { color in
                        bindingRow(for: color)
                    }
                }
            }
        }
    }

    @ViewBuilder
    private func bindingRow(for color: CalendarEventColor) -> some View {
        let colorId = color.rawValue
        HStack(spacing: 10) {
            if let hex = color.hex {
                Circle()
                    .fill(Color(hex: hex))
                    .hcbScaledFrame(width: 14, height: 14)
            }
            Text(color.title)
                .hcbFont(.body)
                .frame(width: 96, alignment: .leading)
            TextField("tag (e.g. work)", text: Binding(
                get: { model.settings.colorTagBindings[colorId] ?? "" },
                set: { model.setColorTagBinding(colorId: colorId, tag: $0) }
            ))
            .textFieldStyle(.roundedBorder)
        }
    }
}
