import SwiftUI
import AppKit

/// Severity has a symbol and an accessible name; text uses the system label color.
struct NativeStatusMessage: View {
    enum Severity { case information, warning, error }
    let text: String
    var severity: Severity = .warning
    private var symbol: String { severity == .error ? "exclamationmark.octagon" : severity == .warning ? "exclamationmark.triangle" : "info.circle" }
    private var name: String { severity == .error ? "Error" : severity == .warning ? "Warning" : "Information" }
    var body: some View {
        Label { Text(verbatim: text).fixedSize(horizontal: false, vertical: true).textSelection(.enabled) } icon: {
            Image(systemName: symbol).foregroundStyle(severity == .error ? Color(nsColor: .systemRed) : Color(nsColor: .secondaryLabelColor))
        }
        .foregroundStyle(Color(nsColor: .labelColor))
        .accessibilityElement(children: .combine).accessibilityLabel(name + ": " + text)
    }
}
