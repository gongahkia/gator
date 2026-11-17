import AppKit
import GatorCore

@MainActor enum NativeComposerPresentation {
    static func slashFeedback(_ text: String) -> String? {
        guard text.trimmingCharacters(in: .whitespacesAndNewlines).hasPrefix("/") else { return nil }
        // Go's upload_submit rejects all leading slashes, including literal paths.
        return "Messages cannot start with / here. For a path, write “Read /path…”; use Recent Conversations or model controls for native actions."
    }
    static func duplicateNames(_ files: [NativeUpload]) -> [String] {
        var seen: Set<String> = [], duplicates: Set<String> = []
        for file in files {
            let name = file.name.trimmingCharacters(in: .whitespacesAndNewlines)
            if !seen.insert(name).inserted { duplicates.insert(name) }
        }
        return duplicates.sorted()
    }
    static func editorHeight(contentHeight: CGFloat, compact: Bool) -> CGFloat {
        min(compact ? 100 : 160, max(compact ? 56 : 76, contentHeight))
    }
}
