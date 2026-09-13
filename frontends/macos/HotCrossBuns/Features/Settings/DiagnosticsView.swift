import AppKit
import SwiftUI

struct DiagnosticsView: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(AppModel.self) private var model
    @State private var cachePath = "Loading..."
    @State private var isWorking = false
    @State private var confirmation: DiagnosticsConfirmation?
    @State private var copiedAt: Date?
    @State private var lastCrash: String?
    @State private var systemCrashReports: [SystemCrashReport] = []
    @State private var notificationSummary: NotificationScheduleSummary?
    @State private var performanceDiagnostics: PerformanceDiagnosticsSnapshot?
    #if DEBUG
    @State private var bodyProbeCounts: [HCBDebugBodyProbeCount] = []
    #endif
    @State private var logEntries: [LogEntry] = []
    @State private var logLevelFilter: LogLevel = .info
    @State private var logSearchText = ""
    @State private var logCopiedAt: Date?
    @State private var auditEntries: [MutationAuditEntry] = []
    @State private var historySearchText = ""
    @State private var expandedSystemReportID: String?
    @State private var systemReportPreview: String = ""
    @State private var tab: Tab = .overview
    @FocusState private var isSearchFocused: Bool

    private enum Tab: String, CaseIterable, Identifiable {
        case overview, sync, logs, history, support
        var id: String { rawValue }
        var title: String {
            switch self {
            case .overview: "Overview"
            case .sync: "Sync"
            case .logs: "Logs"
            case .history: "History"
            case .support: "Support"
            }
        }
        var systemImage: String {
            switch self {
            case .overview: "gauge.medium"
            case .sync: "arrow.triangle.2.circlepath"
            case .logs: "doc.text"
            case .history: "clock.arrow.circlepath"
            case .support: "lifepreserver"
            }
        }
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                tabBar
                Divider()
                switch tab {
                case .logs:
                    logsTabContent
                case .history:
                    historyTabContent
                default:
                    List {
                        switch tab {
                        case .overview:
                            statusSection
                            performanceSection
                            bodyProbeSection
                            localDataSection
                            selectionsSection
                            if notificationSummary != nil { reminderScheduleSection }
                            cacheSection
                        case .sync:
                            if conflictedMutations.isEmpty == false { syncConflictsSection }
                            if invalidPayloadQuarantined.isEmpty == false { invalidPayloadSection }
                            if retryableQuarantined.isEmpty == false { quarantinedSection }
                            if queuedMutations.isEmpty == false { pendingQueueSection }
                            recoverySection
                        case .support:
                            supportSection
                        case .logs, .history:
                            EmptyView()
                        }
                    }
                }
            }
            .navigationTitle(tab.title)
            .toolbar {
                Button("Close") { dismiss() }
                    .keyboardShortcut(.cancelAction)
            }
            .overlay {
                if isWorking {
                    ProgressView("Working...")
                        .hcbScaledPadding(18)
                        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 16))
                }
            }
            .onChange(of: tab) { _, newTab in
                if newTab == .logs || newTab == .history {
                    isSearchFocused = true
                }
            }
            .task {
                cachePath = await model.cacheFilePath()
                lastCrash = CrashReporter.readLastCrash()
                systemCrashReports = SystemCrashReportReader.recentReports(limit: 5)
                notificationSummary = await model.notificationScheduleSummary()
                performanceDiagnostics = await model.performanceDiagnosticsSnapshot()
                refreshBodyProbeCounts()
                auditEntries = await MutationAuditLog.shared.recentEntries(limit: 100)
                refreshLogs()
            }
            .confirmationDialog(
                confirmation?.title ?? "Confirm",
                isPresented: confirmationBinding,
                titleVisibility: .visible
            ) {
                if let confirmation {
                    Button(confirmation.actionTitle, role: confirmation.role) {
                        handle(confirmation)
                    }
                }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text(confirmation?.message ?? "")
            }
        }
        .frame(minWidth: 620, minHeight: 520)
    }

    private var tabBar: some View {
        HStack(spacing: 0) {
            ForEach(Tab.allCases) { entry in
                tabButton(entry)
            }
        }
        .frame(maxWidth: .infinity)
        .hcbScaledPadding(.vertical, 10)
        .background(Color(nsColor: .windowBackgroundColor))
    }

    private func tabButton(_ entry: Tab) -> some View {
        let isSelected = tab == entry
        return Button {
            tab = entry
        } label: {
            VStack(spacing: 4) {
                Image(systemName: entry.systemImage)
                    .hcbFontSystem(size: 18, weight: isSelected ? .semibold : .regular)
                Text(entry.title)
                    .hcbFont(.caption, weight: isSelected ? .semibold : .regular)
            }
            .foregroundStyle(isSelected ? AppColor.ember : AppColor.ink.opacity(0.75))
            .frame(maxWidth: .infinity)
            .hcbScaledPadding(.vertical, 4)
            .hcbScaledPadding(.horizontal, 12)
            .background(
                RoundedRectangle(cornerRadius: 10)
                    .fill(isSelected ? AppColor.ember.opacity(0.12) : Color.clear)
            )
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }

    private var logsTabContent: some View {
        DiagnosticsSearchListLayout(
            searchText: $logSearchText,
            placeholder: "Search logs",
            header: {
                HStack(spacing: 12) {
                    Picker("Level", selection: $logLevelFilter) {
                        ForEach(LogLevel.allCases, id: \.self) { level in
                            Text(level.rawValue.capitalized).tag(level)
                        }
                    }
                    .pickerStyle(.segmented)
                    .onChange(of: logLevelFilter) { _, _ in refreshLogs() }
                    Spacer()
                    Button {
                        refreshLogs()
                    } label: {
                        Label("Refresh", systemImage: "arrow.clockwise")
                    }
                    .keyboardShortcut("r")
                }
            },
            content: {
                if logEntries.isEmpty {
                    DiagnosticsEmptyState(
                        title: "No log entries at this level yet",
                        systemImage: "doc.text.magnifyingglass"
                    )
                } else if filteredLogEntries.isEmpty {
                    DiagnosticsEmptyState(
                        title: "No logs match this search",
                        systemImage: "magnifyingglass"
                    )
                } else {
                    ForEach(filteredLogEntries) { entry in
                        LogEntryRow(entry: entry)
                        Divider()
                    }
                }
            },
            footer: {
                HStack(spacing: 10) {
                    Text("\(filteredLogEntries.count) of \(logEntries.count) shown")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                    Spacer()
                    Button {
                        let full = AppLogger.shared.loadPersistedLog()
                        Clipboard.copy(full.isEmpty ? logEntries.map { $0.formattedLine() }.joined(separator: "\n") : full)
                        logCopiedAt = Date()
                    } label: {
                        Label(logCopiedAt == nil ? "Copy all logs" : "Copied", systemImage: logCopiedAt == nil ? "doc.on.doc" : "checkmark")
                    }
                    if let url = AppLogger.shared.logDirectoryURL() {
                        Button {
                            NSWorkspace.shared.open(url)
                        } label: {
                            Label("Reveal logs folder", systemImage: "folder")
                        }
                    }
                    Button(role: .destructive) {
                        confirmation = .clearLogs
                    } label: {
                        Label("Clear logs", systemImage: "trash")
                    }
                }
            }
        )
        .focused($isSearchFocused)
    }

    private var historyTabContent: some View {
        DiagnosticsSearchListLayout(
            searchText: $historySearchText,
            placeholder: "Search history",
            header: {
                HStack {
                    Text("Mutation history")
                        .hcbFont(.headline, weight: .semibold)
                    Spacer()
                    Text("\(auditEntries.count) retained")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
            },
            content: {
                if auditEntries.isEmpty {
                    DiagnosticsEmptyState(
                        title: "No history yet",
                        systemImage: "clock.arrow.circlepath"
                    )
                } else if filteredAuditEntries.isEmpty {
                    DiagnosticsEmptyState(
                        title: "No history matches this search",
                        systemImage: "magnifyingglass"
                    )
                } else {
                    ForEach(filteredAuditEntries) { entry in
                        AuditEntryRow(entry: entry)
                        Divider()
                    }
                }
                if systemCrashReports.isEmpty == false || lastCrash != nil {
                    Divider()
                    VStack(alignment: .leading, spacing: 8) {
                        if systemCrashReports.isEmpty == false {
                            Text("\(systemCrashReports.count) recent system crash report\(systemCrashReports.count == 1 ? "" : "s") available in Overview diagnostics.")
                                .hcbFont(.caption)
                                .foregroundStyle(.secondary)
                        }
                        if lastCrash != nil {
                            Text("A previous in-app crash record is available in the diagnostic bundle.")
                                .hcbFont(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
            },
            footer: {
                HStack(spacing: 10) {
                    Text("\(filteredAuditEntries.count) of \(auditEntries.count) shown")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                    Spacer()
                    Button {
                        Clipboard.copy(filteredAuditEntries.map(historyLine).joined(separator: "\n"))
                    } label: {
                        Label("Copy visible history", systemImage: "doc.on.doc")
                    }
                    .disabled(filteredAuditEntries.isEmpty)
                }
            }
        )
        .focused($isSearchFocused)
    }

    // MARK: - Sections

    @ViewBuilder
    private var statusSection: some View {
        Section("Status") {
            DiagnosticRow(label: "Google", value: googleStatus)
            DiagnosticRow(label: "Sync", value: model.syncState.title)
            DiagnosticRow(label: "Mode", value: model.settings.syncMode.title)
            DiagnosticRow(label: "Last sync", value: lastSyncText)
            DiagnosticRow(label: "Keychain", value: model.keychainHealth.displayTitle)
            if model.keychainHealth == .denied {
                Text("macOS denied access to the Keychain. Unlock it (Applications → Utilities → Keychain Access → log in) then Reconnect Google.")
                    .hcbFont(.caption)
                    .foregroundStyle(AppColor.ember)
            }
        }
    }

    @ViewBuilder
    private var localDataSection: some View {
        Section("Local data") {
            DiagnosticRow(label: "Task lists", value: model.taskLists.count.formatted())
            DiagnosticRow(label: "Tasks", value: model.tasks.count.formatted())
            DiagnosticRow(label: "Calendars", value: model.calendars.count.formatted())
            DiagnosticRow(label: "Events", value: model.events.count.formatted())
            DiagnosticRow(label: "Sync checkpoints", value: model.syncCheckpoints.count.formatted())
            DiagnosticRow(label: "Pending writes", value: model.pendingMutations.count.formatted())
        }
    }

    @ViewBuilder
    private var performanceSection: some View {
        if let performanceDiagnostics {
            Section("Performance") {
                if let nearRealtime = performanceDiagnostics.nearRealtime {
                    DiagnosticRow(label: "Next poll cadence", value: "\(nearRealtime.delaySeconds.formatted(.number.precision(.fractionLength(1))))s")
                    DiagnosticRow(label: "Poll attempt", value: nearRealtime.attempt.formatted())
                    DiagnosticRow(label: "Poll conditions", value: pollConditionsText(nearRealtime))
                    DiagnosticRow(label: "Poll sample", value: nearRealtime.recordedAt.formatted(date: .omitted, time: .standard))
                } else {
                    DiagnosticRow(label: "Near-realtime poll", value: "No sample yet")
                }

                let integrations = performanceDiagnostics.integrations
                DiagnosticRow(label: "Notifications sync", value: durationText(integrations.notificationDurationMilliseconds, updatedAt: integrations.notificationUpdatedAt))
                DiagnosticRow(label: "Spotlight sync", value: durationText(integrations.spotlight.durationMilliseconds, updatedAt: integrations.spotlight.updatedAt))
                DiagnosticRow(label: "Spotlight indexed", value: "\(integrations.spotlight.indexedTaskCount) task\(integrations.spotlight.indexedTaskCount == 1 ? "" : "s"), \(integrations.spotlight.indexedEventCount) event\(integrations.spotlight.indexedEventCount == 1 ? "" : "s")")
                DiagnosticRow(label: "Spotlight removed", value: integrations.spotlight.deletedItemCount.formatted())
                DiagnosticRow(label: "Spotlight mode", value: integrations.spotlight.rebuiltDomains ? "Full rebuild" : "Incremental")
            }
        }
    }

    @ViewBuilder
    private var bodyProbeSection: some View {
        #if DEBUG
        if HCBDebugBodyProbeRecorder.isEnabled {
            Section("SwiftUI body probe") {
                HStack {
                    Text("Counts are collected only while HCB_BODY_PROBE=1 is set.")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                    Spacer()
                    Button {
                        refreshBodyProbeCounts()
                    } label: {
                        Label("Refresh", systemImage: "arrow.clockwise")
                    }
                    Button {
                        HCBDebugBodyProbeRecorder.reset()
                        refreshBodyProbeCounts()
                    } label: {
                        Label("Reset", systemImage: "trash")
                    }
                }
                if bodyProbeCounts.isEmpty {
                    Text("No probed views have rendered yet.")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                } else {
                    ForEach(bodyProbeCounts.prefix(20)) { count in
                        DiagnosticRow(label: count.name, value: count.count.formatted())
                    }
                }
            }
        }
        #endif
    }

    @ViewBuilder
    private var selectionsSection: some View {
        Section("Selections") {
            DiagnosticRow(label: "Selected task lists", value: selectedTaskListText)
            DiagnosticRow(label: "Selected calendars", value: selectedCalendarText)
            DiagnosticRow(label: "Local reminders", value: model.settings.enableLocalNotifications ? "Enabled" : "Disabled")
            DiagnosticRow(label: "Onboarding", value: model.settings.hasCompletedOnboarding ? "Completed" : "Not completed")
        }
    }

    @ViewBuilder
    private var reminderScheduleSection: some View {
        if let summary = notificationSummary {
            Section("Reminder schedule") {
                DiagnosticRow(label: "Scheduled events", value: summary.scheduledEvents.formatted())
                DiagnosticRow(label: "Scheduled tasks", value: summary.scheduledTasks.formatted())
                if summary.hasFailures {
                    DiagnosticRow(label: "Failed events", value: summary.failedEvents.formatted())
                    DiagnosticRow(label: "Failed tasks", value: summary.failedTasks.formatted())
                    Text("macOS rejected \(summary.totalFailed) reminder\(summary.totalFailed == 1 ? "" : "s"). Check System Settings → Notifications → Hot Cross Buns, or the app logs for the underlying error.")
                        .hcbFont(.caption)
                        .foregroundStyle(.red)
                }
                if summary.hasDeferred {
                    DiagnosticRow(label: "Deferred events", value: summary.deferredEvents.formatted())
                    DiagnosticRow(label: "Deferred tasks", value: summary.deferredTasks.formatted())
                    Text("More reminders exist in the next \(summary.windowDays) days than macOS allows the app to schedule at once. They will be scheduled as earlier ones fire or are cancelled.")
                        .hcbFont(.caption)
                        .foregroundStyle(AppColor.ember)
                } else if summary.hasFailures == false {
                    Text("All reminders within the next \(summary.windowDays) days are scheduled.")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
            }
        }
    }

    @ViewBuilder
    private var cacheSection: some View {
        Section("Cache") {
            Text(cachePath)
                .font(.footnote.monospaced())
                .foregroundStyle(.secondary)
                .textSelection(.enabled)
        }
    }

    @ViewBuilder
    private var syncConflictsSection: some View {
        Section("Sync conflicts") {
            Text("Google rejected these writes with HTTP 412 — someone else edited the same item between the time you made your change and when HCB sent it. Choose whose version wins.")
                .hcbFont(.caption)
                .foregroundStyle(.secondary)
            ForEach(conflictedMutations) { mutation in
                ConflictMutationRow(
                    mutation: mutation,
                    onKeepMine: {
                        Task {
                            _ = await model.forceOverwriteConflictedMutation(id: mutation.id)
                        }
                    },
                    onKeepServer: {
                        _ = model.clearPendingMutation(id: mutation.id)
                    }
                )
            }
        }
    }

    @ViewBuilder
    private var invalidPayloadSection: some View {
        Section("Invalid queued payloads") {
            Text("Google rejected these writes with HTTP 400 because the payload shape or content was invalid. The payload is preserved so you can copy it for debugging, then retry after fixing the source data.")
                .hcbFont(.caption)
                .foregroundStyle(.secondary)
            ForEach(invalidPayloadQuarantined) { mutation in
                PendingMutationRow(
                    mutation: mutation,
                    onDrop: { _ = model.clearPendingMutation(id: mutation.id) },
                    onCopyPayload: { copyPayload(mutation.payload) }
                )
            }
        }
    }

    @ViewBuilder
    private var quarantinedSection: some View {
        Section("Quarantined changes") {
            Text("These writes exceeded the automatic retry ceiling (\(BackoffPolicy.nearRealtime.maxAttempts) attempts). They stay on this Mac until you retry or discard.")
                .hcbFont(.caption)
                .foregroundStyle(.secondary)
            ForEach(retryableQuarantined) { mutation in
                PendingMutationRow(
                    mutation: mutation,
                    onDrop: { _ = model.clearPendingMutation(id: mutation.id) },
                    onRetry: { _ = model.requeueQuarantinedMutation(id: mutation.id) },
                    onCopyPayload: { copyPayload(mutation.payload) }
                )
            }
        }
    }

    @ViewBuilder
    private var pendingQueueSection: some View {
        Section("Pending sync queue") {
            ForEach(queuedMutations) { mutation in
                PendingMutationRow(mutation: mutation) {
                    _ = model.clearPendingMutation(id: mutation.id)
                }
            }
            Button(role: .destructive) {
                model.clearAllPendingMutations()
            } label: {
                Label("Clear entire queue", systemImage: "trash")
            }
        }
    }

    @ViewBuilder
    private var recoverySection: some View {
        Section("Recovery") {
            Button {
                runRecoveryAction {
                    await model.refreshNow()
                }
            } label: {
                Label("Refresh now", systemImage: "arrow.clockwise")
            }
            .disabled(isWorking || model.account == nil)

            Button {
                confirmation = .fullResync
            } label: {
                Label("Force full resync", systemImage: "arrow.triangle.2.circlepath.circle")
            }
            .disabled(isWorking || model.account == nil)

            Button {
                runRecoveryAction {
                    await model.rebuildLocalIntegrationIndexesForDiagnostics()
                }
            } label: {
                Label("Rebuild Spotlight and reminders", systemImage: "magnifyingglass.circle")
            }
            .disabled(isWorking)

            Button(role: .destructive) {
                confirmation = .clearCache
            } label: {
                Label("Clear cached Google data", systemImage: "externaldrive.badge.xmark")
            }
            .disabled(isWorking)
        }
    }

    @ViewBuilder
    private var systemCrashReportsSection: some View {
        Section("System crash reports") {
            Text("macOS writes a symbolicated report each time the app crashes. Use these to see the exact Swift stack.")
                .hcbFont(.caption)
                .foregroundStyle(.secondary)
            ForEach(systemCrashReports) { report in
                SystemCrashRow(
                    report: report,
                    isExpanded: expandedSystemReportID == report.id,
                    preview: expandedSystemReportID == report.id ? systemReportPreview : nil,
                    onToggle: {
                        if expandedSystemReportID == report.id {
                            expandedSystemReportID = nil
                            systemReportPreview = ""
                        } else {
                            expandedSystemReportID = report.id
                            systemReportPreview = SystemCrashReportReader.readContents(of: report) ?? "Could not read report file."
                        }
                    },
                    onCopy: {
                        if let contents = SystemCrashReportReader.readContents(of: report) {
                            Clipboard.copy(contents)
                        }
                    },
                    onReveal: {
                        NSWorkspace.shared.activateFileViewerSelecting([report.url])
                    }
                )
            }
            if let dir = SystemCrashReportReader.directoryURL() {
                Button {
                    NSWorkspace.shared.open(dir)
                } label: {
                    Label("Open folder in Finder", systemImage: "folder")
                }
            }
        }
    }

    @ViewBuilder
    private var previousCrashSection: some View {
        if let crash = lastCrash {
            Section("Previous crash") {
                Text(crash)
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
                    .lineLimit(12)
                HStack {
                    Button {
                        Clipboard.copy(crash)
                    } label: {
                        Label("Copy crash log", systemImage: "doc.on.doc")
                    }
                    Button(role: .destructive) {
                        CrashReporter.clearLastCrash()
                        lastCrash = nil
                    } label: {
                        Label("Clear", systemImage: "trash")
                    }
                }
            }
        }
    }

    @ViewBuilder
    private var supportSection: some View {
        Section("Support") {
            Button {
                copyDiagnostics()
            } label: {
                Label(copiedAt == nil ? "Copy diagnostic summary" : "Copied diagnostic summary", systemImage: copiedAt == nil ? "doc.on.doc" : "checkmark")
            }
            Button {
                Task { await exportDiagnosticBundle() }
            } label: {
                Label("Export diagnostic bundle…", systemImage: "square.and.arrow.up")
            }
            .help("Save a text file with logs, pending mutations, and state for bug reports. Emails and tokens are redacted.")
        }
    }

    private var conflictedMutations: [PendingMutation] {
        model.pendingMutations.filter(\.isConflict)
    }

    // Quarantined but not a conflict — i.e., hit the transient-error ceiling.
    // Conflicts get their own section with "Keep mine" / "Keep server" copy.
    private var retryableQuarantined: [PendingMutation] {
        model.pendingMutations.filter {
            $0.isQuarantined
                && $0.isConflict == false
                && (($0.lastErrorSummary ?? "").hasPrefix("Invalid payload") == false)
        }
    }

    private var invalidPayloadQuarantined: [PendingMutation] {
        model.pendingMutations.filter {
            $0.isQuarantined
                && $0.isConflict == false
                && (($0.lastErrorSummary ?? "").hasPrefix("Invalid payload"))
        }
    }

    private var queuedMutations: [PendingMutation] {
        model.pendingMutations.filter { $0.isQuarantined == false }
    }

    private var filteredLogEntries: [LogEntry] {
        let query = logSearchText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard query.isEmpty == false else { return logEntries }
        return logEntries.filter { entry in
            let metadata = entry.metadata
                .sorted { $0.key < $1.key }
                .map { "\($0.key)=\($0.value)" }
                .joined(separator: " ")
            let visibleTimestamp = entry.timestamp.formatted(date: .abbreviated, time: .standard)
            return [entry.message, entry.category.rawValue, metadata, visibleTimestamp]
                .joined(separator: "\n")
                .localizedCaseInsensitiveContains(query)
        }
    }

    private var filteredAuditEntries: [MutationAuditEntry] {
        let query = historySearchText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard query.isEmpty == false else { return auditEntries }
        return auditEntries.filter { entry in
            [entry.summary, entry.kind, entry.timestamp.formatted(date: .abbreviated, time: .shortened)]
                .joined(separator: "\n")
                .localizedCaseInsensitiveContains(query)
        }
    }

    private var googleStatus: String {
        if let account = model.account {
            return account.displayName
        }

        return model.authState.title
    }

    private var lastSyncText: String {
        guard let date = model.lastSuccessfulSyncAt else {
            return "Never"
        }

        return date.formatted(date: .abbreviated, time: .shortened)
    }

    private func durationText(_ milliseconds: Double?, updatedAt: Date?) -> String {
        guard let milliseconds, let updatedAt else { return "No sample yet" }
        return "\(milliseconds.formatted(.number.precision(.fractionLength(1)))) ms at \(updatedAt.formatted(date: .omitted, time: .standard))"
    }

    private func pollConditionsText(_ diagnostics: NearRealtimePollDiagnostics) -> String {
        var parts: [String] = []
        parts.append(diagnostics.isMainWindowFocused ? "focused" : "unfocused")
        if diagnostics.isNetworkConstrained {
            parts.append("constrained")
        }
        if diagnostics.isLowPowerModeEnabled {
            parts.append("low power")
        }
        return parts.joined(separator: ", ")
    }

    private func refreshBodyProbeCounts() {
        #if DEBUG
        bodyProbeCounts = HCBDebugBodyProbeRecorder.snapshot()
        #endif
    }

    private var selectedTaskListText: String {
        if model.settings.hasConfiguredTaskListSelection {
            return "\(model.settings.selectedTaskListIDs.count) of \(model.taskLists.count)"
        }

        return model.taskLists.isEmpty ? "Not loaded" : "All"
    }

    private var selectedCalendarText: String {
        let selectedCount = model.calendars.filter(\.isSelected).count
        if model.settings.hasConfiguredCalendarSelection {
            return "\(model.settings.selectedCalendarIDs.count) of \(model.calendars.count)"
        }

        return model.calendars.isEmpty ? "Not loaded" : "\(selectedCount) default"
    }

    private var confirmationBinding: Binding<Bool> {
        Binding(
            get: { confirmation != nil },
            set: { isPresented in
                if isPresented == false {
                    confirmation = nil
                }
            }
        )
    }

    private func handle(_ confirmation: DiagnosticsConfirmation) {
        switch confirmation {
        case .fullResync:
            runRecoveryAction {
                await model.forceFullResync()
            }
        case .clearCache:
            runRecoveryAction {
                await model.clearCachedGoogleDataAndRefresh()
            }
        case .clearLogs:
            AppLogger.shared.clearLogs()
            refreshLogs()
        }
    }

    private func runRecoveryAction(_ action: @escaping @MainActor () async -> Void) {
        guard isWorking == false else {
            return
        }

        isWorking = true
        Task {
            await action()
            cachePath = await model.cacheFilePath()
            isWorking = false
        }
    }

    private func copyDiagnostics() {
        Clipboard.copy(model.diagnosticSummary(cachePath: cachePath))
        copiedAt = Date()
    }

    private func refreshLogs() {
        logEntries = AppLogger.shared.recentEntries(limit: 200, minimumLevel: logLevelFilter)
    }

    private func historyLine(_ entry: MutationAuditEntry) -> String {
        "\(entry.timestamp.formatted(date: .abbreviated, time: .shortened)) [\(entry.kind)] \(entry.summary)"
    }

    private func copyPayload(_ payload: Data) {
        let value: String
        if let object = try? JSONSerialization.jsonObject(with: payload),
           JSONSerialization.isValidJSONObject(object),
           let pretty = try? JSONSerialization.data(withJSONObject: object, options: [.prettyPrinted, .sortedKeys]),
           let string = String(data: pretty, encoding: .utf8) {
            value = string
        } else {
            value = String(data: payload, encoding: .utf8) ?? payload.base64EncodedString()
        }
        Clipboard.copy(value)
    }

    @MainActor
    private func exportDiagnosticBundle() async {
        let url = await DiagnosticBundle.exportToDisk(
            model: model,
            cachePath: cachePath,
            notificationSummary: notificationSummary
        )
        if url != nil {
            copiedAt = Date() // reuse the button-label success indicator
        }
    }
}

private struct DiagnosticsSearchListLayout<Header: View, Content: View, Footer: View>: View {
    @Binding var searchText: String
    let placeholder: String
    @ViewBuilder let header: Header
    @ViewBuilder let content: Content
    @ViewBuilder let footer: Footer

    var body: some View {
        VStack(spacing: 0) {
            VStack(alignment: .leading, spacing: 10) {
                header
                HStack(spacing: 8) {
                    Image(systemName: "magnifyingglass")
                        .foregroundStyle(.secondary)
                    TextField(placeholder, text: $searchText)
                        .textFieldStyle(.plain)
                    if searchText.isEmpty == false {
                        Button {
                            searchText = ""
                        } label: {
                            Image(systemName: "xmark.circle.fill")
                                .foregroundStyle(.secondary)
                        }
                        .buttonStyle(.plain)
                        .keyboardShortcut(.cancelAction)
                    }
                }
                .padding(.horizontal, 10)
                .padding(.vertical, 7)
                .background(.quaternary.opacity(0.35), in: RoundedRectangle(cornerRadius: 8))
            }
            .padding(14)

            Divider()

            ScrollView {
                LazyVStack(alignment: .leading, spacing: 0) {
                    content
                }
                .padding(14)
                .frame(maxWidth: .infinity, alignment: .topLeading)
            }
            .frame(maxHeight: .infinity)

            Divider()

            footer
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
                .background(.bar)
        }
    }
}

private struct DiagnosticsEmptyState: View {
    let title: String
    let systemImage: String

    var body: some View {
        ContentUnavailableView(title, systemImage: systemImage)
            .frame(maxWidth: .infinity, minHeight: 240)
    }
}

private struct AuditEntryRow: View {
    let entry: MutationAuditEntry

    var body: some View {
        HStack(alignment: .top, spacing: 8) {
            Image(systemName: symbol)
                .hcbFont(.caption)
                .foregroundStyle(tint)
                .hcbScaledFrame(width: 14)
            VStack(alignment: .leading, spacing: 1) {
                Text(entry.summary)
                    .font(.caption.monospaced())
                    .lineLimit(2)
                Text("\(entry.kind) · \(entry.timestamp.formatted(date: .abbreviated, time: .shortened))")
                    .font(.caption2.monospaced())
                    .foregroundStyle(.secondary)
            }
            Spacer(minLength: 0)
        }
    }

    private var symbol: String {
        switch entry.kind {
        case "task.complete", "task.reopen": "checkmark.circle"
        case "task.delete", "event.delete": "trash"
        case "task.edit", "event.edit": "pencil"
        default: "circle"
        }
    }

    private var tint: Color {
        switch entry.kind {
        case "task.delete", "event.delete": AppColor.ember
        case "task.edit", "event.edit": AppColor.blue
        default: AppColor.moss
        }
    }
}

private struct LogEntryRow: View {
    let entry: LogEntry

    var body: some View {
        HStack(alignment: .top, spacing: 8) {
            Image(systemName: entry.level.systemSymbol)
                .hcbFont(.caption)
                .foregroundStyle(tint)
                .hcbScaledFrame(width: 14)
            VStack(alignment: .leading, spacing: 1) {
                Text("[\(entry.category.rawValue)] \(entry.message)")
                    .font(.caption.monospaced())
                    .foregroundStyle(AppColor.ink)
                    .lineLimit(3)
                HStack(spacing: 6) {
                    Text(entry.timestamp.formatted(date: .omitted, time: .standard))
                        .font(.caption2.monospaced())
                        .foregroundStyle(.secondary)
                    if entry.metadata.isEmpty == false {
                        Text(entry.metadata
                            .sorted { $0.key < $1.key }
                            .map { "\($0.key)=\($0.value)" }
                            .joined(separator: " "))
                            .font(.caption2.monospaced())
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                }
            }
            Spacer(minLength: 0)
        }
        .hcbScaledPadding(.vertical, 1)
    }

    private var tint: Color {
        switch entry.level {
        case .debug: .secondary
        case .info: AppColor.blue
        case .warn: AppColor.ember
        case .error: .red
        }
    }
}

private struct SystemCrashRow: View {
    let report: SystemCrashReport
    let isExpanded: Bool
    let preview: String?
    let onToggle: () -> Void
    let onCopy: () -> Void
    let onReveal: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 10) {
                Image(systemName: "ant.circle.fill")
                    .foregroundStyle(AppColor.ember)
                VStack(alignment: .leading, spacing: 1) {
                    Text(report.filename)
                        .hcbFont(.subheadline, weight: .medium)
                        .lineLimit(1)
                        .truncationMode(.middle)
                    Text(report.modificationDate.formatted(date: .abbreviated, time: .shortened))
                        .hcbFont(.caption2)
                        .foregroundStyle(.secondary)
                }
                Spacer(minLength: 8)
                Button(isExpanded ? "Hide" : "View", action: onToggle)
                    .buttonStyle(.borderless)
                Button("Copy", action: onCopy)
                    .buttonStyle(.borderless)
                Button("Reveal", action: onReveal)
                    .buttonStyle(.borderless)
            }
            if isExpanded, let preview {
                Text(preview)
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .hcbScaledPadding(8)
                    .background(
                        RoundedRectangle(cornerRadius: 8)
                            .fill(AppColor.cream.opacity(0.6))
                    )
            }
        }
    }
}

// Conflict-specific row: shows a summary of the pending change (decoded from
// the mutation payload) and two big-intent buttons — "Keep my change" force-
// overwrites the server state via a fresh update without If-Match, and
// "Keep server version" simply discards the queued mutation.
private struct ConflictMutationRow: View {
    let mutation: PendingMutation
    let onKeepMine: () -> Void
    let onKeepServer: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Image(systemName: "arrow.triangle.branch")
                    .foregroundStyle(.red)
                Text(title)
                    .hcbFont(.subheadline, weight: .semibold)
                Spacer(minLength: 0)
            }
            Text(payloadSummary)
                .hcbFont(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(4)
                .textSelection(.enabled)
            Text("Queued \(mutation.createdAt.formatted(date: .abbreviated, time: .shortened)) · resource \(mutation.resourceID)")
                .font(.caption2.monospacedDigit())
                .foregroundStyle(.secondary)
            HStack(spacing: 8) {
                Button(action: onKeepMine) {
                    Label("Keep my change", systemImage: "arrow.up.forward.circle")
                }
                .buttonStyle(.borderedProminent)
                Button(role: .destructive, action: onKeepServer) {
                    Label("Keep server version", systemImage: "icloud.and.arrow.down")
                }
                .buttonStyle(.bordered)
            }
        }
        .hcbScaledPadding(.vertical, 4)
    }

    private var title: String {
        switch (mutation.resourceType, mutation.action) {
        case (.task, .update): "Task edit conflict"
        case (.task, .completion): "Task completion conflict"
        case (.task, .delete): "Task delete conflict"
        case (.event, .update): "Event edit conflict"
        case (.event, .delete): "Event delete conflict"
        default: "\(mutation.resourceType.rawValue) \(mutation.action.rawValue) conflict"
        }
    }

    // Decode the payload into a short human-readable summary of the fields
    // HCB was trying to write. Failures fall back to the mutation id.
    private var payloadSummary: String {
        switch (mutation.resourceType, mutation.action) {
        case (.task, .update):
            if let p = try? PendingMutationEncoder.decodeTaskUpdate(mutation.payload) {
                var parts = ["title: \(p.title)"]
                if p.notes.isEmpty == false { parts.append("notes: \(p.notes.prefix(80))") }
                if let due = p.dueDate { parts.append("due: \(due.formatted(date: .abbreviated, time: .omitted))") }
                return parts.joined(separator: " · ")
            }
        case (.task, .completion):
            if let p = try? PendingMutationEncoder.decodeTaskCompletion(mutation.payload) {
                return p.isCompleted ? "mark complete" : "mark needs action"
            }
        case (.task, .delete):
            return "delete task"
        case (.event, .update):
            if let p = try? PendingMutationEncoder.decodeEventUpdate(mutation.payload) {
                var parts = ["summary: \(p.summary)"]
                parts.append("start: \(p.startDate.formatted(date: .abbreviated, time: p.isAllDay ? .omitted : .shortened))")
                parts.append("end: \(p.endDate.formatted(date: .abbreviated, time: p.isAllDay ? .omitted : .shortened))")
                if p.location.isEmpty == false { parts.append("at \(p.location)") }
                return parts.joined(separator: " · ")
            }
        case (.event, .delete):
            return "delete event"
        default:
            break
        }
        return "Mutation id \(mutation.id.uuidString)"
    }
}

private struct PendingMutationRow: View {
    let mutation: PendingMutation
    let onDrop: () -> Void
    var onRetry: (() -> Void)? = nil
    var onCopyPayload: (() -> Void)? = nil

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: symbol)
                .foregroundStyle(tint)
                .hcbScaledFrame(width: 18, alignment: .center)
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .hcbFont(.subheadline, weight: .medium)
                Text(subtitle)
                    .font(.caption2.monospacedDigit())
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
                if let err = mutation.lastErrorSummary, err.isEmpty == false {
                    Text(err)
                        .hcbFont(.caption2)
                        .foregroundStyle(.red)
                        .lineLimit(2)
                }
            }
            Spacer(minLength: 8)
            if let onRetry {
                Button(action: onRetry) {
                    Label("Retry", systemImage: "arrow.clockwise")
                }
                .labelStyle(.iconOnly)
                .buttonStyle(.borderless)
                .help("Reset retry counter and release this mutation back to the replay queue")
            }
            if let onCopyPayload {
                Button(action: onCopyPayload) {
                    Label("Copy payload", systemImage: "doc.on.doc")
                }
                .labelStyle(.iconOnly)
                .buttonStyle(.borderless)
                .help("Copy the preserved queued payload to the clipboard")
            }
            Button(role: .destructive, action: onDrop) {
                Label("Drop", systemImage: "xmark.circle")
            }
            .labelStyle(.iconOnly)
            .buttonStyle(.borderless)
            .help("Remove this pending mutation from the queue")
        }
    }

    private var title: String {
        switch (mutation.resourceType, mutation.action) {
        case (.task, .create): "New task"
        case (.task, .update): "Task edit"
        case (.task, .completion): "Task completion"
        case (.task, .delete): "Task delete"
        case (.event, .create): "New event"
        case (.event, .update): "Event edit"
        case (.event, .delete): "Event delete"
        default: "Pending \(mutation.resourceType.rawValue) \(mutation.action.rawValue)"
        }
    }

    private var subtitle: String {
        let attempts = mutation.attemptCount
        let suffix = attempts > 0 ? " · \(attempts) attempt\(attempts == 1 ? "" : "s")" : ""
        return "\(mutation.resourceID) · queued \(mutation.createdAt.formatted(date: .abbreviated, time: .shortened))\(suffix)"
    }

    private var symbol: String {
        switch mutation.action {
        case .create: "plus.circle"
        case .update: "pencil.circle"
        case .completion: "checkmark.circle"
        case .delete: "trash.circle"
        }
    }

    private var tint: Color {
        switch mutation.action {
        case .create: AppColor.moss
        case .update: AppColor.blue
        case .completion: AppColor.moss
        case .delete: AppColor.ember
        }
    }
}

private struct DiagnosticRow: View {
    let label: String
    let value: String

    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(label)
                .foregroundStyle(.secondary)
            Spacer(minLength: 16)
            Text(value)
                .multilineTextAlignment(.trailing)
        }
    }
}

private enum DiagnosticsConfirmation: Identifiable {
    case fullResync
    case clearCache
    case clearLogs

    var id: String {
        switch self {
        case .fullResync:
            "fullResync"
        case .clearCache:
            "clearCache"
        case .clearLogs:
            "clearLogs"
        }
    }

    var title: String {
        switch self {
        case .fullResync:
            "Force full resync?"
        case .clearCache:
            "Clear cached Google data?"
        case .clearLogs:
            "Clear local logs?"
        }
    }

    var message: String {
        switch self {
        case .fullResync:
            "This clears local sync checkpoints and asks Google for fresh task and calendar state. It keeps your cached data visible during the refresh."
        case .clearCache:
            "This removes cached task lists, tasks, calendars, events, checkpoints, and pending local writes from this device, then refreshes from Google if connected. Your Google account data is not deleted."
        case .clearLogs:
            "This removes local app log files and the in-memory log view on this Mac. It does not change Google data, cached tasks, events, or mutation history."
        }
    }

    var actionTitle: String {
        switch self {
        case .fullResync:
            "Force Full Resync"
        case .clearCache:
            "Clear Cache"
        case .clearLogs:
            "Clear Logs"
        }
    }

    var role: ButtonRole? {
        switch self {
        case .fullResync:
            nil
        case .clearCache, .clearLogs:
            .destructive
        }
    }
}

private enum Clipboard {
    static func copy(_ value: String) {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(value, forType: .string)
    }
}

#Preview {
    DiagnosticsView()
        .environment(AppModel.preview)
}
