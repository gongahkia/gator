import Foundation

enum SettingsSearchTab: String, CaseIterable, Identifiable, Sendable {
    case general, profile, appearance, hotkeys, alerts, advanced, about

    var id: String { rawValue }

    var title: String {
        switch self {
        case .general: "General"
        case .profile: "Profile"
        case .appearance: "Appearance"
        case .hotkeys: "Hotkeys"
        case .alerts: "Alerts"
        case .advanced: "Advanced"
        case .about: "About"
        }
    }

    var systemImage: String {
        switch self {
        case .general: "gearshape"
        case .profile: "person.crop.circle"
        case .appearance: "paintbrush"
        case .hotkeys: "keyboard"
        case .alerts: "bell"
        case .advanced: "gearshape.2"
        case .about: "info.circle"
        }
    }
}

enum SettingsSectionAnchor: String, Sendable {
    case profileOAuth
    case profileAccounts
    case language
    case sync
    case openAtLogin
    case diagnostics
    case agentAccess
    case appearance
    case background
    case layout
    case hotkeys
    case notifications
    case menuBar
    case advancedCalendars
    case taskLists
    case perTabFilters
    case data
    case backups
    case encryption
    case history
    case customFilters
    case templates
    case updates
    case version
}

struct SettingsSearchResult: Identifiable, Equatable, Sendable {
    let id: String
    let tab: SettingsSearchTab
    let anchor: SettingsSectionAnchor
    let title: String
    let keywords: [String]
    var status: String?

    init(
        tab: SettingsSearchTab,
        anchor: SettingsSectionAnchor,
        title: String,
        keywords: [String],
        status: String? = nil
    ) {
        self.id = "\(tab.rawValue).\(anchor.rawValue).\(title)"
        self.tab = tab
        self.anchor = anchor
        self.title = title
        self.keywords = keywords
        self.status = status
    }
}

enum SettingsSearchIndex {
    static func results(
        customShortcutCount: Int = 0,
        shortcutConflictCount: Int = 0,
        customFilterCount: Int = 0,
        taskTemplateCount: Int = 0,
        eventTemplateCount: Int = 0,
        updateStatus: String? = nil
    ) -> [SettingsSearchResult] {
        [
            .init(tab: .profile, anchor: .profileOAuth, title: "Google OAuth client", keywords: ["google", "oauth", "client", "api", "profile", "identity"]),
            .init(tab: .profile, anchor: .profileAccounts, title: "Google account", keywords: ["connect", "disconnect", "account", "profile", "google", "identity", "provider", "sign in", "add account"]),
            .init(tab: .general, anchor: .language, title: "Language", keywords: ["locale", "translation", "onboarding", "system default"]),
            .init(tab: .general, anchor: .sync, title: "Sync", keywords: ["refresh", "resync", "background", "retention"]),
            .init(tab: .general, anchor: .openAtLogin, title: "Open at login", keywords: ["launch", "startup", "background"]),
            .init(tab: .general, anchor: .diagnostics, title: "Diagnostics", keywords: ["logs", "history", "support", "debug"]),
            .init(tab: .general, anchor: .agentAccess, title: "Agent access", keywords: ["mcp", "agent", "ai", "local server", "token", "permission", "openclaw"]),
            .init(tab: .appearance, anchor: .appearance, title: "Appearance", keywords: ["theme", "color", "font", "text", "animation", "motion", "reduce motion", "disable animations"]),
            .init(tab: .appearance, anchor: .background, title: "Background", keywords: ["translucency", "opacity", "image", "wallpaper"]),
            .init(tab: .appearance, anchor: .layout, title: "Layout", keywords: ["sidebar", "navigation", "placement", "tabs", "left", "right", "top", "bottom", "scale", "calendar"]),
            .init(tab: .hotkeys, anchor: .hotkeys, title: "Hotkeys", keywords: ["shortcut", "keyboard", "command", "conflict"], status: hotkeyStatus(customShortcutCount, shortcutConflictCount)),
            .init(tab: .alerts, anchor: .notifications, title: "Notifications", keywords: ["reminder", "alert", "bell", "dock"]),
            .init(tab: .alerts, anchor: .menuBar, title: "Menu bar", keywords: ["menu bar", "badge", "extra", "status item"]),
            .init(tab: .advanced, anchor: .advancedCalendars, title: "Calendars", keywords: ["calendar", "completed", "selection"]),
            .init(tab: .advanced, anchor: .taskLists, title: "Task lists", keywords: ["tasks", "lists", "selection"]),
            .init(tab: .advanced, anchor: .perTabFilters, title: "Per-tab filters", keywords: ["filter", "tasks", "notes", "tab"]),
            .init(tab: .advanced, anchor: .data, title: "Data controls", keywords: ["export", "import", "reset", "cache"]),
            .init(tab: .advanced, anchor: .backups, title: "Local backups", keywords: ["backup", "restore", "daily"]),
            .init(tab: .advanced, anchor: .encryption, title: "Encryption", keywords: ["security", "cache", "history", "passphrase"]),
            .init(tab: .advanced, anchor: .history, title: "History", keywords: ["audit", "mutation", "diagnostics"], status: nil),
            .init(tab: .advanced, anchor: .customFilters, title: "Custom filters", keywords: ["filter", "dsl", "query", "menu bar"], status: "\(customFilterCount)"),
            .init(tab: .advanced, anchor: .templates, title: "Templates", keywords: ["template", "task template", "event template", "prompt"], status: "\(taskTemplateCount + eventTemplateCount)"),
            .init(tab: .about, anchor: .updates, title: "Updates", keywords: ["update", "github", "release", "download"], status: updateStatus),
            .init(tab: .about, anchor: .version, title: "Version", keywords: ["about", "version", "build"])
        ]
    }

    static func filter(_ results: [SettingsSearchResult], query: String, limit: Int = 8) -> [SettingsSearchResult] {
        let tokens = query
            .lowercased()
            .split(whereSeparator: { $0.isWhitespace })
            .map(String.init)
        guard tokens.isEmpty == false else { return [] }
        return results
            .map { result -> (SettingsSearchResult, Int)? in
                let haystack = ([result.title, result.tab.title, result.status ?? ""] + result.keywords)
                    .joined(separator: "\n")
                    .lowercased()
                guard tokens.allSatisfy({ haystack.contains($0) }) else { return nil }
                let score = tokens.reduce(0) { partial, token in
                    partial
                        + (result.title.lowercased().contains(token) ? 4 : 0)
                        + (result.keywords.contains(where: { $0.lowercased().contains(token) }) ? 2 : 0)
                }
                return (result, score)
            }
            .compactMap { $0 }
            .sorted {
                if $0.1 == $1.1 { return $0.0.title < $1.0.title }
                return $0.1 > $1.1
            }
            .prefix(limit)
            .map(\.0)
    }

    private static func hotkeyStatus(_ custom: Int, _ conflicts: Int) -> String? {
        if conflicts > 0 { return "\(conflicts) conflict\(conflicts == 1 ? "" : "s")" }
        if custom > 0 { return "\(custom) custom" }
        return nil
    }
}
