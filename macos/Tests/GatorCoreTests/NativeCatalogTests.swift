import Foundation
import Testing
@testable import GatorCore

func catalogItemValue(_ id: String = "older", title: String = "Retained topic", version: String = String(repeating: "a", count: 64)) -> JSONValue {
    .object(["id": .string(id), "title": .string(title), "source": .string("/fixture/workspace"), "version": .string(version), "updated_at": .string("2026-11-18T00:00:00Z"), "pinned": .bool(false), "archived": .bool(false), "snippet": .string("public retained content")])
}
func catalogReply(_ request: NativeCatalogRequest, items: [JSONValue] = [catalogItemValue()], pageID: String = "page-current", next: String = "", error: String = "", conflict: Bool = false, extra: [String: JSONValue] = [:]) throws -> NativeCatalogResult {
    var value: [String: JSONValue] = ["request_id": .string(request.id), "action": .string(request.action.rawValue), "page_id": .string(pageID), "error": .string(error), "conflict": .bool(conflict)]
    if request.action == .query { var page: [String: JSONValue] = ["items": .array(items), "next": .string(next)]; page.merge(extra) { _, new in new }; value["page"] = .object(page) }
    else if let item = items.first { value["item"] = item }
    return try NativeCatalogResult(.object(value))
}
actor CatalogReplyGate {
    nonisolated let requests: AsyncStream<NativeCatalogRequest>
    private let output: AsyncStream<NativeCatalogRequest>.Continuation
    private var pending: [String: CheckedContinuation<NativeCatalogResult, any Error>] = [:]
    private var history: [NativeCatalogRequest] = []
    init() { let stream = AsyncStream<NativeCatalogRequest>.makeStream(); requests = stream.stream; output = stream.continuation }
    func send(_ command: NativeCatalogRequest) async throws -> NativeCatalogResult {
        history.append(command)
        if command.action == .cancel { output.yield(command); return try catalogReply(command) }
        return try await withCheckedThrowingContinuation { continuation in pending[command.id] = continuation; output.yield(command) }
    }
    func respond(_ command: NativeCatalogRequest, _ result: NativeCatalogResult) { pending.removeValue(forKey: command.id)?.resume(returning: result) }
    func fail(_ command: NativeCatalogRequest) { pending.removeValue(forKey: command.id)?.resume(throwing: BridgeFailure("Unconfirmed fixture disconnect")) }
    func count(_ action: NativeCatalogRequest.Action) -> Int { history.filter { $0.action == action }.count }
}

@Test func catalogProtocolEncodesOnlyTypedQueriesScopesAndExactVersionedActions() throws {
    for scope in NativeCatalogScope.allCases {
        var request = NativeCatalogRequest(.query); request.text = "jade"; request.scope = scope; request.after = "opaque-anchor"
        let frame = try JSONDecoder().decode(JSONValue.self, from: BridgeCommand.host(.catalog(request)).data(nonce: "nonce"))
        #expect(frame["type"].string == "native_catalog"); #expect(frame["native_catalog"]["scope"].string == scope.rawValue)
        #expect(frame["native_catalog"]["text"].string == "jade"); #expect(frame["native_catalog"]["after"].string == "opaque-anchor")
        #expect(frame["text"] == .null)
    }
    let item = try #require(catalogReply(NativeCatalogRequest(.query)).page?.items.first)
    var rename = NativeCatalogRequest(.rename); rename.item = item; rename.pageID = "reviewed-page"; rename.title = "New title"
    #expect(rename.fields["id"]?.string == item.id); #expect(rename.fields["version"]?.string == item.version)
    #expect(rename.fields["page_id"]?.string == "reviewed-page"); #expect(rename.fields["action"]?.string == "rename")
    #expect(rename.fields["title"]?.string == "New title")
}

@Test func catalogDecodingPreservesEmptyPagesCoverageAndRejectsDuplicateRows() throws {
    let request = NativeCatalogRequest(.query)
    let empty = try #require(catalogReply(request, items: [], extra: ["incomplete": .bool(true), "skipped": .number(3), "index_fallback": .bool(true)]).page)
    #expect(empty.items.isEmpty); #expect(empty.skipped == 3); #expect(empty.incomplete); #expect(empty.indexFallback)
    #expect(empty.coverageNotice.contains("incomplete")); #expect(empty.coverageNotice.contains("index is unavailable"))
    #expect(throws: (any Error).self) { try catalogReply(request, items: [catalogItemValue(), catalogItemValue()]) }
    let page = try #require(catalogReply(request).page)
    #expect(page.items.first?.date != nil)
    #expect(page.items.first?.snippet == "public retained content")
}

@Test func catalogBoundedSwiftProjectionAndSelectionMeasurements() throws {
    let value = JSONValue.object(["request_id": .string("measured"), "action": .string("query"), "page_id": .string("measured-page"), "page": .object(["items": .array((0..<50).map { catalogItemValue("record-\($0)") })])])
    var result = try NativeCatalogResult(value)
    let projection = try ContinuousClock().measure { for _ in 0..<100 { result = try NativeCatalogResult(value) } }
    let selection = ContinuousClock().measure { for _ in 0..<1_000 { #expect(result.page?.items.first { $0.id == "record-49" }?.id == "record-49") } }
    print("GATOR_P02_SWIFT_BENCH pageSize=50 projection100=\(projection) selection1000=\(selection)")
}

@Test @MainActor func catalogTitleValidationUsesByteLimitAndRejectsControlCharacters() {
    for text in ["", " \n ", "a\nb", String(repeating: "😀", count: 65)] { #expect(NativeConversationCatalog.validateTitle(text) != nil) }
    for text in ["A title", "  Trimmed by Go  ", String(repeating: "😀", count: 64), "東京 Café"] { #expect(NativeConversationCatalog.validateTitle(text) == nil) }
}

@Test @MainActor func catalogSupersededQueryScopeAndClosedSearchCannotPublishStaleResults() async throws {
    let gate = CatalogReplyGate(), state = NativeConversationCatalog(debounce: .zero)
    var requests = gate.requests.makeAsyncIterator()
    state.present { try await gate.send($0) }
    let first = try #require(await requests.next())
    state.query = "new query"; state.scope = .archived; state.search()
    let latest = try #require(await requests.next())
    #expect(latest.text == "new query"); #expect(latest.scope == .archived); #expect(state.selection == nil)
    await gate.respond(first, try catalogReply(first, items: [catalogItemValue("stale")]))
    #expect(state.items.isEmpty)
    await gate.respond(latest, try catalogReply(latest, items: [catalogItemValue("newest")]))
    try await waitUntil { !state.loading }
    #expect(state.items.map(\.id) == ["newest"])
    state.refresh(); let pending = try #require(await requests.next())
    state.dismiss(); let cancel = try #require(await requests.next())
    #expect(cancel.action == .cancel); #expect(cancel.cancelID == pending.id)
    await gate.respond(pending, try catalogReply(pending, items: [catalogItemValue("closed-stale")]))
    #expect(!state.isPresented); #expect(state.items.isEmpty)
}

@Test @MainActor func catalogPageAnchorsResetWithQueryAndSelectionDoesNotFollowRowIndexes() async throws {
    let gate = CatalogReplyGate(), state = NativeConversationCatalog(debounce: .zero)
    var requests = gate.requests.makeAsyncIterator(); state.present { try await gate.send($0) }
    let first = try #require(await requests.next()); await gate.respond(first, try catalogReply(first, next: "go-cursor"))
    try await waitUntil { !state.loading }; state.selection = "older"; #expect(state.selectedItem?.id == "older")
    state.next(); let second = try #require(await requests.next())
    #expect(second.after == "go-cursor"); #expect(state.selection == nil); #expect(state.pageNumber == 2)
    await gate.respond(second, try catalogReply(second, items: [catalogItemValue("second-page")]))
    try await waitUntil { !state.loading }; #expect(state.hasPrevious); #expect(!state.hasNext)
    state.previous(); let back = try #require(await requests.next()); #expect(back.after.isEmpty)
    await gate.respond(back, try catalogReply(back)); try await waitUntil { !state.loading }
    state.query = "different"; state.search(); let changed = try #require(await requests.next())
    #expect(changed.after.isEmpty); #expect(state.anchors.isEmpty); #expect(state.pageNumber == 1)
    await gate.respond(changed, try catalogReply(changed, items: [])); try await waitUntil { !state.loading }
    #expect(state.items.isEmpty); state.dismiss()
}

@Test @MainActor func catalogRenameConflictKeepsAttemptAndRequiresReviewThenExplicitFreshSave() async throws {
    let gate = CatalogReplyGate(), state = NativeConversationCatalog(debounce: .zero)
    var requests = gate.requests.makeAsyncIterator(); state.present { try await gate.send($0) }
    let query = try #require(await requests.next()); await gate.respond(query, try catalogReply(query))
    try await waitUntil { !state.loading }; state.selection = "older"; state.beginRename(); state.renameText = "Native attempted title"; state.rename()
    let rename = try #require(await requests.next())
    #expect(state.acting); #expect(state.editor != nil)
    await gate.respond(rename, try catalogReply(rename, error: "this conversation changed; refresh it before organizing it", conflict: true))
    try await waitUntil { !state.acting }
    #expect(state.renameText == "Native attempted title"); #expect(state.needsReview); #expect(!state.canSave)
    state.rename(); #expect(await gate.count(.rename) == 1)
    state.reviewRename(); let review = try #require(await requests.next())
    #expect(review.action == .inspect); #expect(review.item?.id == "older")
    let changed = catalogItemValue(title: "Terminal-side title", version: String(repeating: "b", count: 64))
    await gate.respond(review, try catalogReply(review, items: [changed], pageID: "page-reviewed"))
    try await waitUntil { !state.acting }
    #expect(state.renameText == "Native attempted title"); #expect(state.canSave); #expect(await gate.count(.rename) == 1)
    state.rename(); let fresh = try #require(await requests.next())
    #expect(fresh.item?.version == String(repeating: "b", count: 64)); #expect(fresh.pageID == "page-reviewed")
    await gate.respond(fresh, try catalogReply(fresh, items: [catalogItemValue(title: "Native attempted title", version: String(repeating: "c", count: 64))], pageID: "page-renamed"))
    try await waitUntil { !state.acting }
    #expect(state.editor == nil); #expect(state.items.first?.title == "Native attempted title"); #expect(state.query.isEmpty)
    state.dismiss()
}

@Test @MainActor func catalogUnconfirmedRenameNeverRetriesAndQueryRefreshRequiresFreshReview() async throws {
    let gate = CatalogReplyGate(), state = NativeConversationCatalog(debounce: .zero)
    var requests = gate.requests.makeAsyncIterator(); state.present { try await gate.send($0) }
    let query = try #require(await requests.next()); await gate.respond(query, try catalogReply(query))
    try await waitUntil { !state.loading }; state.selection = "older"; state.beginRename(); state.renameText = "Keep this attempt"; state.rename()
    let rename = try #require(await requests.next()); await gate.fail(rename)
    try await waitUntil { !state.acting }; #expect(state.issue.contains("unconfirmed")); #expect(state.renameText == "Keep this attempt"); #expect(!state.canSave)
    state.rename(); #expect(await gate.count(.rename) == 1)
    state.refresh(); let refreshed = try #require(await requests.next()); await gate.respond(refreshed, try catalogReply(refreshed))
    try await waitUntil { !state.loading }; #expect(!state.canSave); #expect(state.renameText == "Keep this attempt")
    state.cancelRename(); #expect(state.editor == nil); state.dismiss()
}

@Test @MainActor func catalogOpenResultIsExplicitAndDoesNotReplacePresentationOnLateClose() async throws {
    let gate = CatalogReplyGate(), state = NativeConversationCatalog(debounce: .zero)
    var requests = gate.requests.makeAsyncIterator(); state.present { try await gate.send($0) }
    let query = try #require(await requests.next()); await gate.respond(query, try catalogReply(query))
    try await waitUntil { !state.loading }; state.selection = "older"; state.openSelected()
    let open = try #require(await requests.next()); #expect(state.openedID == nil)
    state.dismiss(); await gate.respond(open, try catalogReply(open))
    try await waitUntil { !state.acting }; #expect(state.openedID == nil)
}
