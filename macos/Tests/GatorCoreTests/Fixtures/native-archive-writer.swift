import Foundation
import Darwin

@main struct NativeArchiveWriterFixture {
    static func main() async throws {
        let root = URL(fileURLWithPath: CommandLine.arguments[1])
        let name = CommandLine.arguments[2], mode = CommandLine.arguments[3]
        let context = "conversation:" + name
        let store = NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "safety", writeHook: { stage in
            if mode == "beforeReplace" && stage == .beforeReplace || mode == "afterReplace" && stage == .afterReplace {
                try Data().write(to: root.appendingPathComponent(name + ".stage"))
                while !FileManager.default.fileExists(atPath: root.appendingPathComponent("finish").path) { usleep(5_000) }
            }
        })
        var archive = try await store.load()
        archive.drafts[context] = NativeDraft(text: "child " + name)
        try Data().write(to: root.appendingPathComponent(name + ".ready"))
        let deadline = ContinuousClock.now + .seconds(20)
        while !FileManager.default.fileExists(atPath: root.appendingPathComponent("start").path) {
            guard ContinuousClock.now < deadline else { throw BridgeFailure("Fixture start gate timed out") }
            try await Task.sleep(for: .milliseconds(5))
        }
        do {
            try await store.saveOwner(archive, context: context)
            guard mode != "conflict" else { throw BridgeFailure("Stale writer unexpectedly saved") }
        } catch {
            guard mode == "conflict", error.localizedDescription.contains("Another native writer changed") else { throw error }
            try Data("conflict".utf8).write(to: root.appendingPathComponent(name + ".result"))
        }
    }
}
