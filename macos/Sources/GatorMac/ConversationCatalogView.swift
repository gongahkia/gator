import SwiftUI
import AppKit
import GatorCore

struct CatalogFindActionKey: FocusedValueKey { typealias Value = @MainActor () -> Void }
extension FocusedValues {
    var findInCatalog: CatalogFindActionKey.Value? {
        get { self[CatalogFindActionKey.self] }
        set { self[CatalogFindActionKey.self] = newValue }
    }
}

struct AllConversationsButton: View {
    let sessions: ManagedSessionCoordinator
    @Environment(\.openWindow) private var openWindow
    var body: some View {
        Button { sessions.showCatalog(); openWindow(id: "catalog"); NSApp.activate(ignoringOtherApps: true) } label: {
            Label("All Conversations…", systemImage: "text.magnifyingglass")
        }.disabled(!sessions.canNavigate)
            .help("Search all retained conversations, including older and archived chats")
            .accessibilityLabel("All Conversations").accessibilityIdentifier("gator.catalog.open")
    }
}

struct ConversationCatalogView: View {
    let sessions: ManagedSessionCoordinator
    @Bindable private var catalog: NativeConversationCatalog
    @Environment(\.openWindow) private var openWindow
    @Environment(\.dismissWindow) private var dismissWindow
    @FocusState private var focus: Field?
    private enum Field { case search, rename }
    init(sessions: ManagedSessionCoordinator) { self.sessions = sessions; self.catalog = sessions.catalog }
    var body: some View {
        VStack(spacing: 0) {
            searchControls.padding(14)
            Divider()
            results
            Divider()
            status.padding(.horizontal, 14).padding(.vertical, 8)
            if let editor = catalog.editor { Divider(); renameEditor(editor).padding(14) }
            footer.padding(14)
        }
        .background(NativeWindowConfiguration(compact: false, registersConversation: false).frame(width: 0, height: 0))
        .tint(GatorTheme.accent)
        .focusedSceneValue(\.findInCatalog, { @MainActor in catalog.focusSearch() })
        .task { sessions.showCatalog() }
        .task(id: catalog.focusRequest) { await Task.yield(); focus = nil; await Task.yield(); focus = .search }
        .task(id: catalog.editor?.id) { if catalog.editor != nil { await Task.yield(); focus = .rename } }
        .onDisappear { catalog.dismiss() }
        .onChange(of: catalog.query) { _, _ in catalog.search() }
        .onChange(of: catalog.scope) { _, _ in catalog.search() }
        .onChange(of: catalog.openedID) { _, id in
            guard id != nil else { return }
            openWindow(id: "conversation"); dismissWindow(id: "catalog"); NSApp.activate(ignoringOtherApps: true)
        }
        .onExitCommand { if catalog.editor != nil { catalog.cancelRename() } else { dismissWindow(id: "catalog") } }
        .accessibilityIdentifier("gator.catalog.window")
    }
    private var searchControls: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 8) {
                Image(systemName: "magnifyingglass").foregroundStyle(.secondary).accessibilityHidden(true)
                TextField("Search retained conversations", text: $catalog.query)
                    .textFieldStyle(.roundedBorder).focused($focus, equals: .search).disabled(catalog.acting)
                    .accessibilityLabel("Search all retained conversations").accessibilityIdentifier("gator.catalog.query")
                    .onSubmit { catalog.search() }
                Button("Clear") { catalog.query = ""; catalog.focusSearch() }.disabled(catalog.query.isEmpty || catalog.acting)
            }
            HStack {
                Picker("Scope", selection: $catalog.scope) {
                    ForEach(NativeCatalogScope.allCases) { Text($0.label).tag($0) }
                }.pickerStyle(.segmented).labelsHidden().disabled(catalog.acting)
                    .accessibilityLabel("Conversation search scope")
                Button("Refresh", systemImage: "arrow.clockwise") { sessions.showCatalog(); catalog.refresh() }
                    .disabled(catalog.acting).help("Refresh this catalog page")
            }
            Text("Search titles, folders and indexed work messages. Conversation-only dialogue is not indexed. Use transcript Find for the current loaded chat.")
                .font(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
        }
    }
    private var results: some View {
        List(selection: $catalog.selection) {
            ForEach(catalog.items) { item in
                CatalogConversationRow(item: item, current: item.id == sessions.current.conversationID)
                    .tag(item.id)
                    .contextMenu { Button("Rename…") { catalog.selection = item.id; catalog.beginRename() }.disabled(catalog.acting || catalog.editor != nil) }
            }
        }.listStyle(.inset).disabled(catalog.loading || catalog.acting)
            .accessibilityLabel("Retained conversation results")
            .onKeyPress(.return) { guard catalog.editor == nil else { return .ignored }; catalog.openSelected(); return .handled }
            .overlay {
                if catalog.loading { ProgressView("Searching retained conversations…").padding().background(.regularMaterial, in: RoundedRectangle(cornerRadius: 8)) }
                else if catalog.items.isEmpty { Text(catalog.issue.isEmpty ? "No conversations on this page match the search and scope." : "Catalog results are unavailable.").foregroundStyle(.secondary).padding().multilineTextAlignment(.center) }
            }
    }
    private var status: some View {
        VStack(alignment: .leading, spacing: 5) {
            if !catalog.issue.isEmpty { NativeStatusMessage(text: catalog.issue, severity: .error) }
            if let page = catalog.page, !page.coverageNotice.isEmpty { NativeStatusMessage(text: page.coverageNotice, severity: .warning) }
            if !catalog.notice.isEmpty { Text(verbatim: catalog.notice).font(.callout).textSelection(.enabled) }
            if catalog.acting { ProgressView("Waiting for Go confirmation…").controlSize(.small) }
        }.frame(maxWidth: .infinity, alignment: .leading)
    }
    private func renameEditor(_ item: NativeCatalogItem) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Rename conversation").font(.headline)
            Text(verbatim: item.title).font(.callout).lineLimit(1)
            Text(verbatim: item.id).font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
            TextField("Conversation title", text: $catalog.renameText)
                .textFieldStyle(.roundedBorder).focused($focus, equals: .rename).disabled(catalog.acting)
                .accessibilityLabel("Conversation title").accessibilityIdentifier("gator.catalog.rename-title")
            if let validation = catalog.titleValidation { NativeStatusMessage(text: validation, severity: .warning) }
            HStack {
                Button("Cancel") { catalog.cancelRename() }.disabled(catalog.acting)
                if catalog.needsReview { Button("Refresh and Review") { catalog.reviewRename() }.disabled(catalog.acting || catalog.loading) }
                Spacer()
                Button("Save Title") { catalog.rename() }.disabled(!catalog.canSave)
            }
        }
    }
    private var footer: some View {
        HStack(spacing: 10) {
            Button("Previous") { catalog.previous() }.disabled(!catalog.hasPrevious || catalog.loading || catalog.acting || catalog.editor != nil)
            Text("Page \(catalog.pageNumber) · \(catalog.items.count) on this page").font(.caption).foregroundStyle(.secondary).monospacedDigit()
            Button("Next") { catalog.next() }.disabled(!catalog.hasNext || catalog.loading || catalog.acting || catalog.editor != nil)
            Spacer(minLength: 0)
            if catalog.editor == nil {
                Button("Rename…") { catalog.beginRename() }.disabled(catalog.selectedItem == nil || catalog.loading || catalog.acting)
                Button("Open") { catalog.openSelected() }.disabled(catalog.selectedItem == nil || catalog.loading || catalog.acting)
            }
        }
    }
}

private struct CatalogConversationRow: View {
    let item: NativeCatalogItem
    let current: Bool
    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: current ? "checkmark.circle.fill" : item.archived ? "archivebox" : "bubble.left").foregroundStyle(current ? GatorTheme.accent : .secondary).accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 4) {
                HStack {
                    Text(verbatim: item.title.isEmpty ? item.id : item.title).fontWeight(current ? .semibold : .regular).lineLimit(2)
                    if item.pinned { Image(systemName: "pin.fill").font(.caption).accessibilityLabel("Pinned") }
                    Spacer(minLength: 4)
                    if let date = item.date { Text(date, style: .date).font(.caption).foregroundStyle(.secondary) }
                }
                Text(verbatim: item.source).font(.caption).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle)
                if let snippet = item.snippet, !snippet.isEmpty { Text(verbatim: snippet).font(.callout).lineLimit(2) }
                if current || item.archived { Text([current ? "Current conversation" : "", item.archived ? "Archived" : ""].filter { !$0.isEmpty }.joined(separator: " · ")).font(.caption).foregroundStyle(.secondary) }
            }
        }.padding(.vertical, 5).help(item.title + "\n" + item.source + "\n" + item.id)
            .accessibilityElement(children: .combine).accessibilityIdentifier("gator.catalog.row." + item.id)
    }
}
