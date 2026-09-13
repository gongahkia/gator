import XCTest
@testable import HotCrossBunsMac

final class ExporterTests: XCTestCase {
    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }()

    private func day(_ y: Int, _ m: Int, _ d: Int, hour: Int = 0, minute: Int = 0) -> Date {
        calendar.date(from: DateComponents(year: y, month: m, day: d, hour: hour, minute: minute))!
    }

    private func makeEvent(
        id: String = "evt-1",
        calendarID: String = "primary",
        summary: String = "Planning",
        details: String = "Sprint review",
        start: Date? = nil,
        end: Date? = nil,
        allDay: Bool = false,
        reminders: [Int] = []
    ) -> CalendarEventMirror {
        CalendarEventMirror(
            id: id,
            calendarID: calendarID,
            summary: summary,
            details: details,
            startDate: start ?? day(2026, 4, 18, hour: 14),
            endDate: end ?? day(2026, 4, 18, hour: 15),
            isAllDay: allDay,
            status: .confirmed,
            recurrence: [],
            etag: nil,
            updatedAt: nil,
            reminderMinutes: reminders
        )
    }

    private func makeTask(
        id: String = "t1",
        taskListID: String = "L1",
        title: String = "Pay rent",
        notes: String = "ACH",
        due: Date? = nil,
        completed: Bool = false
    ) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: taskListID,
            parentID: nil,
            title: title,
            notes: notes,
            status: completed ? .completed : .needsAction,
            dueDate: due,
            completedAt: nil,
            isDeleted: false,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: nil
        )
    }

    private func makePendingMutation(
        id: String,
        accountID: GoogleAccount.ID?,
        resourceID: String
    ) -> PendingMutation {
        PendingMutation(
            id: UUID(uuidString: id)!,
            accountID: accountID,
            createdAt: day(2026, 4, 18),
            resourceType: .task,
            resourceID: resourceID,
            action: .update,
            payload: Data()
        )
    }

    func testTaskMarkdownIncludesListDueNotes() {
        let md = TaskMarkdownExporter.markdown(for: makeTask(due: day(2026, 4, 20)), taskListTitle: "Personal")
        XCTAssertTrue(md.contains("- [ ] Pay rent"))
        XCTAssertTrue(md.contains("List: Personal"))
        XCTAssertTrue(md.contains("Due:"))
        XCTAssertTrue(md.contains("Notes: ACH"))
    }

    func testTaskMarkdownCompletedUsesCheckedBox() {
        let md = TaskMarkdownExporter.markdown(for: makeTask(completed: true))
        XCTAssertTrue(md.hasPrefix("- [x]"))
    }

    func testEventMarkdownAllDay() {
        let md = EventMarkdownExporter.markdown(for: makeEvent(allDay: true, reminders: []))
        XCTAssertTrue(md.contains("## Planning"))
        XCTAssertTrue(md.contains("(all day)"))
    }

    func testEventMarkdownTimed() {
        let md = EventMarkdownExporter.markdown(for: makeEvent())
        XCTAssertTrue(md.contains("–"))
    }

    func testICSContainsRequiredFields() {
        let ics = EventICSExporter.ics(for: makeEvent(reminders: [10]))
        XCTAssertTrue(ics.contains("BEGIN:VCALENDAR"))
        XCTAssertTrue(ics.contains("END:VCALENDAR"))
        XCTAssertTrue(ics.contains("BEGIN:VEVENT"))
        XCTAssertTrue(ics.contains("END:VEVENT"))
        XCTAssertTrue(ics.contains("SUMMARY:Planning"))
        XCTAssertTrue(ics.contains("UID:evt-1@hotcrossbuns"))
        XCTAssertTrue(ics.contains("TRIGGER:-PT10M"))
    }

    func testICSAllDayUsesValueDate() {
        let ics = EventICSExporter.ics(for: makeEvent(
            start: day(2026, 4, 18),
            end: day(2026, 4, 19),
            allDay: true
        ))
        XCTAssertTrue(ics.contains("DTSTART;VALUE=DATE:20260418"))
        XCTAssertTrue(ics.contains("DTEND;VALUE=DATE:20260419"))
    }

    func testICSEscapesSpecialCharacters() {
        let ics = EventICSExporter.ics(for: makeEvent(summary: "Design; review, part 1"))
        XCTAssertTrue(ics.contains("SUMMARY:Design\\; review\\, part 1"))
    }

    func testICSLinesUseCRLF() {
        let ics = EventICSExporter.ics(for: makeEvent())
        XCTAssertTrue(ics.contains("\r\n"))
    }

    func testPortableExportBundlesReachablePointersAndSkipsMissingOnes() throws {
        let root = FileManager.default.temporaryDirectory
            .appending(path: "hcb-portable-export-\(UUID().uuidString)", directoryHint: .isDirectory)
        let source = root.appending(path: "source.txt")
        let exportURL = root.appending(path: "archive.hcbexport", directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }
        try "hello".data(using: .utf8)?.write(to: source)

        let missing = root.appending(path: "missing.txt")
        let notes = [
            LocalFileAttachment.markdownPointer(for: source, kind: .file),
            LocalFileAttachment.markdownPointer(for: missing, kind: .file)
        ].joined(separator: "\n")
        let state = CachedAppState(
            account: nil,
            taskLists: [],
            tasks: [makeTask(notes: notes)],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )

        let summary = try PortableExportArchive.write(state: state, to: exportURL)

        XCTAssertEqual(summary.copiedAttachmentCount, 1)
        XCTAssertEqual(summary.skippedAttachmentCount, 1)
        XCTAssertTrue(FileManager.default.fileExists(atPath: exportURL.appending(path: "hot-cross-buns-state.json").path))
        XCTAssertTrue(FileManager.default.fileExists(atPath: exportURL.appending(path: "Attachments/source.txt").path))

        let manifestData = try Data(contentsOf: exportURL.appending(path: "manifest.json"))
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        let manifest = try decoder.decode(PortableExportManifest.self, from: manifestData)
        XCTAssertEqual(manifest.attachments.count, 1)
        XCTAssertEqual(manifest.attachments.first?.byteCount, 5)
        XCTAssertNotNil(manifest.attachments.first?.sha256)
        XCTAssertEqual(manifest.skippedPointers, [missing.absoluteString])
    }

    func testPortableImportCopiesBundledAttachmentsAndRewritesPointers() throws {
        let root = FileManager.default.temporaryDirectory
            .appending(path: "hcb-portable-import-\(UUID().uuidString)", directoryHint: .isDirectory)
        let source = root.appending(path: "source.txt")
        let exportURL = root.appending(path: "archive.hcbexport", directoryHint: .isDirectory)
        let importedAttachmentsURL = root.appending(path: "ImportedAttachments", directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }
        try "hello".data(using: .utf8)?.write(to: source)

        let pointer = LocalFileAttachment.markdownPointer(displayName: "Original display", url: source, kind: .file)
        let state = CachedAppState(
            account: nil,
            taskLists: [],
            tasks: [makeTask(notes: pointer)],
            calendars: [],
            events: [makeEvent(details: pointer)],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        _ = try PortableExportArchive.write(state: state, to: exportURL)

        let preview = try PortableExportArchive.previewImport(from: exportURL)
        XCTAssertEqual(preview.bundledAttachmentCount, 1)
        XCTAssertEqual(preview.missingBundledAttachmentCount, 0)
        XCTAssertEqual(preview.corruptBundledAttachmentCount, 0)

        let imported = try PortableExportArchive.importState(
            from: exportURL,
            attachmentsDirectoryURL: importedAttachmentsURL
        )

        XCTAssertEqual(imported.summary.importedAttachmentCount, 1)
        XCTAssertEqual(imported.summary.missingBundledAttachmentCount, 0)
        XCTAssertEqual(imported.summary.corruptBundledAttachmentCount, 0)
        let taskAttachment = try XCTUnwrap(LocalFileAttachment.parseAll(in: imported.state.tasks[0].notes).first)
        let eventAttachment = try XCTUnwrap(LocalFileAttachment.parseAll(in: imported.state.events[0].details).first)
        XCTAssertEqual(taskAttachment.displayName, "Original display")
        XCTAssertEqual(taskAttachment.url.deletingLastPathComponent(), importedAttachmentsURL)
        XCTAssertEqual(eventAttachment.url, taskAttachment.url)
        XCTAssertTrue(FileManager.default.fileExists(atPath: taskAttachment.url.path))
    }

    func testPortableImportSkipsBundledAttachmentWithChecksumMismatch() throws {
        let root = FileManager.default.temporaryDirectory
            .appending(path: "hcb-portable-corrupt-import-\(UUID().uuidString)", directoryHint: .isDirectory)
        let source = root.appending(path: "source.txt")
        let exportURL = root.appending(path: "archive.hcbexport", directoryHint: .isDirectory)
        let importedAttachmentsURL = root.appending(path: "ImportedAttachments", directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }
        try "hello".data(using: .utf8)?.write(to: source)

        let pointer = LocalFileAttachment.markdownPointer(for: source, kind: .file)
        let state = CachedAppState(
            account: nil,
            taskLists: [],
            tasks: [makeTask(notes: pointer)],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        _ = try PortableExportArchive.write(state: state, to: exportURL)
        try "tampered".data(using: .utf8)?.write(to: exportURL.appending(path: "Attachments/source.txt"), options: [.atomic])

        let preview = try PortableExportArchive.previewImport(from: exportURL)
        XCTAssertEqual(preview.corruptBundledAttachmentCount, 1)

        let imported = try PortableExportArchive.importState(
            from: exportURL,
            attachmentsDirectoryURL: importedAttachmentsURL
        )

        XCTAssertEqual(imported.summary.importedAttachmentCount, 0)
        XCTAssertEqual(imported.summary.corruptBundledAttachmentCount, 1)
        let importedAttachment = try XCTUnwrap(LocalFileAttachment.parseAll(in: imported.state.tasks[0].notes).first)
        XCTAssertEqual(importedAttachment.url, source)
    }

    func testPortableImportPreviewReportsDryRunDiffAgainstCurrentState() throws {
        let root = FileManager.default.temporaryDirectory
            .appending(path: "hcb-portable-diff-\(UUID().uuidString)", directoryHint: .isDirectory)
        let exportURL = root.appending(path: "archive.hcbexport", directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }

        let currentState = CachedAppState(
            account: nil,
            taskLists: [],
            tasks: [
                makeTask(id: "changed", title: "Old title"),
                makeTask(id: "removed", title: "Removed")
            ],
            calendars: [],
            events: [makeEvent(id: "removed-event")],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let incomingState = CachedAppState(
            account: nil,
            taskLists: [],
            tasks: [
                makeTask(id: "changed", title: "New title"),
                makeTask(id: "added", title: "Added")
            ],
            calendars: [],
            events: [makeEvent(id: "added-event")],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        _ = try PortableExportArchive.write(state: incomingState, to: exportURL)

        let preview = try PortableExportArchive.previewImport(from: exportURL, comparingTo: currentState)
        let diff = try XCTUnwrap(preview.diff)

        XCTAssertEqual(diff.tasks.added, 1)
        XCTAssertEqual(diff.tasks.removed, 1)
        XCTAssertEqual(diff.tasks.changed, 1)
        XCTAssertEqual(diff.tasks.addedItems.map(\.incomingTitle), ["Added"])
        XCTAssertEqual(diff.tasks.removedItems.map(\.currentTitle), ["Removed"])
        XCTAssertEqual(diff.tasks.changedItems.first?.currentTitle, "Old title")
        XCTAssertEqual(diff.tasks.changedItems.first?.incomingTitle, "New title")
        XCTAssertEqual(diff.events.added, 1)
        XCTAssertEqual(diff.events.removed, 1)
        XCTAssertEqual(diff.events.changed, 0)
        XCTAssertEqual(diff.events.addedItems.map(\.incomingTitle), ["Planning"])
        XCTAssertEqual(diff.events.removedItems.map(\.currentTitle), ["Planning"])
        XCTAssertFalse(diff.settingsWillChange)
    }

    func testPortableExportImportPreservesAccountWorkspacesAndCountsPendingMutations() throws {
        let root = FileManager.default.temporaryDirectory
            .appending(path: "hcb-portable-multi-account-\(UUID().uuidString)", directoryHint: .isDirectory)
        let exportURL = root.appending(path: "archive.hcbexport", directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }

        let personal = GoogleAccount.preview
        let work = GoogleAccount(
            id: "work-account",
            email: "work@example.com",
            displayName: "Work",
            grantedScopes: [GoogleScope.tasks],
            authProvider: .customDesktopOAuth
        )
        let workWorkspace = AccountWorkspaceSnapshot(
            accountID: work.id,
            taskLists: [TaskListMirror(id: "work-list", title: "Work", updatedAt: nil, etag: nil)],
            tasks: [makeTask(id: "work-task", taskListID: "work-list", title: "Work task")],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: [
                makePendingMutation(
                    id: "00000000-0000-0000-0000-000000000202",
                    accountID: work.id,
                    resourceID: "work-task"
                )
            ]
        )
        let state = CachedAppState(
            account: personal,
            accounts: [personal, work],
            activeAccountID: personal.id,
            accountWorkspaces: [workWorkspace],
            taskLists: [TaskListMirror(id: "personal-list", title: "Personal", updatedAt: nil, etag: nil)],
            tasks: [makeTask(id: "personal-task", taskListID: "personal-list", title: "Personal task")],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: [
                makePendingMutation(
                    id: "00000000-0000-0000-0000-000000000101",
                    accountID: personal.id,
                    resourceID: "personal-task"
                )
            ]
        )
        _ = try PortableExportArchive.write(state: state, to: exportURL)

        let preview = try PortableExportArchive.previewImport(from: exportURL, comparingTo: .empty)
        let diff = try XCTUnwrap(preview.diff)
        let imported = try PortableExportArchive.importState(
            from: exportURL,
            attachmentsDirectoryURL: root.appending(path: "ImportedAttachments", directoryHint: .isDirectory)
        ).state

        XCTAssertEqual(diff.pendingMutationCount, 2)
        XCTAssertEqual(imported.accounts.map(\.id), [personal.id, work.id])
        XCTAssertEqual(imported.activeAccountID, personal.id)
        XCTAssertEqual(imported.tasks.map(\.id), ["personal-task"])
        XCTAssertEqual(imported.accountWorkspaces.first { $0.accountID == work.id }?.tasks.map(\.id), ["work-task"])
        XCTAssertEqual(Set(imported.accountWorkspaces.flatMap { $0.pendingMutations.compactMap(\.accountID) }), [personal.id, work.id])
    }

    func testPortableImportRewritesWorkspaceAttachmentPointers() throws {
        let root = FileManager.default.temporaryDirectory
            .appending(path: "hcb-portable-workspace-attachments-\(UUID().uuidString)", directoryHint: .isDirectory)
        let source = root.appending(path: "workspace-source.txt")
        let exportURL = root.appending(path: "archive.hcbexport", directoryHint: .isDirectory)
        let importedAttachmentsURL = root.appending(path: "ImportedAttachments", directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }
        try "workspace".data(using: .utf8)?.write(to: source)

        let personal = GoogleAccount.preview
        let work = GoogleAccount(
            id: "work-account",
            email: "work@example.com",
            displayName: "Work",
            grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
            authProvider: .customDesktopOAuth
        )
        let pointer = LocalFileAttachment.markdownPointer(displayName: "Workspace file", url: source, kind: .file)
        let workWorkspace = AccountWorkspaceSnapshot(
            accountID: work.id,
            taskLists: [TaskListMirror(id: "work-list", title: "Work", updatedAt: nil, etag: nil)],
            tasks: [makeTask(id: "work-task", taskListID: "work-list", notes: pointer)],
            calendars: [
                CalendarListMirror(id: "work-cal", summary: "Work", colorHex: "#000000", isSelected: true, accessRole: "owner")
            ],
            events: [makeEvent(id: "work-event", calendarID: "work-cal", details: pointer)],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let state = CachedAppState(
            account: personal,
            accounts: [personal, work],
            activeAccountID: personal.id,
            accountWorkspaces: [workWorkspace],
            taskLists: [],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        _ = try PortableExportArchive.write(state: state, to: exportURL)

        let preview = try PortableExportArchive.previewImport(from: exportURL)
        let imported = try PortableExportArchive.importState(
            from: exportURL,
            attachmentsDirectoryURL: importedAttachmentsURL
        ).state
        let importedWorkspace = try XCTUnwrap(imported.accountWorkspaces.first { $0.accountID == work.id })
        let taskAttachment = try XCTUnwrap(LocalFileAttachment.parseAll(in: importedWorkspace.tasks[0].notes).first)
        let eventAttachment = try XCTUnwrap(LocalFileAttachment.parseAll(in: importedWorkspace.events[0].details).first)

        XCTAssertEqual(preview.bundledAttachmentCount, 1)
        XCTAssertEqual(taskAttachment.displayName, "Workspace file")
        XCTAssertEqual(taskAttachment.url.deletingLastPathComponent(), importedAttachmentsURL)
        XCTAssertEqual(eventAttachment.url, taskAttachment.url)
        XCTAssertTrue(FileManager.default.fileExists(atPath: taskAttachment.url.path))
    }

    func testPortableExportCanFilterTaskListsCalendarsAndFutureEvents() throws {
        let root = FileManager.default.temporaryDirectory
            .appending(path: "hcb-portable-filter-\(UUID().uuidString)", directoryHint: .isDirectory)
        let exportURL = root.appending(path: "archive.hcbexport", directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }

        let cutoff = day(2026, 4, 18)
        let state = CachedAppState(
            account: nil,
            taskLists: [
                TaskListMirror(id: "keep-list", title: "Keep", updatedAt: nil, etag: nil),
                TaskListMirror(id: "drop-list", title: "Drop", updatedAt: nil, etag: nil)
            ],
            tasks: [
                makeTask(id: "keep-task", taskListID: "keep-list"),
                makeTask(id: "drop-task", taskListID: "drop-list")
            ],
            calendars: [
                CalendarListMirror(id: "keep-cal", summary: "Keep", colorHex: "#fff", isSelected: true, accessRole: "owner"),
                CalendarListMirror(id: "drop-cal", summary: "Drop", colorHex: "#000", isSelected: true, accessRole: "owner")
            ],
            events: [
                makeEvent(id: "keep-event", calendarID: "keep-cal", start: day(2026, 4, 20), end: day(2026, 4, 20, hour: 1)),
                makeEvent(id: "past-event", calendarID: "keep-cal", start: day(2026, 4, 1), end: day(2026, 4, 1, hour: 1)),
                makeEvent(id: "drop-event", calendarID: "drop-cal", start: day(2026, 4, 20), end: day(2026, 4, 20, hour: 1))
            ],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )

        _ = try PortableExportArchive.write(
            state: state,
            to: exportURL,
            options: PortableExportOptions(
                taskListIDs: ["keep-list"],
                calendarIDs: ["keep-cal"],
                eventsStartingAtOrAfter: cutoff
            )
        )

        let preview = try PortableExportArchive.previewImport(from: exportURL)

        XCTAssertEqual(preview.taskListCount, 1)
        XCTAssertEqual(preview.taskCount, 1)
        XCTAssertEqual(preview.calendarCount, 1)
        XCTAssertEqual(preview.eventCount, 1)
    }

    @MainActor
    func testAppModelPortableImportWritesPreImportBackupBeforeReplacingState() async throws {
        let root = FileManager.default.temporaryDirectory
            .appending(path: "hcb-portable-backup-\(UUID().uuidString)", directoryHint: .isDirectory)
        let exportURL = root.appending(path: "archive.hcbexport", directoryHint: .isDirectory)
        let backupURL = root.appending(path: "Backups", directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }

        let incomingState = CachedAppState(
            account: nil,
            taskLists: [],
            tasks: [makeTask(id: "imported", title: "Imported")],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        _ = try PortableExportArchive.write(state: incomingState, to: exportURL)
        let model = makeModel(localBackupService: LocalBackupService(directoryURL: backupURL))

        let summary = try await model.importPortableArchive(from: exportURL)

        let preImportBackupURL = try XCTUnwrap(summary.preImportBackupURL)
        XCTAssertTrue(FileManager.default.fileExists(atPath: preImportBackupURL.path))
        XCTAssertEqual(model.tasks.map(\.id), ["imported"])
        XCTAssertEqual(model.localBackupSummary?.backupCount, 1)
    }

    @MainActor
    func testAppModelPortableImportCanPreserveUnselectedSections() async throws {
        let root = FileManager.default.temporaryDirectory
            .appending(path: "hcb-portable-partial-import-\(UUID().uuidString)", directoryHint: .isDirectory)
        let firstExportURL = root.appending(path: "first.hcbexport", directoryHint: .isDirectory)
        let secondExportURL = root.appending(path: "second.hcbexport", directoryHint: .isDirectory)
        let backupURL = root.appending(path: "Backups", directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }

        let firstState = CachedAppState(
            account: nil,
            taskLists: [],
            tasks: [makeTask(id: "old-task", title: "Old")],
            calendars: [],
            events: [makeEvent(id: "kept-event", summary: "Keep")],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let secondState = CachedAppState(
            account: nil,
            taskLists: [],
            tasks: [makeTask(id: "new-task", title: "New")],
            calendars: [],
            events: [makeEvent(id: "new-event", summary: "New")],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        _ = try PortableExportArchive.write(state: firstState, to: firstExportURL)
        _ = try PortableExportArchive.write(state: secondState, to: secondExportURL)
        let model = makeModel(localBackupService: LocalBackupService(directoryURL: backupURL))

        _ = try await model.importPortableArchive(from: firstExportURL)
        _ = try await model.importPortableArchive(
            from: secondExportURL,
            options: PortableImportOptions(
                includeTasks: true,
                includeEvents: false,
                includeSettings: false,
                includeSyncMetadata: false
            )
        )

        XCTAssertEqual(model.tasks.map(\.id), ["new-task"])
        XCTAssertEqual(model.events.map(\.id), ["kept-event"])
        XCTAssertEqual(model.localBackupSummary?.backupCount, 2)
    }

    @MainActor
    private func makeModel(localBackupService: LocalBackupService) -> AppModel {
        let transport = GoogleAPITransport(
            baseURL: URL(string: "https://www.googleapis.com")!,
            tokenProvider: StaticAccessTokenProvider(token: "test-token")
        )
        let tasksClient = GoogleTasksClient(transport: transport)
        let calendarClient = GoogleCalendarClient(transport: transport)
        return AppModel(
            authService: GoogleAuthService(),
            tasksClient: tasksClient,
            calendarClient: calendarClient,
            syncScheduler: SyncScheduler(tasksClient: tasksClient, calendarClient: calendarClient),
            cacheStore: LocalCacheStore(fileURL: nil),
            localBackupService: localBackupService
        )
    }
}
