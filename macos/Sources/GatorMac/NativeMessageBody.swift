import SwiftUI
import GatorCore

struct NativeMessageBody: View {
    let text: String
    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            ForEach(NativeMessageContent.blocks(text)) { block in
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
        }.textSelection(.enabled).lineSpacing(3).frame(maxWidth: .infinity, alignment: .leading)
    }
    private func inline(_ text: String) -> Text {
        Text(Self.inlineContent(text))
    }
    static func inlineContent(_ text: String) -> AttributedString {
        (try? AttributedString(markdown: text, options: .init(interpretedSyntax: .inlineOnlyPreservingWhitespace))) ?? AttributedString(text)
    }
}
