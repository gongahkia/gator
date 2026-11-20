#if DEBUG
import AppKit
import CryptoKit
import GatorCore

// read-only, opt-in diagnostics for disposable native acceptance profiles.
@MainActor final class NativeAcceptanceDiagnostics {
    private weak var sessions: ManagedSessionCoordinator?
    private let directory: URL
    private var source: (any DispatchSourceSignal)?
    private var sequence = 0

    init?(_ sessions: ManagedSessionCoordinator) {
        guard let path = ProcessInfo.processInfo.environment["GATOR_NATIVE_ACCEPTANCE_DIRECTORY"] else { return nil }
        let directory = URL(fileURLWithPath: path).resolvingSymlinksInPath()
        let prefix = URL(fileURLWithPath: "/tmp").resolvingSymlinksInPath().appendingPathComponent("gator-native-test-presence-r2-").path
        guard directory.path.hasPrefix(prefix), FileManager.default.fileExists(atPath: directory.path) else { return nil }
        self.directory = directory; self.sessions = sessions
        signal(SIGUSR2, SIG_IGN)
        let source = DispatchSource.makeSignalSource(signal: SIGUSR2, queue: .main)
        source.setEventHandler { [weak self] in MainActor.assumeIsolated { self?.capture() } }
        self.source = source; source.resume()
        capture()
    }
    func stop() { source?.cancel(); source = nil }
    private func capture() {
        guard let sessions else { return }
        sequence += 1
        let owner = sessions.current, review = owner.currentApprovalReview
        let token = review.map { SHA256.hash(data: Data($0.token.utf8)).map { String(format: "%02x", $0) }.joined() } ?? ""
        var canvases: [NativeMascotCanvas] = []
        func visit(_ view: NSView) {
            if let canvas = view as? NativeMascotCanvas { canvases.append(canvas) }
            for child in view.subviews { visit(child) }
        }
        for window in NSApp.windows { if let view = window.contentView { visit(view) } }
        let value: [String: Any] = [
            "pid": getpid(), "sequence": sequence, "epoch": Date().timeIntervalSince1970,
            "conversation": owner.conversationID, "generation": owner.generation.uuidString,
            "pendingInteraction": review?.interactionID ?? 0, "tokenSHA256": token,
            "messageIndex": review?.messageIndex ?? -1, "reviewVisible": owner.review != nil,
            "ownerCount": sessions.ownerCount, "closing": sessions.stopping,
            "windows": NSApp.windows.map { ["id": $0.identifier?.rawValue ?? "", "visible": $0.isVisible, "key": $0.isKeyWindow] as [String: Any] },
            "canvases": canvases.map { ["activeFrameBytes": $0.player.activeFrameBytes, "animations": $0.player.layer.animationKeys()?.count ?? 0] }
        ]
        do {
            let data = try JSONSerialization.data(withJSONObject: value, options: [.sortedKeys])
            let destination = directory.appendingPathComponent("native-diagnostics.json")
            try data.write(to: destination, options: .atomic)
            try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: destination.path)
        } catch { /* diagnostics cannot change execution or presentation on failure. */ }
    }
}
#endif
