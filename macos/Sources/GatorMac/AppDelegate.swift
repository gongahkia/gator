import AppKit
import GatorCore

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    var application: ManagedSessionCoordinator?
    var quickAccess: NotchQuickAccess?
    var openConversationWindow: (() -> Void)?
    private var terminating = false
    private var mayTerminate = false
    private var presentationObservers: [NSObjectProtocol] = []
    func applicationDidFinishLaunching(_ notification: Notification) {
        for name in [NSWindow.didBecomeKeyNotification, NSWindow.willCloseNotification] {
            presentationObservers.append(NotificationCenter.default.addObserver(forName: name, object: nil, queue: .main) { _ in
                Task { @MainActor in
                    await Task.yield()
                    NativeApplicationPresentation.update()
                }
            })
        }
        NativeApplicationPresentation.update()
    }
    func applicationWillTerminate(_ notification: Notification) {
        quickAccess?.stop()
        for observer in presentationObservers { NotificationCenter.default.removeObserver(observer) }
        presentationObservers.removeAll()
    }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows: Bool) -> Bool {
        // a notch panel may be visible even when the conversation is closed.
        openConversationWindow?()
        return true
    }
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        if mayTerminate { return .terminateNow }
        requestQuit()
        return .terminateCancel
    }
    // Native controls and Cmd+Q enter this guard directly. Termination is only
    // requested again after the draft flush and child cleanup have succeeded.
    func requestQuit(_ state: ManagedSessionCoordinator? = nil) {
        if let state { application = state }
        guard !terminating else { return }
        guard let application else { mayTerminate = true; NSApp.terminate(nil); return }
        terminating = true
        var confirmed = false
        if let reason = application.quitGuard {
            let alert = NSAlert()
            alert.messageText = "Quit Gator?"; alert.informativeText = reason
            alert.alertStyle = .warning
            alert.addButton(withTitle: "Keep Gator Open"); alert.addButton(withTitle: "Quit and Keep Retained Work")
            NSApp.activate(ignoringOtherApps: true)
            guard alert.runModal() == .alertSecondButtonReturn else { terminating = false; return }
            confirmed = true
        }
        Task {
            let success = await application.shutdown(confirmed: confirmed)
            if !success {
                terminating = false
                let alert = NSAlert(); alert.messageText = "Gator stayed open"
                alert.informativeText = application.persistenceIssue.isEmpty ? (application.issue.isEmpty ? "Work changed while preparing to quit. Review it and try again." : application.issue) : application.persistenceIssue
                alert.runModal()
            }
            if success { mayTerminate = true; NSApp.terminate(nil) }
        }
    }
}
