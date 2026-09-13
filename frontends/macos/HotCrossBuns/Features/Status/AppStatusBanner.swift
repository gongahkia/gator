import SwiftUI

struct AppStatusBanner: View {
    let syncState: SyncState
    let authState: AuthState
    let mutationError: String?
    var cacheWarning: String? = nil
    var isSyncPaused: Bool = false
    var quarantinedCount: Int = 0
    var invalidPayloadCount: Int = 0
    var conflictCount: Int = 0
    var deferredReminderSummary: NotificationScheduleSummary? = nil
    var syncFailureKind: SyncFailureKind? = nil
    var networkReachability: NetworkReachability = .unknown
    // Days elapsed since the previous launch. Nil on first launch / same-day
    // relaunch. Used to render an "N days since last open — fetching" row
    // during .syncing so a long-absence cold launch isn't a silent freeze.
    var daysSinceLastLaunch: Int? = nil
    var syncScope: SyncScopeSummary? = nil
    var openSyncIssues: (() -> Void)? = nil
    let retry: () -> Void
    let dismiss: () -> Void

    var body: some View {
        if let failure = failureContext {
            HStack(alignment: .top, spacing: 12) {
                Image(systemName: failure.systemImage)
                    .hcbFont(.title3)
                    .foregroundStyle(failure.tint)
                VStack(alignment: .leading, spacing: 4) {
                    Text(failure.title)
                        .hcbFont(.headline)
                    Text(failure.message)
                        .hcbFont(.footnote)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                Spacer(minLength: 8)
                if let action = failure.action {
                    Button(failure.actionLabel, action: action)
                        .buttonStyle(.bordered)
                } else if failure.canRetry {
                    Button("Retry", action: retry)
                        .buttonStyle(.bordered)
                }
                if failure.canDismiss {
                    Button(action: dismiss) {
                        Image(systemName: "xmark")
                    }
                    .buttonStyle(.borderless)
                    .accessibilityLabel("Dismiss status message")
                }
            }
            .hcbScaledPadding(14)
            .background(.thinMaterial, in: RoundedRectangle(cornerRadius: 12))
            .overlay(
                RoundedRectangle(cornerRadius: 12)
                    .stroke(failure.tint.opacity(0.35), lineWidth: 1)
            )
            .hcbScaledPadding(.horizontal, 14)
            .hcbScaledPadding(.top, 8)
        } else if let info = infoContext {
            HStack(alignment: .center, spacing: 12) {
                ProgressView()
                    .controlSize(.small)
                Text(info.title)
                    .hcbFont(.subheadline)
                    .foregroundStyle(.secondary)
                Spacer(minLength: 8)
                Button(action: dismiss) {
                    Image(systemName: "xmark")
                }
                .buttonStyle(.borderless)
                .accessibilityLabel("Dismiss status message")
            }
            .hcbScaledPadding(.vertical, 10)
            .hcbScaledPadding(.horizontal, 14)
            .background(.thinMaterial, in: RoundedRectangle(cornerRadius: 10))
            .hcbScaledPadding(.horizontal, 14)
            .hcbScaledPadding(.top, 8)
        }
    }

    private var infoContext: InfoContext? {
        guard case .syncing = syncState else { return nil }
        guard let title = Self.syncingInfoTitle(daysSinceLastLaunch: daysSinceLastLaunch, syncScope: syncScope) else { return nil }
        return InfoContext(title: title)
    }

    static func syncingInfoTitle(daysSinceLastLaunch: Int?, syncScope: SyncScopeSummary?) -> String? {
        guard let days = daysSinceLastLaunch, days >= 1 else { return nil }
        let suffix = days == 1 ? "day" : "days"
        if let scope = syncScope, scope.hasScope {
            return "\(days) \(suffix) since last open — fetching roughly \(scope.copy) from Google…"
        }
        return "\(days) \(suffix) since last open — fetching tasks and events from Google…"
    }

    private var failureContext: FailureContext? {
        if case .failed(let message) = authState {
            return FailureContext(
                title: "Reconnect Google to keep syncing",
                message: message,
                systemImage: "person.crop.circle.badge.exclamationmark",
                tint: .red,
                canRetry: false
            )
        }

        if conflictCount > 0 {
            let noun = conflictCount == 1 ? "conflict" : "conflicts"
            return FailureContext(
                title: "\(conflictCount) sync \(noun) — whose change wins?",
                message: "Google rejected these writes because someone edited the same item elsewhere. Review them in Sync Issues to keep your change or take the server version.",
                systemImage: "arrow.triangle.branch",
                tint: .red,
                canRetry: false,
                actionLabel: "Review",
                action: openSyncIssues,
                canDismiss: false
            )
        }

        if invalidPayloadCount > 0 {
            let noun = invalidPayloadCount == 1 ? "queued write has" : "queued writes have"
            return FailureContext(
                title: "\(invalidPayloadCount) \(noun) invalid data",
                message: "Google rejected these payloads as malformed. Review them in Sync Issues, copy the payloads if you need to diagnose the bug, then retry after fixing the source data.",
                systemImage: "doc.badge.exclamationmark",
                tint: .red,
                canRetry: false,
                actionLabel: "Review",
                action: openSyncIssues,
                canDismiss: false
            )
        }

        if quarantinedCount > 0 {
            let noun = quarantinedCount == 1 ? "change" : "changes"
            return FailureContext(
                title: "\(quarantinedCount) \(noun) need your attention",
                message: "Google rejected these writes after several retries. Review them in Sync Issues to retry or discard them.",
                systemImage: "exclamationmark.octagon",
                tint: .red,
                canRetry: false,
                actionLabel: "Review",
                action: openSyncIssues,
                canDismiss: false
            )
        }

        if let mutationError, mutationError.isEmpty == false {
            return FailureContext(
                title: "Last change didn't save",
                message: mutationError,
                systemImage: "exclamationmark.triangle",
                tint: AppColor.ember,
                canRetry: false,
                canDismiss: true
            )
        }

        if let cacheWarning, cacheWarning.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
            return FailureContext(
                title: Self.cacheWarningTitle(for: cacheWarning),
                message: cacheWarning,
                systemImage: "externaldrive.badge.exclamationmark",
                tint: AppColor.ember,
                canRetry: false,
                canDismiss: true
            )
        }

        if isSyncPaused {
            let copy = Self.syncFailureCopy(
                fallbackMessage: "Google wasn't reachable after several attempts. Your local changes are safe and will sync when you retry.",
                isPaused: true,
                failureKind: syncFailureKind,
                networkReachability: networkReachability
            )
            return FailureContext(
                title: copy.title,
                message: copy.message,
                systemImage: copy.systemImage,
                tint: AppColor.ember,
                canRetry: true,
                canDismiss: true
            )
        }

        if case .failed(let message) = syncState {
            let copy = Self.syncFailureCopy(
                fallbackMessage: message,
                isPaused: false,
                failureKind: syncFailureKind,
                networkReachability: networkReachability
            )
            return FailureContext(
                title: copy.title,
                message: copy.message,
                systemImage: copy.systemImage,
                tint: AppColor.ember,
                canRetry: true,
                canDismiss: true
            )
        }

        if let summary = deferredReminderSummary, summary.hasDeferred {
            let totalDeferred = summary.deferredEvents + summary.deferredTasks
            let noun = totalDeferred == 1 ? "reminder was" : "reminders were"
            return FailureContext(
                title: "\(totalDeferred) \(noun) deferred on this Mac",
                message: "macOS only allows 64 pending local notifications per app. Hot Cross Buns scheduled the nearest items first and will roll in the rest automatically as space frees up.",
                systemImage: "bell.badge",
                tint: AppColor.ember,
                canRetry: false,
                actionLabel: "Review",
                action: openSyncIssues,
                canDismiss: false
            )
        }

        return nil
    }

    static func cacheWarningTitle(for message: String) -> String {
        let lowercased = message.lowercased()
        if lowercased.contains("encrypted") || lowercased.contains("unlock") {
            return "Local cache is locked"
        }
        if lowercased.contains("restored") || lowercased.contains("recovered") {
            return "Local cache recovered"
        }
        return "Local cache needs attention"
    }

    static func syncFailureCopy(
        fallbackMessage: String,
        isPaused: Bool,
        failureKind: SyncFailureKind?,
        networkReachability: NetworkReachability
    ) -> (title: String, message: String, systemImage: String) {
        let effectiveKind: SyncFailureKind = {
            if networkReachability == .offline {
                return .offline
            }
            return failureKind ?? .other
        }()

        switch effectiveKind {
        case .offline:
            return (
                isPaused ? "Sync paused while you're offline" : "You're offline",
                "Changes are queued locally and will sync when you reconnect.",
                isPaused ? "pause.circle" : "wifi.slash"
            )
        case .rateLimited:
            return (
                isPaused ? "Sync paused after rate limiting" : "Google is rate-limiting requests",
                "Hot Cross Buns will retry automatically in the current backoff window, usually about 1-2 minutes. Your local changes are safe.",
                isPaused ? "pause.circle" : "speedometer"
            )
        case .quotaExceeded:
            return (
                isPaused ? "Sync paused because Google quota is exhausted" : "Google API quota is exhausted",
                "Automatic retry will not help until Google resets quota. Check the Google Cloud quota pages for the Calendar and Tasks APIs, or switch to Manual sync to reduce usage.",
                isPaused ? "pause.circle" : "gauge.with.dots.needle.67percent"
            )
        case .serviceUnavailable:
            return (
                isPaused ? "Sync paused while Google is unavailable" : "Google Calendar or Tasks is briefly unavailable",
                "Hot Cross Buns will retry automatically as soon as the service recovers.",
                isPaused ? "pause.circle" : "exclamationmark.arrow.triangle.2.circlepath"
            )
        case .authRequired:
            return (
                "Reconnect Google to keep syncing",
                fallbackMessage,
                "person.crop.circle.badge.exclamationmark"
            )
        case .notFound:
            return (
                isPaused ? "Sync paused because cached Google data is missing" : "Signed in, but Google couldn't find cached sync data",
                "Login worked. Google returned 404 while syncing a saved task list or calendar, usually because it was deleted or belongs to a different Google account. Try Retry once; if it repeats, open Settings > Diagnostics and run Force Full Resync.",
                isPaused ? "pause.circle" : "tray.and.arrow.down"
            )
        case .invalidPayload, .other:
            return (
                isPaused ? "Sync paused — tap Retry when you're ready" : "Couldn't reach Google — try Refresh",
                fallbackMessage,
                isPaused ? "pause.circle" : "exclamationmark.arrow.triangle.2.circlepath"
            )
        }
    }
}

private struct FailureContext {
    var title: String
    var message: String
    var systemImage: String
    var tint: Color
    var canRetry: Bool
    var actionLabel: String = "Review"
    var action: (() -> Void)? = nil
    var canDismiss: Bool = true
}

private struct InfoContext {
    var title: String
}

struct SyncScopeSummary: Equatable, Sendable {
    var tasks: Int
    var events: Int

    var hasScope: Bool {
        tasks > 0 || events > 0
    }

    var copy: String {
        let taskPart = "\(tasks) task\(tasks == 1 ? "" : "s")"
        let eventPart = "\(events) event\(events == 1 ? "" : "s")"
        return "\(taskPart) and \(eventPart)"
    }
}

#Preview {
    AppStatusBanner(
        syncState: .failed(message: "Google API request failed with status 403."),
        authState: .signedOut,
        mutationError: nil,
        retry: {},
        dismiss: {}
    )
    .padding()
}
