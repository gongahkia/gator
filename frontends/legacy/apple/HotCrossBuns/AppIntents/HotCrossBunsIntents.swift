import AppIntents
import Foundation

struct AddGoogleTaskIntent: AppIntent {
    static var title: LocalizedStringResource = "Add Google Task"
    static var description = IntentDescription("Open Hot Cross Buns to create a Google Task.")
    static var openAppWhenRun = true

    func perform() async throws -> some IntentResult & ProvidesDialog {
        AppIntentHandoff.save(.addTask)
        return .result(dialog: "Opening the task editor.")
    }
}

struct AddGoogleCalendarEventIntent: AppIntent {
    static var title: LocalizedStringResource = "Add Google Calendar Event"
    static var description = IntentDescription("Open Hot Cross Buns to create a Google Calendar event.")
    static var openAppWhenRun = true

    func perform() async throws -> some IntentResult & ProvidesDialog {
        AppIntentHandoff.save(.addEvent)
        return .result(dialog: "Opening the event editor.")
    }
}

struct OpenHotCrossBunsCalendarIntent: AppIntent {
    static var title: LocalizedStringResource = "Open Calendar in Hot Cross Buns"
    static var description = IntentDescription("Open the Calendar view in Hot Cross Buns.")
    static var openAppWhenRun = true

    func perform() async throws -> some IntentResult & ProvidesDialog {
        AppIntentHandoff.save(.calendar)
        return .result(dialog: "Opening Calendar.")
    }
}

struct OpenHotCrossBunsStoreIntent: AppIntent {
    static var title: LocalizedStringResource = "Open Store in Hot Cross Buns"
    static var description = IntentDescription("Open the Store view (tasks and notes) in Hot Cross Buns.")
    static var openAppWhenRun = true

    func perform() async throws -> some IntentResult & ProvidesDialog {
        AppIntentHandoff.save(.store)
        return .result(dialog: "Opening Store.")
    }
}

struct HotCrossBunsShortcuts: AppShortcutsProvider {
    static var shortcutTileColor: ShortcutTileColor = .orange

    static var appShortcuts: [AppShortcut] {
        AppShortcut(
            intent: AddGoogleTaskIntent(),
            phrases: [
                "Add a task in \(.applicationName)",
                "Create a Google task in \(.applicationName)"
            ],
            shortTitle: "Add Task",
            systemImageName: "checklist"
        )

        AppShortcut(
            intent: AddGoogleCalendarEventIntent(),
            phrases: [
                "Add an event in \(.applicationName)",
                "Create a Google Calendar event in \(.applicationName)"
            ],
            shortTitle: "Add Event",
            systemImageName: "calendar.badge.plus"
        )

        AppShortcut(
            intent: OpenHotCrossBunsCalendarIntent(),
            phrases: [
                "Open calendar in \(.applicationName)",
                "Show my day in \(.applicationName)"
            ],
            shortTitle: "Open Calendar",
            systemImageName: "calendar"
        )

        AppShortcut(
            intent: OpenHotCrossBunsStoreIntent(),
            phrases: [
                "Open store in \(.applicationName)",
                "Show my tasks in \(.applicationName)"
            ],
            shortTitle: "Open Store",
            systemImageName: "brain.head.profile"
        )
    }
}
