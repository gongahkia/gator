import AppKit
import RealityKit

struct NativeMeshArchive: Decodable, Sendable {
    let version: Int
    let sourceSHA256: String
    let counts: [String: Int]
    let root: Node
    struct Node: Decodable, Sendable {
        let name: String
        let translation: SIMD3<Float>
        let rotation: SIMD3<Float>
        let scale: SIMD3<Float>
        let visible: Bool
        let mesh: Mesh?
        let children: [Node]
    }
    struct Mesh: Decodable, Sendable {
        let positions: [SIMD3<Float>]
        let normals: [SIMD3<Float>]
        let indices: [UInt32]
        let color: [Float]
        let emission: [Float]
        let roughness: Float
        let metallic: Float
    }
    nonisolated static func read(_ url: URL) async throws -> Self {
        let archive = try JSONDecoder().decode(Self.self, from: Data(contentsOf: url, options: .mappedIfSafe))
        guard archive.version == 1, archive.counts == ["meshes": 26, "vertices": 8609, "triangles": 14164] else {
            throw NSError(domain: "UnexpectedBMeshArchive", code: 1)
        }
        return archive
    }
    @MainActor func entity() async throws -> Entity {
        var materials: [[Float]: PhysicallyBasedMaterial] = [:]
        func color(_ values: [Float]) throws -> NSColor {
            guard values.count == 3, values.allSatisfy({ $0.isFinite && $0 >= 0 && $0 <= 1 }) else { throw NSError(domain: "InvalidBMaterial", code: 1) }
            func srgb(_ value: Float) -> CGFloat { CGFloat(value <= 0.0031308 ? value * 12.92 : 1.055 * pow(value, 1 / 2.4) - 0.055) }
            return NSColor(srgbRed: srgb(values[0]), green: srgb(values[1]), blue: srgb(values[2]), alpha: 1)
        }
        func build(_ node: Node) async throws -> Entity {
            try Task.checkCancellation()
            let entity = Entity(); entity.name = node.name
            let radians = node.rotation * (.pi / 180)
            entity.transform = Transform(scale: node.scale,
                                         rotation: simd_quatf(angle: radians.z, axis: [0,0,1]) * simd_quatf(angle: radians.y, axis: [0,1,0]) * simd_quatf(angle: radians.x, axis: [1,0,0]), translation: node.translation)
            entity.isEnabled = node.visible
            if let data = node.mesh {
                guard !data.positions.isEmpty, data.positions.count == data.normals.count,
                      data.indices.count.isMultiple(of: 3), data.indices.allSatisfy({ $0 < data.positions.count }) else { throw NSError(domain: "InvalidBMesh", code: 1) }
                var descriptor = MeshDescriptor(name: node.name)
                descriptor.positions = .init(data.positions); descriptor.normals = .init(data.normals)
                descriptor.primitives = .triangles(data.indices)
                let mesh = try await MeshResource(from: [descriptor])
                let key = data.color + data.emission + [data.roughness, data.metallic]
                let material: PhysicallyBasedMaterial
                if let cached = materials[key] { material = cached }
                else {
                    var created = PhysicallyBasedMaterial()
                    created.baseColor = .init(tint: try color(data.color))
                    created.emissiveColor = .init(color: try color(data.emission)); created.emissiveIntensity = 1
                    created.roughness = .init(scale: data.roughness); created.metallic = .init(scale: data.metallic)
                    created.faceCulling = .none; materials[key] = created; material = created
                }
                let surface = ModelEntity(mesh: mesh, materials: [material]); surface.name = "Surface"
                entity.addChild(surface)
            }
            for child in node.children { entity.addChild(try await build(child)) }
            return entity
        }
        return try await build(root)
    }
}
