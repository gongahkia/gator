import AppKit
import UniformTypeIdentifiers
import CoreSpotlight
import SwiftUI

private struct UpdatePromptWindowObserver: ViewModifier {
    let sequence: Int
    let openWindow: OpenWindowAction

    func body(content: Content) -> some View {
        content.onChange(of: sequence) { _, _ in
            openWindow(id: "update-available")
        }
    }
}

private struct InstallGuideWindowObserver: ViewModifier {
    let sequence: Int
    let openWindow: OpenWindowAction

    func body(content: Content) -> some View {
        content.onChange(of: sequence) { _, _ in
            openWindow(id: "install-update")
        }
    }
}

private struct ActionCenterRefreshObserver: ViewModifier {
    let isPresented: Bool
    let displayRevision: String
    let refresh: () -> Void

    func body(content: Content) -> some View {
        content
            .onChange(of: isPresented) { _, isPresented in
                if isPresented {
                    refresh()
                }
            }
            .onChange(of: displayRevision) { _, _ in
                refresh()
            }
    }
}

private struct UpdaterToastModifier: ViewModifier {
    @Environment(UpdaterController.self) private var updater

    private var message: Binding<String?> {
        Binding(
            get: {
                guard updater.toastState?.target == .main else { return nil }
                return updater.toastState?.message
            },
            set: { newValue in
                if newValue == nil {
                    updater.clearToast()
                }
            }
        )
    }

    func body(content: Content) -> some View {
        let toast = updater.toastState?.target == .main ? updater.toastState : nil
        let title = toast?.title ?? "Update check complete"
        return content.overlay {
            BulkResultToast(
                message: message,
                isWarning: toast?.isWarning ?? false,
                successTitle: title,
                warningTitle: title,
                successSymbol: "arrow.down.circle.fill",
                warningSymbol: "wifi.exclamationmark"
            )
        }
    }
}

private struct SettingsTransferPresentationModifier: ViewModifier {
    @Binding var message: String?
    @Binding var isWarning: Bool
    @Binding var importPreview: SettingsImportPreview?
    @Binding var pendingImport: SettingsTransferBundle?
    let applyImport: () -> Void

    private var isImportPresented: Binding<Bool> {
        Binding(
            get: { importPreview != nil },
            set: { isPresented in
                if isPresented == false {
                    importPreview = nil
                    pendingImport = nil
                }
            }
        )
    }

    func body(content: Content) -> some View {
        content
            .confirmationDialog(
                "Import Settings?",
                isPresented: isImportPresented,
                titleVisibility: .visible,
                actions: {
                    if let importPreview {
                        importActions(importPreview)
                    }
                },
                message: {
                    Text(importPreview?.message ?? "")
                }
            )
            .overlay {
                BulkResultToast(
                    message: $message,
                    isWarning: isWarning,
                    successTitle: "Settings transfer complete",
                    warningTitle: "Settings transfer failed",
                    successSymbol: "gearshape.fill"
                )
            }
    }

    @ViewBuilder
    private func importActions(_ preview: SettingsImportPreview) -> some View {
        if preview.changeCount == 0 {
            Button("Import Settings", action: applyImport)
        } else {
            Button("Import Settings", role: .destructive, action: applyImport)
        }
        Button("Cancel", role: .cancel) {
            pendingImport = nil
        }
    }
}

private struct NavigationSurfaceToggleToolbarModifier: ViewModifier {
    let placement: NavigationSurfacePlacement
    let isPresented: Bool
    let toggle: () -> Void

    private var toolbarPlacement: ToolbarItemPlacement {
        switch placement {
        case .right:
            return .primaryAction
        case .left, .top, .bottom:
            return .navigation
        }
    }

    private var label: String {
        isPresented ? "Hide navigation" : "Show navigation"
    }

    func body(content: Content) -> some View {
        switch placement {
        case .left:
            content
        case .right, .top, .bottom:
            content
                .toolbar(removing: .sidebarToggle)
                .toolbar {
                    ToolbarItem(placement: toolbarPlacement) {
                        Button(action: toggle) {
                            Image(systemName: placement.systemImage)
                                .imageScale(.large)
                        }
                        .help(label)
                        .accessibilityLabel(label)
                    }
                }
        }
    }
}

private struct ActionCenterDynamicContent: Equatable, Sendable {
    var holdGroups: [ActionCenterHoldGroup]
    var overdueTasks: [TaskMirror]
    var overdueTaskCount: Int

    static let empty = ActionCenterDynamicContent(
        holdGroups: [],
        overdueTasks: [],
        overdueTaskCount: 0
    )
}

private struct PersistentSidebarSurfaceHost: View {
    let selection: SidebarItem
    let tabRouter: TabRouter
    let calendarEventViewFilter: CalendarEventViewFilter
    let activeSidebarTransition: HCBTransitionMeasurement?

    var body: some View {
        ZStack {
            ForEach(SidebarItem.allCases) { item in
                surface(for: item)
                    .opacity(selection == item ? 1 : 0)
                    .allowsHitTesting(selection == item)
                    .accessibilityHidden(selection != item)
                    .zIndex(selection == item ? 1 : 0)
            }
        }
        .animation(.easeOut(duration: 0.10), value: selection)
    }

    private func surface(for item: SidebarItem) -> some View {
        let routerKey = item.rawValue
        let router = tabRouter.router(for: routerKey)
        return NavigationStack(path: tabRouter.binding(for: routerKey)) {
            item.makeContentView(router: router)
                .environment(\.calendarEventViewFilter, calendarEventViewFilter)
                .hcbTransitionFirstContent(
                    selection == item ? activeSidebarTransition : nil,
                    metadata: ["surface": item.rawValue]
                )
                .withAppDestinations()
        }
        .environment(\.routerPath, router)
        .withSheetDestinations(router: router)
    }
}

struct MacSidebarShell: View {
    @Environment(AppModel.self) private var model
    @Environment(UpdaterController.self) private var updater
    @Environment(NetworkMonitor.self) private var networkMonitor
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.openSettings) private var openSettings
    @Environment(\.openWindow) private var openWindow
    @Environment(\.hcbReduceMotion) private var reduceMotion
    @Environment(\.accessibilityReduceMotion) private var systemReduceMotion
    @SceneStorage("sidebarSelection") private var storedSelection: String = SidebarItem.calendar.rawValue
    @AppStorage(CalendarViewFilterState.storageKey) private var storedCalendarViewFilters: String = ""
    @AppStorage("calendar.sidebarFilters.collapsed") private var calendarSidebarFiltersCollapsed = false

    private let layoutScaleMin: Double = 0.80
    private let layoutScaleMax: Double = 1.50
    private let layoutScaleStep: Double = 0.05

    private var settingsStore: SettingsStore { model.settingsStore }
    private var syncStatusStore: SyncStatusStore { model.syncStatusStore }
    private var taskStore: TaskStore { model.taskStore }
    private var calendarStore: CalendarStore { model.calendarStore }
    private var menuBarStore: MenuBarStore { model.menuBarStore }
    private var shellChrome: ShellChromeSettings { settingsStore.shellChrome }
    private var statusBanner: StatusBannerSnapshot { syncStatusStore.banner }

    private var layoutZoomScale: CGFloat {
        CGFloat(max(layoutScaleMin, min(shellChrome.uiLayoutScale, layoutScaleMax)))
    }

    private var effectiveReduceMotion: Bool {
        reduceMotion || systemReduceMotion || shellChrome.disableAnimations
    }

    private var textSizePoints: Double {
        HCBTextSize.clamp(shellChrome.uiTextSizePoints)
    }

    // Keep the source list compact by default while still allowing the system
    // split-view grip to breathe with larger text or sidebar icon settings.
    private let sidebarMinWidthBase: CGFloat = 160
    private let sidebarIdealWidthBase: CGFloat = 188
    private let sidebarMaxWidthBase: CGFloat = 240
    // Top inset reserved so the traffic-light buttons don't overlap the
    // first sidebar row when the window chrome sits over the sidebar.
    private let trafficLightInsetBase: CGFloat = 28

    private var sidebarMinWidth: CGFloat { sidebarMinWidthBase * layoutZoomScale }
    private var sidebarIdealWidth: CGFloat { sidebarIdealWidthBase * layoutZoomScale }
    private var sidebarMaxWidth: CGFloat { sidebarMaxWidthBase * layoutZoomScale }
    private var trafficLightInset: CGFloat { trafficLightInsetBase * layoutZoomScale }

    @State private var selection: SidebarItem = .calendar
    @State private var sidebarVisibility: NavigationSplitViewVisibility = .all
    @State private var tabRouter = TabRouter()
    @State private var isPresentingOnboarding = false
    @State private var commandPalettePanelController = CommandPalettePanelController()
    @State private var isPresentingInsertTemplate = false
    @State private var isPresentingInsertEventTemplate = false // §6.13b
    @State private var appCommandActions = AppCommandActions()
    @State private var appShortcutMonitor: Any?
    @State private var deepLinkErrorMessage: String?
    @State private var clipboardToastMessage: String?
    @State private var settingsTransferMessage: String?
    @State private var settingsTransferIsWarning = false
    @State private var settingsImportPreview: SettingsImportPreview?
    @State private var pendingSettingsImport: SettingsTransferBundle?
    @State private var isPresentingFeatureTour = false
    @State private var isActionCenterPresented = false
    @State private var actionCenterDynamicContent = ActionCenterDynamicContent.empty
    @State private var actionCenterRefreshTask: Task<Void, Never>?
    @State private var isCustomNavigationSurfacePresented = true
    // Leader-key chord state (§6.9). `nil` = inactive; non-nil = collecting
    // keys after a ⌘K press. timeoutTask cancels the collecting state after
    // 3s of inactivity so a stray leader press doesn't lock out single-key
    // typing forever.
    @State private var chordKeys: [String]?
    @State private var chordTimeoutTask: Task<Void, Never>?
    @State private var isMainWindowFocused = true
    @State private var activeSidebarTransition: HCBTransitionMeasurement?

    @ViewBuilder
    private var shellCore: some View {
        Group {
            switch shellChrome.sidebarPlacement {
            case .left:
                nativeLeftNavigationShell
            case .right:
                trailingNavigationShell
            case .top, .bottom:
                horizontalNavigationShell(placement: shellChrome.sidebarPlacement)
            }
        }
        .animation(HCBMotion.animation(.easeInOut(duration: 0.12), reduceMotion: effectiveReduceMotion), value: shellChrome.sidebarPlacement)
        .animation(HCBMotion.animation(.easeInOut(duration: 0.12), reduceMotion: effectiveReduceMotion), value: layoutZoomScale)
        .animation(HCBMotion.animation(.easeInOut(duration: 0.12), reduceMotion: effectiveReduceMotion), value: textSizePoints)
    }

    private var nativeLeftNavigationShell: some View {
        NavigationSplitView(columnVisibility: $sidebarVisibility) {
            sidebar
        } detail: {
            detail
        }
        .navigationSplitViewStyle(.balanced)
    }

    private var trailingNavigationShell: some View {
        HStack(spacing: 0) {
            detail
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            if isCustomNavigationSurfacePresented {
                Divider()
                trailingSidebar
            }
        }
    }

    @ViewBuilder
    private func horizontalNavigationShell(placement: NavigationSurfacePlacement) -> some View {
        VStack(spacing: 0) {
            if placement == .top {
                if isCustomNavigationSurfacePresented {
                    horizontalNavigationBar
                    Divider()
                }
                detail
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                detail
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                if isCustomNavigationSurfacePresented {
                    Divider()
                    horizontalNavigationBar
                }
            }
        }
    }

    var body: some View {
        shellCore
            .id(shellChrome.colorSchemeID) // force re-render so AppColor.X picks up the new palette
            .environment(\.locale, shellChrome.appLanguage.locale)
            .withHCBAppearance(shellChrome)
            .environment(\.hcbShortcutOverrides, shellChrome.shortcutOverrides)
            .hcbPreferredColorScheme(shellChrome)
            .modifier(NavigationSurfaceToggleToolbarModifier(
                placement: shellChrome.sidebarPlacement,
                isPresented: isNavigationSurfacePresented,
                toggle: toggleNavigationSurface
            ))
            .appBackground()
            .safeAreaInset(edge: .top) {
                AppStatusBanner(
                    syncState: statusBanner.syncState,
                    authState: statusBanner.authState,
                    mutationError: statusBanner.mutationError,
                    cacheWarning: statusBanner.cacheWarning,
                    isSyncPaused: statusBanner.isSyncPaused,
                    quarantinedCount: statusBanner.quarantinedCount,
                    invalidPayloadCount: statusBanner.invalidPayloadCount,
                    conflictCount: statusBanner.conflictCount,
                    deferredReminderSummary: statusBanner.deferredReminderSummary,
                    syncFailureKind: statusBanner.syncFailureKind,
                    networkReachability: networkMonitor.reachability,
                    daysSinceLastLaunch: statusBanner.daysSinceLastLaunch,
                    syncScope: statusBanner.syncScope,
                    openSyncIssues: { openWindow(id: "sync-issues") },
                    retry: {
                        model.resumeSync()
                        Task { await model.refreshNow() }
                    },
                    dismiss: { model.clearFailureState() }
                )
            }
            .sheet(isPresented: $isPresentingOnboarding) {
                OnboardingView()
                    .environment(model)
                    .withHCBAppearance(shellChrome)
            }
            .sheet(
                isPresented: $isPresentingFeatureTour,
                onDismiss: { model.markFeatureTourSeen() }
            ) {
                FeatureTourView(
                    dismiss: {
                        isPresentingFeatureTour = false
                        model.markFeatureTourSeen()
                    },
                    openHelp: {
                        isPresentingFeatureTour = false
                        model.markFeatureTourSeen()
                        openWindow(id: "help")
                    }
                )
                .withHCBAppearance(shellChrome)
            }
            .sheet(isPresented: $isPresentingInsertTemplate) {
                InsertTaskTemplateSheet()
                    .environment(model)
                    .withHCBAppearance(shellChrome)
            }
            .sheet(isPresented: $isPresentingInsertEventTemplate) {
                InsertEventTemplateSheet()
                    .environment(model)
                    .withHCBAppearance(shellChrome)
            }
            .modifier(SettingsTransferPresentationModifier(
                message: $settingsTransferMessage,
                isWarning: $settingsTransferIsWarning,
                importPreview: $settingsImportPreview,
                pendingImport: $pendingSettingsImport,
                applyImport: applyPendingSettingsImport
            ))
            .overlay {
                UndoToast()
            }
            .overlay {
                DeepLinkErrorToast(message: $deepLinkErrorMessage)
            }
            .overlay {
                BulkResultToast(
                    message: $clipboardToastMessage,
                    successTitle: "Copied",
                    successSymbol: "doc.on.clipboard"
                )
            }
            .modifier(UpdaterToastModifier())
            .overlay(alignment: .bottomTrailing) {
                if let keys = chordKeys {
                    ChordHUD(
                        currentKeys: keys,
                        hints: HCBChordMatcher.hudHints(current: keys, in: HCBChordRegistry.defaults)
                    )
                    .hcbScaledPadding(18)
                    .transition(HCBMotion.transition(.move(edge: .bottom).combined(with: .opacity), reduceMotion: effectiveReduceMotion))
                }
            }
            .modifier(ActionCenterPresentationModifier(
                snapshot: actionCenterSnapshot,
                isPresented: $isActionCenterPresented,
                reduceMotion: effectiveReduceMotion,
                onOpenHold: openActionCenterHold,
                onConfirmHold: confirmActionCenterHold,
                onCancelHoldGroup: cancelActionCenterHoldGroup,
                onOpenTask: openActionCenterTask,
                onCompleteTask: completeActionCenterTask,
                onSnoozeTask: snoozeActionCenterTask,
                onOpenSyncIssues: openActionCenterSyncIssues,
                onOpenSettings: openActionCenterSettings,
                onRetrySync: retryActionCenterSync,
                onDismissStatus: { model.clearFailureState() }
            ))
            .loadingOverlay(model.loadingOverlay)
            .transaction { transaction in
                if model.loadingOverlay != nil {
                    transaction.animation = nil
                } else if case .syncing = statusBanner.syncState {
                    transaction.animation = nil
                }
            }
            .animation(HCBMotion.animation(.easeOut(duration: 0.16), reduceMotion: effectiveReduceMotion), value: chordKeys)
            .focusedSceneValue(\.appCommandActions, appCommandActions)
            .onAppear(perform: handleShellAppear)
            .onChange(of: shellChrome.hasCompletedOnboarding) { _, _ in
                presentFeatureTourIfNeeded()
            }
            .onReceive(NotificationCenter.default.publisher(for: .hcbOpenStoreTab)) { _ in
                selectSidebarItem(.store, source: "notification.openStore")
            }
            .onReceive(NotificationCenter.default.publisher(for: .hcbOpenNotesTab)) { _ in
                selectSidebarItem(.notes, source: "notification.openNotes")
            }
            .onReceive(NotificationCenter.default.publisher(for: .hcbOpenSettingsWindow)) { _ in
                openInstrumentedSettings(source: "notification")
            }
            .onReceive(NotificationCenter.default.publisher(for: .hcbRevealTaskInStore)) { note in
                guard let taskID = note.object as? TaskMirror.ID else { return }
                let targetTab: SidebarItem = taskStore.task(id: taskID)?.dueDate == nil ? .notes : .store
                selectSidebarItem(targetTab, source: "notification.revealTask")
                tabRouter.router(for: sidebarItemKey(targetTab)).present(.editTask(taskID))
            }
            .onReceive(NotificationCenter.default.publisher(for: .hcbRevealEventInCalendar)) { note in
                guard let eventID = note.object as? CalendarEventMirror.ID else { return }
                selectSidebarItem(.calendar, source: "notification.revealEvent")
                tabRouter.router(for: sidebarItemKey(.calendar)).present(.editEvent(eventID))
            }
            .onReceive(NotificationCenter.default.publisher(for: .hcbClipboardMessage)) { note in
                clipboardToastMessage = (note.object as? String) ?? "Copied to clipboard."
            }
            .onDisappear {
                actionCenterRefreshTask?.cancel()
                uninstallAppShortcutMonitor()
            }
            .onChange(of: shellChrome.colorSchemeID, initial: true) { _, newID in
                HCBColorSchemeStore.current = HCBColorScheme.scheme(id: newID, customSchemes: shellChrome.customColorSchemes) ?? .notion
            }
            .onChange(of: shellChrome.enableGlobalHotkey) { _, newValue in
                guard let delegate = NSApp.delegate as? AppDelegate else { return }
                delegate.appModel = model
                let state = delegate.configureGlobalHotkey(
                    enabled: newValue,
                    binding: shellChrome.globalHotkeyBinding
                )
                if newValue == false, model.globalHotkeyRegistrationState == .needsAccessibilityPermission {
                    return
                }
                model.setGlobalHotkeyRegistrationState(state)
            }
            .onChange(of: shellChrome.globalHotkeyBinding) { _, newValue in
                guard let delegate = NSApp.delegate as? AppDelegate else { return }
                delegate.appModel = model
                let state = delegate.configureGlobalHotkey(
                    enabled: shellChrome.enableGlobalHotkey,
                    binding: newValue
                )
                model.setGlobalHotkeyRegistrationState(state)
            }
            .onChange(of: selection) { _, newValue in
                activeSidebarTransition?.scheduleSettled(
                    after: .milliseconds(260),
                    metadata: ["surface": newValue.rawValue]
                )
                storedSelection = newValue.rawValue
                configureCommandActions()
            }
            .onChange(of: shellChrome.hiddenSidebarItems) { _, hidden in
                // if the currently-selected tab just got hidden, fall back
                // to the first still-visible item so the detail pane isn't
                // rendering a tab that's no longer in the sidebar.
                if hidden.contains(selection.rawValue), selection.isHideable,
                   let first = visibleSidebarItems.first {
                    selectSidebarItem(first, source: "hiddenSidebarFallback")
                }
            }
            .modifier(UpdatePromptWindowObserver(sequence: updater.updatePromptSequence, openWindow: openWindow))
            .modifier(InstallGuideWindowObserver(sequence: updater.installGuideSequence, openWindow: openWindow))
            .modifier(ActionCenterRefreshObserver(
                isPresented: isActionCenterPresented,
                displayRevision: actionCenterDisplayRevision,
                refresh: refreshActionCenterDynamicContent
            ))
            .modifier(ShellZoomObservers(zoomIn: performZoomIn, zoomOut: performZoomOut, zoomReset: performZoomReset))
            .modifier(MainWindowFocusObserver(isFocused: $isMainWindowFocused))
            .task {
                await performInitialLoad()
                await runTransitionProfileScenarioIfNeeded()
            }
            .task { await updater.performAutomaticCheckIfNeeded() }
            .onChange(of: shellChrome.hasCompletedOnboarding) { _, hasCompleted in
                // Re-present onboarding when the flag is flipped back
                // (Settings → "Run setup again"); dismiss when the
                // onboarding flow marks itself completed.
                if hasCompleted {
                    isPresentingOnboarding = false
                } else {
                    isPresentingOnboarding = true
                }
            }
            .task(id: nearRealtimeLoopID) {
                await runNearRealtimeSyncLoop()
            }
            .onChange(of: scenePhase) { _, newPhase in
                handleScenePhaseChange(newPhase)
            }
            .onOpenURL(perform: handleIncomingURL)
            .onContinueUserActivity(CSSearchableItemActionType) { userActivity in
                handleSpotlightActivity(userActivity)
            }
    }

    private func routeQuickSwitcherEntity(_ entity: QuickSwitcherEntity) {
        switch entity {
        case .task(let task):
            selectSidebarItem(.store, source: "quickSwitcher.task")
            tabRouter.router(for: sidebarItemKey(.store)).present(.editTask(task.id))
        case .event(let event):
            selectSidebarItem(.calendar, source: "quickSwitcher.event")
            tabRouter.router(for: sidebarItemKey(.calendar)).present(.editEvent(event.id))
        case .taskList:
            selectSidebarItem(.store, source: "quickSwitcher.taskList")
        case .calendar:
            // Calendar tab has no per-calendar scope control; just land there.
            selectSidebarItem(.calendar, source: "quickSwitcher.calendar")
        case .customFilter(let f):
            selectSidebarItem(.store, source: "quickSwitcher.customFilter")
            model.markCustomFilterUsed(f.id)
        }
    }

    private var actionCenterSnapshot: ActionCenterSnapshot {
        ActionCenterBuilder.build(
            holdGroups: actionCenterDynamicContent.holdGroups,
            overdueTasks: actionCenterDynamicContent.overdueTasks,
            overdueTaskCount: actionCenterDynamicContent.overdueTaskCount,
            pendingMutations: model.pendingMutations,
            notificationSummary: statusBanner.deferredReminderSummary,
            authState: statusBanner.authState,
            syncState: statusBanner.syncState,
            isSyncPaused: statusBanner.isSyncPaused,
            mutationError: statusBanner.mutationError,
            syncFailureKind: statusBanner.syncFailureKind,
            networkReachability: networkMonitor.reachability
        )
    }

    private var actionCenterDisplayRevision: String {
        [
            "\(calendarStore.calendarDisplayRevision)",
            "\(taskStore.taskDisplayRevision)",
            "\(taskStore.notesDisplayRevision)",
            "\(statusBanner.pendingMutationCount)",
            "\(statusBanner.conflictCount)",
            "\(statusBanner.quarantinedCount)"
        ].joined(separator: ":")
    }

    private func refreshActionCenterDynamicContent() {
        let revision = actionCenterDisplayRevision
        let visibleTaskListIDs = taskStore.visibleTaskListIDs
        let overdueLimit = ActionCenterBuilder.defaultOverdueTaskDisplayLimit
        let today = Calendar.current.startOfDay(for: Date())

        actionCenterRefreshTask?.cancel()
        actionCenterRefreshTask = Task { @MainActor in
            let holdGroups = await model.availabilityHoldGroupsFromQuery()
            if Task.isCancelled { return }
            let overdue = await model.overdueTasksFromQuery(
                before: today,
                visibleTaskListIDs: visibleTaskListIDs,
                limit: overdueLimit
            )
            guard Task.isCancelled == false,
                  actionCenterDisplayRevision == revision else {
                return
            }
            actionCenterDynamicContent = ActionCenterDynamicContent(
                holdGroups: Self.actionCenterHoldGroups(from: holdGroups),
                overdueTasks: overdue.tasks,
                overdueTaskCount: overdue.totalCount
            )
        }
    }

    private static func actionCenterHoldGroups(from groups: [AvailabilityHoldGroup]) -> [ActionCenterHoldGroup] {
        groups
            .map { group in
                ActionCenterHoldGroup(
                    id: group.id,
                    metadata: group.metadata,
                    events: group.events
                )
            }
            .sorted { lhs, rhs in
                if lhs.nextStartDate == rhs.nextStartDate { return lhs.id < rhs.id }
                return lhs.nextStartDate < rhs.nextStartDate
            }
    }

    private func openActionCenterHold(_ hold: CalendarEventMirror) {
        selectSidebarItem(.calendar, source: "actionCenter.hold")
        tabRouter.router(for: sidebarItemKey(.calendar)).present(.editEvent(hold.id))
        isActionCenterPresented = false
    }

    private func confirmActionCenterHold(_ hold: CalendarEventMirror) {
        Task {
            _ = await model.confirmAvailabilityHold(hold)
        }
    }

    private func cancelActionCenterHoldGroup(_ group: ActionCenterHoldGroup) {
        Task {
            _ = await model.cancelAvailabilityHoldGroup(groupID: group.id)
        }
    }

    private func openActionCenterTask(_ task: TaskMirror) {
        let targetTab: SidebarItem = task.dueDate == nil ? .notes : .store
        selectSidebarItem(targetTab, source: "actionCenter.task")
        tabRouter.router(for: sidebarItemKey(targetTab)).present(.editTask(task.id))
        isActionCenterPresented = false
    }

    private func completeActionCenterTask(_ task: TaskMirror) {
        Task {
            _ = await model.setTaskCompleted(true, task: task)
        }
    }

    private func snoozeActionCenterTask(_ task: TaskMirror) {
        Task {
            let tomorrow = TaskSnoozeSupport.targetDate(daysFromToday: 1)
            _ = await model.updateTask(task, title: task.title, notes: task.notes, dueDate: tomorrow)
        }
    }

    private func openActionCenterSyncIssues() {
        openWindow(id: "sync-issues")
        isActionCenterPresented = false
    }

    private func openActionCenterSettings() {
        openInstrumentedSettings(source: "actionCenter")
        isActionCenterPresented = false
    }

    private func retryActionCenterSync() {
        model.resumeSync()
        Task { await model.refreshNow() }
    }

    private func handleScenePhaseChange(_ newPhase: ScenePhase) {
        switch newPhase {
        case .background, .inactive:
            // Force any in-flight debounced cache write to disk before the OS may suspend or terminate us.
            Task { await model.flushPendingCacheSave() }
        case .active:
            // Coming back to the foreground is an implicit "try again."
            model.resumeSync()
            if model.consumePendingSharedItems() {
                presentSheet(.quickAddTask, on: .store)
            }
            Task {
                await model.refreshForCurrentSyncMode()
                handlePendingAppIntentRoute()
            }
        @unknown default:
            break
        }
    }

    private func handleIncomingURL(_ url: URL) {
        // Deep-link scheme is routed to HCBDeepLinkRouter; everything else
        // (primarily the Google OAuth redirect) stays on the existing auth
        // path so sign-in is never intercepted.
        if url.scheme?.lowercased() == HCBDeepLinkRouter.scheme {
            handleDeepLink(url)
        } else {
            model.handleAuthRedirect(url)
        }
    }

    private func handleDeepLink(_ url: URL) {
        switch HCBDeepLinkRouter.route(url) {
        case .success(let action):
            dispatchDeepLinkAction(action)
        case .failure(let err):
            deepLinkErrorMessage = err.message
        }
    }

    private func dispatchDeepLinkAction(_ action: HCBDeepLinkAction) {
        switch action {
        case .openApp:
            HCBMainWindowPresenter.shared.show()
        case .openTask(let id):
            selectSidebarItem(.store, source: "deeplink.task")
            tabRouter.router(for: sidebarItemKey(.store)).present(.editTask(id))
        case .openEvent(let id):
            selectSidebarItem(.calendar, source: "deeplink.event")
            tabRouter.router(for: sidebarItemKey(.calendar)).present(.editEvent(id))
        case .newTask(let prefill):
            // Stage the prefill before presenting — AddTaskSheet reads model
            // state on .task and nils the prefill once consumed.
            model.pendingTaskPrefill = prefill
            presentSheet(.addTask, on: .store)
        case .newEvent(let prefill):
            model.pendingEventPrefill = prefill
            presentSheet(.addEvent, on: .calendar)
        case .search(let query):
            model.pendingPaletteQuery = query
            presentCommandPalette()
        }
    }

    private func handleSpotlightActivity(_ activity: NSUserActivity) {
        guard let uniqueID = activity.userInfo?[CSSearchableItemActivityIdentifier] as? String,
              let identifier = SpotlightIdentifier(uniqueIdentifier: uniqueID) else {
            return
        }

        switch identifier {
        case .task(let id):
            selectSidebarItem(.store, source: "spotlight.task")
            tabRouter.router(for: sidebarItemKey(.store)).present(.editTask(id))
        case .event(let id):
            selectSidebarItem(.calendar, source: "spotlight.event")
            tabRouter.router(for: sidebarItemKey(.calendar)).present(.editEvent(id))
        }
    }

    private var sidebar: some View {
        expandedSidebar
            .navigationSplitViewColumnWidth(
                min: sidebarMinWidth,
                ideal: sidebarIdealWidth,
                max: sidebarMaxWidth
            )
            .hcbSurface(.sidebar) // §6.11 per-surface font override
    }

    // SidebarItem.allCases filtered by user-hidden set. Keep a fallback so
    // imported or legacy settings cannot render the window with no tabs.
    private var visibleSidebarItems: [SidebarItem] {
        let visible = SidebarItem.allCases.filter { item in
            item.isHideable == false || shellChrome.hiddenSidebarItems.contains(item.rawValue) == false
        }
        return visible.isEmpty ? [.calendar] : visible
    }

    private var expandedSidebar: some View {
        VStack(spacing: 0) {
            // Reserved gap so the traffic-light window buttons don't overlap
            // the first row.
            Color.clear
                .frame(height: trafficLightInset)
            List(selection: sidebarSelectionBinding) {
                ForEach(visibleSidebarItems) { item in
                    sidebarRow(for: item)
                    if item == .calendar, shouldShowCalendarSidebarFilters {
                        CalendarSidebarFilters(state: calendarViewFilterStateBinding)
                            .listRowInsets(EdgeInsets(top: 0, leading: 18, bottom: 6, trailing: 12))
                            .listRowSeparator(.hidden)
                    }
                }
            }
            .listStyle(.sidebar)
            .scrollContentBackground(.hidden)
            if shellChrome.cacheEncryptionEnabled {
                Divider()
                encryptedCacheFooter
            }
        }
        .frame(
            minWidth: sidebarMinWidth,
            idealWidth: sidebarIdealWidth,
            maxWidth: sidebarMaxWidth,
            alignment: .topLeading
        )
    }

    private var trailingSidebar: some View {
        VStack(spacing: 0) {
            ScrollView {
                VStack(alignment: .leading, spacing: 4) {
                    ForEach(visibleSidebarItems) { item in
                        verticalNavigationButton(for: item)
                        if item == .calendar, shouldShowCalendarSidebarFilters {
                            CalendarSidebarFilters(state: calendarViewFilterStateBinding)
                                .hcbScaledPadding(.leading, 8)
                                .hcbScaledPadding(.trailing, 4)
                                .hcbScaledPadding(.bottom, 6)
                        }
                    }
                }
                .hcbScaledPadding(.horizontal, 10)
                .hcbScaledPadding(.vertical, 12)
            }
            .scrollIndicators(.hidden)
            if shellChrome.cacheEncryptionEnabled {
                Divider()
                encryptedCacheFooter
            }
        }
        .frame(
            minWidth: sidebarMinWidth,
            idealWidth: sidebarIdealWidth,
            maxWidth: sidebarMaxWidth,
            alignment: .topLeading
        )
        .background(.bar)
        .hcbSurface(.sidebar)
    }

    private var horizontalNavigationBar: some View {
        HStack(spacing: 8) {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 6) {
                    ForEach(visibleSidebarItems) { item in
                        horizontalNavigationButton(for: item)
                    }
                }
            }
            if shellChrome.cacheEncryptionEnabled {
                Divider()
                    .hcbScaledFrame(height: 24)
                Label("Cache encrypted", systemImage: "lock.fill")
                    .labelStyle(.iconOnly)
                    .foregroundStyle(AppColor.moss)
                    .help("Local cache encryption uses a key stored in macOS Keychain. There is no separate app idle timer; access follows your Mac lock state.")
                    .accessibilityLabel("Cache encrypted")
            }
        }
        .hcbScaledPadding(.horizontal, 12)
        .hcbScaledPadding(.vertical, 8)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.bar)
        .hcbSurface(.sidebar)
    }

    private func verticalNavigationButton(for item: SidebarItem) -> some View {
        let isSelected = selection == item
        return Button {
            selectSidebarItem(item, source: "sidebar.vertical")
        } label: {
            HStack(spacing: 8) {
                Image(systemName: item.systemImage)
                    .frame(width: 20, alignment: .center)
                Text(item.title)
                    .lineLimit(1)
                Spacer(minLength: 8)
                if let badgeValue = badge(for: item) {
                    navigationBadge(badgeValue, isSelected: isSelected)
                }
            }
            .hcbFont(.body, weight: isSelected ? .semibold : .regular)
            .foregroundColor(isSelected ? AppColor.ember : .primary)
            .hcbScaledPadding(.horizontal, 10)
            .hcbScaledPadding(.vertical, 7)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(
                RoundedRectangle(cornerRadius: 8, style: .continuous)
                    .fill(isSelected ? AppColor.ember.opacity(0.14) : Color.clear)
            )
            .contentShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
        }
        .buttonStyle(.plain)
        .help(sidebarHelp(for: item))
        .accessibilityLabel(navigationAccessibilityLabel(for: item))
        .accessibilityAddTraits(isSelected ? .isSelected : [])
        // Keep sidebar actions available when the sidebar is in compact icon-only mode.
        .contextMenu {
            sidebarContextMenu(for: item)
        }
    }

    private func horizontalNavigationButton(for item: SidebarItem) -> some View {
        let isSelected = selection == item
        return Button {
            selectSidebarItem(item, source: "sidebar.horizontal")
        } label: {
            HStack(spacing: 6) {
                Image(systemName: item.systemImage)
                    .frame(width: 18, alignment: .center)
                Text(item.title)
                    .lineLimit(1)
                if let badgeValue = badge(for: item) {
                    navigationBadge(badgeValue, isSelected: isSelected)
                }
            }
            .hcbFont(.subheadline, weight: isSelected ? .semibold : .regular)
            .foregroundColor(isSelected ? AppColor.ember : .primary)
            .hcbScaledPadding(.horizontal, 10)
            .hcbScaledPadding(.vertical, 6)
            .hcbScaledFrame(minWidth: 92, minHeight: 30, alignment: .center)
            .background(
                RoundedRectangle(cornerRadius: 8, style: .continuous)
                    .fill(isSelected ? AppColor.ember.opacity(0.14) : Color.clear)
            )
            .contentShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
        }
        .buttonStyle(.plain)
        .help(sidebarHelp(for: item))
        .accessibilityLabel(navigationAccessibilityLabel(for: item))
        .accessibilityAddTraits(isSelected ? .isSelected : [])
        .contextMenu {
            sidebarContextMenu(for: item)
        }
    }

    private func navigationBadge(_ value: String, isSelected: Bool) -> some View {
        Text(value)
            .hcbFont(.caption2, weight: .semibold)
            .monospacedDigit()
            .foregroundColor(isSelected ? AppColor.ember : .secondary)
            .hcbScaledPadding(.horizontal, 6)
            .hcbScaledPadding(.vertical, 1)
            .background(
                Capsule()
                    .fill(isSelected ? AppColor.ember.opacity(0.18) : Color.secondary.opacity(0.12))
            )
            .accessibilityHidden(true)
    }

    private var encryptedCacheFooter: some View {
        Label {
            VStack(alignment: .leading, spacing: 2) {
                Text("Cache encrypted")
                    .hcbFont(.caption, weight: .semibold)
                Text("Locks when Mac locks")
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
            }
        } icon: {
            Image(systemName: "lock.fill")
                .foregroundStyle(AppColor.moss)
        }
        .labelStyle(.titleAndIcon)
        .frame(maxWidth: .infinity, alignment: .leading)
        .hcbScaledPadding(.horizontal, 14)
        .hcbScaledPadding(.vertical, 10)
        .help("Local cache encryption uses a key stored in macOS Keychain. There is no separate app idle timer; access follows your Mac lock state.")
    }

    @ViewBuilder
    private var detail: some View {
        PersistentSidebarSurfaceHost(
            selection: selection,
            tabRouter: tabRouter,
            calendarEventViewFilter: calendarEventViewFilter,
            activeSidebarTransition: activeSidebarTransition
        )
    }

    private var calendarViewFilterStateBinding: Binding<CalendarViewFilterState> {
        Binding(
            get: { CalendarViewFilterState.decoded(from: storedCalendarViewFilters) },
            set: { storedCalendarViewFilters = $0.encodedString() }
        )
    }

    private var calendarEventViewFilter: CalendarEventViewFilter {
        CalendarEventViewFilter(
            state: CalendarViewFilterState.decoded(from: storedCalendarViewFilters),
            colorTagBindings: shellChrome.colorTagBindings
        )
    }

    private var shouldShowCalendarSidebarFilters: Bool {
        selection == .calendar && calendarStore.account != nil && calendarSidebarFiltersCollapsed == false
    }

    private var hasWritableCalendars: Bool {
        calendarStore.calendars.contains { $0.accessRole == "owner" || $0.accessRole == "writer" }
    }

    private var sidebarSelectionBinding: Binding<SidebarItem?> {
        Binding(
            get: { selection },
            set: { newValue in
                if let newValue {
                    selectSidebarItem(newValue, source: "sidebar.listSelection")
                }
            }
        )
    }

    private var nearRealtimeLoopID: String {
        [
            shellChrome.syncMode.rawValue,
            scenePhase == .active ? "active" : "inactive",
            isMainWindowFocused ? "focused" : "unfocused",
            calendarStore.account?.id ?? "signed-out"
        ].joined(separator: ":")
    }

    private var isNavigationSurfacePresented: Bool {
        if shellChrome.sidebarPlacement == .left {
            return sidebarVisibility != .detailOnly
        }
        return isCustomNavigationSurfacePresented
    }

    private func performInitialLoad() async {
        guard HCBLaunchMode.current.skipsAppStartupWork == false else {
            selection = SidebarItem(rawValue: storedSelection) ?? .calendar
            return
        }
        if HCBTransitionProfileScenario.current != nil {
            selection = SidebarItem(rawValue: storedSelection) ?? .calendar
            isPresentingOnboarding = false
            return
        }
        await model.loadInitialState()
        isPresentingOnboarding = shellChrome.hasCompletedOnboarding == false
        await model.restoreGoogleSession()
        if case .signedIn = model.authState {
            await model.refreshForCurrentSyncMode()
        }
        handlePendingAppIntentRoute()
    }

    private func handleShellAppear() {
        selection = SidebarItem(rawValue: storedSelection) ?? .calendar
        HCBColorSchemeStore.current = HCBColorScheme.scheme(id: shellChrome.colorSchemeID, customSchemes: shellChrome.customColorSchemes) ?? .notion
        // Pre-refactor builds persisted `hiddenSidebarItems` with a single
        // "store" entry meaning "Store (tasks+notes)". The split maps that
        // to the new paired set so a user who explicitly hid the old tab
        // doesn't get it re-shown as "Tasks" plus "Notes".
        if shellChrome.hiddenSidebarItems == ["store"] {
            var next = settingsStore.settings
            next.hiddenSidebarItems = ["store", "notes"]
            model.updateSettings(next)
        }
        configureCommandActions()
        configureGlobalHotkey()
        installAppShortcutMonitor()
        presentFeatureTourIfNeeded()
        refreshActionCenterDynamicContent()
    }

    private func presentFeatureTourIfNeeded() {
        guard shellChrome.hasCompletedOnboarding,
              shellChrome.hasSeenFeatureTour == false,
              isPresentingOnboarding == false
        else { return }
        isPresentingFeatureTour = true
    }

    private func configureGlobalHotkey() {
        guard let delegate = NSApp.delegate as? AppDelegate else { return }
        delegate.appModel = model
        let state = delegate.configureGlobalHotkey(
            enabled: shellChrome.enableGlobalHotkey,
            binding: shellChrome.globalHotkeyBinding
        )
        model.setGlobalHotkeyRegistrationState(state)
        if state == .needsAccessibilityPermission {
            model.setEnableGlobalHotkey(false)
        }
    }

    private func configureCommandActions() {
        appCommandActions.newTask = { presentSheet(.quickAddTask, on: .store) }
        appCommandActions.newNote = { presentSheet(.quickAddNote, on: .notes) }
        appCommandActions.newEvent = { presentSheet(.quickAddEvent, on: .calendar) }
        appCommandActions.refresh = { Task { await model.refreshNow() } }
        appCommandActions.forceResync = { Task { await model.forceFullResync() } }
        appCommandActions.switchTo = { item in
            // If the target tab is currently hidden via Layout settings, treat
            // the keyboard shortcut as a no-op rather than auto-unhide.
            guard visibleSidebarItems.contains(item) else { return }
            selectSidebarItem(item, source: "command.switchTo")
        }
        appCommandActions.toggleSidebar = { toggleNavigationSurface() }
        appCommandActions.openSettingsWindow = { openInstrumentedSettings(source: "command") }
        appCommandActions.openDiagnostics = { openInstrumentedWindow(id: "diagnostics", source: "command") }
        appCommandActions.toggleActionCenter = { isActionCenterPresented.toggle() }
        appCommandActions.openCommandPalette = { presentCommandPalette() }
        appCommandActions.openHelp = { openWindow(id: "help") }
        appCommandActions.openHistory = { openWindow(id: "history") }
        appCommandActions.printToday = { Task { await TodayPrinter.print(model: model) } }
        appCommandActions.exportDayICS = { exportICS(range: .day) }
        appCommandActions.exportWeekICS = { exportICS(range: .week) }
        appCommandActions.exportSettings = { exportSettings() }
        appCommandActions.importSettings = { importSettings() }
        appCommandActions.zoomIn = { performZoomIn() }
        appCommandActions.zoomOut = { performZoomOut() }
        appCommandActions.zoomReset = { performZoomReset() }
    }

    private func presentCommandPalette() {
        // Merged palette — actions *and* entity search (tasks, notes, events,
        // lists, calendars, saved filters). Presenting it as a lightweight
        // panel avoids the modal sheet animation on every Cmd-P.
        HCBTransitionProfiler.startStored(
            key: HCBTransitionKeys.commandPalette,
            name: "commandPalette.open",
            metadata: ["source": "commandPalette"]
        )
        commandPalettePanelController.present(
            model: model,
            commands: commandPaletteCommands,
            onSelectEntity: routeQuickSwitcherEntity
        )
    }

    private func toggleNavigationSurface() {
        if shellChrome.sidebarPlacement == .left {
            sidebarVisibility = sidebarVisibility == .detailOnly ? .all : .detailOnly
        } else {
            isCustomNavigationSurfacePresented.toggle()
        }
    }

    private func runTransitionProfileScenarioIfNeeded() async {
        guard let scenario = HCBTransitionProfileScenario.current else { return }
        let iterations = HCBTransitionProfileScenario.iterations
        let delay = HCBTransitionProfileScenario.stepDelay
        let startDelay = HCBTransitionProfileScenario.startDelay

        HCBTransitionProfiler.startStored(
            key: "profile.scenario",
            name: "profileScenario.\(scenario.rawValue)",
            metadata: [
                "iterations": String(iterations),
                "scenario": scenario.rawValue,
                "startDelayMs": String(HCBTransitionProfileScenario.startDelayMilliseconds)
            ]
        )

        await sleepForProfile(startDelay)
        switch scenario {
        case .sidebar:
            await runSidebarProfile(iterations: iterations, delay: delay)
        case .calendarModes:
            await runCalendarModeProfile(iterations: iterations, delay: delay)
        case .sheets:
            await runSheetProfile(iterations: iterations, delay: delay)
        case .commandPalette:
            await runCommandPaletteProfile(iterations: iterations, delay: delay)
        case .settingsDiagnostics:
            await runSettingsDiagnosticsProfile(iterations: iterations, delay: delay)
        case .all:
            await runSidebarProfile(iterations: iterations, delay: delay)
            await runCalendarModeProfile(iterations: iterations, delay: delay)
            await runSheetProfile(iterations: iterations, delay: delay)
            await runCommandPaletteProfile(iterations: iterations, delay: delay)
            await runSettingsDiagnosticsProfile(iterations: iterations, delay: delay)
        }

        HCBTransitionProfiler.markSettled(
            for: "profile.scenario",
            metadata: [
                "iterations": String(iterations),
                "scenario": scenario.rawValue
            ]
        )
    }

    private func runSidebarProfile(iterations: Int, delay: Duration) async {
        let surfaces: [SidebarItem] = [.calendar, .store, .notes]
        for index in 0..<iterations {
            selectSidebarItem(surfaces[index % surfaces.count], source: "profileScenario.sidebar")
            await sleepForProfile(delay)
        }
    }

    private func runCalendarModeProfile(iterations: Int, delay: Duration) async {
        selectSidebarItem(.calendar, source: "profileScenario.calendarModes")
        await sleepForProfile(delay)
        for _ in 0..<iterations {
            NotificationCenter.default.post(name: .hcbProfileNextCalendarMode, object: nil)
            await sleepForProfile(delay)
        }
    }

    private func runSheetProfile(iterations: Int, delay: Duration) async {
        let routes: [(SidebarItem, SheetDestination)] = [
            (.store, .quickAddTask),
            (.notes, .quickAddNote),
            (.calendar, .quickAddEvent)
        ]
        for index in 0..<iterations {
            let (item, sheet) = routes[index % routes.count]
            presentSheet(sheet, on: item)
            await sleepForProfile(delay)
            let router = tabRouter.router(for: sidebarItemKey(item))
            router.presentedSheet = nil
            router.cancelActiveSheetTransition(metadata: ["reason": "profileScenario.dismiss"])
            await sleepForProfile(.milliseconds(120))
        }
    }

    private func runCommandPaletteProfile(iterations: Int, delay: Duration) async {
        for _ in 0..<iterations {
            presentCommandPalette()
            await sleepForProfile(delay)
            commandPalettePanelController.close()
            await sleepForProfile(.milliseconds(120))
        }
    }

    private func runSettingsDiagnosticsProfile(iterations: Int, delay: Duration) async {
        for _ in 0..<iterations {
            openInstrumentedSettings(source: "profileScenario.settings")
            await sleepForProfile(delay)
            closeKeyAuxiliaryWindow()
            await sleepForProfile(.milliseconds(120))

            openInstrumentedWindow(id: "diagnostics", source: "profileScenario.diagnostics")
            await sleepForProfile(delay)
            closeKeyAuxiliaryWindow()
            await sleepForProfile(.milliseconds(120))
        }
    }

    private func closeKeyAuxiliaryWindow() {
        guard let window = NSApp.keyWindow, window.title != "Hot Cross Buns" else { return }
        window.performClose(nil)
    }

    private func sleepForProfile(_ duration: Duration) async {
        do {
            try await Task.sleep(for: duration)
        } catch {
            return
        }
    }

    private enum ICSRange {
        case day
        case week
    }

    private func exportICS(range: ICSRange) {
        let cal = Calendar.current
        let now = Date()
        let rangeStart: Date
        let rangeEnd: Date
        let suggestedFilename: String
        switch range {
        case .day:
            rangeStart = cal.startOfDay(for: now)
            rangeEnd = cal.date(byAdding: .day, value: 1, to: rangeStart) ?? rangeStart
            suggestedFilename = "hot-cross-buns-\(now.formatted(.iso8601.year().month().day())).ics"
        case .week:
            rangeStart = CalendarGridLayout.startOfWeek(containing: now, calendar: cal)
            rangeEnd = cal.date(byAdding: .day, value: 7, to: rangeStart) ?? rangeStart
            suggestedFilename = "hot-cross-buns-week-\(rangeStart.formatted(.iso8601.year().month().day())).ics"
        }
        let selected = calendarStore.calendarSnapshot.selectedCalendarIDs
        let events = calendarStore.events
            .filter { selected.contains($0.calendarID) }
            .filter { $0.status != .cancelled }
            .filter { $0.endDate > rangeStart && $0.startDate < rangeEnd }
            .sorted { $0.startDate < $1.startDate }
        let ics = EventICSExporter.ics(for: events)
        let panel = NSSavePanel()
        panel.nameFieldStringValue = suggestedFilename
        panel.allowedContentTypes = [UTType(filenameExtension: "ics") ?? .data]
        panel.canCreateDirectories = true
        if panel.runModal() == .OK, let url = panel.url {
            try? ics.data(using: .utf8)?.write(to: url)
        }
    }

    private func exportSettings() {
        let panel = NSSavePanel()
        panel.nameFieldStringValue = "Hot Cross Buns Settings.json"
        panel.allowedContentTypes = [.json]
        panel.canCreateDirectories = true

        guard panel.runModal() == .OK, let url = panel.url else { return }

        do {
            let data = try SettingsTransferBundle.encode(model.settingsExportBundle())
            try data.write(to: url, options: [.atomic])
            settingsTransferIsWarning = false
            settingsTransferMessage = "Settings exported to \(url.lastPathComponent)."
        } catch {
            settingsTransferIsWarning = true
            settingsTransferMessage = error.localizedDescription
        }
    }

    private func importSettings() {
        let panel = NSOpenPanel()
        panel.allowedContentTypes = [.json]
        panel.allowsMultipleSelection = false
        panel.canChooseDirectories = false

        guard panel.runModal() == .OK, let url = panel.url else { return }

        do {
            let data = try Data(contentsOf: url)
            let bundle = try SettingsTransferBundle.decode(data)
            pendingSettingsImport = bundle
            settingsImportPreview = model.previewSettingsImport(bundle)
        } catch {
            settingsTransferIsWarning = true
            settingsTransferMessage = error.localizedDescription
        }
    }

    private func applyPendingSettingsImport() {
        guard let pendingSettingsImport else { return }
        model.applySettingsImport(pendingSettingsImport)
        self.pendingSettingsImport = nil
        settingsTransferIsWarning = false
        settingsTransferMessage = "Settings imported."
    }

    private func performZoomIn() {
        adjustLayoutScale(by: layoutScaleStep)
    }

    private func performZoomOut() {
        adjustLayoutScale(by: -layoutScaleStep)
    }

    private func performZoomReset() {
        var next = settingsStore.settings
        next.uiLayoutScale = 1.0
        model.updateSettings(next)
    }

    private func adjustLayoutScale(by delta: Double) {
        var next = settingsStore.settings
        next.uiLayoutScale = max(layoutScaleMin, min(next.uiLayoutScale + delta, layoutScaleMax))
        model.updateSettings(next)
    }

    private func installAppShortcutMonitor() {
        guard appShortcutMonitor == nil else { return }
        appShortcutMonitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { event in
            handleAppShortcut(event) ? nil : event
        }
    }

    private func uninstallAppShortcutMonitor() {
        if let appShortcutMonitor {
            NSEvent.removeMonitor(appShortcutMonitor)
            self.appShortcutMonitor = nil
        }
    }

    private func handleAppShortcut(_ event: NSEvent) -> Bool {
        let modifiers = event.modifierFlags.intersection(.deviceIndependentFlagsMask)

        // While collecting a chord, no modifiers needed — next keydown is
        // consumed as the next key in the sequence. Esc cancels.
        if chordKeys != nil {
            if event.keyCode == 53 { // escape
                cancelChord()
                return true
            }
            // Any modifier besides shift (for capital letters) breaks chord
            // mode and falls through to normal handling.
            if modifiers.intersection([.command, .option, .control]).isEmpty == false {
                cancelChord()
                return false
            }
            guard let char = event.charactersIgnoringModifiers?.first else {
                return false
            }
            advanceChord(with: String(char).lowercased())
            return true
        }

        guard modifiers.contains(.command) else { return false }
        guard modifiers.intersection([.option, .control]).isEmpty else { return false }
        guard event.isARepeat == false else { return false }

        let rawKey = event.characters?.first
        let plainKey = event.charactersIgnoringModifiers?.first

        // ⌘K enters chord mode. Check before the other shortcuts so it wins
        // over any downstream binding that might share the key.
        if modifiers == [.command], plainKey == "k" {
            startChord()
            return true
        }

        switch (rawKey, plainKey) {
        case ("+", _), (_, "="):
            performZoomIn()
            return true
        case ("-", _), (_, "-"):
            performZoomOut()
            return true
        case ("0", _), (_, "0"):
            performZoomReset()
            return true
        default:
            return false
        }
    }

    // MARK: - chord state machine

    private func startChord() {
        chordKeys = []
        armChordTimeout()
    }

    private func advanceChord(with key: String) {
        var keys = chordKeys ?? []
        keys.append(key)
        chordKeys = keys

        let bindings = HCBChordRegistry.defaults
        let survivors = HCBChordMatcher.matches(current: keys, in: bindings)
        if survivors.isEmpty {
            // No binding starts with this prefix — cancel. Don't eat
            // subsequent keystrokes; the user expects typing to resume.
            cancelChord()
            return
        }
        if let terminal = HCBChordMatcher.isExactTerminal(current: keys, in: bindings) {
            executeChord(terminal)
            return
        }
        // Still matching; keep collecting.
        armChordTimeout()
    }

    private func executeChord(_ binding: HCBChordBinding) {
        cancelChord()
        appCommandActions.execute(binding.command)
    }

    private func cancelChord() {
        chordTimeoutTask?.cancel()
        chordTimeoutTask = nil
        chordKeys = nil
    }

    private func armChordTimeout() {
        chordTimeoutTask?.cancel()
        chordTimeoutTask = Task { @MainActor in
            try? await Task.sleep(for: .seconds(3))
            if Task.isCancelled { return }
            chordKeys = nil
        }
    }

    private var commandPaletteCommands: [CommandPaletteCommand] {
        [
            CommandPaletteCommand(
                id: "new-task",
                title: "New Task",
                subtitle: "Natural-language task capture",
                symbol: "checklist",
                shortcut: "Cmd+N",
                keywords: ["task", "create", "new", "quick", "add"]
            ) {
                presentSheet(.quickAddTask, on: .store)
            },
            CommandPaletteCommand(
                id: "new-note",
                title: "New Note",
                subtitle: "Natural-language note capture",
                symbol: "note.text.badge.plus",
                shortcut: "",
                keywords: ["note", "create", "new", "quick", "add"]
            ) {
                presentSheet(.quickAddNote, on: .notes)
            },
            CommandPaletteCommand(
                id: "new-event",
                title: "New Event",
                subtitle: "Natural-language event capture",
                symbol: "calendar.badge.plus",
                shortcut: "Cmd+Shift+N",
                keywords: ["event", "calendar", "create", "new", "quick", "add"]
            ) {
                presentSheet(.quickAddEvent, on: .calendar)
            },
            CommandPaletteCommand(
                id: "refresh",
                title: "Refresh Sync",
                subtitle: "Sync Google Tasks and Calendar now",
                symbol: "arrow.clockwise",
                shortcut: "Cmd+R",
                keywords: ["refresh", "sync", "reload"]
            ) {
                Task { await model.refreshNow() }
            },
            CommandPaletteCommand(
                id: "force-resync",
                title: "Force Full Resync",
                subtitle: "Clear checkpoints and perform a full sync",
                symbol: "arrow.triangle.2.circlepath.circle",
                shortcut: "Cmd+Shift+R",
                keywords: ["resync", "full", "reset", "checkpoint"]
            ) {
                Task { await model.forceFullResync() }
            },
            CommandPaletteCommand(
                id: "action-center",
                title: "Action Center",
                subtitle: "Review pending sync, reminder, and availability items",
                symbol: "bell.badge",
                shortcut: "",
                keywords: ["action", "center", "notifications", "reminders", "pending", "review"]
            ) {
                isActionCenterPresented.toggle()
            },
            CommandPaletteCommand(
                id: "open-diagnostics",
                title: "Diagnostics and Recovery",
                subtitle: "Open sync health and reset tools",
                symbol: "stethoscope",
                shortcut: "Cmd+Option+D",
                keywords: ["diagnostics", "recovery", "errors", "health"]
            ) {
                openInstrumentedWindow(id: "diagnostics", source: "commandPalette")
            },
            CommandPaletteCommand(
                id: "go-calendar",
                title: "Go to Calendar",
                subtitle: "Google Calendar grid with today status",
                symbol: "calendar",
                shortcut: "Cmd+1",
                keywords: ["calendar", "events", "today", "forecast"]
            ) {
                selectSidebarItem(.calendar, source: "commandPalette.goCalendar")
            },
            CommandPaletteCommand(
                id: "go-store",
                title: "Go to Tasks",
                subtitle: "Kanban board of dated tasks across every list",
                symbol: "checklist",
                shortcut: "Cmd+2",
                keywords: ["tasks", "kanban", "lists", "store"]
            ) {
                selectSidebarItem(.store, source: "commandPalette.goTasks")
            },
            CommandPaletteCommand(
                id: "go-notes",
                title: "Go to Notes",
                subtitle: "Quick-capture cards — tasks without a due date",
                symbol: "note.text",
                shortcut: "Cmd+3",
                keywords: ["notes", "capture", "undated", "someday"]
            ) {
                selectSidebarItem(.notes, source: "commandPalette.goNotes")
            },
            CommandPaletteCommand(
                id: "toggle-sidebar",
                title: "Toggle Sidebar",
                subtitle: "Show or hide the navigation surface",
                symbol: "sidebar.left",
                shortcut: "",
                keywords: ["sidebar", "navigation", "show", "hide", "toggle"]
            ) {
                toggleNavigationSurface()
            },
            CommandPaletteCommand(
                id: "open-settings",
                title: "Settings…",
                subtitle: "Appearance, account, sync, keyboard, calendars",
                symbol: "gearshape",
                shortcut: "Cmd+,",
                keywords: ["settings", "preferences", "appearance", "font", "theme"]
            ) {
                NotificationCenter.default.post(name: .hcbOpenSettingsWindow, object: nil)
            },
            CommandPaletteCommand(
                id: "insert-task-template",
                title: "Insert Task Template…",
                subtitle: "Pre-fill a task from a saved template",
                symbol: "doc.text",
                shortcut: "",
                keywords: ["template", "insert", "snippet", "prefill", "task"]
            ) {
                isPresentingInsertTemplate = true
            },
            CommandPaletteCommand(
                id: "insert-event-template",
                title: "Insert Event Template…",
                subtitle: "Pre-fill an event from a saved template",
                symbol: "calendar",
                shortcut: "",
                keywords: ["template", "insert", "event", "meeting", "calendar", "snippet", "prefill"]
            ) {
                isPresentingInsertEventTemplate = true
            },
            CommandPaletteCommand(
                id: "open-help",
                title: "Help",
                subtitle: "Keyboard shortcuts, sync behavior, troubleshooting",
                symbol: "questionmark.circle",
                shortcut: "Cmd+?",
                keywords: ["help", "docs", "shortcuts", "keys", "guide"]
            ) {
                openWindow(id: "help")
            }
        ]
    }

    private func badge(for item: SidebarItem) -> String? {
        guard calendarStore.account != nil else { return nil }
        switch item {
        case .calendar:
            let count = menuBarStore.projection.events(on: Date()).count
            return count > 0 ? "\(count)" : nil
        case .store:
            // Tasks tab badge counts only tasks that actually belong to the
            // Tasks surface — dated, open. Undated ones live in Notes and
            // carry their own count to avoid double-badging the sidebar.
            let count = taskStore.datedOpenTaskCount
            return count > 0 ? "\(count)" : nil
        case .notes:
            let count = taskStore.undatedOpenTaskCount
            return count > 0 ? "\(count)" : nil
        }
    }

    @ViewBuilder
    private func sidebarRow(for item: SidebarItem) -> some View {
        NavigationLink(value: item) {
            Label {
                HStack {
                    Text(item.title)
                    Spacer()
                    if let badgeValue = badge(for: item) {
                        Text(badgeValue)
                            .font(.caption.monospacedDigit())
                            .foregroundStyle(.secondary)
                    }
                }
            } icon: {
                Image(systemName: item.systemImage)
            }
        }
        .help(sidebarHelp(for: item))
        .contextMenu {
            sidebarContextMenu(for: item)
        }
    }

    @ViewBuilder
    private func sidebarContextMenu(for item: SidebarItem) -> some View {
        if item == .calendar {
            Button {
                openShareAvailabilityFromSidebar()
            } label: {
                Label("Share Availability", systemImage: "calendar.badge.clock")
            }
            .disabled(calendarStore.account == nil || hasWritableCalendars == false)

            Divider()

            Button {
                calendarSidebarFiltersCollapsed.toggle()
            } label: {
                Label(
                    calendarSidebarFiltersCollapsed ? "Expand View Filters" : "Collapse View Filters",
                    systemImage: calendarSidebarFiltersCollapsed ? "chevron.down" : "chevron.up"
                )
            }
            .disabled(calendarStore.account == nil)
        }
    }

    private func openShareAvailabilityFromSidebar() {
        selectSidebarItem(.calendar, source: "sidebar.shareAvailability")
        DispatchQueue.main.async {
            NotificationCenter.default.post(name: .hcbOpenShareAvailability, object: nil)
        }
    }

    private func sidebarHelp(for item: SidebarItem) -> String {
        item.navigationHelp(shortcutOverrides: shellChrome.shortcutOverrides)
    }

    private func navigationAccessibilityLabel(for item: SidebarItem) -> String {
        var components = [item.title]
        if let badgeValue = badge(for: item) {
            let count = Int(badgeValue)
            components.append("\(badgeValue) \(count == 1 ? "item" : "items")")
        }
        if selection == item {
            components.append("selected")
        }
        return components.joined(separator: ", ")
    }

    private func selectSidebarItem(_ item: SidebarItem, source: String) {
        guard selection != item else { return }
        let transition = HCBTransitionProfiler.start(
            "sidebar.\(selection.rawValue)->\(item.rawValue)",
            metadata: [
                "from": selection.rawValue,
                "source": source,
                "to": item.rawValue
            ]
        )
        activeSidebarTransition = transition
        selection = item
        Task { @MainActor in
            await Task.yield()
            transition?.markFirstContent(metadata: [
                "host": "persistent",
                "surface": item.rawValue
            ])
        }
    }

    private func openInstrumentedWindow(id: String, source: String) {
        HCBTransitionProfiler.startStored(
            key: HCBTransitionKeys.window(id),
            name: "window.open.\(id)",
            metadata: [
                "source": source,
                "window": id
            ]
        )
        openWindow(id: id)
    }

    private func openInstrumentedSettings(source: String) {
        HCBTransitionProfiler.startStored(
            key: HCBTransitionKeys.settings,
            name: "window.open.settings",
            metadata: ["source": source, "window": "settings"]
        )
        openSettings()
    }

    private func sidebarItemKey(_ item: SidebarItem) -> String {
        item.rawValue
    }

    private func presentSheet(_ sheet: SheetDestination, on item: SidebarItem) {
        selectSidebarItem(item, source: "sheet.\(sheet.telemetryName)")
        tabRouter.router(for: sidebarItemKey(item)).present(sheet)
    }

    private func runNearRealtimeSyncLoop() async {
        guard HCBLaunchMode.current.isSmokeTest == false else {
            return
        }
        guard scenePhase == .active, shellChrome.syncMode == .nearRealtime, calendarStore.account != nil else {
            return
        }

        let policy = BackoffPolicy.nearRealtime
        var attempt = 0
        var consecutiveTransientFailures = 0

        while Task.isCancelled == false {
            if model.isSyncPaused {
                // Stop the loop entirely once we've surfaced "sync paused".
                // It restarts when scenePhase re-activates or the user taps
                // refresh — both of which clear the flag before the loop is
                // scheduled again.
                return
            }
            if networkMonitor.reachability == .offline {
                // Don't burn cycles polling when offline. The loop restart
                // on scenePhase.active or a user-initiated refresh will
                // re-enter once reachability recovers.
                return
            }
            let delay = NearRealtimePollingCadence.delay(
                policy: policy,
                attempt: attempt,
                isNetworkConstrained: networkMonitor.reachability == .constrained,
                isLowPowerModeEnabled: ProcessInfo.processInfo.isLowPowerModeEnabled,
                isMainWindowFocused: isMainWindowFocused
            )
            model.recordNearRealtimePollDiagnostic(
                delaySeconds: Self.seconds(in: delay),
                attempt: attempt,
                isNetworkConstrained: networkMonitor.reachability == .constrained,
                isLowPowerModeEnabled: ProcessInfo.processInfo.isLowPowerModeEnabled,
                isMainWindowFocused: isMainWindowFocused
            )
            do {
                try await Task.sleep(for: delay)
            } catch {
                return
            }

            let outcome = await model.refreshNow()
            switch outcome {
            case .succeeded, .skipped:
                attempt = 0
                consecutiveTransientFailures = 0
            case .failed(let error) where policy.shouldBackoff(from: error):
                attempt = min(attempt + 1, policy.maxAttempts)
                consecutiveTransientFailures += 1
                if consecutiveTransientFailures >= policy.maxAttempts {
                    // Persistent backend / network trouble — stop hammering
                    // and surface "Sync paused" so the user can choose when
                    // to retry.
                    model.markSyncPaused()
                    return
                }
            case .failed:
                attempt = 0
                consecutiveTransientFailures = 0
            }
        }
    }

    private func handlePendingAppIntentRoute() {
        // Drain all pending routes — rapid intents are queued in order so
        // we don't silently drop one when two fire back to back. The last
        // sheet-presenting route wins visually (SwiftUI only presents one
        // sheet at a time); non-sheet routes (selection changes) all apply.
        let routes = AppIntentHandoff.consumeAll()
        for route in routes {
            switch route {
            case .addTask:
                presentSheet(.addTask, on: .store)
            case .addEvent:
                presentSheet(.addEvent, on: .calendar)
            case .store:
                selectSidebarItem(.store, source: "appIntent.store")
            case .calendar:
                selectSidebarItem(.calendar, source: "appIntent.calendar")
            }
        }
    }

    private static func seconds(in duration: Duration) -> Double {
        let components = duration.components
        return Double(components.seconds) + Double(components.attoseconds) / 1_000_000_000_000_000_000
    }
}

private struct FeatureTourView: View {
    let dismiss: () -> Void
    let openHelp: () -> Void

    private let items: [FeatureTourItem] = [
        FeatureTourItem(
            title: "Quick add",
            detail: "Capture tasks with dates and list hints, or notes without leaving the keyboard.",
            systemImage: "plus.circle.fill"
        ),
        FeatureTourItem(
            title: "Calendar surfaces",
            detail: "Move between agenda, day, week, month, year, and multi-day views from the Calendar toolbar.",
            systemImage: "calendar"
        ),
        FeatureTourItem(
            title: "Tasks and notes",
            detail: "Dated items stay in Tasks; undated items stay in Notes, with per-tab list filters in Settings.",
            systemImage: "checklist"
        ),
        FeatureTourItem(
            title: "Command palette",
            detail: "Open app actions, switch views, and route common workflows from one searchable panel.",
            systemImage: "command.circle"
        ),
        FeatureTourItem(
            title: "Sync issues",
            detail: "Review conflicts, invalid queued writes, and deferred reminders from the status banner or Sync Issues window.",
            systemImage: "exclamationmark.arrow.triangle.2.circlepath"
        ),
        FeatureTourItem(
            title: "Templates and filters",
            detail: "Save reusable task/event templates and custom task filters from Settings.",
            systemImage: "slider.horizontal.3"
        )
    ]

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            HStack(alignment: .center, spacing: 12) {
                Image(systemName: "sparkles")
                    .hcbFont(.title2)
                    .foregroundStyle(AppColor.ember)
                VStack(alignment: .leading, spacing: 4) {
                    Text("Tips")
                        .hcbFont(.title3, weight: .semibold)
                    Text("A few useful surfaces to try first.")
                        .hcbFont(.callout)
                        .foregroundStyle(.secondary)
                }
            }

            VStack(alignment: .leading, spacing: 12) {
                ForEach(items) { item in
                    FeatureTourRow(item: item)
                }
            }

            HStack {
                Button("Open Help", action: openHelp)
                    .buttonStyle(.bordered)
                Spacer()
                Button("Done", action: dismiss)
                    .buttonStyle(.borderedProminent)
                    .keyboardShortcut(.defaultAction)
            }
        }
        .hcbScaledPadding(24)
        .frame(minWidth: 500, idealWidth: 560)
        .appBackground()
    }
}

private struct FeatureTourItem: Identifiable {
    var title: String
    var detail: String
    var systemImage: String

    var id: String { title }
}

private struct FeatureTourRow: View {
    let item: FeatureTourItem

    var body: some View {
        Label {
            VStack(alignment: .leading, spacing: 3) {
                Text(item.title)
                    .hcbFont(.subheadline, weight: .semibold)
                Text(item.detail)
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        } icon: {
            Image(systemName: item.systemImage)
                .foregroundStyle(AppColor.moss)
                .frame(width: 22)
        }
        .labelStyle(.titleAndIcon)
    }
}

private struct ActionCenterPresentationModifier: ViewModifier {
    let snapshot: ActionCenterSnapshot
    @Binding var isPresented: Bool
    let reduceMotion: Bool
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

    func body(content: Content) -> some View {
        content
            .overlay(alignment: .trailing) {
                drawerOverlay
            }
            .animation(HCBMotion.animation(.easeOut(duration: 0.16), reduceMotion: reduceMotion), value: isPresented)
    }

    @ViewBuilder
    private var drawerOverlay: some View {
        if isPresented {
            ZStack(alignment: .trailing) {
                Color.black.opacity(0.001)
                    .ignoresSafeArea()
                    .onTapGesture {
                        isPresented = false
                    }

                ActionCenterDrawer(
                    snapshot: snapshot,
                    onClose: { isPresented = false },
                    onOpenHold: onOpenHold,
                    onConfirmHold: onConfirmHold,
                    onCancelHoldGroup: onCancelHoldGroup,
                    onOpenTask: onOpenTask,
                    onCompleteTask: onCompleteTask,
                    onSnoozeTask: onSnoozeTask,
                    onOpenSyncIssues: onOpenSyncIssues,
                    onOpenSettings: onOpenSettings,
                    onRetrySync: onRetrySync,
                    onDismissStatus: onDismissStatus
                )
                .transition(HCBMotion.transition(.move(edge: .trailing).combined(with: .opacity), reduceMotion: reduceMotion))
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .trailing)
        }
    }
}

// Small helper ViewModifier to keep MacSidebarShell.body within the
// SwiftUI type-checker's budget. Each onReceive inflates ModifiedContent
// generics quickly; bundling three of them halves the chain depth.
private struct ShellZoomObservers: ViewModifier {
    let zoomIn: () -> Void
    let zoomOut: () -> Void
    let zoomReset: () -> Void

    func body(content: Content) -> some View {
        content
            .onReceive(NotificationCenter.default.publisher(for: .hcbZoomIn)) { _ in zoomIn() }
            .onReceive(NotificationCenter.default.publisher(for: .hcbZoomOut)) { _ in zoomOut() }
            .onReceive(NotificationCenter.default.publisher(for: .hcbZoomReset)) { _ in zoomReset() }
    }
}

private struct MainWindowFocusObserver: ViewModifier {
    @Binding var isFocused: Bool

    func body(content: Content) -> some View {
        content.background {
            MainWindowFocusAccessor(isFocused: $isFocused)
                .frame(width: 0, height: 0)
        }
    }
}

private struct MainWindowFocusAccessor: NSViewRepresentable {
    @Binding var isFocused: Bool

    func makeNSView(context: Context) -> NSView {
        let view = NSView(frame: .zero)
        context.coordinator.update(isFocused: $isFocused)
        return view
    }

    func updateNSView(_ nsView: NSView, context: Context) {
        context.coordinator.update(isFocused: $isFocused)
        DispatchQueue.main.async {
            context.coordinator.attach(to: nsView.window)
        }
    }

    func makeCoordinator() -> Coordinator {
        Coordinator(isFocused: $isFocused)
    }

    final class Coordinator {
        private var isFocused: Binding<Bool>
        private weak var window: NSWindow?
        private var observers: [NSObjectProtocol] = []

        init(isFocused: Binding<Bool>) {
            self.isFocused = isFocused
        }

        deinit {
            removeObservers()
        }

        func update(isFocused: Binding<Bool>) {
            self.isFocused = isFocused
            scheduleFocusRefresh()
        }

        func attach(to nextWindow: NSWindow?) {
            guard let nextWindow else { return }
            // The app uses its own sidebar chrome; the macOS title text should stay hidden.
            nextWindow.titleVisibility = .hidden
            guard window !== nextWindow else {
                refreshFocus()
                return
            }

            removeObservers()
            window = nextWindow

            let center = NotificationCenter.default
            observers = [
                center.addObserver(
                    forName: NSWindow.didBecomeKeyNotification,
                    object: nextWindow,
                    queue: .main
                ) { [weak self] _ in
                    self?.refreshFocus()
                },
                center.addObserver(
                    forName: NSWindow.didResignKeyNotification,
                    object: nextWindow,
                    queue: .main
                ) { [weak self] _ in
                    self?.refreshFocus()
                },
                center.addObserver(
                    forName: NSWindow.willCloseNotification,
                    object: nextWindow,
                    queue: .main
                ) { [weak self] _ in
                    self?.setFocus(false)
                },
                center.addObserver(
                    forName: NSApplication.didBecomeActiveNotification,
                    object: NSApp,
                    queue: .main
                ) { [weak self] _ in
                    self?.refreshFocus()
                },
                center.addObserver(
                    forName: NSApplication.didResignActiveNotification,
                    object: NSApp,
                    queue: .main
                ) { [weak self] _ in
                    self?.setFocus(false)
                }
            ]
            scheduleFocusRefresh()
        }

        private func scheduleFocusRefresh() {
            DispatchQueue.main.async { [weak self] in
                self?.refreshFocus()
            }
        }

        private func refreshFocus() {
            setFocus(NSApp.isActive && window?.isKeyWindow == true)
        }

        private func setFocus(_ value: Bool) {
            guard isFocused.wrappedValue != value else { return }
            isFocused.wrappedValue = value
        }

        private func removeObservers() {
            for observer in observers {
                NotificationCenter.default.removeObserver(observer)
            }
            observers.removeAll()
        }
    }
}

#Preview {
    MacSidebarShell()
        .environment(AppModel.preview)
}
