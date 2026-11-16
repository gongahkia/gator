import Foundation

public struct NativeAttachmentImport: Equatable, Sendable {
    let id = UUID()
    public let context: String
    public let generation: UUID
    let existing: [NativeUpload]
}

public enum NativeClipboardPolicy {
    /// File URLs are never treated as image bytes. With a text representation,
    /// ordinary Paste pastes plain text; Paste Image explicitly requests pixels.
    public static func pasteImage(hasText: Bool, hasFileURL: Bool, hasImage: Bool, explicitlyImage: Bool) -> Bool {
        hasImage && !hasFileURL && (explicitlyImage || !hasText)
    }
}
