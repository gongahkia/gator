import Foundation

// Orchestrates conversions between Calendar events, Google Tasks with a
// due date (= "task" in HCB), and Google Tasks without a due date (=
// "note" in HCB). Every conversion is two Google API calls (insert +
// delete) except task↔note, which is a single update on the same row
// since notes and tasks share the same Google resource.
//
// Directions supported:
//   event → task, event → note
//   task → event, task → note
//   note → task, note → event
//
// Conversions that span resource types (event ↔ task, event ↔ note) are
// done create-first, delete-second. If create fails the source stays
// (safe — nothing destroyed). If delete fails after create the user
// sees a temporary duplicate on Google, not data loss.

// Pure mapping helpers. Extracted so unit tests can exercise them
// without a live AppModel.
enum ConversionMapper {
    // Event → Task/Note title + notes. Location folded into the notes
    // since Google Tasks has no location field.
    static func taskNotes(fromEvent event: CalendarEventMirror) -> String {
        var parts: [String] = []
        let trimmedDetails = event.details.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmedDetails.isEmpty == false {
            parts.append(trimmedDetails)
        }
        let trimmedLocation = event.location.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmedLocation.isEmpty == false {
            parts.append("Location: \(trimmedLocation)")
        }
        return parts.joined(separator: "\n\n")
    }

    // Event → Task due date: drop time-of-day so the due date lands on
    // the event's start day in the user's calendar. `calendar` is
    // defaulted to .current at call sites.
    static func taskDueDate(fromEvent event: CalendarEventMirror, calendar: Calendar = .current) -> Date {
        calendar.startOfDay(for: event.startDate)
    }

    // Task → Event end date default: 1 hour after start, or start-of-
    // next-day for all-day events.
    static func eventEnd(fromTaskStart start: Date, isAllDay: Bool, calendar: Calendar = .current) -> Date {
        if isAllDay {
            return calendar.date(byAdding: .day, value: 1, to: calendar.startOfDay(for: start)) ?? start
        }
        return calendar.date(byAdding: .hour, value: 1, to: start) ?? start
    }

    // Fields the user loses when converting an event into a task/note.
    // Used by the confirmation sheet. Not every event has every field;
    // only populated ones are surfaced.
    static func lostFieldsForEventToTask(_ event: CalendarEventMirror, preserveDue: Bool) -> [String] {
        var lost: [String] = []
        if event.isAllDay == false { lost.append("Start/end times (kept as date only)") }
        if preserveDue == false { lost.append("Due date") }
        if event.attendeeEmails.isEmpty == false { lost.append("Attendees (\(event.attendeeEmails.count))") }
        if event.recurrence.isEmpty == false { lost.append("Recurrence rule (this instance only)") }
        if (event.colorId ?? "").isEmpty == false { lost.append("Color") }
        if event.reminderMinutes.isEmpty == false { lost.append("Reminders") }
        if event.meetLink.isEmpty == false { lost.append("Google Meet link") }
        lost.append("Source calendar (\"\(event.calendarID)\")")
        return lost
    }

    static func lostFieldsForTaskToEvent(_ task: TaskMirror, hasDueDate: Bool) -> [String] {
        var lost: [String] = []
        if task.parentID != nil { lost.append("Subtask parent link") }
        lost.append("Source task list")
        if hasDueDate == false { lost.append("No due date supplied; event needs start time") }
        return lost
    }
}

// Coordinator. Holds an unowned reference to AppModel so the service
// can drive the existing create/update/delete paths (which take care
// of optimistic writes, audit log, and offline queue).
@MainActor
final class ConversionService {
    private unowned let model: AppModel

    init(model: AppModel) {
        self.model = model
    }

    // MARK: - Event → Task / Note

    func convertEvent(
        _ event: CalendarEventMirror,
        toTaskListID listID: TaskListMirror.ID,
        keepDueDate: Bool
    ) async -> ConversionResult {
        let title = event.summary.trimmingCharacters(in: .whitespacesAndNewlines)
        guard title.isEmpty == false else {
            return .failure("Event has no title — fill in a summary before converting.")
        }
        let notes = ConversionMapper.taskNotes(fromEvent: event)
        let due = keepDueDate ? ConversionMapper.taskDueDate(fromEvent: event) : nil

        let created = await model.createTask(
            title: title,
            notes: notes,
            dueDate: due,
            taskListID: listID
        )
        guard created else {
            return .failure(model.lastMutationError ?? "Couldn't create the task on Google.")
        }
        let deleted = await model.deleteEvent(event, scope: .thisOccurrence)
        if deleted == false {
            return .partial("Task was created but the source event couldn't be deleted. Delete it manually on Google Calendar to avoid a duplicate.")
        }
        return .success
    }

    // MARK: - Task / Note → Event

    func convertTaskToEvent(
        _ task: TaskMirror,
        calendarID: CalendarListMirror.ID,
        startDate: Date,
        endDate: Date,
        isAllDay: Bool
    ) async -> ConversionResult {
        let summary = task.title.trimmingCharacters(in: .whitespacesAndNewlines)
        guard summary.isEmpty == false else {
            return .failure("Task has no title — fill one in before converting.")
        }
        let created = await model.createEvent(
            summary: summary,
            details: task.notes,
            startDate: startDate,
            endDate: endDate,
            isAllDay: isAllDay,
            reminderMinutes: nil,
            calendarID: calendarID
        )
        guard created else {
            return .failure(model.lastMutationError ?? "Couldn't create the event on Google.")
        }
        let deleted = await model.deleteTask(task)
        if deleted == false {
            return .partial("Event was created but the source task couldn't be deleted. Delete it manually in Google Tasks.")
        }
        return .success
    }

    // MARK: - Task ↔ Note (single update on the same row)

    func convertTaskToNote(_ task: TaskMirror) async -> ConversionResult {
        let ok = await model.updateTask(
            task,
            title: task.title,
            notes: task.notes,
            dueDate: nil
        )
        return ok ? .success : .failure(model.lastMutationError ?? "Couldn't clear the due date.")
    }

    func convertNoteToTask(_ note: TaskMirror, dueDate: Date) async -> ConversionResult {
        let ok = await model.updateTask(
            note,
            title: note.title,
            notes: note.notes,
            dueDate: dueDate
        )
        return ok ? .success : .failure(model.lastMutationError ?? "Couldn't set the due date.")
    }
}

enum ConversionResult: Equatable {
    case success
    case partial(String) // create worked, delete didn't — non-destructive but surfaces a duplicate
    case failure(String)

    var isSuccess: Bool {
        if case .success = self { return true }
        return false
    }

    var userFacingMessage: String? {
        switch self {
        case .success: nil
        case .partial(let msg): msg
        case .failure(let msg): msg
        }
    }
}
