import Foundation
import os

// Tiered local logger that writes to a rotating file plus bridges to
// os.Logger so entries show up in Console.app under the
// "com.gongahkia.hotcrossbuns.mac" subsystem.
//
// Usage:
//   AppLogger.debug("refreshNow start", category: .sync)
//   AppLogger.error("updateTask failed", category: .mutation, metadata: ["taskID": task.id, "status": "429"])
//
// Ring is capped at 5 rotated files of 2 MB each; older entries roll
// off. DiagnosticsView reads back the tail via recentEntries(limit:).
enum LogLevel: String, Sendable, Comparable, CaseIterable {
    case debug, info, warn, error

    var order: Int {
        switch self {
        case .debug: 0
        case .info: 1
        case .warn: 2
        case .error: 3
        }
    }

    static func < (lhs: LogLevel, rhs: LogLevel) -> Bool { lhs.order < rhs.order }

    var systemSymbol: String {
        switch self {
        case .debug: "ant"
        case .info: "info.circle"
        case .warn: "exclamationmark.triangle"
        case .error: "xmark.octagon"
        }
    }
}

enum LogCategory: String, Sendable, CaseIterable {
    case auth
    case google
    case sync
    case mutation
    case replay
    case notifications
    case cache
    case share
    case mcp
    case ui
    case perf
    case misc
}

enum HCBPerformanceTelemetry {
    private static let enabledValue = "1"

    static var isEnabled: Bool {
        ProcessInfo.processInfo.environment["HCB_PERF_TELEMETRY"] == enabledValue
    }

    static func timestamp() -> UInt64 {
        DispatchTime.now().uptimeNanoseconds
    }

    static func elapsedMilliseconds(since start: UInt64) -> String {
        let elapsed = Double(DispatchTime.now().uptimeNanoseconds - start) / 1_000_000
        return String(format: "%.2f", elapsed)
    }

    static func debug(_ message: String, metadata: [String: String] = [:]) {
        guard isEnabled else { return }
        AppLogger.info(message, category: .perf, metadata: metadata)
    }
}

struct LogEntry: Identifiable, Hashable, Sendable {
    let timestamp: Date
    let level: LogLevel
    let category: LogCategory
    let message: String
    let metadata: [String: String]
    let osMetadata: [String: String]

    var id: String { "\(timestamp.timeIntervalSince1970)-\(message.hashValue)" }

    func formattedLine() -> String {
        let timeString = Self.formatter.string(from: timestamp)
        let metaString = metadata.isEmpty ? "" : " " + metadata
            .sorted { $0.key < $1.key }
            .map { "\($0.key)=\($0.value)" }
            .joined(separator: " ")
        return "[\(timeString)] [\(level.rawValue.uppercased())] [\(category.rawValue)] \(message)\(metaString)"
    }

    static let formatter: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()
}

// Serial actor so concurrent log calls from different tasks don't
// interleave partial lines or race on file rotation.
final class AppLogger: @unchecked Sendable {
    static let shared = AppLogger()

    private let queue = DispatchQueue(label: "com.gongahkia.hotcrossbuns.logger", qos: .utility)
    private let osLogger = Logger(subsystem: "com.gongahkia.hotcrossbuns.mac", category: "app")
    private let maxFileBytes: Int = 2 * 1_024 * 1_024 // 2 MB
    private let rotationKeep = 5
    private var inMemoryRing: [LogEntry] = []
    private let inMemoryCap = 500
    // Only entries at or above this level are persisted + surfaced.
    // Debug logs still write to os.Logger (Console.app) regardless.
    // `let` so the @unchecked Sendable claim holds without ambiguity —
    // `var` would be a latent race surface for any future setter added
    // outside the queue.
    let persistentThreshold: LogLevel = .info

    private init() {}

    static func debug(
        _ message: String,
        category: LogCategory = .misc,
        metadata: [String: String] = [:],
        localOnlyMetadata: [String: String] = [:]
    ) {
        shared.log(level: .debug, message: message, category: category, metadata: metadata, localOnlyMetadata: localOnlyMetadata)
    }

    static func info(
        _ message: String,
        category: LogCategory = .misc,
        metadata: [String: String] = [:],
        localOnlyMetadata: [String: String] = [:]
    ) {
        shared.log(level: .info, message: message, category: category, metadata: metadata, localOnlyMetadata: localOnlyMetadata)
    }

    static func warn(
        _ message: String,
        category: LogCategory = .misc,
        metadata: [String: String] = [:],
        localOnlyMetadata: [String: String] = [:]
    ) {
        shared.log(level: .warn, message: message, category: category, metadata: metadata, localOnlyMetadata: localOnlyMetadata)
    }

    static func error(
        _ message: String,
        category: LogCategory = .misc,
        metadata: [String: String] = [:],
        localOnlyMetadata: [String: String] = [:]
    ) {
        shared.log(level: .error, message: message, category: category, metadata: metadata, localOnlyMetadata: localOnlyMetadata)
    }

    func log(
        level: LogLevel,
        message: String,
        category: LogCategory,
        metadata: [String: String],
        localOnlyMetadata: [String: String] = [:]
    ) {
        let fullMetadata = metadata.merging(localOnlyMetadata) { _, localOnly in localOnly }
        let entry = LogEntry(
            timestamp: Date(),
            level: level,
            category: category,
            message: message,
            metadata: fullMetadata,
            osMetadata: metadata
        )
        bridgeToOSLogger(entry)
        queue.async { [weak self] in
            guard let self else { return }
            self.appendToRing(entry)
            guard level >= self.persistentThreshold else { return }
            self.appendToFile(entry)
        }
    }

    // §10 — metadata values may include error descriptions that transitively
    // carry OAuth / API response fragments, so only non-sensitive framing
    // fields (timestamp, level, category, bare message) go through as public.
    // The metadata map is rendered with privacy: .private so Console.app
    // readers without the device-owner role see it as <private>. The local
    // file ring + in-memory ring still capture the full text for in-app
    // Diagnostics.
    private func bridgeToOSLogger(_ entry: LogEntry) {
        let time = LogEntry.formatter.string(from: entry.timestamp)
        let lvl = entry.level.rawValue.uppercased()
        let cat = entry.category.rawValue
        let msg = entry.message
        let meta = entry.osMetadata.isEmpty ? "" : " " + entry.osMetadata
            .sorted { $0.key < $1.key }
            .map { "\($0.key)=\($0.value)" }
            .joined(separator: " ")
        switch entry.level {
        case .debug: osLogger.debug("[\(time, privacy: .public)] [\(lvl, privacy: .public)] [\(cat, privacy: .public)] \(msg, privacy: .public)\(meta, privacy: .private)")
        case .info: osLogger.info("[\(time, privacy: .public)] [\(lvl, privacy: .public)] [\(cat, privacy: .public)] \(msg, privacy: .public)\(meta, privacy: .private)")
        case .warn: osLogger.warning("[\(time, privacy: .public)] [\(lvl, privacy: .public)] [\(cat, privacy: .public)] \(msg, privacy: .public)\(meta, privacy: .private)")
        case .error: osLogger.error("[\(time, privacy: .public)] [\(lvl, privacy: .public)] [\(cat, privacy: .public)] \(msg, privacy: .public)\(meta, privacy: .private)")
        }
    }

    private func appendToRing(_ entry: LogEntry) {
        inMemoryRing.append(entry)
        if inMemoryRing.count > inMemoryCap {
            inMemoryRing.removeFirst(inMemoryRing.count - inMemoryCap)
        }
    }

    private func appendToFile(_ entry: LogEntry) {
        guard let url = Self.currentLogFileURL() else { return }
        let line = entry.formattedLine() + "\n"
        guard let data = line.data(using: .utf8) else { return }
        do {
            try FileManager.default.createDirectory(
                at: url.deletingLastPathComponent(),
                withIntermediateDirectories: true
            )
            if FileManager.default.fileExists(atPath: url.path) == false {
                try data.write(to: url)
            } else {
                let handle = try FileHandle(forWritingTo: url)
                defer { try? handle.close() }
                try handle.seekToEnd()
                try handle.write(contentsOf: data)
            }
            rotateIfNeeded(at: url)
        } catch {
            // Logger must not itself throw — fall back silently so we
            // don't take down the caller when the log dir is unavailable.
        }
    }

    private func rotateIfNeeded(at url: URL) {
        guard
            let attributes = try? FileManager.default.attributesOfItem(atPath: url.path),
            let size = attributes[.size] as? Int,
            size > maxFileBytes
        else { return }
        // Shift app.log.(rotationKeep-1) → delete; others shift up by 1.
        let directory = url.deletingLastPathComponent()
        let base = url.lastPathComponent
        for index in stride(from: rotationKeep - 1, through: 1, by: -1) {
            let src = directory.appending(path: "\(base).\(index)")
            let dst = directory.appending(path: "\(base).\(index + 1)")
            if index + 1 > rotationKeep {
                try? FileManager.default.removeItem(at: src)
                continue
            }
            if FileManager.default.fileExists(atPath: src.path) {
                try? FileManager.default.removeItem(at: dst)
                try? FileManager.default.moveItem(at: src, to: dst)
            }
        }
        let rotated = directory.appending(path: "\(base).1")
        try? FileManager.default.removeItem(at: rotated)
        try? FileManager.default.moveItem(at: url, to: rotated)
    }

    func recentEntries(limit: Int = 200, minimumLevel: LogLevel = .info) -> [LogEntry] {
        queue.sync {
            inMemoryRing
                .filter { $0.level >= minimumLevel }
                .suffix(limit)
        }
    }

    func loadPersistedLog() -> String {
        Self.persistedLogFileURLs()
            .compactMap { try? String(contentsOf: $0, encoding: .utf8) }
            .filter { $0.isEmpty == false }
            .joined(separator: "\n")
    }

    func clearLogs() {
        queue.sync {
            inMemoryRing.removeAll()
            for url in Self.persistedLogFileURLs(includeMissing: true) {
                try? FileManager.default.removeItem(at: url)
            }
        }
    }

    func clearInMemoryEntries() {
        queue.sync {
            inMemoryRing.removeAll()
        }
    }

    func flush() {
        queue.sync {}
    }

    func currentLogFileURL() -> URL? {
        Self.currentLogFileURL()
    }

    func logDirectoryURL() -> URL? {
        Self.logDirectoryURL()
    }

    static func currentLogFileURL() -> URL? {
        Self.logDirectoryURL()?.appending(path: "app.log")
    }

    static func logDirectoryURL() -> URL? {
        guard let support = FileManager.default.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first else { return nil }
        let bundle = Bundle.main.bundleIdentifier ?? "HotCrossBuns"
        return support
            .appending(path: bundle, directoryHint: .isDirectory)
            .appending(path: "logs", directoryHint: .isDirectory)
    }

    private static func persistedLogFileURLs(includeMissing: Bool = false) -> [URL] {
        guard let current = currentLogFileURL() else { return [] }
        let directory = current.deletingLastPathComponent()
        let base = current.lastPathComponent
        let rotated = stride(from: 5, through: 1, by: -1).map {
            directory.appending(path: "\(base).\($0)")
        }
        return (rotated + [current]).filter { includeMissing || FileManager.default.fileExists(atPath: $0.path) }
    }
}
