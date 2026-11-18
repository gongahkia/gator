import SwiftUI
import Observation
import GatorCore

/// Window-local presentation state; never retains or admits a Go owner.
@MainActor @Observable final class NativeTranscriptFind {
    struct Reveal: Equatable { let id = UUID(); let match: NativeTranscriptSearch.Match }
    private(set) var isPresented = false
    var query = "" { didSet { if query != oldValue { updateQuery() } } }
    private(set) var matches: [NativeTranscriptSearch.Match] = []
    private(set) var limited = false
    private(set) var selected: NativeTranscriptSearch.Match?
    private(set) var reveal: Reveal?
    private(set) var focusRequest = UUID()
    @ObservationIgnored private var index: NativeTranscriptSearch?
    var scope: NativeTranscriptSearch.Scope? { index?.scope }

    func open(scope: NativeTranscriptSearch.Scope, messages: [BridgeMessage]) {
        if index?.scope != scope { close() }
        isPresented = true; refresh(scope: scope, messages: messages); focusRequest = UUID()
    }
    func close() {
        isPresented = false; query = ""; matches = []; selected = nil; reveal = nil; index = nil; limited = false
    }
    func refresh(scope: NativeTranscriptSearch.Scope, messages: [BridgeMessage]) {
        guard isPresented else { return }
        guard index == nil || index?.scope == scope else { close(); return }
        let updated = NativeTranscriptSearch(scope: scope, messages: messages)
        index = updated
        let result = updated.search(query)
        matches = result.matches; limited = result.limited
        // Background updates neither reveal a result nor request focus.
        selected = selected.flatMap { old in matches.first { $0.id == old.id && index?.resolve(old) != nil } }
        if let reveal, index?.resolve(reveal.match) == nil { self.reveal = nil }
    }
    func select(_ match: NativeTranscriptSearch.Match) {
        guard isPresented, matches.contains(match), index?.resolve(match) != nil else { return }
        selected = match; reveal = Reveal(match: match)
    }
    func move(_ delta: Int) {
        guard !matches.isEmpty else { return }
        let offset = selected.flatMap { matches.firstIndex(of: $0) }
        let next = offset.map { ($0 + delta + matches.count) % matches.count } ?? (delta < 0 ? matches.count - 1 : 0)
        select(matches[next])
    }
    private func updateQuery() {
        guard let index, isPresented else { return }
        let result = index.search(query); matches = result.matches; limited = result.limited; selected = nil; reveal = nil
        if let first = matches.first { select(first) }
    }
}

struct NativeFindActionKey: FocusedValueKey { typealias Value = @MainActor () -> Void }
extension FocusedValues {
    var findInTranscript: NativeFindActionKey.Value? {
        get { self[NativeFindActionKey.self] }
        set { self[NativeFindActionKey.self] = newValue }
    }
}

struct NativeTranscriptFindBar: View {
    @Bindable var find: NativeTranscriptFind
    let close: () -> Void
    @FocusState private var focused: Bool
    @State private var showResults = false
    private var position: String {
        guard let selected = find.selected, let index = find.matches.firstIndex(of: selected) else { return "\(find.matches.count) matches" }
        return "\(index + 1) of \(find.matches.count)\(find.limited ? "+" : "")"
    }
    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack(spacing: 8) {
                Image(systemName: "magnifyingglass").accessibilityHidden(true)
                TextField("Find in loaded transcript", text: $find.query)
                    .textFieldStyle(.roundedBorder).focused($focused)
                    .accessibilityLabel("Find in loaded transcript").accessibilityIdentifier("gator.find.query")
                    .onKeyPress(.return, phases: .down) { press in find.move(press.modifiers.contains(.shift) ? -1 : 1); return .handled }
                Text(position).font(.caption).monospacedDigit().fixedSize().accessibilityIdentifier("gator.find.count")
                Button { find.move(-1) } label: { Image(systemName: "chevron.up") }.help("Previous match · Shift Return in Find").accessibilityLabel("Previous match").disabled(find.matches.isEmpty)
                Button { find.move(1) } label: { Image(systemName: "chevron.down") }.help("Next match · Return in Find").accessibilityLabel("Next match").disabled(find.matches.isEmpty)
                Button(action: close) { Image(systemName: "xmark") }.help("Close Find · Escape").accessibilityLabel("Close Find")
            }
            HStack(alignment: .top, spacing: 8) {
                if let selected = find.selected {
                    Text(verbatim: "\(selected.ordinal). \(selected.role): \(selected.excerpt)").font(.callout).lineLimit(2).textSelection(.enabled).accessibilityIdentifier("gator.find.excerpt")
                } else { Text(find.query.isEmpty ? "Type to find text in this conversation." : "No matching loaded text.").font(.callout).foregroundStyle(.secondary) }
                Spacer(minLength: 0)
                Button("Results…") { showResults.toggle() }.disabled(find.matches.isEmpty)
                    .popover(isPresented: $showResults, arrowEdge: .bottom) {
                        ScrollViewReader { proxy in
                            List {
                                ForEach(find.matches) { match in
                                    Button { find.select(match); showResults = false; focused = true } label: {
                                        HStack(alignment: .top) {
                                            Image(systemName: find.selected?.id == match.id ? "checkmark.circle.fill" : "text.bubble").accessibilityHidden(true)
                                            VStack(alignment: .leading, spacing: 3) {
                                                Text("Message \(match.ordinal) · \(match.role)").font(.caption).foregroundStyle(.secondary)
                                                Text(verbatim: match.excerpt).lineLimit(3)
                                            }
                                        }.frame(maxWidth: .infinity, alignment: .leading).contentShape(Rectangle())
                                    }.buttonStyle(.plain).padding(.vertical, 4).id(match.id)
                                }
                            }.frame(width: 360, height: 280).onAppear { if let id = find.selected?.id { proxy.scrollTo(id) } }
                        }.padding(6)
                    }
            }
            Text(find.limited ? "Search limited: up to 10,000 messages, 8 MiB of text, 500 matches; query up to 256 characters." : "Loaded text and result headings only. Earlier history and collapsed details are excluded.")
                .font(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
        }.padding(.horizontal, 14).padding(.vertical, 8)
            .task(id: find.focusRequest) {
                // wait for insertion and pending native responder updates before
                // applying an explicit Find request; snapshots do not restart this.
                await Task.yield()
                focused = false
                await Task.yield()
                focused = true
            }
            .onExitCommand(perform: close)
            .onChange(of: find.isPresented) { _, presented in if !presented { showResults = false } }
    }
}
