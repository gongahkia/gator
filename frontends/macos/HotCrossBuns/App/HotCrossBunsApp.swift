import SwiftUI

enum HCBLaunchMode {
    case normal
    case smokeTest
    case unitTest

    static let current: HCBLaunchMode = {
        let processInfo = ProcessInfo.processInfo
        if processInfo.arguments.contains("--smoke-test") {
            return .smokeTest
        }
        if processInfo.environment["XCTestConfigurationFilePath"] != nil {
            return .unitTest
        }
        return .normal
    }()

    var isSmokeTest: Bool {
        self == .smokeTest
    }

    var skipsAppStartupWork: Bool {
        switch self {
        case .normal:
            return false
        case .smokeTest, .unitTest:
            return true
        }
    }

    var logValue: String {
        switch self {
        case .normal:
            return "normal"
        case .smokeTest:
            return "smoke-test"
        case .unitTest:
            return "unit-test"
        }
    }
}

@main
struct HotCrossBunsApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @State private var appModel = HCBLaunchMode.current == .unitTest
        ? AppModel.testHostBootstrap()
        : AppModel.bootstrap()
    @State private var updater = UpdaterController()
    @State private var networkMonitor = NetworkMonitor()

    init() {
        CrashReporter.install()
        AppLogger.info("app launch", category: .misc, metadata: [
            "version": Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "?",
            "build": Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "?",
            "launchMode": HCBLaunchMode.current.logValue
        ])
    }

    var body: some Scene {
        // The planner is intentionally a singleton `Window`, not a
        // `WindowGroup`: all Dock, menu-bar, Services, deep-link, and App
        // Intent entry points return to one source-list workspace backed by
        // the shared AppModel. Duplicate planner windows would make selection,
        // sheet, sync, and undo state ambiguous; HCBMainWindowPresenter owns
        // reopening/focusing this scene from AppKit surfaces.
        Window("Hot Cross Buns", id: "main") {
            MacSidebarShell()
                .environment(appModel)
                .environment(\.locale, appModel.settings.appLanguage.locale)
                .environment(updater)
                .environment(networkMonitor)
                .environment(\.globalHotkeyConfigurator, globalHotkeyConfigurator)
                .hcbMenuBarStatusController(appDelegate.menuBarStatusController, model: appModel)
                .hcbScaledFrame(minWidth: 900, minHeight: 600)
                .dockBadge(
                    overdueCount: appModel.todaySnapshot.overdueCount,
                    enabled: appModel.settings.showDockBadge
                )
                .overlay {
                    WindowSessionRestorer(settings: appModel.settings)
                }
                .hcbWindowRestoration(.main, settings: appModel.settings)
        }
        .defaultSize(width: 1100, height: 720)
        .windowResizability(.contentMinSize)
        .windowToolbarStyle(UnifiedWindowToolbarStyle(showsTitle: false))
        .commands {
            AppCommands()
            CommandGroup(after: .appInfo) {
                Button("Check for Updates…") {
                    updater.checkForUpdates()
                }
            }
        }

        // Dedicated Settings scene. macOS wires ⌘, automatically; the menu
        // item lives under the app menu as "Settings…" per the system
        // convention shown in Apple Calendar and peers. contentMinSize lets
        // the user resize the window while enforcing the minimum defined
        // on HCBSettingsWindow's frame.
        Settings {
            HCBSettingsWindow()
                .environment(appModel)
                .environment(\.locale, appModel.settings.appLanguage.locale)
                .environment(updater)
                .environment(networkMonitor)
                .environment(\.globalHotkeyConfigurator, globalHotkeyConfigurator)
                .hcbMenuBarStatusController(appDelegate.menuBarStatusController, model: appModel)
                .hcbStoredTransitionContent(
                    key: HCBTransitionKeys.settings,
                    metadata: ["window": "settings"]
                )
        }
        .windowResizability(.contentMinSize)

        Window("Hot Cross Buns Help", id: "help") {
            HelpView()
                .environment(appModel)
                .environment(\.locale, appModel.settings.appLanguage.locale)
                .withHCBAppearance(appModel.settings)
                .hcbPreferredColorScheme(appModel.settings)
                .hcbMenuBarStatusController(appDelegate.menuBarStatusController, model: appModel)
                .hcbWindowRestoration(.help, settings: appModel.settings)
        }
        .defaultSize(width: 720, height: 620)
        .windowResizability(.contentMinSize)

        // Floating ledger — opened via View menu (⌘⌥Y), Settings "Open history…" button, or the MenuBar.
        Window("History", id: "history") {
            HistoryWindow()
                .environment(appModel)
                .environment(\.locale, appModel.settings.appLanguage.locale)
                .withHCBAppearance(appModel.settings)
                .hcbPreferredColorScheme(appModel.settings)
                .hcbMenuBarStatusController(appDelegate.menuBarStatusController, model: appModel)
                .hcbWindowRestoration(.history, settings: appModel.settings)
        }
        .defaultSize(width: 760, height: 560)
        .windowResizability(.contentMinSize)

        Window("Sync Issues", id: "sync-issues") {
            SyncIssuesWindow()
                .environment(appModel)
                .environment(\.locale, appModel.settings.appLanguage.locale)
                .withHCBAppearance(appModel.settings)
                .hcbPreferredColorScheme(appModel.settings)
                .hcbMenuBarStatusController(appDelegate.menuBarStatusController, model: appModel)
                .hcbWindowRestoration(.syncIssues, settings: appModel.settings)
        }
        .defaultSize(width: 760, height: 620)
        .windowResizability(.contentMinSize)

        Window("Diagnostics and Recovery", id: "diagnostics") {
            DiagnosticsView()
                .environment(appModel)
                .environment(\.locale, appModel.settings.appLanguage.locale)
                .withHCBAppearance(appModel.settings)
                .hcbPreferredColorScheme(appModel.settings)
                .hcbMenuBarStatusController(appDelegate.menuBarStatusController, model: appModel)
                .hcbWindowRestoration(.diagnostics, settings: appModel.settings)
                .hcbStoredTransitionContent(
                    key: HCBTransitionKeys.window("diagnostics"),
                    metadata: ["window": "diagnostics"]
                )
        }
        .defaultSize(width: 860, height: 680)
        .windowResizability(.contentMinSize)

        Window("Review Duplicates", id: "duplicate-review") {
            DuplicateReviewWindow()
                .environment(appModel)
                .environment(\.locale, appModel.settings.appLanguage.locale)
                .withHCBAppearance(appModel.settings)
                .hcbPreferredColorScheme(appModel.settings)
                .hcbMenuBarStatusController(appDelegate.menuBarStatusController, model: appModel)
        }
        .defaultSize(width: 860, height: 620)
        .windowResizability(.contentMinSize)

        Window("Update Available", id: "update-available") {
            UpdateAvailableWindow()
                .environment(updater)
                .environment(\.locale, appModel.settings.appLanguage.locale)
                .withHCBAppearance(appModel.settings)
                .hcbPreferredColorScheme(appModel.settings)
                .hcbMenuBarStatusController(appDelegate.menuBarStatusController, model: appModel)
        }
        .defaultSize(width: 620, height: 560)
        .windowResizability(.contentMinSize)

        Window("Install Update", id: "install-update") {
            InstallUpdateWindow()
                .environment(updater)
                .environment(\.locale, appModel.settings.appLanguage.locale)
                .withHCBAppearance(appModel.settings)
                .hcbPreferredColorScheme(appModel.settings)
                .hcbMenuBarStatusController(appDelegate.menuBarStatusController, model: appModel)
        }
        .defaultSize(width: 520, height: 360)
        .windowResizability(.contentMinSize)
    }

    private var globalHotkeyConfigurator: GlobalHotkeyConfigurator {
        GlobalHotkeyConfigurator { enabled, binding, model in
            appDelegate.appModel = model
            return appDelegate.configureGlobalHotkey(enabled: enabled, binding: binding)
        }
    }
}
