import CryptoKit
import Foundation

enum TaskMarkdownExporter {
    static func markdown(for task: TaskMirror, taskListTitle: String? = nil) -> String {
        var lines: [String] = []
        let bullet = task.isCompleted ? "- [x]" : "- [ ]"
        lines.append("\(bullet) \(task.title)")
        if let taskListTitle, taskListTitle.isEmpty == false {
            lines.append("  - List: \(taskListTitle)")
        }
        if let due = task.dueDate {
            lines.append("  - Due: \(due.formatted(.dateTime.weekday(.wide).month(.wide).day().year()))")
        }
        if task.notes.isEmpty == false {
            lines.append("  - Notes: \(task.notes)")
        }
        return lines.joined(separator: "\n")
    }
}

enum EventMarkdownExporter {
    static func markdown(for event: CalendarEventMirror, calendarTitle: String? = nil) -> String {
        var lines: [String] = []
        lines.append("## \(event.summary)")
        if let calendarTitle, calendarTitle.isEmpty == false {
            lines.append("- Calendar: \(calendarTitle)")
        }
        if event.location.isEmpty == false {
            lines.append("- Location: \(event.location)")
        }
        if event.isAllDay {
            lines.append("- When: \(event.startDate.formatted(.dateTime.weekday(.wide).month(.wide).day().year())) (all day)")
        } else {
            let start = event.startDate.formatted(.dateTime.weekday(.wide).month(.wide).day().year().hour().minute())
            let end = event.endDate.formatted(.dateTime.hour().minute())
            lines.append("- When: \(start) – \(end)")
        }
        if event.details.isEmpty == false {
            lines.append("")
            lines.append(event.details)
        }
        return lines.joined(separator: "\n")
    }
}

enum EventICSExporter {
    static func ics(for event: CalendarEventMirror) -> String {
        wrap(veventBlocks(for: [event]))
    }

    static func ics(for events: [CalendarEventMirror]) -> String {
        wrap(veventBlocks(for: events))
    }

    private static func wrap(_ vevents: [String]) -> String {
        var lines: [String] = [
            "BEGIN:VCALENDAR",
            "VERSION:2.0",
            "PRODID:-//Hot Cross Buns//EN",
            "CALSCALE:GREGORIAN"
        ]
        for block in vevents {
            lines.append(contentsOf: block.split(separator: "\r\n", omittingEmptySubsequences: false).map(String.init))
        }
        lines.append("END:VCALENDAR")
        return lines.joined(separator: "\r\n") + "\r\n"
    }

    private static func veventBlocks(for events: [CalendarEventMirror]) -> [String] {
        let now = Date()
        return events.map { event in
            let uid = "\(event.id)@hotcrossbuns"
            var lines: [String] = [
                "BEGIN:VEVENT",
                "UID:\(escape(uid))",
                "DTSTAMP:\(icsTimestamp(now))",
                "SUMMARY:\(escape(event.summary))"
            ]
            if event.isAllDay {
                lines.append("DTSTART;VALUE=DATE:\(icsDate(event.startDate))")
                lines.append("DTEND;VALUE=DATE:\(icsDate(event.endDate))")
            } else {
                lines.append("DTSTART:\(icsTimestamp(event.startDate))")
                lines.append("DTEND:\(icsTimestamp(event.endDate))")
            }
            if event.details.isEmpty == false {
                lines.append("DESCRIPTION:\(escape(event.details))")
            }
            if event.location.isEmpty == false {
                lines.append("LOCATION:\(escape(event.location))")
            }
            for minutes in event.reminderMinutes {
                lines.append("BEGIN:VALARM")
                lines.append("ACTION:DISPLAY")
                lines.append("DESCRIPTION:\(escape(event.summary))")
                lines.append("TRIGGER:-PT\(minutes)M")
                lines.append("END:VALARM")
            }
            lines.append("END:VEVENT")
            return lines.joined(separator: "\r\n")
        }
    }

    private static let utcFormatter: DateFormatter = {
        let f = DateFormatter()
        f.calendar = Calendar(identifier: .gregorian)
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = TimeZone(identifier: "UTC")
        f.dateFormat = "yyyyMMdd'T'HHmmss'Z'"
        return f
    }()

    private static let dateOnlyFormatter: DateFormatter = {
        let f = DateFormatter()
        f.calendar = Calendar(identifier: .gregorian)
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = TimeZone(identifier: "UTC")
        f.dateFormat = "yyyyMMdd"
        return f
    }()

    private static func icsTimestamp(_ date: Date) -> String {
        utcFormatter.string(from: date)
    }

    private static func icsDate(_ date: Date) -> String {
        dateOnlyFormatter.string(from: date)
    }

    private static func escape(_ value: String) -> String {
        value
            .replacingOccurrences(of: "\\", with: "\\\\")
            .replacingOccurrences(of: ";", with: "\\;")
            .replacingOccurrences(of: ",", with: "\\,")
            .replacingOccurrences(of: "\n", with: "\\n")
    }
}

struct PortableExportSummary: Equatable, Sendable {
    var directoryURL: URL
    var copiedAttachmentCount: Int
    var skippedAttachmentCount: Int
}

struct PortableExportOptions: Equatable, Sendable {
    var taskListIDs: Set<String>?
    var calendarIDs: Set<String>?
    var eventsStartingAtOrAfter: Date?

    static let all = PortableExportOptions()
}

struct PortableImportOptions: Equatable, Sendable {
    var includeTasks: Bool = true
    var includeEvents: Bool = true
    var includeSettings: Bool = true
    var includeSyncMetadata: Bool = true

    static let all = PortableImportOptions()
}

struct PortableImportPreview: Equatable, Sendable {
    var archiveURL: URL
    var taskCount: Int
    var eventCount: Int
    var calendarCount: Int
    var taskListCount: Int
    var bundledAttachmentCount: Int
    var missingBundledAttachmentCount: Int
    var corruptBundledAttachmentCount: Int
    var skippedPointerCount: Int
    var diff: PortableImportDiff?
}

struct PortableImportDiff: Equatable, Sendable {
    var tasks: PortableImportResourceDiff
    var events: PortableImportResourceDiff
    var calendars: PortableImportResourceDiff
    var taskLists: PortableImportResourceDiff
    var settingsWillChange: Bool
    var pendingMutationCount: Int

    var hasChanges: Bool {
        tasks.hasChanges
            || events.hasChanges
            || calendars.hasChanges
            || taskLists.hasChanges
            || settingsWillChange
            || pendingMutationCount > 0
    }
}

struct PortableImportResourceDiff: Equatable, Sendable {
    var addedItems: [PortableImportDiffItem]
    var removedItems: [PortableImportDiffItem]
    var changedItems: [PortableImportDiffItem]

    var added: Int { addedItems.count }
    var removed: Int { removedItems.count }
    var changed: Int { changedItems.count }

    var hasChanges: Bool {
        added > 0 || removed > 0 || changed > 0
    }
}

struct PortableImportDiffItem: Identifiable, Equatable, Sendable {
    var id: String
    var currentTitle: String?
    var incomingTitle: String?

    var displayTitle: String {
        incomingTitle ?? currentTitle ?? id
    }
}

struct PortableImportSummary: Equatable, Sendable {
    var importedTaskCount: Int
    var importedEventCount: Int
    var importedAttachmentCount: Int
    var missingBundledAttachmentCount: Int
    var corruptBundledAttachmentCount: Int
    var skippedPointerCount: Int
    var preImportBackupURL: URL?
}

struct PortableExportManifest: Codable, Equatable, Sendable {
    static let currentFormatVersion = 1

    struct Attachment: Codable, Equatable, Sendable {
        var kind: String
        var displayName: String
        var originalURL: String
        var bundledRelativePath: String
        var sha256: String?
        var byteCount: Int?
    }

    var formatVersion: Int = currentFormatVersion
    var exportedAt: Date = Date()
    var appVersion: String = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "unknown"
    var stateFile: String = "hot-cross-buns-state.json"
    var attachmentDirectory: String = "Attachments"
    var attachments: [Attachment]
    var skippedPointers: [String]
    var notes: [String] = [
        "hot-cross-buns-state.json preserves settings, connected account metadata, tasks, notes, events, calendars, sync checkpoints, pending mutations, and original local pointer text. Google OAuth tokens are not exported.",
        "Attachments contains reachable local files referenced by Local image/file pointers. Missing, unreadable, and corrupted image pointers are listed in skippedPointers."
    ]
}

enum PortableExportArchive {
    static func write(
        state: CachedAppState,
        to directoryURL: URL,
        options: PortableExportOptions = .all
    ) throws -> PortableExportSummary {
        if FileManager.default.fileExists(atPath: directoryURL.path) {
            throw PortableExportError.destinationAlreadyExists
        }

        let state = filteredState(state, options: options)
        try FileManager.default.createDirectory(at: directoryURL, withIntermediateDirectories: true)
        let attachmentsURL = directoryURL.appending(path: "Attachments", directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: attachmentsURL, withIntermediateDirectories: true)

        let attachments = collectAttachments(in: state)
        var copied: [PortableExportManifest.Attachment] = []
        var skipped: [String] = []
        var copiedByOriginalURL: [String: String] = [:]

        for attachment in attachments {
            let original = attachment.url.absoluteString
            if let existingRelativePath = copiedByOriginalURL[original] {
                copied.append(.init(
                    kind: attachment.kind.rawValue,
                    displayName: attachment.displayName,
                    originalURL: original,
                    bundledRelativePath: existingRelativePath,
                    sha256: copied.first(where: { $0.originalURL == original })?.sha256,
                    byteCount: copied.first(where: { $0.originalURL == original })?.byteCount
                ))
                continue
            }

            guard attachment.canExportOrDownload else {
                skipped.append(original)
                continue
            }

            let didStartAccessing = attachment.url.startAccessingSecurityScopedResource()
            defer {
                if didStartAccessing {
                    attachment.url.stopAccessingSecurityScopedResource()
                }
            }

            let destination = LocalAttachmentStore.uniqueURL(
                in: attachmentsURL,
                preferredName: attachment.url.lastPathComponent
            )
            do {
                try FileManager.default.copyItem(at: attachment.url, to: destination)
                let metadata = try fileIntegrityMetadata(for: destination)
                let relativePath = "Attachments/\(destination.lastPathComponent)"
                copiedByOriginalURL[original] = relativePath
                copied.append(.init(
                    kind: attachment.kind.rawValue,
                    displayName: attachment.displayName,
                    originalURL: original,
                    bundledRelativePath: relativePath,
                    sha256: metadata.sha256,
                    byteCount: metadata.byteCount
                ))
            } catch {
                skipped.append(original)
            }
        }

        let stateData = try JSONEncoder.portableExport.encode(state)
        try stateData.write(to: directoryURL.appending(path: "hot-cross-buns-state.json"), options: [.atomic])

        let manifest = PortableExportManifest(attachments: copied, skippedPointers: skipped)
        let manifestData = try JSONEncoder.portableExport.encode(manifest)
        try manifestData.write(to: directoryURL.appending(path: "manifest.json"), options: [.atomic])

        return PortableExportSummary(
            directoryURL: directoryURL,
            copiedAttachmentCount: copied.count,
            skippedAttachmentCount: skipped.count
        )
    }

    static func previewImport(from archiveURL: URL, comparingTo currentState: CachedAppState? = nil) throws -> PortableImportPreview {
        let manifest = try readManifest(from: archiveURL)
        let state = try readState(from: archiveURL, manifest: manifest)
        let uniqueBundledAttachments = uniqueBundledAttachmentCount(in: manifest, archiveURL: archiveURL)
        return PortableImportPreview(
            archiveURL: archiveURL,
            taskCount: state.tasks.count,
            eventCount: state.events.count,
            calendarCount: state.calendars.count,
            taskListCount: state.taskLists.count,
            bundledAttachmentCount: uniqueBundledAttachments.total,
            missingBundledAttachmentCount: uniqueBundledAttachments.missing,
            corruptBundledAttachmentCount: uniqueBundledAttachments.corrupt,
            skippedPointerCount: manifest.skippedPointers.count,
            diff: currentState.map { diff(from: $0, to: state) }
        )
    }

    static func importState(
        from archiveURL: URL,
        attachmentsDirectoryURL: URL? = LocalAttachmentStore.attachmentsDirectoryURL
    ) throws -> (state: CachedAppState, summary: PortableImportSummary) {
        guard let attachmentsDirectoryURL else {
            throw AttachmentError.attachmentsDirectoryUnavailable
        }
        let manifest = try readManifest(from: archiveURL)
        var state = try readState(from: archiveURL, manifest: manifest)
        try FileManager.default.createDirectory(at: attachmentsDirectoryURL, withIntermediateDirectories: true)

        var replacementByOriginalURL: [String: URL] = [:]
        var missingBundledAttachmentCount = 0
        var corruptBundledAttachmentCount = 0

        for attachment in manifest.attachments {
            guard replacementByOriginalURL[attachment.originalURL] == nil else { continue }
            guard let source = bundledAttachmentURL(for: attachment, in: archiveURL),
                  FileManager.default.fileExists(atPath: source.path),
                  FileManager.default.isReadableFile(atPath: source.path) else {
                missingBundledAttachmentCount += 1
                continue
            }
            guard bundledAttachmentPassesIntegrityCheck(source, manifestAttachment: attachment) else {
                corruptBundledAttachmentCount += 1
                continue
            }

            let destination = LocalAttachmentStore.uniqueURL(
                in: attachmentsDirectoryURL,
                preferredName: source.lastPathComponent
            )
            try FileManager.default.copyItem(at: source, to: destination)
            replacementByOriginalURL[attachment.originalURL] = destination
        }

        rewriteLocalPointers(in: &state, replacements: replacementByOriginalURL)

        return (
            state,
            PortableImportSummary(
                importedTaskCount: state.tasks.count,
                importedEventCount: state.events.count,
                importedAttachmentCount: replacementByOriginalURL.count,
                missingBundledAttachmentCount: missingBundledAttachmentCount,
                corruptBundledAttachmentCount: corruptBundledAttachmentCount,
                skippedPointerCount: manifest.skippedPointers.count,
                preImportBackupURL: nil
            )
        )
    }

    private static func collectAttachments(in state: CachedAppState) -> [LocalFileAttachment] {
        let taskAttachments = state.tasks.flatMap { LocalFileAttachment.parseAll(in: $0.notes) }
        let eventAttachments = state.events.flatMap { LocalFileAttachment.parseAll(in: $0.details) }
        let workspaceTaskAttachments = state.accountWorkspaces.flatMap { workspace in
            workspace.tasks.flatMap { LocalFileAttachment.parseAll(in: $0.notes) }
        }
        let workspaceEventAttachments = state.accountWorkspaces.flatMap { workspace in
            workspace.events.flatMap { LocalFileAttachment.parseAll(in: $0.details) }
        }
        return taskAttachments + eventAttachments + workspaceTaskAttachments + workspaceEventAttachments
    }

    private static func filteredState(_ state: CachedAppState, options: PortableExportOptions) -> CachedAppState {
        var next = state
        if let taskListIDs = options.taskListIDs {
            next.taskLists = state.taskLists.filter { taskListIDs.contains($0.id) }
            next.tasks = state.tasks.filter { taskListIDs.contains($0.taskListID) }
        }
        if let calendarIDs = options.calendarIDs {
            next.calendars = state.calendars.filter { calendarIDs.contains($0.id) }
            next.events = state.events.filter { calendarIDs.contains($0.calendarID) }
        }
        if let eventsStartingAtOrAfter = options.eventsStartingAtOrAfter {
            next.events = next.events.filter { $0.endDate >= eventsStartingAtOrAfter }
        }
        return next
    }

    private static func diff(from current: CachedAppState, to incoming: CachedAppState) -> PortableImportDiff {
        PortableImportDiff(
            tasks: resourceDiff(current: current.tasks, incoming: incoming.tasks, id: \.id, title: \.title),
            events: resourceDiff(current: current.events, incoming: incoming.events, id: \.id, title: \.summary),
            calendars: resourceDiff(current: current.calendars, incoming: incoming.calendars, id: \.id, title: \.summary),
            taskLists: resourceDiff(current: current.taskLists, incoming: incoming.taskLists, id: \.id, title: \.title),
            settingsWillChange: current.settings != incoming.settings,
            pendingMutationCount: pendingMutationCount(in: incoming)
        )
    }

    private static func pendingMutationCount(in state: CachedAppState) -> Int {
        state.accountWorkspaces.isEmpty
            ? state.pendingMutations.count
            : state.accountWorkspaces.reduce(0) { $0 + $1.pendingMutations.count }
    }

    private static func resourceDiff<Value: Equatable>(
        current: [Value],
        incoming: [Value],
        id: (Value) -> String,
        title: (Value) -> String
    ) -> PortableImportResourceDiff {
        let currentByID = Dictionary(current.map { (id($0), $0) }, uniquingKeysWith: { _, latest in latest })
        let incomingByID = Dictionary(incoming.map { (id($0), $0) }, uniquingKeysWith: { _, latest in latest })
        let currentIDs = Set(currentByID.keys)
        let incomingIDs = Set(incomingByID.keys)
        let sharedIDs = currentIDs.intersection(incomingIDs)
        return PortableImportResourceDiff(
            addedItems: incomingIDs.subtracting(currentIDs)
                .compactMap { itemID in
                    incomingByID[itemID].map { PortableImportDiffItem(id: itemID, currentTitle: nil, incomingTitle: title($0)) }
                }
                .sorted(by: diffItemSort),
            removedItems: currentIDs.subtracting(incomingIDs)
                .compactMap { itemID in
                    currentByID[itemID].map { PortableImportDiffItem(id: itemID, currentTitle: title($0), incomingTitle: nil) }
                }
                .sorted(by: diffItemSort),
            changedItems: sharedIDs
                .filter { currentByID[$0] != incomingByID[$0] }
                .compactMap { itemID in
                    guard let currentValue = currentByID[itemID],
                          let incomingValue = incomingByID[itemID] else {
                        return nil
                    }
                    return PortableImportDiffItem(
                        id: itemID,
                        currentTitle: title(currentValue),
                        incomingTitle: title(incomingValue)
                    )
                }
                .sorted(by: diffItemSort)
        )
    }

    private static func diffItemSort(_ lhs: PortableImportDiffItem, _ rhs: PortableImportDiffItem) -> Bool {
        let titleCompare = lhs.displayTitle.localizedCaseInsensitiveCompare(rhs.displayTitle)
        if titleCompare == .orderedSame {
            return lhs.id < rhs.id
        }
        return titleCompare == .orderedAscending
    }

    private static func rewriteLocalPointers(in state: inout CachedAppState, replacements: [String: URL]) {
        guard replacements.isEmpty == false else { return }
        state.tasks = state.tasks.map { task in
            var next = task
            next.notes = LocalFileAttachment.rewritePointers(in: task.notes, replacing: replacements)
            return next
        }
        state.events = state.events.map { event in
            var next = event
            next.details = LocalFileAttachment.rewritePointers(in: event.details, replacing: replacements)
            return next
        }
        state.accountWorkspaces = state.accountWorkspaces.map { workspace in
            var workspace = workspace
            workspace.tasks = workspace.tasks.map { task in
                var next = task
                next.notes = LocalFileAttachment.rewritePointers(in: task.notes, replacing: replacements)
                return next
            }
            workspace.events = workspace.events.map { event in
                var next = event
                next.details = LocalFileAttachment.rewritePointers(in: event.details, replacing: replacements)
                return next
            }
            return workspace
        }
    }

    private static func readManifest(from archiveURL: URL) throws -> PortableExportManifest {
        let manifestURL = archiveURL.appending(path: "manifest.json")
        guard FileManager.default.fileExists(atPath: manifestURL.path) else {
            throw PortableExportError.invalidArchive
        }
        let data = try Data(contentsOf: manifestURL)
        let manifest = try JSONDecoder.portableExport.decode(PortableExportManifest.self, from: data)
        guard manifest.formatVersion == PortableExportManifest.currentFormatVersion else {
            throw PortableExportError.unsupportedArchiveVersion(manifest.formatVersion)
        }
        return manifest
    }

    private static func readState(from archiveURL: URL, manifest: PortableExportManifest) throws -> CachedAppState {
        let stateURL = archiveURL.appending(path: manifest.stateFile)
        guard FileManager.default.fileExists(atPath: stateURL.path) else {
            throw PortableExportError.invalidArchive
        }
        let data = try Data(contentsOf: stateURL)
        return try JSONDecoder.portableExport.decode(CachedAppState.self, from: data)
    }

    private static func uniqueBundledAttachmentCount(in manifest: PortableExportManifest, archiveURL: URL) -> (total: Int, missing: Int, corrupt: Int) {
        var seen: Set<String> = []
        var missing = 0
        var corrupt = 0
        for attachment in manifest.attachments {
            guard seen.insert(attachment.originalURL).inserted else { continue }
            guard let source = bundledAttachmentURL(for: attachment, in: archiveURL),
                  FileManager.default.fileExists(atPath: source.path),
                  FileManager.default.isReadableFile(atPath: source.path) else {
                missing += 1
                continue
            }
            if bundledAttachmentPassesIntegrityCheck(source, manifestAttachment: attachment) == false {
                corrupt += 1
            }
        }
        return (seen.count, missing, corrupt)
    }

    private static func bundledAttachmentURL(for attachment: PortableExportManifest.Attachment, in archiveURL: URL) -> URL? {
        guard bundledRelativePathIsSafe(attachment.bundledRelativePath) else { return nil }
        return archiveURL.appending(path: attachment.bundledRelativePath)
    }

    private static func bundledRelativePathIsSafe(_ relativePath: String) -> Bool {
        guard relativePath.isEmpty == false,
              relativePath.hasPrefix("/") == false,
              relativePath.contains("../") == false,
              relativePath.contains("/..") == false else {
            return false
        }
        return true
    }

    private static func bundledAttachmentPassesIntegrityCheck(_ url: URL, manifestAttachment: PortableExportManifest.Attachment) -> Bool {
        guard manifestAttachment.sha256 != nil || manifestAttachment.byteCount != nil else { return true }
        guard let metadata = try? fileIntegrityMetadata(for: url) else { return false }
        if let byteCount = manifestAttachment.byteCount, metadata.byteCount != byteCount {
            return false
        }
        if let sha256 = manifestAttachment.sha256, metadata.sha256 != sha256 {
            return false
        }
        return true
    }

    private static func fileIntegrityMetadata(for url: URL) throws -> (sha256: String, byteCount: Int) {
        let data = try Data(contentsOf: url)
        let digest = SHA256.hash(data: data)
        let hex = digest.map { String(format: "%02x", $0) }.joined()
        return (hex, data.count)
    }
}

enum PortableExportError: LocalizedError, Equatable {
    case destinationAlreadyExists
    case invalidArchive
    case unsupportedArchiveVersion(Int)

    var errorDescription: String? {
        switch self {
        case .destinationAlreadyExists:
            return "Choose a new export name. A file or folder already exists at that location."
        case .invalidArchive:
            return "This does not look like a valid Hot Cross Buns portable archive."
        case .unsupportedArchiveVersion(let version):
            return "This portable archive uses format version \(version), which this build cannot import."
        }
    }
}

private extension JSONEncoder {
    static var portableExport: JSONEncoder {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        return encoder
    }
}

private extension JSONDecoder {
    static var portableExport: JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return decoder
    }
}
