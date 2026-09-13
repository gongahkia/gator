import Foundation

// Light-touch crash capture: writes a text log on uncaught Obj-C
// exceptions and on common fatal signals (SIGABRT, SIGSEGV, SIGILL,
// SIGBUS). The log lives alongside the cache in Application Support so
// a subsequent launch can surface it via DiagnosticsView.
//
// Limitations: this does not symbolicate, and pure-Swift runtime
// crashes (preconditionFailure, array out of bounds, force-unwrap on
// nil, etc.) land in the signal handler as SIGABRT/SIGILL without a
// Swift stack. A proper crash reporter (Sentry, KSCrash, etc.) would
// be a bigger integration — this file gives enough breadcrumb to tell
// that *something* went wrong between launches.
enum CrashReporter {
    private static let logFilename = "crash.log"

    static func install() {
        NSSetUncaughtExceptionHandler { exception in
            CrashReporter.writeLog(
                header: "Uncaught Obj-C exception",
                detail: "\(exception.name.rawValue): \(exception.reason ?? "")",
                stack: exception.callStackSymbols.joined(separator: "\n")
            )
        }
        for signalValue in [SIGABRT, SIGSEGV, SIGILL, SIGBUS, SIGFPE, SIGPIPE] as [Int32] {
            signal(signalValue) { received in
                CrashReporter.writeLog(
                    header: "Fatal signal \(received)",
                    detail: "Process terminated by signal \(received).",
                    stack: Thread.callStackSymbols.joined(separator: "\n")
                )
                // Re-raise so the OS still produces a system crash report.
                signal(received, SIG_DFL)
                raise(received)
            }
        }
    }

    static func readLastCrash() -> String? {
        guard let url = logFileURL(),
              FileManager.default.fileExists(atPath: url.path)
        else { return nil }
        return try? String(contentsOf: url, encoding: .utf8)
    }

    static func clearLastCrash() {
        guard let url = logFileURL() else { return }
        try? FileManager.default.removeItem(at: url)
    }

    private static func writeLog(header: String, detail: String, stack: String) {
        guard let url = logFileURL() else { return }
        let timestamp = ISO8601DateFormatter().string(from: Date())
        let contents = """
        \(header)
        When: \(timestamp)
        Detail: \(detail)

        Stack:
        \(stack)
        """
        try? FileManager.default.createDirectory(
            at: url.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )
        try? contents.data(using: .utf8)?.write(to: url, options: .atomic)
    }

    private static func logFileURL() -> URL? {
        guard let appSupport = FileManager.default.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first else { return nil }
        let appDir = Bundle.main.bundleIdentifier ?? "HotCrossBuns"
        return appSupport
            .appending(path: appDir, directoryHint: .isDirectory)
            .appending(path: logFilename)
    }
}
