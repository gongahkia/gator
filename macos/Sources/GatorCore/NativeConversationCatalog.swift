import Foundation
import Observation

/// Presentation of Go-owned catalog pages. It holds neither a search index nor execution authority.
@MainActor @Observable public final class NativeConversationCatalog {
    public var query = ""
    public var scope = NativeCatalogScope.active
    public var selection: String?
    public var renameText = ""
    public private(set) var isPresented = false
    public private(set) var loading = false
    public private(set) var acting = false
    public private(set) var issue = ""
    public private(set) var notice = ""
    public private(set) var page: NativeCatalogPage?
    public private(set) var pageID = ""
    public private(set) var editor: NativeCatalogItem?
    public private(set) var needsReview = false
    public private(set) var openedID: String?
    public private(set) var focusRequest = UUID()
    public private(set) var anchors: [String] = []
    @ObservationIgnored private var after = ""
    @ObservationIgnored private var request: (@MainActor (NativeCatalogRequest) async throws -> NativeCatalogResult)?
    @ObservationIgnored private var queryTask: Task<Void, Never>?
    @ObservationIgnored private var actionTask: Task<Void, Never>?
    @ObservationIgnored private var cancelTask: Task<Void, Never>?
    @ObservationIgnored private var queryID: String?
    @ObservationIgnored private let debounce: Duration
    init(debounce: Duration = .milliseconds(180)) { self.debounce = debounce }
    public var items: [NativeCatalogItem] { page?.items ?? [] }
    public var selectedItem: NativeCatalogItem? { items.first { $0.id == selection } }
    public var pageNumber: Int { anchors.count + 1 }
    public var hasNext: Bool { !(page?.next ?? "").isEmpty && anchors.count < 512 }
    public var hasPrevious: Bool { !anchors.isEmpty }
    public var retainsHost: Bool { isPresented || acting || needsReview }
    public var titleValidation: String? { Self.validateTitle(renameText) }
    public var canSave: Bool { editor != nil && !acting && !loading && !needsReview && titleValidation == nil && items.contains { $0.id == editor?.id && $0.version == editor?.version } }
    public static func validateTitle(_ title: String) -> String? {
        let value = title.trimmingCharacters(in: .whitespacesAndNewlines)
        return value.isEmpty || value.utf8.count > 256 || value.unicodeScalars.contains { $0.properties.generalCategory == .control } ? "Use a title of 1–256 UTF-8 bytes on one line." : nil
    }
    func present(request: @escaping @MainActor (NativeCatalogRequest) async throws -> NativeCatalogResult) {
        self.request = request; isPresented = true; openedID = nil; focusRequest = UUID()
        if !acting { search() }
    }
    public func focusSearch() { focusRequest = UUID() }
    public func search() {
        guard !acting else { return }
        if editor != nil { needsReview = true }
        after = ""; anchors = []; startQuery()
    }
    public func refresh() { guard !acting else { return }; if editor != nil { needsReview = true }; startQuery() }
    public func next() {
        guard !loading, !acting, editor == nil, hasNext, let next = page?.next else { return }
        anchors.append(after); after = next; startQuery()
    }
    public func previous() {
        guard !loading, !acting, editor == nil, let prior = anchors.popLast() else { return }
        after = prior; startQuery()
    }
    public func beginRename() {
        guard editor == nil, !loading, !acting, let item = selectedItem else { return }
        editor = item; renameText = item.title; needsReview = false; issue = ""; notice = ""
    }
    public func cancelRename() { guard !acting else { return }; editor = nil; renameText = ""; needsReview = false; issue = "" }
    public func rename() {
        guard canSave, let item = editor else { return }
        var command = NativeCatalogRequest(.rename); command.item = item; command.pageID = pageID; command.title = renameText
        perform(command)
    }
    public func reviewRename() {
        guard !loading, !acting, let editor else { return }
        guard let item = items.first(where: { $0.id == editor.id }) else { issue = "This conversation is outside the current page. Clear or adjust the search, then refresh and review. Your attempted title is kept."; return }
        var command = NativeCatalogRequest(.inspect); command.item = item; command.pageID = pageID
        perform(command)
    }
    public func openSelected() {
        guard !loading, !acting, editor == nil, let item = selectedItem else { return }
        var command = NativeCatalogRequest(.open); command.item = item; command.pageID = pageID
        perform(command)
    }
    public func dismiss() {
        isPresented = false; cancelSearch()
    }
    func cancelSearch() {
        let target = loading ? queryID : nil; queryID = nil; queryTask?.cancel(); queryTask = nil; loading = false
        guard let target, let request else { return }
        var command = NativeCatalogRequest(.cancel); command.cancelID = target
        cancelTask?.cancel()
        cancelTask = Task { do { _ = try await request(command) } catch { /* supersession and shutdown also cancel Go's bounded query */ } }
    }
    func finishActions() async { await actionTask?.value }
    private func startQuery() {
        guard isPresented, let request else { return }
        queryTask?.cancel(); selection = nil; page = nil; pageID = ""; issue = ""; notice = ""; openedID = nil
        var command = NativeCatalogRequest(.query); command.text = query; command.scope = scope; command.after = after
        let id = command.id; queryID = id; loading = true
        queryTask = Task { [weak self, debounce] in
            do {
                if debounce > .zero { try await Task.sleep(for: debounce) }
                let result = try await request(command)
                guard !Task.isCancelled, let self, self.isPresented, self.queryID == id else { return }
                self.loading = false
                if let error = result.error, !error.isEmpty { self.issue = error; return }
                guard result.requestID == id, result.action == "query", let page = result.page, let pageID = result.pageID, !pageID.isEmpty else { throw BridgeFailure("Go did not confirm this catalog page.") }
                self.page = page; self.pageID = pageID
            } catch {
                guard !Task.isCancelled, let self, self.isPresented, self.queryID == id else { return }
                self.loading = false; self.issue = error.localizedDescription
            }
        }
    }
    private func perform(_ command: NativeCatalogRequest) {
        guard isPresented, !acting, let request else { return }
        acting = true; issue = ""; notice = ""
        actionTask = Task { [weak self] in
            do {
                let result = try await request(command)
                guard let self else { return }
                self.acting = false
                guard result.requestID == command.id, result.action == command.action.rawValue else { throw BridgeFailure("The catalog operation was not confirmed.") }
                if let error = result.error, !error.isEmpty {
                    self.issue = error
                    if command.action == .rename || command.action == .inspect { self.needsReview = true }
                    return
                }
                guard let item = result.item, item.id == command.item?.id else { throw BridgeFailure("Go did not confirm the selected conversation.") }
                if command.action == .open { if self.isPresented { self.openedID = item.id }; return }
                guard let pageID = result.pageID, !pageID.isEmpty else { throw BridgeFailure("Refresh before editing this conversation again.") }
                self.pageID = pageID
                if let old = self.page {
                    let replacement = old.items.map { $0.id == item.id ? item : $0 }
                    self.page = old.replacing(items: replacement)
                }
                if command.action == .inspect {
                    self.editor = item; self.needsReview = false
                    self.notice = "Current saved title: \(item.title). Review your attempted title, then choose Save explicitly. Refresh results after finishing to reapply the search."
                } else {
                    self.editor = nil; self.renameText = ""; self.needsReview = false
                    self.notice = "Renamed to “\(item.title)”. Refresh results to reapply the search."
                }
            } catch {
                guard let self else { return }
                self.acting = false; self.issue = error.localizedDescription
                if command.action == .rename || command.action == .inspect { self.needsReview = true; self.issue += " Outcome unconfirmed. Refresh and review before another Save; nothing will be retried automatically." }
            }
        }
    }
}
