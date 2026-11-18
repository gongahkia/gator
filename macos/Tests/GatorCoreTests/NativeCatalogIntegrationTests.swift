import Foundation
import Testing
@testable import GatorCore

struct CatalogIntegrationFixture {
    let root: URL
    let helper: URL
    let ids: [String]
    let configuration: BridgeLaunchConfiguration
    static func prepare(count: Int) throws -> Self {
        let root = try temporaryDirectory()
        let repo = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let helper = root.appendingPathComponent("catalog-fixture")
        _ = try run("/usr/bin/env", ["go", "build", "-o", helper.path, repo.appendingPathComponent("macos/Tests/GatorCoreTests/Fixtures/native-catalog-seed.go").path], directory: repo)
        let ids = try JSONDecoder().decode([String].self, from: run(helper.path, [root.path, String(count)]))
        let directory = root.appendingPathComponent("config/gator"); try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let data = Data(#"{"version":4,"defaults":{"provider":"fixture","model":"fixture"},"custom_providers":[{"id":"fixture","base_url":"http://127.0.0.1:9/chat","models":["fixture"],"default_model":"fixture"}]}"#.utf8)
        try data.write(to: directory.appendingPathComponent("config.json"))
        let environment = ["PATH": ProcessInfo.processInfo.environment["PATH"] ?? "/usr/bin:/bin", "HOME": root.path, "GATOR_CONFIG_DIR": root.appendingPathComponent("config").path, "GATOR_STATE_DIR": root.appendingPathComponent("state").path, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null"]
        let runtime = ProcessInfo.processInfo.environment["GATOR_TEST_BINARY"] ?? "/tmp/gator-p02-runtime"
        return Self(root: root, helper: helper, ids: ids, configuration: BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: runtime), workspace: root.appendingPathComponent("workspace"), environment: environment, conversationID: try #require(ids.last)))
    }
    static func run(_ executable: String, _ args: [String], directory: URL? = nil) throws -> Data {
        let process = Process(), output = Pipe()
        process.executableURL = URL(fileURLWithPath: executable); process.arguments = args; process.currentDirectoryURL = directory
        process.standardOutput = output; process.standardError = FileHandle.standardError
        try process.run(); let data = output.fileHandleForReading.readDataToEndOfFile(); process.waitUntilExit()
        guard process.terminationStatus == 0 else { throw BridgeFailure("Catalog fixture failed: \(args.first ?? executable)") }
        return data
    }
    func change(_ id: String, title: String) throws { _ = try Self.run(helper.path, [root.path, "rename", id, title]) }
    func read(_ id: String) throws -> JSONValue { try JSONDecoder().decode(JSONValue.self, from: Self.run(helper.path, [root.path, "read", id])) }
    func fallback() throws { _ = try Self.run(helper.path, [root.path, "fallback"]) }
    @MainActor func application() -> ManagedSessionCoordinator { ManagedSessionCoordinator(configuration: configuration, nativeDraftDirectory: root.appendingPathComponent("native"), profile: root.path) }
}

@Test @MainActor func catalogActualGoPagesScopesOlderSelectionRenameConflictAndOwnerDrafts() async throws {
    let fixture = try CatalogIntegrationFixture.prepare(count: 310)
    defer { try? FileManager.default.removeItem(at: fixture.root) }
    let app = fixture.application()
    do {
        await app.start(); try await managedEventually("catalog initial owner") { app.current.isConnected }
        let original = app.current, originalID = original.conversationID
        original.editDraft("Unsent foreground draft survives the catalog")
        app.showCatalog(); try await managedEventually("catalog first page") { !app.catalog.loading && app.catalog.page != nil }
        #expect(app.catalog.issue.isEmpty); #expect(app.catalog.items.count == 50); #expect(app.catalog.hasNext)
        var seen = Set(app.catalog.items.map(\.id)), pages = 1
        while app.catalog.hasNext {
            app.catalog.next(); try await managedEventually("next catalog page") { !app.catalog.loading }
            #expect(app.catalog.issue.isEmpty)
            let ids = app.catalog.items.map(\.id); #expect(ids.allSatisfy { !seen.contains($0) }); seen.formUnion(ids); pages += 1
        }
        #expect(seen.count == (0..<310).filter { !$0.isMultiple(of: 7) }.count); #expect(pages == 6)
        app.catalog.previous(); try await managedEventually("previous catalog page") { !app.catalog.loading }; #expect(app.catalog.pageNumber == 5)
        let olderID = fixture.ids[5]
        #expect(original.snapshot?.value["recent_conversations"].array.contains { $0["id"].string == olderID } != true)
        app.catalog.query = "Retained topic 0005"; app.catalog.search()
        try await managedEventually("older global result") { !app.catalog.loading && app.catalog.items.count == 1 }
        #expect(app.catalog.pageNumber == 1); #expect(app.catalog.items.first?.id == olderID)
        app.catalog.selection = olderID; app.catalog.openSelected()
        try await managedEventually("validated older owner") { !app.catalog.acting && app.current.conversationID == olderID }
        #expect(app.ownerCount == 2); #expect(original.draftText == "Unsent foreground draft survives the catalog")
        app.current.editDraft("Independent older draft")
        let messageIDs = app.current.transcript.map(\.id)
        let renameStart = ContinuousClock.now
        app.catalog.beginRename(); app.catalog.renameText = "Native renamed older topic"; app.catalog.rename()
        try await managedEventually("rename confirmation") { !app.catalog.acting }
        #expect(app.catalog.issue.isEmpty); #expect(app.catalog.editor == nil)
        #expect(app.current.title == "Native renamed older topic"); #expect(try fixture.read(olderID)["title"].string == "Native renamed older topic")
        #expect(app.current.draftText == "Independent older draft"); #expect(app.current.transcript.map(\.id) == messageIDs)
        print("GATOR_P02_RENAME requestToConfirmedCachedOwnerTitle=\(renameStart.duration(to: .now))")
        #expect(app.catalog.query == "Retained topic 0005")

        app.catalog.beginRename(); app.catalog.renameText = "Native attempted after conflict"
        try fixture.change(olderID, title: "Concurrent terminal title")
        app.catalog.rename(); try await managedEventually("rename conflict") { !app.catalog.acting }
        #expect(app.catalog.needsReview); #expect(!app.catalog.canSave); #expect(app.catalog.renameText == "Native attempted after conflict")
        #expect(try fixture.read(olderID)["title"].string == "Concurrent terminal title")
        app.catalog.reviewRename(); try await managedEventually("fresh version inspection") { !app.catalog.acting }
        #expect(app.catalog.canSave); #expect(app.catalog.editor?.title == "Concurrent terminal title")
        #expect(try fixture.read(olderID)["title"].string == "Concurrent terminal title")
        app.catalog.rename(); try await managedEventually("explicit fresh save") { !app.catalog.acting }
        #expect(app.current.title == "Native attempted after conflict"); #expect(app.catalog.issue.isEmpty)

        try fixture.change(olderID, title: "Externally renamed cached owner")
        app.catalog.query = "Externally renamed cached owner"; app.catalog.search()
        try await managedEventually("fresh cached metadata") { !app.catalog.loading && app.catalog.items.count == 1 }
        app.catalog.selection = olderID; app.catalog.openSelected()
        try await managedEventually("cached title on exact open") { !app.catalog.acting }
        #expect(app.current.title == "Externally renamed cached owner"); #expect(app.ownerCount == 2)

        app.catalog.scope = .archived; app.catalog.query = "Retained topic 0007"; app.catalog.search()
        try await managedEventually("archived result") { !app.catalog.loading && app.catalog.items.count == 1 }
        #expect(app.catalog.items[0].archived)
        app.catalog.selection = fixture.ids[7]; app.catalog.openSelected()
        try await managedEventually("archived opening") { !app.catalog.acting && app.current.conversationID == fixture.ids[7] }
        #expect(try fixture.read(fixture.ids[7])["archived"].bool)
        #expect(app.ownerCount == 3)
        app.catalog.openSelected(); try await managedEventually("cached selection") { !app.catalog.acting }; #expect(app.ownerCount == 3)
        app.catalog.dismiss(); await app.openConversation(originalID)
        #expect(app.current === original); #expect(app.current.draftText == "Unsent foreground draft survives the catalog")
        #expect(app.owners[olderID]?.draftText == "Independent older draft")
        app.showCatalog(); app.catalog.scope = .all; app.catalog.query = "jade decision"; app.catalog.search()
        try await managedEventually("unavailable history selection") { !app.catalog.loading && app.catalog.items.count == 1 }
        let beforeUnavailable = app.ownerCount
        let revision = fixture.root.appendingPathComponent("state/gator/conversations/\(fixture.ids[0])/revisions/rev-catalog-fixture.json")
        try Data("{".utf8).write(to: revision)
        app.catalog.selection = fixture.ids[0]; app.catalog.openSelected()
        try await managedEventually("unavailable target rejected") { !app.catalog.acting }
        #expect(app.current === original); #expect(app.ownerCount == beforeUnavailable)
        #expect(app.catalog.issue.contains("could not be loaded")); app.catalog.dismiss()
        #expect(await app.shutdown(confirmed: true))
    } catch { _ = await app.shutdown(confirmed: true); throw error }
}

@Test @MainActor func catalogActualGoSmallHundredsAndLargeBoundedMeasurementsAndFallback() async throws {
    for count in [12, 310, 1_500] {
        let fixture = try CatalogIntegrationFixture.prepare(count: count)
        let app = fixture.application()
        do {
            await app.start(); try await managedEventually("measured owner ready") { app.current.isConnected }
            var request = NativeCatalogRequest(.query); request.scope = .all; request.text = "Retained topic"
            let start = ContinuousClock.now, result = try await app.current.nativeCatalog(request), queryTime = start.duration(to: .now)
            let page = try #require(result.page); #expect(page.items.count == min(50, count)); #expect(result.error == nil)
            var body = NativeCatalogRequest(.query); body.scope = .all; body.text = "jade decision"
            let bodyStart = ContinuousClock.now, bodyResult = try await app.current.nativeCatalog(body), bodyTime = bodyStart.duration(to: .now)
            #expect(bodyResult.page?.items.map(\.id) == [fixture.ids[0]])
            #expect(bodyResult.page?.items.first?.snippet?.contains("jade decision") == true)
            // Go currently excludes conversation-only dialogue from its index.
            var dialogueQuery = NativeCatalogRequest(.query); dialogueQuery.scope = .all; dialogueQuery.text = "Public café response"
            let dialogue = try await app.current.nativeCatalog(dialogueQuery)
            #expect(dialogue.page?.items.isEmpty == true)
            print("GATOR_P02_GO_BENCH records=\(count) titleRequestToResult=\(queryTime) workContentRequestToResult=\(bodyTime) pageItems=\(page.items.count) titleFallback=\(page.indexFallback) contentFallback=\(bodyResult.page?.indexFallback ?? false) incomplete=\(bodyResult.page?.incomplete ?? false)")
            if count == 1_500 {
                try fixture.fallback()
                var fallback = NativeCatalogRequest(.query); fallback.scope = .all; fallback.text = "jade decision"
                let value = try await app.current.nativeCatalog(fallback)
                #expect(value.page?.indexFallback == true); #expect(value.page?.items.map(\.id) == [fixture.ids[0]])
                #expect(value.page?.coverageNotice.contains("index is unavailable") == true)
            }
            #expect(await app.shutdown(confirmed: true))
        } catch { _ = await app.shutdown(confirmed: true); throw error }
        try FileManager.default.removeItem(at: fixture.root)
    }
}
