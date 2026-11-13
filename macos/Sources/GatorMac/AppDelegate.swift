import AppKit
import GatorCore

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    var application: GatorApplicationState?
    private var terminating = false
    private var mayTerminate = false
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
    }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        if mayTerminate { return .terminateNow }
        requestQuit()
        return .terminateCancel
    }
    // Native controls and Cmd+Q enter this guard directly. Termination is only
    // requested again after the draft flush and child cleanup have succeeded.
    func requestQuit(_ state: GatorApplicationState? = nil) {
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
                alert.informativeText = application.persistenceIssue.isEmpty ? "Work changed while preparing to quit. Review it and try again." : application.persistenceIssue
                alert.runModal()
            }
            if success { mayTerminate = true; NSApp.terminate(nil) }
        }
    }
}
