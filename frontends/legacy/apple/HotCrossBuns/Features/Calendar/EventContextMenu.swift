import AppKit
import SwiftUI
import UniformTypeIdentifiers

struct EventContextMenu: View {
    @Environment(AppModel.self) private var model

    let event: CalendarEventMirror
    var onOpen: (() -> Void)? = nil
    var onConvertToTask: (() -> Void)? = nil
    var onConvertToNote: (() -> Void)? = nil
    var onDelete: (() -> Void)? = nil

    private var calendarStore: CalendarStore { model.calendarStore }

    private var currentEvent: CalendarEventMirror {
        calendarStore.event(id: event.id) ?? event
    }

    private var movableCalendars: [CalendarListMirror] {
        let event = currentEvent
        return calendarStore.calendars.filter {
            $0.id != event.calendarID && ($0.accessRole == "owner" || $0.accessRole == "writer")
        }
    }

    private var calendarTitle: String? {
        calendarStore.calendars.first(where: { $0.id == currentEvent.calendarID })?.summary
    }

    var body: some View {
        let event = currentEvent
        if let onOpen {
            Button("Open…", action: onOpen)
        }

        Button("Mark as Done") {
            Task { _ = await model.dismissEvent(event) }
        }

        Menu("Duplicate…") {
            Button("Duplicate Here") {
                Task { _ = await model.duplicateEvent(event) }
            }
            Button("Duplicate to Tomorrow") {
                Task { _ = await model.duplicateEvent(event, offsetDays: 1) }
            }
            Button("Duplicate to Next Week") {
                Task { _ = await model.duplicateEvent(event, offsetDays: 7) }
            }
            Button("Duplicate in 2 Weeks") {
                Task { _ = await model.duplicateEvent(event, offsetDays: 14) }
            }
            Button("Duplicate to Next Month") {
                Task { _ = await model.duplicateEvent(event, offsetDays: 30) }
            }
        }

        Menu("Move to Calendar…") {
            if movableCalendars.isEmpty {
                Button("No other writable calendars") {}
                    .disabled(true)
            } else {
                ForEach(movableCalendars) { calendar in
                    Button(calendar.summary) {
                        Task { await move(to: calendar.id) }
                    }
                }
            }
        }

        if onConvertToTask != nil || onConvertToNote != nil {
            Menu("Convert…") {
                if let onConvertToTask {
                    Button("Convert to Task…", action: onConvertToTask)
                }
                if let onConvertToNote {
                    Button("Convert to Note…", action: onConvertToNote)
                }
            }
        }

        Menu("Share…") {
            let eventURL = HCBDeepLinkBuilder.eventURL(for: event)
            Button("Copy Link") {
                copyToPasteboard(eventURL.absoluteString)
                postCopyToast("Event link copied to clipboard.")
            }
            ShareLink(item: eventURL) {
                Text("Share Link…")
            }
            if let googleEventURL {
                Button("Copy Google Calendar Link") {
                    copyToPasteboard(googleEventURL.absoluteString)
                    postCopyToast("Google Calendar link copied to clipboard.")
                }
            }
            Button("Copy as Markdown") {
                let markdown = EventMarkdownExporter.markdown(for: event, calendarTitle: calendarTitle)
                copyToPasteboard(markdown)
                postCopyToast("Event Markdown copied to clipboard.")
            }
            Button("Export .ics…") {
                exportICS()
            }
        }

        if let onDelete {
            Divider()
            Button("Delete", role: .destructive, action: onDelete)
        }
    }

    private var googleEventURL: URL? {
        guard let htmlLink = currentEvent.htmlLink else { return nil }
        return URL(string: htmlLink)
    }

    private func copyToPasteboard(_ value: String) {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(value, forType: .string)
    }

    private func postCopyToast(_ message: String) {
        NotificationCenter.default.post(name: .hcbClipboardMessage, object: message)
    }

    private func move(to calendarID: CalendarListMirror.ID) async {
        let event = currentEvent
        _ = await model.updateEvent(
            event,
            summary: event.summary,
            details: event.details,
            startDate: event.startDate,
            endDate: event.endDate,
            isAllDay: event.isAllDay,
            reminderMinutes: event.reminderMinutes.first,
            calendarID: calendarID,
            location: event.location,
            recurrence: event.recurrence,
            attendeeEmails: event.attendeeEmails,
            scope: .thisOccurrence,
            colorId: event.colorId,
            hcbTaskID: event.hcbTaskID
        )
    }

    private func exportICS() {
        let event = currentEvent
        let content = EventICSExporter.ics(for: event)
        let panel = NSSavePanel()
        let sanitized = event.summary
            .replacingOccurrences(of: "/", with: "-")
            .trimmingCharacters(in: .whitespacesAndNewlines)
        let base = sanitized.isEmpty ? "event" : sanitized
        panel.nameFieldStringValue = "\(base).ics"
        panel.allowedContentTypes = [UTType(filenameExtension: "ics") ?? .data]
        panel.canCreateDirectories = true
        if panel.runModal() == .OK, let url = panel.url {
            try? content.data(using: .utf8)?.write(to: url)
        }
    }
}
