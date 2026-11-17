import SwiftUI
import AppKit
import GatorCore

@main
@MainActor
struct GatorApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @State private var application = NativeEnvironment.application()
    @State private var readingPositions = NativeReadingPositions()

    var body: some Scene {
        MenuBarExtra {
            ConversationView(compact: true, quit: { delegate.requestQuit(application) })
                .environment(application)
                .environment(readingPositions)
                .frame(width: 440, height: 620)
                .task { delegate.application = application; await application.start() }
        } label: {
            GatorMenuBarLabel(sessions: application, delegate: delegate)
        }
        .menuBarExtraStyle(.window)

        Window("Gator", id: "conversation") {
            ConversationView(compact: false, quit: { delegate.requestQuit(application) })
                .environment(application)
                .environment(readingPositions)
                .frame(minWidth: 540, minHeight: 480)
                .task { delegate.application = application; await application.start() }
        }
        .defaultSize(width: 820, height: 740)
        .windowToolbarStyle(.unified)
        .defaultLaunchBehavior(ProcessInfo.processInfo.arguments.contains("--open-window") ? .presented : .suppressed)
        .commands {
            CommandGroup(replacing: .pasteboard) {
                Button("Cut") { NSApp.sendAction(#selector(NSText.cut(_:)), to: nil, from: nil) }.keyboardShortcut("x")
                Button("Copy") { NSApp.sendAction(#selector(NSText.copy(_:)), to: nil, from: nil) }.keyboardShortcut("c")
                Button("Paste") {
                    NativeInputTrace.record("paste menu invoked textResponder=\(NSApp.keyWindow?.firstResponder is NSTextView)")
                    let delivered = NSApp.sendAction(#selector(NSText.paste(_:)), to: nil, from: nil)
                    NativeInputTrace.record("paste menu delivered=\(delivered)")
                }.keyboardShortcut("v")
                Button("Select All") { NSApp.sendAction(#selector(NSText.selectAll(_:)), to: nil, from: nil) }.keyboardShortcut("a")
            }
            CommandMenu("Conversation") {
                Menu("Recent Conversations") { RecentConversationItems(sessions: application) }
                Button("Show Notch Quick Access") { delegate.quickAccess?.show() }.keyboardShortcut("g", modifiers: [.command, .shift])
                    .disabled(delegate.quickAccess?.canShow != true)
                Button("Focus Message") { application.focusComposer() }.keyboardShortcut("l")
                Button("Inspect Current Task") { application.inspectTask(application.current.conversationID) }.keyboardShortcut("i")
                Button("Review Pending Action") { application.closeTaskInspection(); application.current.openApprovalReview() }.keyboardShortcut("r", modifiers: [.command, .shift]).disabled(!application.current.hasReviewableApproval)
            }
            CommandGroup(replacing: .appTermination) {
                Button("Quit Gator") { delegate.requestQuit(application) }.keyboardShortcut("q")
            }
            CommandGroup(replacing: .newItem) {
                Button("New Conversation") { Task { await application.newConversation() } }
                    .keyboardShortcut("n").disabled(!application.canNavigate)
            }
        }
    }
}

@MainActor
enum NativeEnvironment {
    static func application() -> ManagedSessionCoordinator {
        let env = ProcessInfo.processInfo.environment, home = FileManager.default.homeDirectoryForCurrentUser
        let data = env["GATOR_NATIVE_USER_DATA"].map { URL(fileURLWithPath: $0, isDirectory: true) }
            ?? home.appendingPathComponent("Library/Application Support/Gator", isDirectory: true)
        let workspace = env["GATOR_NATIVE_WORKSPACE"].map { URL(fileURLWithPath: $0, isDirectory: true) }
            ?? data.appendingPathComponent("workspace", isDirectory: true)
        let runtime = env["GATOR_NATIVE_RUNTIME"].map { URL(fileURLWithPath: $0) }
            ?? (Bundle.main.resourceURL ?? Bundle.main.bundleURL).appendingPathComponent("gator-runtime")
        let state = env["GATOR_STATE_DIR"] ?? env["XDG_STATE_HOME"] ?? home.appendingPathComponent(".local/state").path
        let profile = URL(fileURLWithPath: state, relativeTo: workspace).standardizedFileURL.path
        return ManagedSessionCoordinator(configuration: BridgeLaunchConfiguration(runtime: runtime, workspace: workspace, environment: env), nativeDraftDirectory: data.appendingPathComponent("native/drafts", isDirectory: true), profile: profile)
    }
}

@MainActor
enum GatorBrand {
    static let menuBarIcon: NSImage = {
        let image = Bundle.main.url(forResource: "GatorMenuBar", withExtension: "png")
            .flatMap(NSImage.init(contentsOf:))
            ?? NSImage(systemSymbolName: "lizard", accessibilityDescription: "Gator")!
        image.size = NSSize(width: 18, height: 18)
        image.isTemplate = true
        image.accessibilityDescription = "Gator"
        return image
    }()
}
