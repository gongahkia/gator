import AppKit
import Foundation

struct DiagnosticBundleEnvironment {
    var now: @Sendable () -> Date
    var bundle: Bundle
    var loadPersistedLog: @Sendable () -> String
    var readLastCrash: @Sendable () -> String?
    var logDirectoryPath: @Sendable () -> String? = { nil }

    static let live = DiagnosticBundleEnvironment(
        now: { Date() },
        bundle: .main,
        loadPersistedLog: { AppLogger.shared.loadPersistedLog() },
        readLastCrash: { CrashReporter.readLastCrash() },
        logDirectoryPath: { AppLogger.shared.logDirectoryURL()?.path }
    )
}

// Single-file diagnostic dump the user can save and share when
// reporting bugs. Concatenates the recent log, pending-mutation queue,
// settings snapshot, last breadcrumb crash, and cache metadata into
// one plain-text blob. No zip — sandboxed apps can't invoke /usr/bin/
// zip reliably, and a single .txt covers the issue-report use case
// without extraction friction.
//
// Redacts personal payload fields, email addresses, and OAuth-looking
// access tokens so the bundle can be pasted into a support issue safely.
enum DiagnosticBundle {
    @MainActor
    static func build(
        model: AppModel,
        cachePath: String,
        notificationSummary: NotificationScheduleSummary?,
        environment: DiagnosticBundleEnvironment = .live
    ) -> String {
        var sections: [String] = []
        sections.append("=== Hot Cross Buns Diagnostic Bundle ===")
        sections.append("Generated: \(ISO8601DateFormatter.diagnostic.string(from: environment.now()))")
        let version = environment.bundle.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "?"
        let build = environment.bundle.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "?"
        sections.append("Version: \(version) (build \(build))")
        sections.append("")
        sections.append(summarySection(
            model: model,
            cachePath: cachePath,
            notificationSummary: notificationSummary,
            logDirectoryPath: environment.logDirectoryPath()
        ))
        sections.append("")
        sections.append("=== Pending Mutations (\(model.pendingMutations.count)) ===")
        if model.pendingMutations.isEmpty {
            sections.append("none")
        } else {
            for mutation in model.pendingMutations {
                sections.append("\(mutation.createdAt) \(mutation.resourceType.rawValue)/\(mutation.action.rawValue) resource=\(mutation.resourceID)")
            }
        }
        sections.append("")
        sections.append("=== Recent Logs (info+, all rotations) ===")
        let logFile = environment.loadPersistedLog()
        sections.append(logFile.isEmpty ? "(no persisted entries yet)" : logFile)
        sections.append("")
        if let lastCrash = environment.readLastCrash() {
            sections.append("=== Last Crash Breadcrumb ===")
            sections.append(lastCrash)
            sections.append("")
        }
        return redact(sections.joined(separator: "\n"))
    }

    // Presents NSSavePanel and writes the bundle to the chosen path.
    // Falls back to writing into the user's Downloads directory if the
    // save panel is cancelled.
    @MainActor
    static func exportToDisk(model: AppModel, cachePath: String, notificationSummary: NotificationScheduleSummary?) async -> URL? {
        let contents = build(model: model, cachePath: cachePath, notificationSummary: notificationSummary)
        let defaultName = "hot-cross-buns-diagnostics-\(Self.filenameTimestamp()).txt"
        let panel = NSSavePanel()
        panel.nameFieldStringValue = defaultName
        panel.allowedContentTypes = [.plainText]
        panel.canCreateDirectories = true
        let response: NSApplication.ModalResponse
        if let keyWindow = NSApp.keyWindow {
            response = await panel.beginSheetModal(for: keyWindow)
        } else {
            response = panel.runModal()
        }
        guard response == .OK, let url = panel.url else { return nil }
        do {
            try contents.data(using: .utf8)?.write(to: url, options: [.atomic])
            return url
        } catch {
            AppLogger.error("diagnostic bundle export failed", category: .ui, metadata: ["error": String(describing: error)])
            return nil
        }
    }

    @MainActor
    private static func summarySection(
        model: AppModel,
        cachePath: String,
        notificationSummary: NotificationScheduleSummary?,
        logDirectoryPath: String?
    ) -> String {
        var lines: [String] = ["=== Summary ==="]
        lines.append("Auth state: \(model.authState.title)")
        if let account = model.account {
            lines.append("Account email: \(redactEmail(account.email))")
        }
        lines.append("Sync mode: \(model.settings.syncMode.title)")
        lines.append("Sync state: \(model.syncState.title)")
        lines.append("Sync paused: \(model.isSyncPaused)")
        lines.append("Task lists: \(model.taskLists.count)")
        lines.append("Tasks: \(model.tasks.count)")
        lines.append("Calendars: \(model.calendars.count)")
        lines.append("Events: \(model.events.count)")
        lines.append("Sync checkpoints: \(model.syncCheckpoints.count)")
        lines.append("Pending mutations: \(model.pendingMutations.count)")
        lines.append("Cache path: \(cachePath)")
        lines.append("Onboarding complete: \(model.settings.hasCompletedOnboarding)")
        lines.append("Local reminders: \(model.settings.enableLocalNotifications)")
        lines.append("Field-redacted Google payload logs: \(model.settings.rawGoogleDiagnosticsEnabled ? "Enabled" : "Disabled")")
        lines.append("Log folder: \(logDirectoryPath ?? "Unavailable")")
        if let summary = notificationSummary {
            lines.append("Reminders scheduled: events=\(summary.scheduledEvents) tasks=\(summary.scheduledTasks) deferred_events=\(summary.deferredEvents) deferred_tasks=\(summary.deferredTasks) window=\(summary.windowDays)d")
        }
        lines.append("Menu bar extra: \(model.settings.showMenuBarExtra)")
        lines.append("Dock badge: \(model.settings.showDockBadge)")
        return lines.joined(separator: "\n")
    }

    private static func filenameTimestamp() -> String {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyyMMdd-HHmmss"
        return f.string(from: Date())
    }

    private static func redact(_ blob: String) -> String {
        DiagnosticPrivacyRedactor.redactDiagnosticText(blob)
    }

    private static func redactEmail(_ email: String) -> String {
        guard let at = email.firstIndex(of: "@") else { return "<redacted>" }
        let local = email[..<at]
        let domain = email[email.index(after: at)...]
        let prefix = local.prefix(2)
        return "\(prefix)***@\(domain)"
    }
}

private extension ISO8601DateFormatter {
    static let diagnostic: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()
}
