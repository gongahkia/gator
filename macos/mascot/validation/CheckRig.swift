// headless SDK-level articulation checks; no NSApplication/window or Go process
import Foundation
import RealityKit

@main @MainActor struct CheckRig {
    static func main() async throws {
        let url = URL(fileURLWithPath: CommandLine.arguments[1])
        let first = try MascotRig(root: await Entity(contentsOf: url))
        let result = try first.verify()
        let second = try MascotRig(root: await Entity(contentsOf: url))
        let secondHead = second.head.transform, secondJaw = second.jaw.transform
        first.apply(MascotPose(open: 0.85, blink: 1, look: -1, tilt: 0.05))
        guard second.head.transform == secondHead, second.jaw.transform == secondJaw, first.root !== second.root else {
            throw NSError(domain: "HeadlessPresentationIsolation", code: 1)
        }
        var missingRejected = false
        do { _ = try await Entity(contentsOf: url.deletingLastPathComponent().appendingPathComponent("missing.usdz")) }
        catch { missingRejected = true }
        guard missingRejected else { throw NSError(domain: "MissingAssetNotRejected", code: 1) }
        let bounds = second.root.visualBounds(relativeTo: second.root).extents
        guard bounds.x > 0, bounds.y > 0, bounds.z > 0 else { throw NSError(domain: "MissingGeometryDepth", code: 1) }
        let report: [String: Any] = ["rig": result, "independentRoots": true, "missingAssetRejected": true,
                                   "geometryExtents": [bounds.x, bounds.y, bounds.z], "windowsCreated": 0, "GoProcessesCreated": 0]
        let data = try JSONSerialization.data(withJSONObject: report, options: [.prettyPrinted, .sortedKeys])
        FileHandle.standardOutput.write(data)
        FileHandle.standardOutput.write(Data("\n".utf8))
    }
}
