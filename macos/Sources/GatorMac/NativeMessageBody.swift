import SwiftUI
import GatorCore

struct NativeMessageBody: View, Equatable {
    let text: String
    nonisolated static func == (lhs: Self, rhs: Self) -> Bool { lhs.text.utf8.elementsEqual(rhs.text.utf8) }
    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            ForEach(NativeMessageContent.blocks(text)) { block in
                NativeMessageBlockView(block: block).equatable()
            }
        }.textSelection(.enabled).lineSpacing(3).frame(maxWidth: .infinity, alignment: .leading)
    }
    static func inlineContent(_ text: String) -> AttributedString {
        (try? AttributedString(markdown: text, options: .init(interpretedSyntax: .inlineOnlyPreservingWhitespace))) ?? AttributedString(text)
    }
}

struct NativeMessageBlockView: View, Equatable {
    let block: NativeMessageBlock
    nonisolated static func == (lhs: Self, rhs: Self) -> Bool {
        let a = lhs.block, b = rhs.block
        guard a.id == b.id, a.kind == b.kind, a.text.utf8.elementsEqual(b.text.utf8) else { return false }
        if case .code(let language) = a.kind, case .code(let other) = b.kind, !language.utf8.elementsEqual(other.utf8) { return false }
        switch (a.listItem, b.listItem) {
        case (nil, nil): return true
        case let (a?, b?): return a.depth == b.depth && a.marker.utf8.elementsEqual(b.marker.utf8) && a.text.utf8.elementsEqual(b.text.utf8)
        default: return false
        }
    }
    var body: some View {
        switch block.kind {
        case .heading(let depth):
            inline(block.text).font(depth <= 2 ? .title3.weight(.semibold) : .headline).accessibilityAddTraits(.isHeader)
        case .code(let language):
            VStack(alignment: .leading, spacing: 6) {
                if !language.isEmpty { Text(verbatim: language).font(.caption).foregroundStyle(.secondary) }
                ScrollView(.horizontal) { Text(verbatim: block.text).font(.system(.body, design: .monospaced)).fixedSize(horizontal: true, vertical: true).padding(.bottom, 2) }
            }.padding(10).background(Color(nsColor: .textBackgroundColor), in: RoundedRectangle(cornerRadius: 8))
                .overlay(RoundedRectangle(cornerRadius: 8).stroke(Color(nsColor: .separatorColor), lineWidth: 0.5))
                .accessibilityElement(children: .contain).accessibilityLabel(language.isEmpty ? "Code" : language + " code")
        case .list:
            if let item = block.listItem {
                HStack(alignment: .firstTextBaseline, spacing: 8) {
                    Text(verbatim: item.marker).monospacedDigit().fixedSize().frame(minWidth: 18, alignment: .trailing)
                    inline(item.text).fixedSize(horizontal: false, vertical: true).frame(maxWidth: .infinity, alignment: .leading)
                }
                .padding(.leading, CGFloat(min(item.depth, 8)) * 14)
                .accessibilityElement(children: .combine).accessibilityHint("List level \(item.depth + 1)")
            }
        case .paragraph:
            inline(block.text).fixedSize(horizontal: false, vertical: true)
        }
    }
    private func inline(_ text: String) -> Text { Text(NativeMessageBody.inlineContent(text)) }
}
