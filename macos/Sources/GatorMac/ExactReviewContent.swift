import SwiftUI
import GatorCore

struct ExactReviewContent: View {
    let value: JSONValue
    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text(verbatim: value["title"].string).font(.headline).accessibilityAddTraits(.isHeader)
            if !value["summary"].string.isEmpty { Text(verbatim: value["summary"].string) }
            ForEach(NativeReviewPresentation.fields(value).filter { !$0.isTechnical }) { field in
                VStack(alignment: .leading, spacing: 4) {
                    Text(verbatim: field.label.capitalized).font(.caption.weight(.semibold)).foregroundStyle(.secondary)
                    Text(verbatim: field.text).fixedSize(horizontal: false, vertical: true)
                }.accessibilityRepresentation { Text(verbatim: field.label + ": " + field.text) }
            }
            if !value["footer"].string.isEmpty, !value["footer"].string.hasPrefix("/") { Text(verbatim: value["footer"].string).font(.caption) }
            DisclosureGroup("Complete exact review data") {
                Text(verbatim: value.pretty).font(.system(.caption, design: .monospaced)).fixedSize(horizontal: false, vertical: true)
            }
        }.textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading)
    }
}
