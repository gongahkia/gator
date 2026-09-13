import SwiftUI

// §6.13b — Two-step sheet for instantiating an event template. Mirrors
// InsertTaskTemplateSheet.
//  1. Pick from the user's library.
//  2. If the template has {{prompt:Label}} placeholders, collect answers.
// Then call AppModel.instantiateEventTemplate.
struct InsertEventTemplateSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(AppModel.self) private var model

    @State private var step: Step = .pick
    @State private var selected: EventTemplate?
    @State private var answers: [String: String] = [:]
    @State private var isSubmitting = false
    @State private var error: String?
    @FocusState private var focusedPrompt: String?

    enum Step { case pick, prompts }

    var body: some View {
        NavigationStack {
            Group {
                switch step {
                case .pick: pickStep
                case .prompts: promptsStep
                }
            }
            .navigationTitle("Insert Event Template")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Close") { dismiss() }
                        .keyboardShortcut(.cancelAction)
                }
            }
        }
        .hcbScaledFrame(minWidth: 460, minHeight: 380)
    }

    // MARK: - step 1

    private var pickStep: some View {
        Group {
            if model.settings.eventTemplates.isEmpty {
                ContentUnavailableView(
                    "No event templates yet",
                    systemImage: "calendar",
                    description: Text("Create one in Settings → Event templates.")
                )
            } else {
                List {
                    ForEach(model.settings.eventTemplates) { template in
                        Button {
                            pick(template)
                        } label: {
                            VStack(alignment: .leading, spacing: 3) {
                                Text(template.name)
                                    .hcbFont(.subheadline, weight: .semibold)
                                Text(template.summary.isEmpty ? "(no title)" : template.summary)
                                    .hcbFont(.caption)
                                    .foregroundStyle(.secondary)
                                    .lineLimit(1)
                                templateMeta(template)
                                let prompts = template.requiredPrompts()
                                if prompts.isEmpty == false {
                                    Text("Will ask for: \(prompts.joined(separator: ", "))")
                                        .hcbFont(.caption2)
                                        .foregroundStyle(.secondary)
                                }
                            }
                            .frame(maxWidth: .infinity, alignment: .leading)
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
        }
    }

    @ViewBuilder
    private func templateMeta(_ template: EventTemplate) -> some View {
        let pieces: [String] = {
            var out: [String] = []
            if template.isAllDay {
                out.append("all-day")
            } else {
                out.append("\(template.durationMinutes) min")
                if template.timeAnchor.isEmpty == false {
                    out.append("@ \(template.timeAnchor)")
                }
            }
            if template.recurrenceRule.isEmpty == false { out.append("recurring") }
            if template.attendees.isEmpty == false { out.append("\(template.attendees.count) guest\(template.attendees.count == 1 ? "" : "s")") }
            return out
        }()
        if pieces.isEmpty == false {
            Text(pieces.joined(separator: " · "))
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    private func pick(_ template: EventTemplate) {
        selected = template
        answers = [:]
        let prompts = template.requiredPrompts()
        if prompts.isEmpty {
            Task { await instantiate() }
        } else {
            step = .prompts
        }
    }

    // MARK: - step 2

    private var promptsStep: some View {
        Form {
            if let template = selected {
                Section("Prompts for \(template.name)") {
                    ForEach(template.requiredPrompts(), id: \.self) { label in
                        TextField(label, text: Binding(
                            get: { answers[label] ?? "" },
                            set: { answers[label] = $0 }
                        ))
                        .textFieldStyle(.roundedBorder)
                        .focused($focusedPrompt, equals: label)
                    }
                }
            }
            if let error {
                Section {
                    Text(error).foregroundStyle(.red).hcbFont(.caption)
                }
            }
        }
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                Button {
                    Task { await instantiate() }
                } label: {
                    if isSubmitting {
                        ProgressView().controlSize(.small)
                    } else {
                        Text("Create Event")
                    }
                }
                .disabled(isSubmitting || allPromptsAnswered == false)
                .keyboardShortcut(.defaultAction)
            }
        }
        .onAppear {
            focusedPrompt = selected?.requiredPrompts().first
        }
    }

    private var allPromptsAnswered: Bool {
        guard let template = selected else { return false }
        return template.requiredPrompts().allSatisfy { label in
            (answers[label]?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false)
        }
    }

    private func instantiate() async {
        guard let template = selected else { return }
        isSubmitting = true
        defer { isSubmitting = false }
        let ok = await model.instantiateEventTemplate(template, prompts: answers)
        if ok {
            dismiss()
        } else {
            error = model.lastMutationError ?? "Could not create event."
        }
    }
}
