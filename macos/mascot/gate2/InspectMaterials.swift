import Foundation
import RealityKit
import AppKit

@main @MainActor struct InspectMaterials {
    static func main() async throws {
        let source = try await Entity(contentsOf: URL(fileURLWithPath: CommandLine.arguments[1]))
        let archive = try await NativeMeshArchive.read(URL(fileURLWithPath: CommandLine.arguments[2]))
        let native = try await archive.entity()
        func colors(_ color: NSColor) -> [CGFloat] {
            guard let color = color.usingColorSpace(.sRGB) else { return [] }
            return [color.redComponent, color.greenComponent, color.blueComponent, color.alphaComponent]
        }
        func collect(_ root: Entity) -> [[String: Any]] {
            var result: [[String: Any]] = []
            func visit(_ node: Entity, prefix: String) {
                let path = prefix + "/" + node.name
                if let model = node.components[ModelComponent.self] {
                    for material in model.materials {
                        var record: [String: Any] = ["path": path, "type": String(describing: type(of: material))]
                        if let material = material as? PhysicallyBasedMaterial {
                            record["color"] = colors(material.baseColor.tint)
                            record["emission"] = colors(material.emissiveColor.color)
                            record["roughness"] = material.roughness.scale
                            record["metallic"] = material.metallic.scale
                            record["specular"] = material.specular.scale
                            record["emissiveIntensity"] = material.emissiveIntensity
                            record["clearcoat"] = material.clearcoat.scale
                        }
                        result.append(record)
                    }
                }
                for child in node.children { visit(child, prefix: path) }
            }
            visit(root, prefix: ""); return result
        }
        guard let sourceRoot = source.findEntity(named: "Gator_B") else { throw NSError(domain: "MissingRoot", code: 1) }
        FileHandle.standardOutput.write(try JSONSerialization.data(withJSONObject: ["USD": collect(sourceRoot), "native": collect(native)], options: [.prettyPrinted,.sortedKeys]))
    }
}
