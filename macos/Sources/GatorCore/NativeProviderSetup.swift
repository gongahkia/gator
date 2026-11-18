import Foundation
import Observation

/// Non-secret presentation only. The secure editor owns its transient key;
/// credential expectations and configuration validation belong to Go.
@MainActor @Observable public final class NativeProviderSetup {
    public private(set) var rows: [NativeProviderRow] = []
    public var selection: String? { didSet { if selection != oldValue { review = nil; editorEpoch = UUID() } } }
    public var forms: [String: NativeProviderForm] = [:]
    public private(set) var review: NativeProviderReview?
    public private(set) var issue = ""
    public private(set) var notice = ""
    public private(set) var busy = false
    public private(set) var isPresented = false
    public private(set) var needsRefresh = false
    public private(set) var editorEpoch = UUID()
    @ObservationIgnored private var sender: (@MainActor (NativeProviderRequest) async throws -> NativeProviderResult)?
    @ObservationIgnored private var operation: Task<Bool, Never>?
    @ObservationIgnored private var operationID = UUID()
    public var selected: NativeProviderRow? { rows.first { $0.id == selection } }
    public var retainsHost: Bool { isPresented || busy || needsRefresh }
    public var canReview: Bool { !busy && !needsRefresh && selected != nil }
    func present(sender: @escaping @MainActor (NativeProviderRequest) async throws -> NativeProviderResult) {
        self.sender = sender; isPresented = true
        if !busy { Task { await refresh() } }
    }
    public func refresh() async {
        guard !busy else { return }
        _ = await perform(NativeProviderRequest(.status))
    }
    public func prepare(replacing: Bool, removal: Bool = false) async -> Bool {
        guard canReview, let row = selected else { return false }
        var request = NativeProviderRequest(removal ? .review_remove : .review)
        request.provider = row.id; request.form = forms[row.id] ?? NativeProviderForm(row); request.replaceKey = replacing
        return await perform(request)
    }
    public func save(apiKey: String) async -> Bool {
        guard !busy, !needsRefresh, let review, review.provider == selection else { return false }
        var request = NativeProviderRequest(review.operation == "remove" ? .remove : .save)
        request.provider = review.provider; request.reviewID = review.id; request.apiKey = apiKey
        self.review = nil
        return await perform(request)
    }
    public func cancelReview() {
        guard !busy else { return }; review = nil; editorEpoch = UUID()
        // A discarded opaque review cannot cause an operation by itself.
    }
    public func dismiss() { isPresented = false; review = nil; editorEpoch = UUID() }
    func finishOperation() async { _ = await operation?.value }
    private func perform(_ request: NativeProviderRequest) async -> Bool {
        guard !busy, let sender else { return false }
        busy = true; issue = ""; notice = ""
        let selectedID = selection, id = UUID()
        operationID = id
        let task = Task { @MainActor [weak self] () -> Bool in
            guard let self else { return false }
            defer { self.busy = false }
            do {
                let result = try await sender(request)
                guard result.requestID == request.id, result.action == request.action.rawValue,
                      request.provider.isEmpty || result.provider == request.provider else { throw BridgeFailure("Provider completion identity did not match. Refresh before reviewing again.") }
                if let rows = result.rows {
                    guard rows.count <= 64, Set(rows.map(\.id)).count == rows.count else { throw BridgeFailure("Provider status was invalid. Refresh before setup.") }
                    self.rows = rows
                    for row in rows where self.forms[row.id] == nil { self.forms[row.id] = NativeProviderForm(row) }
                    if self.selection == nil { self.selection = rows.first?.id }
                }
                if let error = result.error, !error.isEmpty {
                    self.issue = error; self.review = nil; self.needsRefresh = true; self.editorEpoch = UUID(); return false
                }
                if request.action == .status { self.needsRefresh = false; self.review = nil }
                if let review = result.review, self.isPresented, self.selection == selectedID { self.review = review }
                if request.action == .save || request.action == .remove {
                    guard result.confirmed else { throw BridgeFailure("Provider storage was not confirmed. Refresh and review; no retry was sent.") }
                    self.needsRefresh = false; self.notice = result.notice ?? "Go confirmed this provider change. No live request was made."
                    self.editorEpoch = UUID()
                }
                return true
            } catch {
                self.issue = "Provider outcome is unconfirmed. Refresh status, inspect the result and explicitly review again. Nothing is retried automatically."
                self.needsRefresh = true; self.review = nil; self.editorEpoch = UUID(); return false
            }
        }
        operation = task
        let result = await task.value
        if operationID == id { operation = nil }
        return result
    }
}
