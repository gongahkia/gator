import SwiftUI

struct ActionCenterHoldGroup: Identifiable, Equatable, Sendable {
    var id: String
    var metadata: AvailabilityHoldMetadata
    var events: [CalendarEventMirror]

    var nextStartDate: Date {
        events.first?.startDate ?? metadata.createdAt
    }
}

struct ActionCenterIssue: Identifiable, Equatable, Sendable {
    enum Kind: String, Equatable, Sendable {
        case authFailure
        case conflicts
        case invalidPayloads
        case quarantined
        case mutationError
        case syncFailure
        case deferredReminders
    }

    var kind: Kind
    var count: Int
    var title: String
    var message: String
    var systemImage: String
    var actionTitle: String?
    var canRetry: Bool = false
    var canDismiss: Bool = false

    var id: Kind { kind }

    var badgeContribution: Int {
        max(1, count)
    }
}

struct ActionCenterSnapshot: Equatable, Sendable {
    var holdGroups: [ActionCenterHoldGroup]
    var overdueTasks: [TaskMirror]
    var overdueTaskCount: Int
    var issues: [ActionCenterIssue]

    var overdueTaskOverflowCount: Int {
        max(0, overdueTaskCount - overdueTasks.count)
    }

    var actionableCount: Int {
        holdGroups.count
            + overdueTaskCount
            + issues.reduce(0) { $0 + $1.badgeContribution }
    }

    var isEmpty: Bool {
        actionableCount == 0
    }
}

enum ActionCenterBuilder {
    static let defaultOverdueTaskDisplayLimit = 25

    static func build(
        tasks: [TaskMirror],
        events: [CalendarEventMirror],
        pendingMutations: [PendingMutation],
        notificationSummary: NotificationScheduleSummary?,
        authState: AuthState,
        syncState: SyncState,
        isSyncPaused: Bool,
        mutationError: String?,
        syncFailureKind: SyncFailureKind?,
        networkReachability: NetworkReachability,
        referenceDate: Date = Date(),
        calendar: Calendar = .current,
        overdueTaskDisplayLimit: Int = defaultOverdueTaskDisplayLimit
    ) -> ActionCenterSnapshot {
        let holdGroups = buildHoldGroups(events: events)
        let overdue = buildOverdueTasks(
            tasks: tasks,
            referenceDate: referenceDate,
            calendar: calendar,
            displayLimit: overdueTaskDisplayLimit
        )
        return build(
            holdGroups: holdGroups,
            overdueTasks: overdue.tasks,
            overdueTaskCount: overdue.totalCount,
            pendingMutations: pendingMutations,
            notificationSummary: notificationSummary,
            authState: authState,
            syncState: syncState,
            isSyncPaused: isSyncPaused,
            mutationError: mutationError,
            syncFailureKind: syncFailureKind,
            networkReachability: networkReachability
        )
    }

    static func build(
        holdGroups: [ActionCenterHoldGroup],
        overdueTasks: [TaskMirror],
        overdueTaskCount: Int,
        pendingMutations: [PendingMutation],
        notificationSummary: NotificationScheduleSummary?,
        authState: AuthState,
        syncState: SyncState,
        isSyncPaused: Bool,
        mutationError: String?,
        syncFailureKind: SyncFailureKind?,
        networkReachability: NetworkReachability
    ) -> ActionCenterSnapshot {
        let mutationBuckets = buildMutationBuckets(pendingMutations)
        let issues = buildIssues(
            mutationBuckets: mutationBuckets,
            notificationSummary: notificationSummary,
            authState: authState,
            syncState: syncState,
            isSyncPaused: isSyncPaused,
            mutationError: mutationError,
            syncFailureKind: syncFailureKind,
            networkReachability: networkReachability
        )

        return ActionCenterSnapshot(
            holdGroups: holdGroups,
            overdueTasks: overdueTasks,
            overdueTaskCount: overdueTaskCount,
            issues: issues
        )
    }

    static func buildHoldGroups(events: [CalendarEventMirror]) -> [ActionCenterHoldGroup] {
        var grouped: [String: [CalendarEventMirror]] = [:]
        for event in events {
            guard event.status != .cancelled,
                  let metadata = event.availabilityHold,
                  metadata.groupID.isEmpty == false
            else { continue }
            grouped[metadata.groupID, default: []].append(event)
        }

        return grouped.compactMap { groupID, events in
            guard let metadata = events.compactMap(\.availabilityHold).first else { return nil }
            let sortedEvents = events.sorted { lhs, rhs in
                if lhs.startDate == rhs.startDate { return lhs.id < rhs.id }
                return lhs.startDate < rhs.startDate
            }
            return ActionCenterHoldGroup(id: groupID, metadata: metadata, events: sortedEvents)
        }
        .sorted { lhs, rhs in
            if lhs.nextStartDate == rhs.nextStartDate { return lhs.id < rhs.id }
            return lhs.nextStartDate < rhs.nextStartDate
        }
    }

    private static func buildOverdueTasks(
        tasks: [TaskMirror],
        referenceDate: Date,
        calendar: Calendar,
        displayLimit: Int
    ) -> (tasks: [TaskMirror], totalCount: Int) {
        let today = calendar.startOfDay(for: referenceDate)
        var overdue: [TaskMirror] = []
        overdue.reserveCapacity(min(tasks.count, max(0, displayLimit)))

        for task in tasks {
            guard task.isCompleted == false,
                  task.isDeleted == false,
                  task.isHidden == false,
                  let dueDate = task.dueDate,
                  calendar.startOfDay(for: dueDate) < today
            else { continue }
            overdue.append(task)
        }

        overdue.sort { lhs, rhs in
            let left = lhs.dueDate ?? .distantPast
            let right = rhs.dueDate ?? .distantPast
            if left == right { return lhs.title.localizedCaseInsensitiveCompare(rhs.title) == .orderedAscending }
            return left < right
        }

        let capped = displayLimit < 0 ? overdue : Array(overdue.prefix(displayLimit))
        return (capped, overdue.count)
    }

    private static func buildMutationBuckets(_ pendingMutations: [PendingMutation]) -> MutationBuckets {
        var buckets = MutationBuckets()
        for mutation in pendingMutations where mutation.isQuarantined {
            if mutation.isConflict {
                buckets.conflicts += 1
            } else if (mutation.lastErrorSummary ?? "").hasPrefix("Invalid payload") {
                buckets.invalidPayloads += 1
            } else {
                buckets.quarantined += 1
            }
        }
        return buckets
    }

    private static func buildIssues(
        mutationBuckets: MutationBuckets,
        notificationSummary: NotificationScheduleSummary?,
        authState: AuthState,
        syncState: SyncState,
        isSyncPaused: Bool,
        mutationError: String?,
        syncFailureKind: SyncFailureKind?,
        networkReachability: NetworkReachability
    ) -> [ActionCenterIssue] {
        var issues: [ActionCenterIssue] = []
        var hasAuthFailure = false

        if case .failed(let message) = authState {
            hasAuthFailure = true
            issues.append(ActionCenterIssue(
                kind: .authFailure,
                count: 1,
                title: "Reconnect Google to keep syncing",
                message: message,
                systemImage: "person.crop.circle.badge.exclamationmark",
                actionTitle: "Settings"
            ))
        }

        if mutationBuckets.conflicts > 0 {
            let noun = mutationBuckets.conflicts == 1 ? "conflict" : "conflicts"
            issues.append(ActionCenterIssue(
                kind: .conflicts,
                count: mutationBuckets.conflicts,
                title: "\(mutationBuckets.conflicts) sync \(noun)",
                message: "Google rejected these writes because the same items changed elsewhere.",
                systemImage: "arrow.triangle.branch",
                actionTitle: "Review"
            ))
        }

        if mutationBuckets.invalidPayloads > 0 {
            let noun = mutationBuckets.invalidPayloads == 1 ? "queued write has" : "queued writes have"
            issues.append(ActionCenterIssue(
                kind: .invalidPayloads,
                count: mutationBuckets.invalidPayloads,
                title: "\(mutationBuckets.invalidPayloads) \(noun) invalid data",
                message: "Google rejected these payloads as malformed.",
                systemImage: "doc.badge.exclamationmark",
                actionTitle: "Review"
            ))
        }

        if mutationBuckets.quarantined > 0 {
            let noun = mutationBuckets.quarantined == 1 ? "change" : "changes"
            issues.append(ActionCenterIssue(
                kind: .quarantined,
                count: mutationBuckets.quarantined,
                title: "\(mutationBuckets.quarantined) \(noun) need attention",
                message: "These queued writes stopped retrying after repeated Google failures.",
                systemImage: "exclamationmark.octagon",
                actionTitle: "Review"
            ))
        }

        if let mutationError, mutationError.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
            issues.append(ActionCenterIssue(
                kind: .mutationError,
                count: 1,
                title: "Last change did not save",
                message: mutationError,
                systemImage: "exclamationmark.triangle",
                actionTitle: "Dismiss",
                canDismiss: true
            ))
        }

        if let syncIssue = buildSyncIssue(
            syncState: syncState,
            isSyncPaused: isSyncPaused,
            syncFailureKind: syncFailureKind,
            networkReachability: networkReachability,
            suppressAuthRequired: hasAuthFailure
        ) {
            issues.append(syncIssue)
        }

        if let summary = notificationSummary, summary.hasDeferred {
            let totalDeferred = summary.deferredEvents + summary.deferredTasks
            let noun = totalDeferred == 1 ? "reminder was" : "reminders were"
            issues.append(ActionCenterIssue(
                kind: .deferredReminders,
                count: totalDeferred,
                title: "\(totalDeferred) \(noun) deferred",
                message: "The nearest \(summary.totalScheduled) reminders are scheduled; the rest will roll in as macOS notification slots free up.",
                systemImage: "bell.badge",
                actionTitle: "Review"
            ))
        }

        return issues
    }

    private static func buildSyncIssue(
        syncState: SyncState,
        isSyncPaused: Bool,
        syncFailureKind: SyncFailureKind?,
        networkReachability: NetworkReachability,
        suppressAuthRequired: Bool
    ) -> ActionCenterIssue? {
        if suppressAuthRequired, syncFailureKind == .authRequired {
            return nil
        }

        if isSyncPaused {
            let copy = AppStatusBanner.syncFailureCopy(
                fallbackMessage: "Google was not reachable after several attempts. Local changes are queued and will sync when you retry.",
                isPaused: true,
                failureKind: syncFailureKind,
                networkReachability: networkReachability
            )
            return ActionCenterIssue(
                kind: .syncFailure,
                count: 1,
                title: copy.title,
                message: copy.message,
                systemImage: copy.systemImage,
                actionTitle: "Retry",
                canRetry: true,
                canDismiss: true
            )
        }

        guard case .failed(let message) = syncState else { return nil }
        let copy = AppStatusBanner.syncFailureCopy(
            fallbackMessage: message,
            isPaused: false,
            failureKind: syncFailureKind,
            networkReachability: networkReachability
        )
        return ActionCenterIssue(
            kind: .syncFailure,
            count: 1,
            title: copy.title,
            message: copy.message,
            systemImage: copy.systemImage,
            actionTitle: "Retry",
            canRetry: true,
            canDismiss: true
        )
    }

    private struct MutationBuckets {
        var conflicts = 0
        var invalidPayloads = 0
        var quarantined = 0
    }
}

struct ActionCenterDrawer: View {
    let snapshot: ActionCenterSnapshot
    let onClose: () -> Void
    let onOpenHold: (CalendarEventMirror) -> Void
    let onConfirmHold: (CalendarEventMirror) -> Void
    let onCancelHoldGroup: (ActionCenterHoldGroup) -> Void
    let onOpenTask: (TaskMirror) -> Void
    let onCompleteTask: (TaskMirror) -> Void
    let onSnoozeTask: (TaskMirror) -> Void
    let onOpenSyncIssues: () -> Void
    let onOpenSettings: () -> Void
    let onRetrySync: () -> Void
    let onDismissStatus: () -> Void

    @State private var pendingCancelGroup: ActionCenterHoldGroup?

    var body: some View {
        VStack(spacing: 0) {
            header
            Divider()
            content
        }
        .frame(width: 396)
        .frame(maxHeight: .infinity)
        .background(.regularMaterial)
        .overlay(alignment: .leading) {
            Rectangle()
                .fill(AppColor.cardStroke)
                .frame(width: 1)
        }
        .shadow(color: .black.opacity(0.16), radius: 18, x: -4, y: 0)
        .confirmationDialog(
            "Cancel availability holds?",
            isPresented: cancelConfirmationBinding,
            titleVisibility: .visible,
            presenting: pendingCancelGroup
        ) { group in
            Button("Cancel Holds", role: .destructive) {
                pendingCancelGroup = nil
                onCancelHoldGroup(group)
            }
            Button("Keep Holds", role: .cancel) {
                pendingCancelGroup = nil
            }
        } message: { group in
            Text("This deletes \(group.events.count) pending hold\(group.events.count == 1 ? "" : "s") from the calendar.")
        }
    }

    private var cancelConfirmationBinding: Binding<Bool> {
        Binding(
            get: { pendingCancelGroup != nil },
            set: { isPresented in
                if isPresented == false {
                    pendingCancelGroup = nil
                }
            }
        )
    }

    private var header: some View {
        HStack(spacing: 10) {
            Label {
                Text("Notifications")
                    .hcbFont(.headline, weight: .semibold)
            } icon: {
                Image(systemName: snapshot.isEmpty ? "bell" : "bell.badge")
                    .foregroundStyle(AppColor.ember)
            }
            Spacer(minLength: 8)
            if snapshot.actionableCount > 0 {
                Text(badgeText(snapshot.actionableCount))
                    .hcbFont(.caption2, weight: .bold)
                    .monospacedDigit()
                    .foregroundStyle(.white)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(Capsule().fill(AppColor.ember))
                    .accessibilityHidden(true)
            }
            Button(action: onClose) {
                Image(systemName: "xmark")
            }
            .buttonStyle(.borderless)
            .keyboardShortcut(.cancelAction)
            .help("Close notifications")
            .accessibilityLabel("Close notifications")
        }
        .hcbScaledPadding(.horizontal, 16)
        .hcbScaledPadding(.vertical, 14)
    }

    @ViewBuilder
    private var content: some View {
        if snapshot.isEmpty {
            VStack(spacing: 10) {
                Image(systemName: "checkmark.circle")
                    .font(.system(size: 28, weight: .regular))
                    .foregroundStyle(AppColor.moss)
                Text("No notifications")
                    .hcbFont(.subheadline, weight: .semibold)
                Text("Holds, overdue tasks, sync issues, and deferred reminders will appear here.")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .hcbScaledPadding(28)
        } else {
            ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    if snapshot.holdGroups.isEmpty == false {
                        holdSection
                    }
                    if snapshot.overdueTaskCount > 0 {
                        overdueTaskSection
                    }
                    if snapshot.issues.isEmpty == false {
                        issueSection
                    }
                }
                .hcbScaledPadding(16)
            }
        }
    }

    private var holdSection: some View {
        VStack(alignment: .leading, spacing: 10) {
            sectionHeader("Availability Holds", count: snapshot.holdGroups.count)
            ForEach(snapshot.holdGroups) { group in
                holdGroupCard(group)
            }
        }
    }

    private func holdGroupCard(_ group: ActionCenterHoldGroup) -> some View {
        VStack(alignment: .leading, spacing: 9) {
            HStack(spacing: 8) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(group.metadata.title)
                        .hcbFont(.subheadline, weight: .semibold)
                        .lineLimit(1)
                    Text("\(group.events.count) hold\(group.events.count == 1 ? "" : "s")")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer(minLength: 8)
                Button {
                    pendingCancelGroup = group
                } label: {
                    Image(systemName: "trash")
                }
                .buttonStyle(.borderless)
                .help("Cancel hold group")
                .accessibilityLabel("Cancel hold group")
            }

            ForEach(group.events) { event in
                HStack(spacing: 8) {
                    Button {
                        onOpenHold(event)
                    } label: {
                        Text(slotLabel(for: event))
                            .hcbFont(.caption)
                            .lineLimit(1)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .buttonStyle(.plain)
                    .help("Open hold")

                    Button {
                        onConfirmHold(event)
                    } label: {
                        Image(systemName: "checkmark.circle")
                    }
                    .buttonStyle(.borderless)
                    .disabled(OptimisticID.isPending(event.id))
                    .help("Confirm hold")
                    .accessibilityLabel("Confirm hold")

                    Button {
                        onOpenHold(event)
                    } label: {
                        Image(systemName: "arrow.up.forward.square")
                    }
                    .buttonStyle(.borderless)
                    .help("Open hold")
                    .accessibilityLabel("Open hold")
                }
            }
        }
        .hcbScaledPadding(10)
        .background(RoundedRectangle(cornerRadius: 8).fill(AppColor.cardSurface))
        .overlay(
            RoundedRectangle(cornerRadius: 8)
                .strokeBorder(AppColor.cardStroke, lineWidth: 1)
        )
    }

    private var overdueTaskSection: some View {
        VStack(alignment: .leading, spacing: 10) {
            sectionHeader("Overdue Tasks", count: snapshot.overdueTaskCount)
            ForEach(snapshot.overdueTasks) { task in
                overdueTaskRow(task)
            }
            if snapshot.overdueTaskOverflowCount > 0 {
                Text("\(snapshot.overdueTaskOverflowCount) more overdue task\(snapshot.overdueTaskOverflowCount == 1 ? "" : "s")")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .hcbScaledPadding(.horizontal, 4)
            }
        }
    }

    private func overdueTaskRow(_ task: TaskMirror) -> some View {
        HStack(spacing: 10) {
            Image(systemName: "checklist")
                .foregroundStyle(AppColor.ember)
                .frame(width: 18)

            VStack(alignment: .leading, spacing: 2) {
                Text(task.title)
                    .hcbFont(.subheadline, weight: .semibold)
                    .lineLimit(1)
                if let dueDate = task.dueDate {
                    Text("Due \(dueDate.formatted(.dateTime.month(.abbreviated).day().year()))")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            Spacer(minLength: 8)

            Button {
                onCompleteTask(task)
            } label: {
                Image(systemName: "checkmark.circle")
            }
            .buttonStyle(.borderless)
            .help("Complete task")
            .accessibilityLabel("Complete task")

            Button {
                onSnoozeTask(task)
            } label: {
                Image(systemName: "moon.zzz")
            }
            .buttonStyle(.borderless)
            .help("Snooze until tomorrow")
            .accessibilityLabel("Snooze task until tomorrow")

            Button {
                onOpenTask(task)
            } label: {
                Image(systemName: "arrow.up.forward.square")
            }
            .buttonStyle(.borderless)
            .help("Open task")
            .accessibilityLabel("Open task")
        }
        .hcbScaledPadding(10)
        .background(RoundedRectangle(cornerRadius: 8).fill(AppColor.cardSurface))
        .overlay(
            RoundedRectangle(cornerRadius: 8)
                .strokeBorder(AppColor.cardStroke, lineWidth: 1)
        )
    }

    private var issueSection: some View {
        VStack(alignment: .leading, spacing: 10) {
            sectionHeader("Sync and Reminders", count: snapshot.issues.reduce(0) { $0 + $1.badgeContribution })
            ForEach(snapshot.issues) { issue in
                issueRow(issue)
            }
        }
    }

    private func issueRow(_ issue: ActionCenterIssue) -> some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: issue.systemImage)
                .foregroundStyle(issue.kind == .deferredReminders ? AppColor.ember : .red)
                .frame(width: 18)
                .padding(.top, 2)

            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 6) {
                    Text(issue.title)
                        .hcbFont(.subheadline, weight: .semibold)
                        .lineLimit(2)
                    if issue.count > 1 {
                        Text(badgeText(issue.count))
                            .hcbFont(.caption2, weight: .bold)
                            .monospacedDigit()
                            .foregroundStyle(.secondary)
                            .padding(.horizontal, 5)
                            .padding(.vertical, 1)
                            .background(Capsule().fill(Color.secondary.opacity(0.12)))
                    }
                }
                Text(issue.message)
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                issueActions(issue)
            }
            Spacer(minLength: 0)
        }
        .hcbScaledPadding(10)
        .background(RoundedRectangle(cornerRadius: 8).fill(AppColor.cardSurface))
        .overlay(
            RoundedRectangle(cornerRadius: 8)
                .strokeBorder(AppColor.cardStroke, lineWidth: 1)
        )
    }

    @ViewBuilder
    private func issueActions(_ issue: ActionCenterIssue) -> some View {
        HStack(spacing: 8) {
            switch issue.kind {
            case .authFailure:
                Button("Settings", action: onOpenSettings)
                    .buttonStyle(.bordered)
                    .controlSize(.small)
            case .conflicts, .invalidPayloads, .quarantined, .deferredReminders:
                Button("Review", action: onOpenSyncIssues)
                    .buttonStyle(.bordered)
                    .controlSize(.small)
            case .mutationError:
                if issue.canDismiss {
                    Button("Dismiss", action: onDismissStatus)
                        .buttonStyle(.bordered)
                        .controlSize(.small)
                }
            case .syncFailure:
                if issue.canRetry {
                    Button("Retry", action: onRetrySync)
                        .buttonStyle(.bordered)
                        .controlSize(.small)
                }
                if issue.canDismiss {
                    Button("Dismiss", action: onDismissStatus)
                        .buttonStyle(.bordered)
                        .controlSize(.small)
                }
            }
        }
        .padding(.top, 2)
    }

    private func sectionHeader(_ title: String, count: Int) -> some View {
        HStack(spacing: 8) {
            Text(title)
                .hcbFont(.caption, weight: .semibold)
                .foregroundStyle(.secondary)
                .textCase(.uppercase)
            Text(badgeText(count))
                .hcbFont(.caption2, weight: .bold)
                .monospacedDigit()
                .foregroundStyle(.secondary)
                .padding(.horizontal, 5)
                .padding(.vertical, 1)
                .background(Capsule().fill(Color.secondary.opacity(0.12)))
            Spacer(minLength: 0)
        }
    }

    private func slotLabel(for event: CalendarEventMirror) -> String {
        let start = event.startDate.formatted(.dateTime.weekday(.abbreviated).month(.abbreviated).day().hour().minute())
        let end = event.endDate.formatted(.dateTime.hour().minute())
        return "\(start)-\(end)"
    }

    private func badgeText(_ count: Int) -> String {
        count > 99 ? "99+" : "\(count)"
    }
}
