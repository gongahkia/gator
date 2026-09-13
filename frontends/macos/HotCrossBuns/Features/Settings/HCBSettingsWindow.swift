import GoogleSignInSwift
import SwiftUI

private enum SettingsWindowLayout {
    // Match the default grouped Form column width so chrome above the form
    // lines up with the settings sections below it.
    static let contentMaxWidth: CGFloat = 704
}

// Top-level detached Settings window. Opened via the Settings scene in
// HotCrossBunsApp.swift (⌘, auto-wired by macOS). Layout matches Apple
// Calendar / Mail — a top tab bar with focused categories, content below.
// Most tabs host a scrollable Form of the section views HCB already
// ships; wider tool panes such as Hotkeys can use their own layout.
// The main app sidebar no longer carries a Settings tab; all preferences
// live here.
struct HCBSettingsWindow: View {
    @Environment(AppModel.self) private var model
    @Environment(UpdaterController.self) private var updater
    @Environment(\.openWindow) private var openWindow
    @State private var tab: SettingsSearchTab = .general
    @State private var settingsQuery = ""
    @State private var highlightedAnchor: SettingsSectionAnchor?
    @State private var highlightSequence = 0
    @FocusState private var isSettingsSearchFocused: Bool
    // Sub-sheets hosted locally (the detached window has no RouterPath).
    @State private var isSyncDetailsPresented = false
    @State private var isDiagnosticsPresented = false

    var body: some View {
        VStack(spacing: 0) {
            settingsSearchBar
            TabView(selection: $tab) {
                GeneralTab(
                    highlightedAnchor: highlightedAnchor,
                    isSyncDetailsPresented: $isSyncDetailsPresented,
                    isDiagnosticsPresented: $isDiagnosticsPresented
                )
                .tabItem { Label("General", systemImage: "gearshape") }
                .tag(SettingsSearchTab.general)

                ProfileTab(highlightedAnchor: highlightedAnchor)
                .tabItem { Label("Profile", systemImage: "person.crop.circle") }
                .tag(SettingsSearchTab.profile)

                AppearanceTab(highlightedAnchor: highlightedAnchor)
                .tabItem { Label("Appearance", systemImage: "paintbrush") }
                .tag(SettingsSearchTab.appearance)

                HotkeysTab(highlightedAnchor: highlightedAnchor)
                .tabItem { Label(hotkeysTabTitle, systemImage: "keyboard") }
                .tag(SettingsSearchTab.hotkeys)

                AlertsTab(highlightedAnchor: highlightedAnchor)
                .tabItem { Label("Alerts", systemImage: "bell") }
                .tag(SettingsSearchTab.alerts)

                AdvancedTab(highlightedAnchor: highlightedAnchor)
                .tabItem { Label(advancedTabTitle, systemImage: "gearshape.2") }
                .tag(SettingsSearchTab.advanced)

                AboutTab(highlightedAnchor: highlightedAnchor)
                .tabItem { Label(aboutTabTitle, systemImage: "info.circle") }
                .tag(SettingsSearchTab.about)
            }
        }
        // Hotkeys needs a two-column settings surface, so the detached
        // settings window gets a wider default while remaining resizable.
        .frame(minWidth: 680, idealWidth: 820, minHeight: 560, idealHeight: 640)
        // The Settings scene is a separate SwiftUI Scene, so the
        // appearance environment applied in MacSidebarShell doesn't
        // carry through — re-apply here so the color scheme, dark/light
        // mode, scaled fonts, and background match the main window.
        .id(model.settings.colorSchemeID)
        .environment(\.locale, model.settings.appLanguage.locale)
        .withHCBAppearance(model.settings)
        .environment(\.hcbShortcutOverrides, model.settings.shortcutOverrides)
        .hcbPreferredColorScheme(model.settings)
        .appBackground()
        .navigationTitle(tab.title)
        .sheet(isPresented: $isSyncDetailsPresented) {
            SyncSettingsSheet()
                .environment(model)
                .withHCBAppearance(model.settings)
                .hcbPreferredColorScheme(model.settings)
        }
        .sheet(isPresented: $isDiagnosticsPresented) {
            DiagnosticsView()
                .environment(model)
                .withHCBAppearance(model.settings)
                .hcbPreferredColorScheme(model.settings)
        }
        .overlay {
            let toast = updater.toastState?.target == .settings ? updater.toastState : nil
            BulkResultToast(
                message: Binding(
                    get: {
                        guard updater.toastState?.target == .settings else { return nil }
                        return updater.toastState?.message
                    },
                    set: { newValue in
                        if newValue == nil {
                            updater.clearToast()
                        }
                    }
                ),
                isWarning: toast?.isWarning ?? false,
                successTitle: toast?.title ?? "Update check complete",
                warningTitle: toast?.title ?? "Update check failed",
                successSymbol: "arrow.down.circle.fill",
                warningSymbol: "wifi.exclamationmark"
            )
        }
        .onChange(of: updater.updatePromptSequence) { _, _ in
            openWindow(id: "update-available")
        }
        .onChange(of: updater.installGuideSequence) { _, _ in
            openWindow(id: "install-update")
        }
    }

    private var settingsSearchBar: some View {
        VStack(spacing: 0) {
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 8) {
                    Image(systemName: "magnifyingglass")
                        .foregroundStyle(.secondary)
                    TextField("Search settings", text: $settingsQuery)
                        .textFieldStyle(.plain)
                        .focused($isSettingsSearchFocused)
                    if settingsQuery.isEmpty == false {
                        Button {
                            settingsQuery = ""
                        } label: {
                            Image(systemName: "xmark.circle.fill")
                                .foregroundStyle(.secondary)
                        }
                        .buttonStyle(.plain)
                        .keyboardShortcut(.cancelAction)
                        .help("Clear search")
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 10)
                .padding(.vertical, 7)
                .background(.quaternary.opacity(0.35), in: RoundedRectangle(cornerRadius: 8))

                if settingsQuery.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
                    settingsSearchResults
                }
            }
            .frame(maxWidth: SettingsWindowLayout.contentMaxWidth, alignment: .leading)
            .padding(.horizontal, 16)
            .padding(.top, 14)
            .padding(.bottom, settingsQuery.isEmpty ? 8 : 10)
        }
        .frame(maxWidth: .infinity)
        .background(.bar)
        .onAppear {
            isSettingsSearchFocused = true
        }
    }

    private var settingsSearchResults: some View {
        let results = SettingsSearchIndex.filter(searchIndex, query: settingsQuery)
        return VStack(alignment: .leading, spacing: 4) {
            if results.isEmpty {
                Text("No settings match.")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .padding(.vertical, 4)
            } else {
                ForEach(results) { result in
                    Button {
                        selectSearchResult(result)
                    } label: {
                        HStack(spacing: 8) {
                            Image(systemName: result.tab.systemImage)
                                .frame(width: 18)
                                .foregroundStyle(.secondary)
                            Text(result.title)
                                .lineLimit(1)
                            Spacer()
                            if let status = result.status, status.isEmpty == false {
                                Text(status)
                                    .hcbFont(.caption2, weight: .semibold)
                                    .foregroundStyle(.secondary)
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 1)
                                    .background(Capsule().fill(.quaternary.opacity(0.55)))
                            }
                            Text(result.tab.title)
                                .hcbFont(.caption)
                                .foregroundStyle(.secondary)
                        }
                        .padding(.horizontal, 8)
                        .padding(.vertical, 5)
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                }
            }
        }
    }

    private var searchIndex: [SettingsSearchResult] {
        SettingsSearchIndex.results(
            customShortcutCount: model.settings.shortcutOverrides.count,
            shortcutConflictCount: shortcutConflictCount,
            customFilterCount: model.settings.customFilters.count,
            taskTemplateCount: model.settings.taskTemplates.count,
            eventTemplateCount: model.settings.eventTemplates.count,
            updateStatus: updateBadgeText
        )
    }

    private func selectSearchResult(_ result: SettingsSearchResult) {
        tab = result.tab
        highlightedAnchor = result.anchor
        settingsQuery = ""
        highlightSequence += 1
        let sequence = highlightSequence
        let anchor = result.anchor
        Task {
            try? await Task.sleep(for: .seconds(2))
            await MainActor.run {
                if highlightSequence == sequence, highlightedAnchor == anchor {
                    highlightedAnchor = nil
                }
            }
        }
    }

    private var hotkeysTabTitle: String {
        let custom = model.settings.shortcutOverrides.count
        if shortcutConflictCount > 0 { return "Hotkeys (\(shortcutConflictCount)!)" }
        return custom > 0 ? "Hotkeys (\(custom))" : "Hotkeys"
    }

    private var advancedTabTitle: String {
        let count = model.settings.customFilters.count + model.settings.taskTemplates.count + model.settings.eventTemplates.count
        return count > 0 ? "Advanced (\(count))" : "Advanced"
    }

    private var aboutTabTitle: String {
        guard let updateBadgeText else { return "About" }
        return "About (\(updateBadgeText))"
    }

    private var updateBadgeText: String? {
        if updater.isChecking { return "Checking" }
        if let release = updater.availableRelease {
            let state = updater.downloadState
            if state.releaseTag == release.tagName {
                switch state.phase {
                case .ready: return "Ready"
                case .failed: return "Failed"
                case .downloading: return "Downloading"
                case .idle: break
                }
            }
            return "Update"
        }
        return nil
    }

    private var shortcutConflictCount: Int {
        let overrides = model.settings.shortcutOverrides
        var conflicts = Set<HCBShortcutCommand>()
        for command in HCBShortcutCommand.allCases {
            guard let binding = overrides[command.rawValue] else { continue }
            let matches = hcbConflictingCommands(
                proposed: binding,
                for: command,
                overrides: overrides
            )
            if matches.isEmpty == false {
                conflicts.insert(command)
                matches.forEach { conflicts.insert($0) }
            }
        }
        return conflicts.count
    }
}

// MARK: - Profile tab

private struct ProfileTab: View {
    @Environment(AppModel.self) private var model
    @Environment(\.hcbReduceMotion) private var reduceMotion
    let highlightedAnchor: SettingsSectionAnchor?

    var body: some View {
        ScrollViewReader { proxy in
            Form {
                Section("Google OAuth client") {
                    SettingsHighlightRow(anchor: .profileOAuth, highlightedAnchor: highlightedAnchor)
                    GoogleOAuthClientSetupView()
                }
                .id(SettingsSectionAnchor.profileOAuth)

                Section("Google accounts") {
                    SettingsHighlightRow(anchor: .profileAccounts, highlightedAnchor: highlightedAnchor)
                    AccountStatusView(
                        authState: model.authState,
                        account: model.account,
                        accounts: model.connectedAccounts,
                        activeAccountID: model.activeAccountID,
                        canConnect: model.isGoogleAuthConfigured,
                        connect: { Task { await model.connectGoogleAccount() } },
                        disconnect: { Task { await model.disconnectGoogleAccount() } },
                        switchAccount: { accountID in Task { await model.switchGoogleAccount(to: accountID) } },
                        disconnectAccount: { accountID in Task { await model.disconnectGoogleAccount(id: accountID) } }
                    )
                }
                .id(SettingsSectionAnchor.profileAccounts)
            }
            .formStyle(.grouped)
            .onChange(of: highlightedAnchor) { _, anchor in
                scroll(proxy, to: anchor, allowed: [.profileOAuth, .profileAccounts], reduceMotion: reduceMotion)
            }
        }
    }
}

// MARK: - About tab

private struct AboutTab: View {
    @Environment(\.hcbReduceMotion) private var reduceMotion
    let highlightedAnchor: SettingsSectionAnchor?

    var body: some View {
        ScrollViewReader { proxy in
            Form {
                UpdatesSection()
                    .id(SettingsSectionAnchor.updates)
                AppVersionSection()
                    .id(SettingsSectionAnchor.version)
            }
            .formStyle(.grouped)
            .onChange(of: highlightedAnchor) { _, anchor in
                scroll(proxy, to: anchor, allowed: [.updates, .version], reduceMotion: reduceMotion)
            }
        }
    }
}

// MARK: - General tab

private struct GeneralTab: View {
    @Environment(AppModel.self) private var model
    @Environment(\.openWindow) private var openWindow
    @Environment(\.hcbReduceMotion) private var reduceMotion
    let highlightedAnchor: SettingsSectionAnchor?
    @Binding var isSyncDetailsPresented: Bool
    @Binding var isDiagnosticsPresented: Bool
    @State private var showOnboardingResetConfirmed = false
    @State private var customRetentionAmount: Int = 60
    @State private var customRetentionUnit: RetentionUnit = .days

    enum RetentionUnit: String, CaseIterable, Identifiable {
        case days, weeks, years
        var id: String { rawValue }
        var title: String {
            switch self {
            case .days: "Days"
            case .weeks: "Weeks"
            case .years: "Years"
            }
        }
        func toDays(_ amount: Int) -> Int {
            switch self {
            case .days: amount
            case .weeks: amount * 7
            case .years: amount * 365
            }
        }
    }

    var body: some View {
        ScrollViewReader { proxy in
            Form {
                Section("Language") {
                    SettingsHighlightRow(anchor: .language, highlightedAnchor: highlightedAnchor)
                    AppLanguagePicker(title: "App language")
                    Text("Use the app in a supported language. System Default follows your macOS language order.")
                        .hcbFont(.footnote)
                        .foregroundStyle(.secondary)
                }
                .id(SettingsSectionAnchor.language)

                OpenAtLoginSection()
                    .id(SettingsSectionAnchor.openAtLogin)

                Section("Diagnostics") {
                    SettingsHighlightRow(anchor: .diagnostics, highlightedAnchor: highlightedAnchor)
                    Button {
                        isDiagnosticsPresented = true
                    } label: {
                        Label("Open diagnostics", systemImage: "stethoscope")
                    }
                    Text("Inspect logs, mutation history, sync queues, and support bundles.")
                        .hcbFont(.footnote)
                        .foregroundStyle(.secondary)
                    Toggle("Include field-redacted Google payloads in local logs", isOn: rawGoogleDiagnosticsBinding)
                    Text("Off by default. When enabled, future local logs may include troubleshooting snippets with known titles, notes, summaries, locations, attendees, links, and tokens redacted before export.")
                        .hcbFont(.footnote)
                        .foregroundStyle(.secondary)
                }
                .id(SettingsSectionAnchor.diagnostics)

                AgentAccessSection()
                    .id(SettingsSectionAnchor.agentAccess)

                if model.account != nil {
                    Section("Sync") {
                        SettingsHighlightRow(anchor: .sync, highlightedAnchor: highlightedAnchor)
                        Picker("Mode", selection: syncModeBinding) {
                            ForEach(SyncMode.allCases) { mode in
                                Text(mode.title).tag(mode)
                            }
                        }
                        Text(model.settings.syncMode.detail)
                            .hcbFont(.footnote)
                            .foregroundStyle(.secondary)
                        syncModeGuidance
                        Picker("Keep past events", selection: eventRetentionBinding) {
                            Text("30 days").tag(30)
                            Text("90 days").tag(90)
                            Text("180 days").tag(180)
                            Text("1 year").tag(365)
                            Text("2 years").tag(730)
                            Text("Forever").tag(0)
                        }
                        Picker("Keep completed tasks", selection: completedTaskRetentionBinding) {
                            Text("30 days").tag(30)
                            Text("90 days").tag(90)
                            Text("180 days").tag(180)
                            Text("1 year").tag(365)
                            Text("2 years").tag(730)
                            Text("Forever").tag(0)
                        }
                        HStack(spacing: 8) {
                            Text("Custom")
                                .frame(maxWidth: .infinity, alignment: .leading)
                            TextField("Amount", value: $customRetentionAmount, format: .number)
                                .textFieldStyle(.roundedBorder)
                                .frame(width: 70)
                                .multilineTextAlignment(.trailing)
                            Picker("Unit", selection: $customRetentionUnit) {
                                ForEach(RetentionUnit.allCases) { unit in
                                    Text(unit.title).tag(unit)
                                }
                            }
                            .labelsHidden()
                            .frame(width: 90)
                            Button("Apply") {
                                let days = customRetentionUnit.toDays(max(0, customRetentionAmount))
                                model.setEventRetentionDaysBack(days)
                            }
                            .keyboardShortcut(.defaultAction)
                            .disabled(customRetentionAmount <= 0)
                        }
                        Text("Older events and completed tasks are dropped from the local cache to keep memory + disk tight. Drops never touch Google — a Force Resync refetches according to these retention windows.")
                            .hcbFont(.caption2)
                            .foregroundStyle(.secondary)

                        if hasSyncAttentionItems {
                            Button {
                                openWindow(id: "sync-issues")
                            } label: {
                                Label(syncAttentionLabel, systemImage: "exclamationmark.bubble")
                            }
                        }
                    }
                    .id(SettingsSectionAnchor.sync)

                    Section("Setup") {
                        Button {
                            model.resetOnboarding()
                            showOnboardingResetConfirmed = true
                        } label: {
                            Label("Run setup again", systemImage: "sparkles")
                        }
                    }
                    .alert("Setup will run now", isPresented: $showOnboardingResetConfirmed) {
                        Button("OK") { showOnboardingResetConfirmed = false }
                    } message: {
                        Text("The onboarding flow has been reset. Switch back to the main Hot Cross Buns window to go through setup again.")
                    }
                }
            }
            .formStyle(.grouped)
            .onChange(of: highlightedAnchor) { _, anchor in
                scroll(proxy, to: anchor, allowed: [.language, .openAtLogin, .diagnostics, .agentAccess, .sync], reduceMotion: reduceMotion)
            }
        }
    }

    private var syncModeBinding: Binding<SyncMode> {
        Binding(
            get: { model.settings.syncMode },
            set: { model.updateSyncMode($0) }
        )
    }

    private var eventRetentionBinding: Binding<Int> {
        Binding(
            get: { model.settings.eventRetentionDaysBack },
            set: { model.setEventRetentionDaysBack($0) }
        )
    }

    private var completedTaskRetentionBinding: Binding<Int> {
        Binding(
            get: { model.settings.completedTaskRetentionDaysBack },
            set: { model.setCompletedTaskRetentionDaysBack($0) }
        )
    }

    private var rawGoogleDiagnosticsBinding: Binding<Bool> {
        Binding(
            get: { model.settings.rawGoogleDiagnosticsEnabled },
            set: { model.setRawGoogleDiagnosticsEnabled($0) }
        )
    }

    private var syncModeGuidance: some View {
        VStack(alignment: .leading, spacing: 6) {
            ForEach(SyncMode.allCases) { mode in
                HStack(alignment: .top, spacing: 8) {
                    Image(systemName: mode == model.settings.syncMode ? "checkmark.circle.fill" : "circle")
                        .foregroundStyle(mode == model.settings.syncMode ? AppColor.moss : .secondary)
                    Text("\(mode.title): \(mode.guidance)")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
        }
    }

    private var hasSyncAttentionItems: Bool {
        model.conflictedMutationCount > 0
            || model.quarantinedMutationCount > 0
            || (model.lastNotificationScheduleSummary?.hasDeferred ?? false)
    }

    private var syncAttentionLabel: String {
        var parts: [String] = []
        if model.conflictedMutationCount > 0 {
            parts.append("\(model.conflictedMutationCount) conflict\(model.conflictedMutationCount == 1 ? "" : "s")")
        }
        if model.quarantinedMutationCount > 0 {
            parts.append("\(model.quarantinedMutationCount) queued write\(model.quarantinedMutationCount == 1 ? "" : "s")")
        }
        if let summary = model.lastNotificationScheduleSummary, summary.hasDeferred {
            let total = summary.deferredEvents + summary.deferredTasks
            parts.append("\(total) deferred reminder\(total == 1 ? "" : "s")")
        }
        return "Review sync issues: " + parts.joined(separator: " • ")
    }
}

// MARK: - Appearance tab

private struct AppearanceTab: View {
    @Environment(\.hcbReduceMotion) private var reduceMotion
    let highlightedAnchor: SettingsSectionAnchor?

    var body: some View {
        ScrollViewReader { proxy in
            Form {
                AppearanceSection()
                    .id(SettingsSectionAnchor.appearance)
                PerSurfaceFontSection()
                    .id(SettingsSectionAnchor.background)
                LayoutSection()
                    .id(SettingsSectionAnchor.layout)
            }
            .formStyle(.grouped)
            .onChange(of: highlightedAnchor) { _, anchor in
                scroll(proxy, to: anchor, allowed: [.appearance, .background, .layout], reduceMotion: reduceMotion)
            }
        }
    }
}

// MARK: - Hotkeys tab

private struct HotkeysTab: View {
    @Environment(\.hcbReduceMotion) private var reduceMotion
    let highlightedAnchor: SettingsSectionAnchor?

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                KeybindingsSection()
                    .id(SettingsSectionAnchor.hotkeys)
                    .hcbScaledPadding(20)
            }
            .onChange(of: highlightedAnchor) { _, anchor in
                scroll(proxy, to: anchor, allowed: [.hotkeys], reduceMotion: reduceMotion)
            }
        }
    }
}

// MARK: - Alerts tab

private struct AlertsTab: View {
    @Environment(AppModel.self) private var model
    @Environment(\.openWindow) private var openWindow
    @Environment(\.hcbReduceMotion) private var reduceMotion
    let highlightedAnchor: SettingsSectionAnchor?
    @State private var permissionPrimer: PermissionPrimer?
    @State private var showLocalNotificationsInfo = false
    @State private var showNotificationsDeniedAlert = false

    var body: some View {
        ScrollViewReader { proxy in
            Form {
                Section("Notifications") {
                    SettingsHighlightRow(anchor: .notifications, highlightedAnchor: highlightedAnchor)
                    Toggle("Local reminders", isOn: localNotificationsBinding)
                    if model.settings.enableLocalNotifications {
                        taskReminderControls
                        if let summary = model.lastNotificationScheduleSummary, summary.hasDeferred {
                            Button {
                                openWindow(id: "sync-issues")
                            } label: {
                                Label(reminderCapacityLabel(summary), systemImage: "bell.badge")
                            }
                        }
                    }
                }
                .id(SettingsSectionAnchor.notifications)

                CompletionSoundSection()

                Section("Menu bar") {
                    SettingsHighlightRow(anchor: .menuBar, highlightedAnchor: highlightedAnchor)
                    Toggle("Menu bar extra", isOn: menuBarExtraBinding)
                    Picker("Menu bar panel", selection: menuBarStyleBinding) {
                        ForEach(AppSettings.MenuBarStyle.allCases, id: \.self) { style in
                            Text(style.title).tag(style)
                        }
                    }
                    .disabled(model.settings.showMenuBarExtra == false)
                    if model.settings.menuBarStyle == .adaptive {
                        adaptiveMenuBarControls
                    }
                    MenuBarIconPickerRow()
                    .disabled(model.settings.showMenuBarExtra == false)
                    Toggle("Menu bar badge for overdue tasks", isOn: menuBarBadgeBinding)
                        .disabled(model.settings.showMenuBarExtra == false)
                }
                .id(SettingsSectionAnchor.menuBar)

                Section("Dock") {
                    Toggle("Dock badge for overdue tasks", isOn: dockBadgeBinding)
                }

                GlobalHotkeySection()
            }
            .formStyle(.grouped)
            .onChange(of: highlightedAnchor) { _, anchor in
                scroll(proxy, to: anchor, allowed: [.notifications, .menuBar], reduceMotion: reduceMotion)
            }
        }
        .sheet(item: $permissionPrimer) { primer in
            PermissionPrimerView(primer: primer) {
                permissionPrimer = nil
                Task {
                    let result = await model.requestEnableLocalNotifications()
                    await MainActor.run {
                        if result == .authorized {
                            showLocalNotificationsInfo = true
                        } else {
                            showNotificationsDeniedAlert = true
                        }
                    }
                }
            } onCancel: {
                permissionPrimer = nil
            }
        }
        .alert("Local reminders enabled", isPresented: $showLocalNotificationsInfo) {
            Button("OK") { showLocalNotificationsInfo = false }
        } message: {
            Text("Hot Cross Buns will schedule up to 64 pending reminders on this Mac for the soonest-upcoming due tasks and Calendar events. 64 is an Apple-imposed ceiling for local notifications per app — later items get scheduled automatically as earlier ones fire or complete.")
        }
        .alert("Notifications are off for Hot Cross Buns", isPresented: $showNotificationsDeniedAlert) {
            Button("Open Notifications Settings") {
                HotCrossBunsSystemSettings.open(HotCrossBunsSystemSettings.notificationsURL)
            }
            Button("Cancel", role: .cancel) {
                showNotificationsDeniedAlert = false
            }
        } message: {
            Text("macOS blocked notifications for Hot Cross Buns. Open System Settings > Notifications > Hot Cross Buns to allow device-local reminders.")
        }
    }

    private var localNotificationsBinding: Binding<Bool> {
        Binding(
            get: { model.settings.enableLocalNotifications },
            set: { newValue in
                if newValue {
                    permissionPrimer = .notifications
                } else {
                    model.updateLocalNotificationsEnabled(false)
                }
            }
        )
    }

    private var menuBarExtraBinding: Binding<Bool> {
        Binding(
            get: { model.settings.showMenuBarExtra },
            set: { model.setShowMenuBarExtra($0) }
        )
    }

    private var menuBarStyleBinding: Binding<AppSettings.MenuBarStyle> {
        Binding(
            get: { model.settings.menuBarStyle },
            set: { model.setMenuBarStyle($0) }
        )
    }

    private var menuBarAdaptiveStatusSourceBinding: Binding<AppSettings.MenuBarAdaptiveStatusSource> {
        Binding(
            get: { model.settings.menuBarAdaptiveStatusSource },
            set: { model.setMenuBarAdaptiveStatusSource($0) }
        )
    }

    private var menuBarAdaptiveEmptyBehaviorBinding: Binding<AppSettings.MenuBarAdaptiveEmptyBehavior> {
        Binding(
            get: { model.settings.menuBarAdaptiveEmptyBehavior },
            set: { model.setMenuBarAdaptiveEmptyBehavior($0) }
        )
    }

    private var menuBarAdaptivePanelContentBinding: Binding<AppSettings.MenuBarAdaptivePanelContent> {
        Binding(
            get: { model.settings.menuBarAdaptivePanelContent },
            set: { model.setMenuBarAdaptivePanelContent($0) }
        )
    }

    private var menuBarBadgeBinding: Binding<Bool> {
        Binding(
            get: { model.settings.showMenuBarBadge },
            set: { model.setShowMenuBarBadge($0) }
        )
    }

    private var dockBadgeBinding: Binding<Bool> {
        Binding(
            get: { model.settings.showDockBadge },
            set: { model.setShowDockBadge($0) }
        )
    }

    // App-wide task reminder controls. Threshold = how many days before a task's
    // due date to fire a local notification. Time = hour:minute it fires at.
    // Per-task reminder offsets are gone: Google Tasks API has no reminder field.
    @ViewBuilder
    private var adaptiveMenuBarControls: some View {
        Group {
            Picker("Status source", selection: menuBarAdaptiveStatusSourceBinding) {
                ForEach(AppSettings.MenuBarAdaptiveStatusSource.allCases, id: \.self) { source in
                    Text(source.title).tag(source)
                }
            }
            Picker("When empty", selection: menuBarAdaptiveEmptyBehaviorBinding) {
                ForEach(AppSettings.MenuBarAdaptiveEmptyBehavior.allCases, id: \.self) { behavior in
                    Text(behavior.title).tag(behavior)
                }
            }
            Picker("Panel contents", selection: menuBarAdaptivePanelContentBinding) {
                ForEach(AppSettings.MenuBarAdaptivePanelContent.allCases, id: \.self) { content in
                    Text(content.title).tag(content)
                }
            }
        }
        .disabled(model.settings.showMenuBarExtra == false)
    }

    @ViewBuilder
    private var taskReminderControls: some View {
        Picker("Remind me before due", selection: taskReminderThresholdBinding) {
            Text("Disabled").tag(0)
            Text("1 day before").tag(1)
            Text("3 days before").tag(3)
            Text("1 week before").tag(7)
            Text("2 weeks before").tag(14)
            Text("1 month before").tag(30)
        }
        .pickerStyle(.menu)
        if model.settings.taskReminderThresholdDays > 0 {
            DatePicker(
                "Fire at",
                selection: taskReminderTimeBinding,
                displayedComponents: [.hourAndMinute]
            )
            Text("Every open task with a due date fires a single notification on this Mac at the chosen time, `N` days before. Per-task offsets are not stored anywhere — the rule is app-wide.")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    private var taskReminderThresholdBinding: Binding<Int> {
        Binding(
            get: { model.settings.taskReminderThresholdDays },
            set: { model.setTaskReminderThresholdDays($0) }
        )
    }

    private var taskReminderTimeBinding: Binding<Date> {
        Binding(
            get: {
                var comps = DateComponents()
                comps.hour = model.settings.taskReminderHour
                comps.minute = model.settings.taskReminderMinute
                return Calendar.current.date(from: comps) ?? Date()
            },
            set: { newValue in
                let comps = Calendar.current.dateComponents([.hour, .minute], from: newValue)
                model.setTaskReminderTime(hour: comps.hour ?? 9, minute: comps.minute ?? 0)
            }
        )
    }

    private func reminderCapacityLabel(_ summary: NotificationScheduleSummary) -> String {
        let total = summary.deferredEvents + summary.deferredTasks
        return "Review \(total) deferred reminder\(total == 1 ? "" : "s")"
    }
}

// MARK: - Advanced tab

private struct AdvancedTab: View {
    @Environment(AppModel.self) private var model
    @Environment(\.hcbReduceMotion) private var reduceMotion
    let highlightedAnchor: SettingsSectionAnchor?

    var body: some View {
        ScrollViewReader { proxy in
            Form {
                Section("Calendars") {
                    SettingsHighlightRow(anchor: .advancedCalendars, highlightedAnchor: highlightedAnchor)
                    Toggle("Show completed tasks and dismissed events in calendar views", isOn: showCompletedItemsInCalendarBinding)
                    if model.calendars.isEmpty {
                        Text("Refresh after connecting Google to load calendars.")
                            .foregroundStyle(.secondary)
                    } else {
                        ForEach(model.calendars) { calendar in
                            Toggle(isOn: calendarBinding(calendar.id)) {
                                HStack(spacing: 10) {
                                    Circle()
                                        .fill(Color(hex: calendar.colorHex))
                                        .hcbScaledFrame(width: 10, height: 10)
                                    Text(calendar.summary)
                                }
                            }
                        }
                        Text("Before you customize this list, Hot Cross Buns follows calendars selected in Google Calendar, including subscribed and read-only calendars.")
                            .hcbFont(.caption2)
                            .foregroundStyle(.secondary)
                    }
                }
                .id(SettingsSectionAnchor.advancedCalendars)

                Section("Task lists") {
                    SettingsHighlightRow(anchor: .taskLists, highlightedAnchor: highlightedAnchor)
                    if model.taskLists.isEmpty {
                        Text("Refresh after connecting Google to load task lists.")
                            .foregroundStyle(.secondary)
                    } else {
                        ForEach(model.taskLists) { taskList in
                            Toggle(isOn: taskListBinding(taskList.id)) {
                                Label(taskList.title, systemImage: "checklist")
                            }
                        }
                    }
                }
                .id(SettingsSectionAnchor.taskLists)

                PerTabListFilterSection()
                    .id(SettingsSectionAnchor.perTabFilters)

                DataControlSection()
                    .id(SettingsSectionAnchor.data)
                LocalBackupSection()
                    .id(SettingsSectionAnchor.backups)
                EncryptionSection()
                    .id(SettingsSectionAnchor.encryption)
                HistorySection()
                    .id(SettingsSectionAnchor.history)
                CustomFiltersSection(highlightedAnchor: highlightedAnchor)
                    .id(SettingsSectionAnchor.customFilters)
                TemplatesSection(highlightedAnchor: highlightedAnchor)
                    .id(SettingsSectionAnchor.templates)
            }
            .formStyle(.grouped)
            .onChange(of: highlightedAnchor) { _, anchor in
                scroll(
                    proxy,
                    to: anchor,
                    allowed: [.advancedCalendars, .taskLists, .perTabFilters, .data, .backups, .encryption, .history, .customFilters, .templates],
                    reduceMotion: reduceMotion
                )
            }
        }
    }

    private func calendarBinding(_ id: CalendarListMirror.ID) -> Binding<Bool> {
        Binding(
            get: { model.calendars.first(where: { $0.id == id })?.isSelected ?? false },
            set: { _ in model.toggleCalendar(id) }
        )
    }

    private var showCompletedItemsInCalendarBinding: Binding<Bool> {
        Binding(
            get: { model.settings.showCompletedItemsInCalendar },
            set: { model.setShowCompletedItemsInCalendar($0) }
        )
    }

    private func taskListBinding(_ id: TaskListMirror.ID) -> Binding<Bool> {
        Binding(
            get: { model.isTaskListSelected(id) },
            set: { _ in model.toggleTaskList(id) }
        )
    }
}

struct SettingsHighlightRow: View {
    let anchor: SettingsSectionAnchor
    let highlightedAnchor: SettingsSectionAnchor?

    var body: some View {
        if highlightedAnchor == anchor {
            Label("Matched from settings search", systemImage: "scope")
                .hcbFont(.caption)
                .foregroundStyle(AppColor.ember)
        }
    }
}

private func scroll(
    _ proxy: ScrollViewProxy,
    to anchor: SettingsSectionAnchor?,
    allowed: Set<SettingsSectionAnchor>,
    reduceMotion: Bool
) {
    guard let anchor, allowed.contains(anchor) else { return }
    DispatchQueue.main.async {
        HCBMotion.perform(reduceMotion: reduceMotion, animation: .easeInOut(duration: 0.2)) {
            proxy.scrollTo(anchor, anchor: .top)
        }
    }
}

#Preview {
    HCBSettingsWindow()
        .environment(AppModel.preview)
}
