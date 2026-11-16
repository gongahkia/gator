import SwiftUI

struct NativeErrorDisclosure: View {
    let text: String
    var body: some View {
        DisclosureGroup {
            Text(verbatim: text).font(.callout).fixedSize(horizontal: false, vertical: true).textSelection(.enabled)
        } label: {
            Label { Text(verbatim: text.components(separatedBy: .newlines).first ?? "Gator needs attention").lineLimit(2) } icon: { Image(systemName: "exclamationmark.triangle") }
        }.font(.caption).accessibilityHint("Expand to read and select the complete error.")
    }
}
