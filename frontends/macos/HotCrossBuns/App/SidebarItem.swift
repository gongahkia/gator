import SwiftUI

// Three top-level sidebar tabs. `.store` is the rawValue for the Tasks tab
// for back-compat with persisted SceneStorage / hiddenSidebarItems values
// that shipped under the old "Store" name; display string is "Tasks".
enum SidebarItem: String, CaseIterable, Identifiable, Hashable {
    case calendar
    case store // displays as "Tasks"; raw kept for persistence back-compat
    case notes

    var id: String { rawValue }

    var title: String {
        switch self {
        case .calendar: "Calendar"
        case .store: "Tasks"
        case .notes: "Notes"
        }
    }

    var systemImage: String {
        switch self {
        case .calendar: "calendar"
        case .store: "checklist"
        case .notes: "note.text"
        }
    }

    var keyboardEquivalent: KeyEquivalent? {
        switch self {
        case .calendar: "1"
        case .store: "2"
        case .notes: "3"
        }
    }

    var shortcutCommand: HCBShortcutCommand {
        switch self {
        case .calendar: .goToCalendar
        case .store: .goToStore
        case .notes: .goToNotes
        }
    }

    // Every tab is user-hideable. The Layout section enforces that at least
    // one remains visible.
    var isHideable: Bool {
        switch self {
        case .calendar, .store, .notes: true
        }
    }

    @MainActor
    func navigationHelp(shortcutOverrides: [String: HCBKeyBinding] = [:]) -> String {
        let binding = hcbEffectiveBinding(shortcutCommand, overrides: shortcutOverrides)
        return "Jump to \(title) (\(binding.displayLabel))"
    }

    @MainActor
    @ViewBuilder
    func makeContentView(router: RouterPath) -> some View {
        // router injected via custom EnvironmentKey (non-observing). Consumers
        // call router.present(...) but never display router state, so they
        // shouldn't subscribe to its publishes. See RouterPath.swift for the
        // RouterPathKey rationale.
        switch self {
        case .calendar:
            CalendarHomeView().environment(\.routerPath, router)
        case .store:
            StoreView().environment(\.routerPath, router)
        case .notes:
            NotesView().environment(\.routerPath, router)
        }
    }
}

extension Notification.Name {
    // Posted when any part of the app wants to switch the sidebar to the
    // Tasks tab. MacSidebarShell observes this and updates selection.
    static let hcbOpenStoreTab = Notification.Name("hcb.open.store.tab")
    static let hcbOpenNotesTab = Notification.Name("hcb.open.notes.tab")
    // Despite the historical name, this now routes to Tasks vs Notes based
    // on whether the task currently has a due date.
    static let hcbRevealTaskInStore = Notification.Name("hcb.reveal.task.in.store")
    static let hcbRevealEventInCalendar = Notification.Name("hcb.reveal.event.in.calendar")
    static let hcbOpenShareAvailability = Notification.Name("hcb.open.share.availability")
    static let hcbClipboardMessage = Notification.Name("hcb.clipboard.message")
    // Posted by the "Settings…" menu command + ⌘,; AppDelegate opens the
    // detached Settings window in response.
    static let hcbOpenSettingsWindow = Notification.Name("hcb.open.settings.window")
}
