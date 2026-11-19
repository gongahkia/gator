import AppKit
import RealityKit
import CryptoKit

@main @MainActor struct InspectGeometry {
    static func main() async throws {
        let source = try await Entity(contentsOf: URL(fileURLWithPath: CommandLine.arguments[1]))
        let archive = try await NativeMeshArchive.read(URL(fileURLWithPath: CommandLine.arguments[2]))
        let native = try await archive.entity()
        guard let original = source.findEntity(named: "Gator_B") else { throw NSError(domain: "MissingOriginal", code: 1) }
        var records: [[String: Any]] = []
        func inspect(_ a: Entity, _ b: Entity, path: String) throws {
            guard a.name == b.name, a.children.count == b.children.count else { throw NSError(domain: "GraphMismatch", code: 1) }
            let path = path + "/" + a.name
            if let left = a.components[ModelComponent.self], let right = b.components[ModelComponent.self] {
                let aa = left.mesh.contents.models.flatMap { Array($0.parts) }
                let bb = right.mesh.contents.models.flatMap { Array($0.parts) }
                guard aa.count == bb.count else { throw NSError(domain: "PartCountMismatch", code: 1) }
                for (first, second) in zip(aa, bb) {
                    let positions = zip(first.positions.elements, second.positions.elements).map { simd_length($0 - $1) }
                    let normals = zip(first.normals?.elements ?? [], second.normals?.elements ?? []).map { simd_length($0 - $1) }
                    records.append(["path": path, "positionCounts": [first.positions.count, second.positions.count],
                                    "normalCounts": [first.normals?.count ?? 0, second.normals?.count ?? 0],
                                    "indexCounts": [first.triangleIndices?.count ?? 0, second.triangleIndices?.count ?? 0],
                                    "indicesEqual": first.triangleIndices?.elements == second.triangleIndices?.elements,
                                    "maximumPositionDelta": positions.max() ?? 0, "maximumNormalDelta": normals.max() ?? 0,
                                    "sourceBuffers": first.buffers.keys.map(\.description).sorted(),
                                    "nativeBuffers": second.buffers.keys.map(\.description).sorted()])
                }
            }
            for (left, right) in zip(a.children, b.children) { try inspect(left, right, path: path) }
        }
        try inspect(original, native, path: "")
        let output: [String: Any] = ["sourceRoot": source.name, "sourceRootTransform": String(describing: source.transform),
                                    "originalTransform": String(describing: original.transform), "parts": records]
        FileHandle.standardOutput.write(try JSONSerialization.data(withJSONObject: output, options: [.prettyPrinted, .sortedKeys]))
    }
}
