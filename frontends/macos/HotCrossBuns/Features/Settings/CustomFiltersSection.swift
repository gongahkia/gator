import SwiftUI

struct CustomFiltersSection: View {
    @Environment(AppModel.self) private var model
    var highlightedAnchor: SettingsSectionAnchor? = nil
    @State private var editor: CustomFilterDefinition?
    @State private var isCreating = false

    var body: some View {
        Section("Custom Filters") {
            SettingsHighlightRow(anchor: .customFilters, highlightedAnchor: highlightedAnchor)
            SettingsFeatureFlow(
                systemImage: "line.3.horizontal.decrease.circle",
                title: "Saved filter",
                steps: [
                    "Pick matching rules",
                    "Save a named filter",
                    "Pin it to the menu bar when it should be one click away"
                ]
            )
            if model.settings.customFilters.isEmpty {
                Text("No custom filters yet. Save a combination of due-window, list, star, and tag criteria — or a full DSL query — as a reusable saved filter.")
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
            } else {
                ForEach(model.settings.customFilters) { filter in
                    Button {
                        editor = filter
                    } label: {
                        HStack {
                            Label(filter.name, systemImage: filter.systemImage)
                            Spacer()
                            if queryHasError(filter) {
                                Image(systemName: "exclamationmark.triangle.fill")
                                    .foregroundStyle(.orange)
                                    .help("Query has an error — open to fix.")
                            }
                            Text(summary(filter))
                                .hcbFont(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                    .buttonStyle(.plain)
                    .swipeActions(edge: .trailing) {
                        Button(role: .destructive) {
                            model.deleteCustomFilter(filter.id)
                        } label: {
                            Label("Delete", systemImage: "trash")
                        }
                    }
                    .contextMenu {
                        Button {
                            editor = filter
                        } label: {
                            Label("Edit", systemImage: "pencil")
                        }
                        Button {
                            model.duplicateCustomFilter(filter)
                        } label: {
                            Label("Duplicate", systemImage: "plus.square.on.square")
                        }
                        Button {
                            editor = model.duplicateCustomFilter(filter)
                        } label: {
                            Label("Duplicate and Edit", systemImage: "pencil.and.list.clipboard")
                        }
                        Divider()
                        Button(role: .destructive) {
                            model.deleteCustomFilter(filter.id)
                        } label: {
                            Label("Delete", systemImage: "trash")
                        }
                    }
                }
            }

            Button {
                isCreating = true
            } label: {
                Label("New Filter", systemImage: "plus")
            }
        }
        .sheet(isPresented: $isCreating) {
            CustomFilterEditor(draft: CustomFilterDefinition(name: "New Filter"), onSave: { filter in
                model.upsertCustomFilter(filter)
                isCreating = false
            }, onCancel: { isCreating = false })
        }
        .sheet(item: $editor) { filter in
            CustomFilterEditor(draft: filter, onSave: { updated in
                model.upsertCustomFilter(updated)
                editor = nil
            }, onCancel: { editor = nil })
        }
    }

    private func queryHasError(_ filter: CustomFilterDefinition) -> Bool {
        guard filter.isUsingQueryDSL, let expr = filter.queryExpression else { return false }
        if case .failure = QueryCompiler.compile(expr) { return true }
        return false
    }

    private func summary(_ filter: CustomFilterDefinition) -> String {
        if filter.isUsingQueryDSL {
            return "DSL"
        }
        var parts: [String] = [filter.dueWindow.title]
        if filter.includeCompleted { parts.append("+ completed") }
        if filter.taskListIDs.isEmpty == false { parts.append("\(filter.taskListIDs.count) lists") }
        if filter.tagsAny.isEmpty == false { parts.append(filter.tagsAny.map { "#\($0)" }.joined(separator: " ")) }
        return parts.joined(separator: " · ")
    }
}

private struct CustomFilterEditor: View {
    @Environment(AppModel.self) private var model
    @State var draft: CustomFilterDefinition
    @State private var tagsText: String
    @State private var queryText: String
    @State private var testedTasks: [TaskMirror] = []
    @FocusState private var isNameFocused: Bool
    let onSave: (CustomFilterDefinition) -> Void
    let onCancel: () -> Void

    init(draft: CustomFilterDefinition, onSave: @escaping (CustomFilterDefinition) -> Void, onCancel: @escaping () -> Void) {
        _draft = State(initialValue: draft)
        _tagsText = State(initialValue: draft.tagsAny.joined(separator: " "))
        _queryText = State(initialValue: draft.queryExpression ?? "")
        self.onSave = onSave
        self.onCancel = onCancel
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 20) {
                        SettingsSheetSection("Filter") {
                            SettingsSheetRow("Name") {
                                TextField("", text: $draft.name)
                                    .textFieldStyle(.roundedBorder)
                                    .focused($isNameFocused)
                            }
                            SettingsSheetRow("SF Symbol") {
                                TextField("", text: $draft.systemImage)
                                    .font(.body.monospaced())
                                    .textFieldStyle(.roundedBorder)
                            }
                            SettingsSheetRow("") {
                                Toggle("Pin to menu bar", isOn: $draft.pinnedToMenuBar)
                                    .toggleStyle(.checkbox)
                            }
                            UsedByRow(items: filterUsedBy)
                        }

                        FilterOutcomePreview(
                            name: draft.name,
                            matchCount: previewMatchCount,
                            isPinned: draft.pinnedToMenuBar,
                            usesDSL: usingDSL,
                            testedTasks: testedTasks
                        )

                        SettingsSheetSection("Query (advanced)") {
                            TextEditor(text: $queryText)
                                .font(.body.monospaced())
                                .frame(minHeight: 120)
                                .scrollContentBackground(.hidden)
                                .background(AppColor.cardSurface)
                                .overlay(
                                    RoundedRectangle(cornerRadius: 6)
                                        .strokeBorder(borderColor, lineWidth: 0.5)
                                )
                            queryStatusRow
                            DisclosureGroup("DSL cheatsheet") {
                                VStack(alignment: .leading, spacing: 4) {
                                    cheat("title:bug",         "task title contains 'bug'")
                                    cheat("notes:\"grocery\"", "notes contain 'grocery' (quoted for spaces)")
                                    cheat("list:Work",         "task belongs to list matching 'Work' (by title or id)")
                                    cheat("tag:deep",          "task has #deep tag")
                                    cheat("#deep",             "same as tag:deep")
                                    cheat("completed",         "task is completed")
                                    cheat("overdue",           "due date is before today")
                                    cheat("has:notes",         "notes field non-empty")
                                    cheat("has:due",           "due date set")
                                    cheat("has:tag",           "any tag present")
                                    cheat("due:today",         "due today")
                                    cheat("due<+7d",           "due within the next 7 days")
                                    cheat("due>=2026-01-01",   "due on or after an absolute date")
                                    cheat("not completed",     "NOT completed")
                                    cheat("A AND B",           "both; whitespace also implies AND")
                                    cheat("A OR B",            "either")
                                    cheat("(A OR B) AND C",    "parentheses group precedence")
                                }
                                .hcbFont(.caption)
                                .foregroundStyle(.secondary)
                            }
                            Text("When the query field is non-empty, the fields below are ignored. Invalid queries match nothing — your tasks aren't modified.")
                                .hcbFont(.footnote)
                                .foregroundStyle(.secondary)
                        }

                        SettingsSheetSection("Due") {
                            SettingsSheetRow("Due window") {
                                Picker("", selection: $draft.dueWindow) {
                                    ForEach(DueWindow.allCases, id: \.self) { window in
                                        Text(window.title).tag(window)
                                    }
                                }
                                .labelsHidden()
                                .frame(maxWidth: 260, alignment: .leading)
                            }
                            .disabled(usingDSL)
                        }

                        SettingsSheetSection("Qualifiers") {
                            Toggle("Include completed", isOn: $draft.includeCompleted)
                                .toggleStyle(.checkbox)
                        }
                        .disabled(usingDSL)
                        if usingDSL {
                            Text("Structured fields are disabled while the query field is non-empty.")
                                .hcbFont(.caption2)
                                .foregroundStyle(.secondary)
                        }

                        SettingsSheetSection("Lists") {
                            Text("Leave empty to match every list.")
                                .hcbFont(.caption2)
                                .foregroundStyle(.secondary)
                            VStack(alignment: .leading, spacing: 8) {
                                ForEach(model.taskLists) { list in
                                    Toggle(list.title, isOn: Binding(
                                        get: { draft.taskListIDs.contains(list.id) },
                                        set: { isOn in
                                            if isOn { draft.taskListIDs.insert(list.id) }
                                            else { draft.taskListIDs.remove(list.id) }
                                        }
                                    ))
                                    .toggleStyle(.checkbox)
                                }
                            }
                        }
                        .disabled(usingDSL)

                        SettingsSheetSection("Tags") {
                            SettingsSheetRow("Any match") {
                                TextField("#work #urgent", text: $tagsText)
                                    .textFieldStyle(.roundedBorder)
                            }
                        }
                        .disabled(usingDSL)
                    }
                    .padding(24)
                    .frame(maxWidth: .infinity, alignment: .topLeading)
                }

                Divider()

                validationSummary

                SettingsSheetActions(cancelTitle: "Cancel", onCancel: onCancel) {
                    Button("Test Filter") {
                        testedTasks = previewMatches(limit: 5)
                    }
                    .keyboardShortcut(.defaultAction)
                    .disabled(filterValidationErrors.contains { $0.hasPrefix("Fix") })
                    Button("Save") {
                        var out = draft
                        out.tagsAny = tagsText
                            .split(whereSeparator: { $0.isWhitespace })
                            .map { String($0).trimmingCharacters(in: CharacterSet(charactersIn: "#")) }
                            .filter { $0.isEmpty == false }
                        let trimmedQuery = queryText.trimmingCharacters(in: .whitespacesAndNewlines)
                        out.queryExpression = trimmedQuery.isEmpty ? nil : trimmedQuery
                        onSave(out)
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(saveDisabled)
                }
            }
            .navigationTitle("Filter")
        }
        .frame(width: 720, height: 640)
        .onAppear {
            isNameFocused = true
            testedTasks = previewMatches(limit: 5)
        }
    }

    private var usingDSL: Bool {
        queryText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
    }

    private var compileResult: Result<CompiledQuery, QueryCompileError>? {
        let trimmed = queryText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return nil }
        return QueryCompiler.compile(queryText)
    }

    private var borderColor: Color {
        if case .failure = compileResult { return .orange }
        return .secondary.opacity(0.25)
    }

    @ViewBuilder
    private var queryStatusRow: some View {
        switch compileResult {
        case .none:
            Text("Leave empty to use the structured fields below.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        case .failure(let err):
            HStack(alignment: .top, spacing: 6) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .foregroundStyle(.orange)
                Text(errorText(err))
                    .hcbFont(.footnote)
                    .foregroundStyle(.orange)
            }
        case .success(let q):
            let count = matchCount(for: q)
            HStack(spacing: 6) {
                Image(systemName: "checkmark.circle.fill")
                    .foregroundStyle(.green)
                Text("Valid · \(count) task\(count == 1 ? "" : "s") match now")
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private func matchCount(for q: CompiledQuery) -> Int {
        let ctx = QueryContext(now: Date(), calendar: .current, taskLists: model.taskLists)
        var count = 0
        for task in model.tasks where task.isDeleted == false {
            if q.matches(task, context: ctx) { count += 1 }
        }
        return count
    }

    private var previewMatchCount: Int {
        previewMatches(limit: nil).count
    }

    private func previewMatches(limit: Int?) -> [TaskMirror] {
        if let compileResult {
            switch compileResult {
            case .success(let query):
                let ctx = QueryContext(now: Date(), calendar: .current, taskLists: model.taskLists)
                let matches = model.tasks.filter { $0.isDeleted == false && query.matches($0, context: ctx) }
                return limit.map { Array(matches.prefix($0)) } ?? matches
            case .failure:
                return []
            }
        }

        var preview = draft
        preview.tagsAny = parsedTags
        preview.queryExpression = nil
        let matches = preview.filter(model.tasks, taskLists: model.taskLists)
        return limit.map { Array(matches.prefix($0)) } ?? matches
    }

    private var parsedTags: [String] {
        tagsText
            .split(whereSeparator: { $0.isWhitespace })
            .map { String($0).trimmingCharacters(in: CharacterSet(charactersIn: "#")) }
            .filter { $0.isEmpty == false }
    }

    private func errorText(_ err: QueryCompileError) -> String {
        if err.position >= 0 {
            return "\(err.message) (at char \(err.position + 1))"
        }
        return err.message
    }

    private var saveDisabled: Bool {
        if draft.name.trimmingCharacters(in: .whitespaces).isEmpty { return true }
        // Block save on a broken DSL — we never want to persist a filter that
        // the sidebar will immediately render as an error row.
        if case .failure = compileResult { return true }
        return false
    }

    private var filterValidationErrors: [String] {
        var errors: [String] = []
        if draft.name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            errors.append("Name is required.")
        }
        if case .failure(let error) = compileResult {
            errors.append("Fix DSL: \(errorText(error))")
        }
        return errors
    }

    private var filterUsedBy: [String] {
        var items = ["Command Palette"]
        if draft.pinnedToMenuBar {
            items.append("Menu Bar")
        }
        return items
    }

    private var validationSummary: some View {
        Group {
            if filterValidationErrors.isEmpty == false {
                HStack(alignment: .top, spacing: 8) {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .foregroundStyle(.orange)
                    VStack(alignment: .leading, spacing: 2) {
                        ForEach(filterValidationErrors, id: \.self) { error in
                            Text(error)
                        }
                    }
                    .hcbFont(.caption)
                    .foregroundStyle(.orange)
                    Spacer()
                }
                .padding(.horizontal, 24)
                .padding(.vertical, 10)
                .background(.bar)
            }
        }
    }

    @ViewBuilder
    private func cheat(_ code: String, _ desc: String) -> some View {
        HStack(alignment: .top, spacing: 8) {
            Text(code)
                .font(.caption.monospaced())
                .foregroundStyle(AppColor.ink)
                .frame(minWidth: 140, alignment: .leading)
            Text(desc)
                .foregroundStyle(.secondary)
            Spacer(minLength: 0)
        }
    }
}

private struct FilterOutcomePreview: View {
    let name: String
    let matchCount: Int
    let isPinned: Bool
    let usesDSL: Bool
    let testedTasks: [TaskMirror]

    var body: some View {
        SettingsSheetSection("Outcome") {
            HStack(spacing: 10) {
                SettingsOutcomeCard(
                    systemImage: "number",
                    title: "\(matchCount)",
                    detail: "current match\(matchCount == 1 ? "" : "es")"
                )
                SettingsOutcomeCard(
                    systemImage: isPinned ? "menubar.rectangle" : "rectangle.dashed",
                    title: isPinned ? "Menu bar" : "Saved only",
                    detail: isPinned ? "visible in quick access" : "available in settings"
                )
                SettingsOutcomeCard(
                    systemImage: usesDSL ? "curlybraces" : "slider.horizontal.3",
                    title: usesDSL ? "DSL query" : "Structured rules",
                    detail: displayName
                )
            }
            if testedTasks.isEmpty {
                Text("Test preview: no matching tasks.")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
            } else {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Test preview")
                        .hcbFont(.caption, weight: .semibold)
                        .foregroundStyle(.secondary)
                    ForEach(testedTasks) { task in
                        Label(TagExtractor.stripped(from: task.title), systemImage: task.isCompleted ? "checkmark.circle.fill" : "circle")
                            .hcbFont(.caption)
                            .lineLimit(1)
                    }
                }
            }
        }
    }

    private var displayName: String {
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? "unnamed filter" : trimmed
    }
}

struct SettingsFeatureFlow: View {
    let systemImage: String
    let title: String
    let steps: [String]

    var body: some View {
        HStack(alignment: .center, spacing: 10) {
            Image(systemName: systemImage)
                .foregroundStyle(Color.accentColor)
                .frame(width: 20)
            Text(title)
                .hcbFont(.caption, weight: .semibold)
            ForEach(Array(steps.enumerated()), id: \.offset) { index, step in
                Image(systemName: "chevron.right")
                    .hcbFont(.caption2, weight: .semibold)
                    .foregroundStyle(.tertiary)
                Text(step)
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            Spacer(minLength: 0)
        }
        .padding(.vertical, 4)
        .accessibilityElement(children: .combine)
    }
}

struct SettingsOutcomeCard: View {
    let systemImage: String
    let title: String
    let detail: String

    var body: some View {
        HStack(alignment: .center, spacing: 8) {
            Image(systemName: systemImage)
                .foregroundStyle(Color.accentColor)
                .frame(width: 18)
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .hcbFont(.caption, weight: .semibold)
                    .lineLimit(1)
                Text(detail)
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 8)
        .frame(maxWidth: .infinity, minHeight: 54, alignment: .leading)
        .background(.quaternary.opacity(0.25), in: RoundedRectangle(cornerRadius: 8))
    }
}

struct SettingsSheetSection<Content: View>: View {
    private let title: String
    @ViewBuilder private let content: Content

    init(_ title: String, @ViewBuilder content: () -> Content) {
        self.title = title
        self.content = content()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(title)
                .hcbFont(.headline, weight: .semibold)
                .foregroundStyle(AppColor.ink)
            VStack(alignment: .leading, spacing: 10) {
                content
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct SettingsSheetRow<Content: View>: View {
    private let title: String
    @ViewBuilder private let content: Content

    init(_ title: String, @ViewBuilder content: () -> Content) {
        self.title = title
        self.content = content()
    }

    var body: some View {
        HStack(alignment: .top, spacing: 14) {
            Text(title)
                .hcbFont(.body, weight: .semibold)
                .foregroundStyle(title.isEmpty ? .clear : .secondary)
                .frame(width: 150, alignment: .trailing)
                .accessibilityHidden(title.isEmpty)
            content
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct SettingsSheetActions<Content: View>: View {
    let cancelTitle: String
    let onCancel: () -> Void
    @ViewBuilder let content: Content

    init(cancelTitle: String, onCancel: @escaping () -> Void, @ViewBuilder content: () -> Content) {
        self.cancelTitle = cancelTitle
        self.onCancel = onCancel
        self.content = content()
    }

    var body: some View {
        HStack(spacing: 12) {
            Spacer()
            Button(cancelTitle, action: onCancel)
                .keyboardShortcut(.cancelAction)
            content
        }
        .padding(.horizontal, 24)
        .padding(.vertical, 16)
        .background(.bar)
    }
}

struct UsedByRow: View {
    let items: [String]

    var body: some View {
        SettingsSheetRow("Used by") {
            HStack(spacing: 6) {
                ForEach(items, id: \.self) { item in
                    Text(item)
                        .hcbFont(.caption, weight: .semibold)
                        .foregroundStyle(.secondary)
                        .padding(.horizontal, 8)
                        .padding(.vertical, 3)
                        .background(Capsule().fill(.quaternary.opacity(0.45)))
                }
            }
        }
    }
}
