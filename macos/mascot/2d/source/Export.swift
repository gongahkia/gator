// authoring executable only; 3D frameworks never enter the playback application
import AppKit
import RealityKit
import ImageIO
import UniformTypeIdentifiers
import CryptoKit

@main @MainActor struct ExportSprites {
    static func main() async throws {
        let root = URL(fileURLWithPath: CommandLine.arguments[1]), source = URL(fileURLWithPath: CommandLine.arguments[2])
        let original = try await Entity(contentsOf: source)
        var materials = 0
        func verify(_ entity: Entity) throws {
            if let model = entity.components[ModelComponent.self] {
                for material in model.materials {
                    guard let material = material as? PhysicallyBasedMaterial, material.faceCulling == .back else { throw NSError(domain: "CanonicalMaterialRequired", code: 1) }
                    materials += 1
                }
            }
            for child in entity.children { try verify(child) }
        }
        try verify(original); guard materials == 26, let device = MTLCreateSystemDefaultDevice() else { throw NSError(domain: "MissingConceptB", code: 1) }
        let engine = try Gate2Engine(template: original, device: device, configuration: EngineConfiguration(pixels: 224, msaa: true, offscreen: true), audit: LifetimeProbe())
        var clips: [[String: Any]] = []
        for style in ["rendered", "classic"] {
            for recipe in SpriteRecipe.all {
                let destination = root.appendingPathComponent(style).appendingPathComponent(recipe.name)
                try FileManager.default.createDirectory(at: destination, withIntermediateDirectories: true)
                for (index, pose) in recipe.poses.enumerated() {
                    let path = destination.appendingPathComponent(String(format: "%02d.png", index))
                    if style == "rendered" {
                        engine.rig?.apply(MascotPose(open: pose.open, blink: pose.blink, look: pose.look, tilt: pose.tilt, concern: pose.concern))
                        let image = try await engine.poster()
                        guard let bitmap = image.representations.first as? NSBitmapImageRep, let data = bitmap.representation(using: .png, properties: [:]) else { throw NSError(domain: "MissingRenderedPNG", code: 1) }
                        try data.write(to: path)
                    } else {
                        guard let context = CGContext(data: nil, width: 224, height: 224, bitsPerComponent: 8, bytesPerRow: 224 * 4, space: CGColorSpace(name: CGColorSpace.sRGB)!, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue) else { throw NSError(domain: "VectorContext", code: 1) }
                        ClassicGator.draw(context, pixels: 224, pose: pose)
                        guard let image = context.makeImage(), let writer = CGImageDestinationCreateWithURL(path as CFURL, UTType.png.identifier as CFString, 1, nil) else { throw NSError(domain: "VectorPNG", code: 1) }
                        CGImageDestinationAddImage(writer, image, nil); guard CGImageDestinationFinalize(writer) else { throw NSError(domain: "VectorWrite", code: 1) }
                    }
                }
                clips.append(["style": style, "state": recipe.name, "frames": recipe.poses.count, "duration": recipe.duration, "staticFrame": recipe.poses.count - 1])
            }
        }
        await engine.close()
        let manifest: [String: Any] = ["version": 1, "pixels": 224, "sourceUSDZSHA256": SHA256.hash(data: try Data(contentsOf: source)).map { String(format: "%02x", $0) }.joined(), "canonicalImportedMaterials": materials, "clips": clips]
        try JSONSerialization.data(withJSONObject: manifest, options: [.prettyPrinted, .sortedKeys]).write(to: root.appendingPathComponent("manifest.json"))
        print("Exported \(clips.count) state-specific clips; original USDZ materials; no reconstructed material path")
    }
}
