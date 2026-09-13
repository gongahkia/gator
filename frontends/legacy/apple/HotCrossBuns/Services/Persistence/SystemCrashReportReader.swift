import Foundation

// Reads macOS-generated crash reports from `~/Library/Logs/Diagnostic
// Reports`. Unlike our own CrashReporter breadcrumbs, these are fully
// symbolicated against the system's dSYM cache, so Swift runtime
// fatals (preconditionFailure, force-unwrap on nil, array-out-of-
// bounds, Mach exceptions) show up as proper stacks pointing to the
// file and line that crashed. No third-party SDK, no cloud upload —
// the OS writes these for every app crash whether we ask or not; we
// just surface them.
//
// File format on macOS 12+: `.ips` is a JSON header line followed by a
// JSON body. We don't parse it — the user just needs to see / copy /
// reveal it in Finder, so we treat the whole file as opaque text.
struct SystemCrashReport: Identifiable, Hashable, Sendable {
    var url: URL
    var modificationDate: Date
    var filename: String { url.lastPathComponent }
    var id: String { url.path }
}

enum SystemCrashReportReader {
    // Reports are indexed by executable name (filename prefix) rather
    // than bundle ID because the filename uses the executable, and
    // reading each `.ips` body just to verify the bundle ID is overkill.
    // `HotCrossBunsMac` is our executable name (see project.yml).
    private static var executableMatchPrefixes: [String] {
        var prefixes: [String] = []
        if let name = Bundle.main.infoDictionary?["CFBundleExecutable"] as? String, name.isEmpty == false {
            prefixes.append(name)
        }
        prefixes.append("HotCrossBuns")
        return prefixes
    }

    static func recentReports(limit: Int = 5) -> [SystemCrashReport] {
        guard let directory = directoryURL() else { return [] }
        let fileManager = FileManager.default
        guard let entries = try? fileManager.contentsOfDirectory(
            at: directory,
            includingPropertiesForKeys: [.contentModificationDateKey],
            options: [.skipsHiddenFiles]
        ) else { return [] }

        let prefixes = executableMatchPrefixes
        let matched = entries.compactMap { url -> SystemCrashReport? in
            let name = url.lastPathComponent
            let isReport = name.hasSuffix(".ips") || name.hasSuffix(".crash") || name.hasSuffix(".hang")
            guard isReport else { return nil }
            guard prefixes.contains(where: { name.hasPrefix($0) }) else { return nil }
            let mod = (try? url.resourceValues(forKeys: [.contentModificationDateKey]))?.contentModificationDate ?? .distantPast
            return SystemCrashReport(url: url, modificationDate: mod)
        }
        .sorted { $0.modificationDate > $1.modificationDate }
        return Array(matched.prefix(limit))
    }

    // Reads the file contents as UTF-8 text. Returns nil if the file
    // can't be read (sandbox, permissions, deleted mid-read).
    static func readContents(of report: SystemCrashReport) -> String? {
        try? String(contentsOf: report.url, encoding: .utf8)
    }

    static func directoryURL() -> URL? {
        let home = FileManager.default.homeDirectoryForCurrentUser
        return home
            .appending(path: "Library", directoryHint: .isDirectory)
            .appending(path: "Logs", directoryHint: .isDirectory)
            .appending(path: "DiagnosticReports", directoryHint: .isDirectory)
    }
}
