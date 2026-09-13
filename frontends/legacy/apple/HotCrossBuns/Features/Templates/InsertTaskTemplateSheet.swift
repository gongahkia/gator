import SwiftUI

// Two-step sheet for instantiating a task template (§6.13).
//  1. Pick a template from the user's library.
//  2. If the template has {{prompt:Label}} placeholders, collect answers.
// Then call AppModel.instantiateTaskTemplate and dismiss.
struct InsertTaskTemplateSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(AppModel.self) private var model

    @State private var step: Step = .pick
    @State private var selected: TaskTemplate?
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
            .navigationTitle("Insert Task Template")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Close") { dismiss() }
                        .keyboardShortcut(.cancelAction)
                }
            }
        }
        .hcbScaledFrame(minWidth: 440, minHeight: 360)
    }

    // MARK: - step 1

    private var pickStep: some View {
        Group {
            if model.settings.taskTemplates.isEmpty {
                ContentUnavailableView(
                    "No templates yet",
                    systemImage: "doc.text",
                    description: Text("Create one in Settings → Task templates.")
                )
            } else {
                List {
                    ForEach(model.settings.taskTemplates) { template in
                        Button {
                            pick(template)
                        } label: {
                            VStack(alignment: .leading, spacing: 3) {
                                Text(template.name)
                                    .hcbFont(.subheadline, weight: .semibold)
                                Text(template.title.isEmpty ? "(no title)" : template.title)
                                    .hcbFont(.caption)
                                    .foregroundStyle(.secondary)
                                    .lineLimit(1)
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

    private func pick(_ template: TaskTemplate) {
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
                        Text("Create Task")
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
        let ok = await model.instantiateTaskTemplate(template, prompts: answers)
        if ok {
            dismiss()
        } else {
            error = model.lastMutationError ?? "Could not create task."
        }
    }
}
