import AppKit

@MainActor enum NativeDropInput {
    static func hasFiles(_ board: NSPasteboard) -> Bool { board.types?.contains(.fileURL) == true || board.types?.contains(.init("NSFilenamesPboardType")) == true }
    static func urls(_ board: NSPasteboard) -> [URL]? {
        guard hasFiles(board), let objects = board.readObjects(forClasses: [NSURL.self], options: [.urlReadingFileURLsOnly: true]), !objects.isEmpty else { return nil }
        let urls = objects.compactMap { $0 as? URL }
        guard urls.count == objects.count, urls.allSatisfy({ $0.isFileURL }) else { return nil }
        return urls
    }
}
@MainActor enum NativeInputTrace {
    private static var count = 0
    static func record(_ message: String) {
        #if DEBUG
        guard ProcessInfo.processInfo.environment["GATOR_NATIVE_INPUT_TRACE"] == "1", count < 64 else { return }
        count += 1
        // Fixed event metadata only: no text, pixels, paths, credentials or tokens.
        try? FileHandle.standardError.write(contentsOf: Data(("[native-input] " + message + "\n").utf8))
        #endif
    }
}
