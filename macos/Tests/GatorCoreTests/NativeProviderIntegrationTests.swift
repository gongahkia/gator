import Foundation
import Testing
@testable import GatorCore

private let providerInertSecret = "P04_INTEGRATION_INERT_NOT_REAL_e92b8"
private func readProviderAuth(_ root: URL) throws -> JSONValue {
    let path = root.appendingPathComponent("state/gator/auth.json")
    return try JSONDecoder().decode(JSONValue.self, from: Data(contentsOf: path))
}
@MainActor private func refreshProviders(_ app: ManagedSessionCoordinator) async throws {
    app.showProviders()
    try await managedEventually("provider status") { !app.providers.busy && !app.providers.rows.isEmpty }
    #expect(app.providers.issue.isEmpty)
}
@MainActor private func prepareProvider(_ owner: GatorApplicationState, replacing: Bool) async throws -> NativeProviderReview {
    var request = NativeProviderRequest(.review); request.provider = "openai"; request.replaceKey = replacing
    var form = NativeProviderForm(); form.model = "p04-fixture-model"; form.endpointMode = "override"; form.endpoint = "http://127.0.0.1:9/v1"
    request.form = form
    let result = try await owner.nativeProvider(request)
    #expect(result.error == nil)
    return try #require(result.review)
}

@Test @MainActor func providerActualGoSaveReplaceRemoveRelaunchAndSecretArtifactBoundary() async throws {
    let fixture = try CatalogIntegrationFixture.prepare(count: 2)
    defer { try? FileManager.default.removeItem(at: fixture.root) }
    let connection = GoBridgeConnection()
    let app = ManagedSessionCoordinator(configuration: fixture.configuration, store: NativeDraftStore(directory: fixture.root.appendingPathComponent("native"), profile: fixture.root.path), factory: { connection })
    await app.start()
    let original = app.current, originalID = original.conversationID, originalTranscript = original.transcript
    original.editDraft("Unsent chat survives provider setup")
    #expect(await original.flushDrafts())
    try await refreshProviders(app)
    #expect(app.providers.rows.count == 38)
    #expect(!app.providers.rows.contains { ["pi", "ollama", "fixture"].contains($0.id) })
    app.providers.selection = "openai"
    var form = try #require(app.providers.forms["openai"])
    form.model = "p04-fixture-model"; form.endpointMode = "override"; form.endpoint = "http://127.0.0.1:9/v1"
    app.providers.forms["openai"] = form
    #expect(await app.providers.prepare(replacing: true))
    #expect(app.providers.review?.setsDefaults == true)
    #expect(!FileManager.default.fileExists(atPath: fixture.root.appendingPathComponent("state/gator/auth.json").path))
    let reviewData = try JSONEncoder().encode(original.snapshot?.value)
    #expect(!String(decoding: reviewData, as: UTF8.self).contains(providerInertSecret))
    app.providers.cancelReview()
    #expect(!FileManager.default.fileExists(atPath: fixture.root.appendingPathComponent("state/gator/auth.json").path))
    #expect(await app.providers.prepare(replacing: true))
    #expect(await app.providers.save(apiKey: providerInertSecret))
    #expect(app.providers.issue.isEmpty); #expect(!app.providers.notice.isEmpty)
    #expect(try readProviderAuth(fixture.root)["openai"]["key"].string == providerInertSecret)
    #expect(app.providers.selected?.stored == true); #expect(app.providers.selected?.ready == true)
    #expect(app.ownerCount == 1); #expect(app.current === original); #expect(original.conversationID == originalID)
    #expect(original.draftText == "Unsent chat survives provider setup"); #expect(original.transcript == originalTranscript)
    #expect(original.snapshot?.running == false); #expect(original.snapshot?.hasApproval == false)
    #expect(original.snapshot?.value["queued"].int == 0)
    let pid = try #require(await connection.processIdentifier())
    let processInput = try CatalogIntegrationFixture.run("/bin/ps", ["eww", "-p", String(pid), "-o", "command="])
    #expect(!String(decoding: processInput, as: UTF8.self).contains(providerInertSecret))
    #expect(try readProviderAuth(fixture.root)["openai"]["type"].string == "api_key")
    let authPath = fixture.root.appendingPathComponent("state/gator/auth.json")
    let permissions = try FileManager.default.attributesOfItem(atPath: authPath.path)[.posixPermissions] as? Int
    #expect(permissions == 0o600)
    #expect(await app.providers.prepare(replacing: false)); #expect(await app.providers.save(apiKey: ""))
    #expect(try readProviderAuth(fixture.root)["openai"]["key"].string == providerInertSecret)
    #expect(await app.providers.prepare(replacing: false)); #expect(await app.providers.save(apiKey: "   "))
    #expect(try readProviderAuth(fixture.root)["openai"]["key"].string == providerInertSecret)
    #expect(await app.providers.prepare(replacing: true)); #expect(await app.providers.save(apiKey: providerInertSecret + "_REPLACED"))
    #expect(try readProviderAuth(fixture.root)["openai"]["key"].string == providerInertSecret + "_REPLACED")
    // Only the canonical credential file may contain the distinctive fixture
    // key. Check every other controlled text artifact, including drafts/history.
    for file in try providerControlledFiles(fixture.root) {
        guard try file.resourceValues(forKeys: [.isRegularFileKey]).isRegularFile == true, file.resolvingSymlinksInPath().path != authPath.resolvingSymlinksInPath().path, file.resolvingSymlinksInPath().path != fixture.helper.resolvingSymlinksInPath().path else { continue }
        let data = try Data(contentsOf: file)
        #expect(data.range(of: Data(providerInertSecret.utf8)) == nil, "Secret escaped the canonical credential file: \(file.path.replacingOccurrences(of: fixture.root.path, with: ""))")
    }
    #expect(!String(decoding: try JSONEncoder().encode(original.snapshot?.value), as: UTF8.self).contains(providerInertSecret))
    app.providers.dismiss(); #expect(await app.shutdown(confirmed: true))
    let reopened = fixture.application(); await reopened.start(); try await refreshProviders(reopened)
    reopened.providers.selection = "openai"
    #expect(reopened.providers.selected?.stored == true); #expect(reopened.providers.selected?.model == "p04-fixture-model")
    #expect(reopened.providers.selected?.destination == "127.0.0.1:9")
    #expect(reopened.current.draftText == "Unsent chat survives provider setup")
    #expect(await reopened.providers.prepare(replacing: false, removal: true))
    #expect(reopened.providers.review?.setsDefaults == false)
    #expect(await reopened.providers.save(apiKey: ""))
    #expect(try readProviderAuth(fixture.root)["openai"] == .null)
    let config = try JSONDecoder().decode(JSONValue.self, from: Data(contentsOf: fixture.root.appendingPathComponent("config/gator/config.json")))
    #expect(config["defaults"]["provider"].string == "openai"); #expect(config["defaults"]["model"].string == "p04-fixture-model")
    reopened.providers.dismiss(); #expect(await reopened.shutdown(confirmed: true))
}

@Test @MainActor func providerActualGoIndependentOwnersRejectCompetingReviewedWritesAndDoNotStealChat() async throws {
    let fixture = try CatalogIntegrationFixture.prepare(count: 2)
    defer { try? FileManager.default.removeItem(at: fixture.root) }
    let first = fixture.application()
    var config = fixture.configuration; config.conversationID = fixture.ids[0]
    let second = ManagedSessionCoordinator(configuration: config, nativeDraftDirectory: fixture.root.appendingPathComponent("native"), profile: fixture.root.path)
    await first.start(); await second.start()
    first.current.editDraft("First draft"); second.current.editDraft("Other draft")
    let a = try await prepareProvider(first.current, replacing: true), b = try await prepareProvider(second.current, replacing: true)
    var save = NativeProviderRequest(.save); save.provider = a.provider; save.reviewID = a.id; save.apiKey = providerInertSecret
    #expect(try await first.current.nativeProvider(save).confirmed)
    save = NativeProviderRequest(.save); save.provider = b.provider; save.reviewID = b.id; save.apiKey = providerInertSecret + "_COMPETING"
    let rejected = try await second.current.nativeProvider(save)
    #expect(!rejected.confirmed); #expect(rejected.conflict)
    #expect(try readProviderAuth(fixture.root)["openai"]["key"].string == providerInertSecret)
    #expect(first.current.draftText == "First draft"); #expect(second.current.draftText == "Other draft")
    #expect(first.ownerCount == 1); #expect(second.ownerCount == 1)
    #expect(await first.shutdown(confirmed: true)); #expect(await second.shutdown(confirmed: true))
}

private func providerControlledFiles(_ root: URL) throws -> [URL] {
    let files = try #require(FileManager.default.enumerator(at: root, includingPropertiesForKeys: [.isRegularFileKey]))
    return files.compactMap { $0 as? URL }
}
