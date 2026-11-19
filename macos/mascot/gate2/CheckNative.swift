import Foundation
import RealityKit

@main @MainActor struct CheckNative {
    static func main() async throws {
        let asset = URL(fileURLWithPath: CommandLine.arguments[1])
        let cache = URL(fileURLWithPath: CommandLine.arguments[2])
        let original = try await Entity(contentsOf: asset)
        let archive = try await NativeMeshArchive.read(cache)
        let native = try await archive.entity()
        guard let source = original.findEntity(named: "Gator_B") else { throw NSError(domain: "OriginalRootMissing", code: 1) }
        var comparisons = 0
        func visit(_ first: Entity, _ second: Entity) throws {
            guard first.name == second.name, first.children.count == second.children.count,
                  simd_length(first.position - second.position) < 0.00001,
                  simd_length(first.scale - second.scale) < 0.00001,
                  abs(abs(simd_dot(first.orientation.vector, second.orientation.vector)) - 1) < 0.00001,
                  first.isEnabled == second.isEnabled else {
                throw NSError(domain: "NativeNodeMismatch", code: comparisons, userInfo: [NSLocalizedDescriptionKey: "\(first.name) children \(first.children.map { ($0.name, $0.isEnabled, String(describing: $0.components[OpacityComponent.self])) })/\(second.children.map { ($0.name, $0.isEnabled) }) enabled \(first.isEnabled)/\(second.isEnabled) opacity \(String(describing: first.components[OpacityComponent.self]))"])
            }
            comparisons += 1
            for (left, right) in zip(first.children, second.children) { try visit(left, right) }
        }
        let originalRig = try MascotRig(root: source), nativeRig = try MascotRig(root: native)
        // compare the explicit neutral pose used by both runtime renderers;
        // the USD loader doesn't map authored visibility onto isEnabled.
        try visit(source, native)
        let sourceBounds = source.visualBounds(relativeTo: source), nativeBounds = native.visualBounds(relativeTo: native)
        guard simd_length(sourceBounds.extents - nativeBounds.extents) < 0.00001 else { throw NSError(domain: "NativeBoundsMismatch", code: 1) }
        let rigResult = try nativeRig.verify()
        for pose in [MascotPose(), MascotPose(open: 1), MascotPose(blink: 1), MascotPose(look: 1, tilt: 0.06), MascotPose(open: 0.85, blink: 0.5)] {
            originalRig.apply(pose); nativeRig.apply(pose); try visit(source, native)
        }
        let result: [String: Any] = ["nodeAndPoseComparisons": comparisons, "rig": rigResult, "sourceSHA256": archive.sourceSHA256,
                                     "geometryCounts": archive.counts, "bounds": [nativeBounds.extents.x, nativeBounds.extents.y, nativeBounds.extents.z], "windowsCreated": 0]
        FileHandle.standardOutput.write(try JSONSerialization.data(withJSONObject: result, options: [.prettyPrinted, .sortedKeys]))
    }
}
