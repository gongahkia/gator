import AppKit
import SwiftUI

struct QuickAddView: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.hcbReduceMotion) private var reduceMotion
    @Environment(AppModel.self) private var model

    // Note mode reuses the task parser end-to-end — notes are undated
    // tasks, so any date the parser extracts auto-promotes the note into
    // a task on submit. We surface a warning chip so the semantic change
    // is visible before the user commits.
    var noteMode: Bool = false

    @State private var input: String = ""
    @State private var parsed: ParsedQuickAddTask = ParsedQuickAddTask(title: "", dueDate: nil, taskListHint: nil, matchedTokens: [])
    @State private var selectedTaskListID: TaskListMirror.ID?
    @State private var isSubmitting = false
    @State private var errorMessage: String?
    @State private var isGrammarExpanded = false
    @FocusState private var focusedField: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            header

            TextField(placeholderText, text: $input, axis: .vertical)
                .textFieldStyle(.plain)
                .font(.system(.title3, design: .rounded, weight: .medium))
                .lineLimit(1...4)
                .focused($focusedField)
                .onSubmit { Task { await submit() } }
                .onChange(of: input) { _, newValue in reparse(newValue) }
                .accessibilityLabel("Task title with optional date and list")
                .accessibilityHint("Type a task title. Words like tomorrow or #work are parsed into due date and list.")
                .hcbScaledPadding(14)
                .background(
                    RoundedRectangle(cornerRadius: 16)
                        .fill(AppColor.cream.opacity(0.8))
                )
                .overlay(
                    RoundedRectangle(cornerRadius: 16)
                        .strokeBorder(AppColor.cardStroke, lineWidth: 0.8)
                )

            if isGrammarExpanded {
                grammarReference
            }

            previewStrip

            if let errorMessage {
                Text(errorMessage)
                    .hcbFont(.caption)
                    .foregroundStyle(AppColor.ember)
            }

            HStack {
                listPicker
                Spacer()
                Button("Cancel") { dismiss() }
                    .keyboardShortcut(.cancelAction)
                Button {
                    Task { await submit() }
                } label: {
                    if isSubmitting {
                        ProgressView().controlSize(.small)
                    } else {
                        Text(submitButtonTitle)
                    }
                }
                .buttonStyle(.borderedProminent)
                .tint(AppColor.ember)
                .keyboardShortcut(.return, modifiers: [])
                .disabled(canSubmit == false)
            }
        }
        .hcbScaledPadding(22)
        .hcbScaledFrame(width: 560)
        .onAppear {
            if input.isEmpty, let shared = model.pendingSharedPrefill, shared.isEmpty == false {
                // Share-extension handoff takes precedence over clipboard
                // because the user's explicit share gesture is a stronger
                // intent signal than whatever happens to be on the pasteboard.
                input = shared
                reparse(shared)
                model.pendingSharedPrefill = nil
            } else if input.isEmpty, let clip = clipboardSuggestion() {
                input = clip
                reparse(clip)
            }
            focusedField = true
            selectedTaskListID = selectedTaskListID ?? defaultListID
        }
    }

    // Pull a single-line candidate from the pasteboard if it looks like
    // something the user would want as a task title. URLs and short text
    // pass through verbatim; large payloads (copied Xcode stack traces,
    // etc.) are ignored so the sheet doesn't pre-fill with noise.
    private func clipboardSuggestion() -> String? {
        let pb = NSPasteboard.general
        guard let raw = pb.string(forType: .string) else { return nil }
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false, trimmed.count <= 200 else { return nil }
        // Reject multi-line clipboards unless they collapse to a short
        // single line; otherwise skip.
        let lines = trimmed.components(separatedBy: .newlines).filter { $0.isEmpty == false }
        if lines.count > 1 {
            if let url = URL(string: trimmed), url.scheme != nil { return trimmed }
            return nil
        }
        return trimmed
    }

    private var header: some View {
        HStack(spacing: 10) {
            Image(systemName: noteMode ? "note.text.badge.plus" : "checklist")
                .foregroundStyle(AppColor.ember)
            Text(noteMode ? "New Note" : "New Task")
                .hcbFont(.headline)
            Spacer(minLength: 0)
            Button {
                HCBMotion.perform(reduceMotion: reduceMotion, animation: .easeInOut(duration: 0.18)) {
                    isGrammarExpanded.toggle()
                }
            } label: {
                Image(systemName: isGrammarExpanded ? "info.circle.fill" : "info.circle")
            }
            .buttonStyle(.borderless)
            .help(isGrammarExpanded ? "Hide quick-add grammar" : "Show quick-add grammar")
            .accessibilityLabel(isGrammarExpanded ? "Hide quick-add grammar" : "Show quick-add grammar")
            Text("Return to add, Esc to cancel")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    private var placeholderText: String {
        noteMode
            ? "Capture a thought — try \"followup on pricing #ideas\""
            : "Add a task — try \"email rent receipt tmr #personal\""
    }

    private var submitButtonTitle: String {
        if noteMode {
            return parsed.dueDate == nil ? "Add Note" : "Add as Task"
        }
        return "Add Task"
    }

    private var previewStrip: some View {
        HStack(spacing: 8) {
            if parsed.title.isEmpty {
                Label("Type a title", systemImage: "text.cursor")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
            } else {
                chip(icon: "text.alignleft", text: parsed.title, tint: AppColor.ink)
            }
            if let due = parsed.dueDate {
                chip(icon: "calendar", text: tokenDisplay(.dueDate) ?? due.formatted(date: .abbreviated, time: .omitted), tint: AppColor.moss)
            }
            if parsed.taskListHint != nil {
                chip(icon: "number", text: resolvedListName, tint: AppColor.blue)
            }
            if noteMode, parsed.dueDate != nil {
                chip(icon: "exclamationmark.triangle.fill", text: "Will become a task", tint: AppColor.ember)
            }
            Spacer(minLength: 0)
        }
        .hcbScaledFrame(minHeight: 26)
    }

    private var listPicker: some View {
        Picker("List", selection: Binding(
            get: { selectedTaskListID ?? defaultListID },
            set: { selectedTaskListID = $0 }
        )) {
            ForEach(model.taskLists) { list in
                Text(list.title).tag(Optional(list.id))
            }
        }
        .pickerStyle(.menu)
        .fixedSize()
        .labelsHidden()
    }

    private var grammarReference: some View {
        VStack(alignment: .leading, spacing: 8) {
            ForEach(NaturalLanguageTaskParser.helpEntries) { entry in
                VStack(alignment: .leading, spacing: 4) {
                    Text(entry.title)
                        .hcbFont(.caption, weight: .semibold)
                    Text(entry.examples.joined(separator: "  ·  "))
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                        .textSelection(.enabled)
                }
            }
        }
        .hcbScaledPadding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.thinMaterial, in: RoundedRectangle(cornerRadius: 10))
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .strokeBorder(AppColor.cardStroke, lineWidth: 0.8)
        )
        .transition(.opacity.combined(with: .move(edge: .top)))
    }

    private func chip(icon: String, text: String, tint: Color) -> some View {
        Label {
            Text(text).lineLimit(1)
        } icon: {
            Image(systemName: icon)
        }
        .hcbFont(.caption, weight: .medium)
        .hcbScaledPadding(.horizontal, 10)
        .hcbScaledPadding(.vertical, 5)
        .background(
            Capsule().fill(tint.opacity(0.15))
        )
        .foregroundStyle(tint)
    }

    private var canSubmit: Bool {
        parsed.title.isEmpty == false && (selectedTaskListID ?? defaultListID) != nil && model.account != nil && isSubmitting == false
    }

    private var defaultListID: TaskListMirror.ID? {
        if let hint = parsed.taskListHint {
            if let match = model.taskLists.first(where: { $0.title.localizedCaseInsensitiveCompare(hint) == .orderedSame }) {
                return match.id
            }
            if let match = model.taskLists.first(where: { $0.title.localizedCaseInsensitiveContains(hint) }) {
                return match.id
            }
        }
        return model.taskLists.first?.id
    }

    private var resolvedListName: String {
        if let id = selectedTaskListID ?? defaultListID {
            return model.taskListTitle(for: id, fallback: parsed.taskListHint ?? "")
        }
        return parsed.taskListHint ?? ""
    }

    private func tokenDisplay(_ kind: ParsedQuickAddTask.MatchedToken.Kind) -> String? {
        parsed.matchedTokens.first(where: { $0.kind == kind })?.display
    }

    private func reparse(_ text: String) {
        let parser = NaturalLanguageTaskParser()
        parsed = parser.parse(text)
        if selectedTaskListID == nil, let hintID = defaultListID {
            selectedTaskListID = hintID
        }
    }

    private func submit() async {
        guard canSubmit else { return }
        guard let listID = selectedTaskListID ?? defaultListID else { return }
        isSubmitting = true
        errorMessage = nil
        let didCreate = await model.createTask(
            title: parsed.title,
            notes: "",
            dueDate: parsed.dueDate,
            taskListID: listID
        )
        isSubmitting = false
        if didCreate {
            dismiss()
        } else {
            errorMessage = model.lastMutationError ?? "Couldn't add task."
        }
    }
}
