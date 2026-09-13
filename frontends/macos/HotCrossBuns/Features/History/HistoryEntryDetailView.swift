import AppKit
import SwiftUI

// Push-destination view for a single audit-log entry. Shows enough detail
// that the user can manually reconstruct the affected resource if they
// need to (lost task, sync dropped an event, mis-applied undo, etc.).
//
// Layout follows the native macOS "detail pane" idiom — Form with grouped
// sections, disclosure groups for raw data, bordered Copy buttons. No
// custom chrome beyond what AppColor tokens provide, so the window's color
// scheme flows through.
struct HistoryEntryDetailView: View {
    let entry: MutationAuditEntry

    var body: some View {
        Form {
            Section {
                LabeledContent("Action", value: friendlyAction)
                LabeledContent("When", value: entry.timestamp.formatted(date: .complete, time: .standard))
                if entry.resourceID.isEmpty == false {
                    LabeledContent("Resource ID") {
                        HStack(spacing: 6) {
                            Text(entry.resourceID)
                                .font(.body.monospaced())
                                .foregroundStyle(.secondary)
                                .textSelection(.enabled)
                                .lineLimit(1)
                                .truncationMode(.middle)
                            Button {
                                copy(entry.resourceID)
                            } label: {
                                Image(systemName: "doc.on.doc")
                            }
                            .buttonStyle(.borderless)
                            .help("Copy resource ID")
                        }
                    }
                }
            } header: {
                Text(entry.summary)
                    .font(.title3.weight(.semibold))
                    .textCase(nil) // SwiftUI Section headers uppercase by default; keep the human summary as-is
                    .textSelection(.enabled)
            } footer: {
                if entry.priorSnapshotJSON == nil && entry.postSnapshotJSON == nil {
                    Text("No snapshot was recorded for this action. Reconstruct manually from the fields above, or from the Google Tasks / Calendar web UI.")
                }
            }

            if entry.metadata.isEmpty == false {
                Section("Details") {
                    ForEach(entry.metadata.keys.sorted(), id: \.self) { key in
                        LabeledContent(metadataLabel(key), value: friendlyMetadataValue(key: key, raw: entry.metadata[key] ?? ""))
                    }
                }
            }

            if let json = entry.priorSnapshotJSON {
                snapshotSection(title: "Before this action", json: json)
            }

            if let json = entry.postSnapshotJSON {
                snapshotSection(title: "After this action", json: json)
            }
        }
        .formStyle(.grouped)
        .navigationTitle(friendlyAction)
        .navigationSubtitle(entry.timestamp.formatted(.relative(presentation: .numeric)))
    }

    // User-facing label for the raw `kind` string (e.g. "task.complete" →
    // "Completed a task"). Falls back to the summary so unknown kinds still
    // show something meaningful.
    private var friendlyAction: String {
        switch entry.kind {
        case "task.create": "Created a task"
        case "task.edit": "Edited a task"
        case "task.delete": "Deleted a task"
        case "task.complete": "Completed a task"
        case "task.reopen": "Reopened a task"
        case "task.duplicate": "Duplicated a task"
        case "task.move": "Moved a task"
        case "task.restore": "Restored a task"
        case "event.create": "Created an event"
        case "event.edit": "Edited an event"
        case "event.delete": "Deleted an event"
        case "event.dismiss": "Marked an event done"
        case "event.restore": "Restored an event"
        default:
            if entry.kind.hasPrefix("bulk.") { "Bulk action" }
            else if entry.kind.hasPrefix("sync.") { "Sync pulled changes" }
            else if entry.kind.hasPrefix("clipboard.") { "Clipboard action" }
            else { entry.summary }
        }
    }

    // Boolean / count strings rendered as native English rather than raw
    // "true" / "false" the JSON encoder produced.
    private func friendlyMetadataValue(key: String, raw: String) -> String {
        switch raw {
        case "true": return "Yes"
        case "false": return "No"
        default: return raw
        }
    }

    // Renders a snapshot. Tries to decode into TaskMirror / CalendarEventMirror
    // based on the kind prefix; falls back to raw JSON-only if decoding fails.
    // Always offers a "Copy JSON" affordance regardless of decode success.
    @ViewBuilder
    private func snapshotSection(title: String, json: String) -> some View {
        Section(title) {
            if entry.kind.hasPrefix("task.") {
                if let task = decodeTask(json) {
                    taskFields(task)
                    copyActions(json: json, markdown: taskMarkdown(task))
                } else {
                    Text("Couldn't decode snapshot. Raw JSON below.")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                    rawJSON(json)
                    copyActions(json: json, markdown: nil)
                }
            } else if entry.kind.hasPrefix("event.") {
                if let event = decodeEvent(json) {
                    eventFields(event)
                    copyActions(json: json, markdown: eventMarkdown(event))
                } else {
                    Text("Couldn't decode snapshot. Raw JSON below.")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                    rawJSON(json)
                    copyActions(json: json, markdown: nil)
                }
            } else {
                rawJSON(json)
                copyActions(json: json, markdown: nil)
            }
        }
    }

    @ViewBuilder
    private func taskFields(_ task: TaskMirror) -> some View {
        LabeledContent("Title", value: task.title.isEmpty ? "Untitled" : task.title)
        if task.notes.isEmpty == false {
            LabeledContent("Notes") {
                Text(task.notes)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
        if let due = task.dueDate {
            LabeledContent("Due", value: due.formatted(date: .complete, time: .omitted))
        } else {
            LabeledContent("Due", value: "—")
        }
        LabeledContent("Status", value: task.isCompleted ? "Completed" : "Needs action")
        LabeledContent("List ID") {
            Text(task.taskListID)
                .font(.body.monospaced())
                .foregroundStyle(.secondary)
                .textSelection(.enabled)
        }
        if let parent = task.parentID {
            LabeledContent("Parent task") {
                Text(parent)
                    .font(.body.monospaced())
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
            }
        }
        if let updated = task.updatedAt {
            LabeledContent("Last updated", value: updated.formatted(date: .abbreviated, time: .shortened))
        }
        DisclosureGroup("Raw JSON") { rawJSON(encode(task) ?? "—") }
    }

    @ViewBuilder
    private func eventFields(_ event: CalendarEventMirror) -> some View {
        LabeledContent("Summary", value: event.summary.isEmpty ? "Untitled" : event.summary)
        if event.details.isEmpty == false {
            LabeledContent("Details") {
                Text(event.details)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
        LabeledContent("Starts", value: event.startDate.formatted(date: .complete, time: event.isAllDay ? .omitted : .shortened))
        LabeledContent("Ends", value: event.endDate.formatted(date: .complete, time: event.isAllDay ? .omitted : .shortened))
        LabeledContent("All day", value: event.isAllDay ? "Yes" : "No")
        if event.location.isEmpty == false {
            LabeledContent("Location", value: event.location)
        }
        if event.recurrence.isEmpty == false {
            LabeledContent("Recurrence", value: event.recurrence.joined(separator: "\n"))
        }
        if event.reminderMinutes.isEmpty == false {
            LabeledContent("Reminders", value: event.reminderMinutes.map { "\($0)m" }.joined(separator: ", "))
        }
        if event.attendeeEmails.isEmpty == false {
            LabeledContent("Attendees", value: event.attendeeEmails.joined(separator: "\n"))
        }
        if event.meetLink.isEmpty == false {
            LabeledContent("Google Meet", value: event.meetLink)
        }
        LabeledContent("Calendar ID") {
            Text(event.calendarID)
                .font(.body.monospaced())
                .foregroundStyle(.secondary)
                .textSelection(.enabled)
        }
        DisclosureGroup("Raw JSON") { rawJSON(encode(event) ?? "—") }
    }

    @ViewBuilder
    private func copyActions(json: String, markdown: String?) -> some View {
        HStack(spacing: 8) {
            Button {
                copy(json)
            } label: {
                Label("Copy JSON", systemImage: "doc.on.clipboard")
            }
            if let md = markdown {
                Button {
                    copy(md)
                } label: {
                    Label("Copy as Markdown", systemImage: "doc.richtext")
                }
            }
        }
        .buttonStyle(.bordered)
        .controlSize(.small)
    }

    @ViewBuilder
    private func rawJSON(_ json: String) -> some View {
        // ScrollView + read-only TextEditor would be ideal but a TextEditor
        // inside Form+grouped misbehaves on macOS 14. Plain Text + selection
        // is the reliable native path and keeps the form rhythm.
        Text(json)
            .font(.caption.monospaced())
            .foregroundStyle(.secondary)
            .textSelection(.enabled)
            .frame(maxWidth: .infinity, alignment: .leading)
    }

    // MARK: - Decode / encode helpers

    private static let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .iso8601
        return d
    }()

    private static let encoder: JSONEncoder = {
        let e = JSONEncoder()
        e.outputFormatting = [.prettyPrinted, .sortedKeys]
        e.dateEncodingStrategy = .iso8601
        return e
    }()

    private func decodeTask(_ json: String) -> TaskMirror? {
        guard let data = json.data(using: .utf8) else { return nil }
        return try? Self.decoder.decode(TaskMirror.self, from: data)
    }

    private func decodeEvent(_ json: String) -> CalendarEventMirror? {
        guard let data = json.data(using: .utf8) else { return nil }
        return try? Self.decoder.decode(CalendarEventMirror.self, from: data)
    }

    private func encode<T: Encodable>(_ value: T) -> String? {
        guard let data = try? Self.encoder.encode(value) else { return nil }
        return String(data: data, encoding: .utf8)
    }

    private func taskMarkdown(_ task: TaskMirror) -> String {
        var out = "# \(task.title.isEmpty ? "Untitled" : task.title)\n\n"
        if task.notes.isEmpty == false { out += "\(task.notes)\n\n" }
        if let due = task.dueDate {
            out += "- Due: \(due.formatted(date: .complete, time: .omitted))\n"
        }
        out += "- Status: \(task.isCompleted ? "Completed" : "Needs action")\n"
        out += "- List ID: `\(task.taskListID)`\n"
        if let parent = task.parentID { out += "- Parent: `\(parent)`\n" }
        return out
    }

    private func eventMarkdown(_ event: CalendarEventMirror) -> String {
        var out = "# \(event.summary.isEmpty ? "Untitled" : event.summary)\n\n"
        if event.details.isEmpty == false { out += "\(event.details)\n\n" }
        out += "- Starts: \(event.startDate.formatted(date: .complete, time: event.isAllDay ? .omitted : .shortened))\n"
        out += "- Ends: \(event.endDate.formatted(date: .complete, time: event.isAllDay ? .omitted : .shortened))\n"
        out += "- All day: \(event.isAllDay ? "Yes" : "No")\n"
        if event.location.isEmpty == false { out += "- Location: \(event.location)\n" }
        if event.recurrence.isEmpty == false { out += "- Recurrence: \(event.recurrence.joined(separator: ", "))\n" }
        if event.reminderMinutes.isEmpty == false { out += "- Reminders: \(event.reminderMinutes.map { "\($0)m" }.joined(separator: ", "))\n" }
        if event.attendeeEmails.isEmpty == false { out += "- Attendees: \(event.attendeeEmails.joined(separator: ", "))\n" }
        if event.meetLink.isEmpty == false { out += "- Meet: \(event.meetLink)\n" }
        out += "- Calendar ID: `\(event.calendarID)`\n"
        return out
    }

    private func metadataLabel(_ key: String) -> String {
        switch key {
        case "list": "List ID"
        case "calendar": "Calendar ID"
        case "fromListID": "From list ID"
        case "toListID": "To list ID"
        case "fromListTitle": "From list"
        case "toListTitle": "To list"
        case "sourceTitle": "Source title"
        case "priorCompleted": "Was completed"
        case "count": "Count"
        default: key
        }
    }

    private func copy(_ text: String) {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(text, forType: .string)
    }
}
