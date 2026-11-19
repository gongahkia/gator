import AppKit
import RealityKit
import Metal

@main @MainActor struct RenderImages {
    static func main() async throws {
        let output = URL(fileURLWithPath: CommandLine.arguments[1])
        try FileManager.default.createDirectory(at: output, withIntermediateDirectories: true)
        let source = try await Entity(contentsOf: URL(fileURLWithPath: CommandLine.arguments[2]))
        let native = try await NativeMeshArchive.read(URL(fileURLWithPath: CommandLine.arguments[3])).entity()
        guard let original = source.findEntity(named: "Gator_B"), let device = MTLCreateSystemDefaultDevice() else { throw NSError(domain: "MissingResource", code: 1) }
        func replace(_ destination: Entity, _ original: Entity, meshes: Bool, materials: Bool) throws {
            guard destination.name == original.name, destination.children.count == original.children.count else { throw NSError(domain: "GraphMismatch", code: 1) }
            if var current = destination.components[ModelComponent.self], let authored = original.components[ModelComponent.self] {
                if meshes { current.mesh = authored.mesh }
                if materials { current.materials = authored.materials }
                destination.components.set(current)
            }
            for (a,b) in zip(destination.children, original.children) { try replace(a,b,meshes: meshes,materials: materials) }
        }
        let originalMaterials = native.clone(recursive: true)
        try replace(originalMaterials, original, meshes: false, materials: true)
        let originalMeshes = native.clone(recursive: true)
        try replace(originalMeshes, original, meshes: true, materials: false)
        let originalBoth = native.clone(recursive: true)
        try replace(originalBoth, original, meshes: true, materials: true)
        let backFaces = native.clone(recursive: true)
        func correctCulling(_ entity: Entity) {
            if var model = entity.components[ModelComponent.self] {
                model.materials = model.materials.map { material in
                    guard var material = material as? PhysicallyBasedMaterial else { return material }
                    material.faceCulling = .back; return material
                }
                entity.components.set(model)
            }
            for child in entity.children { correctCulling(child) }
        }
        correctCulling(backFaces)
        func fieldVariant(_ key: String) -> Entity {
            let result=backFaces.clone(recursive:true)
            func visit(_ a:Entity,_ b:Entity){
                if var model=a.components[ModelComponent.self],var material=model.materials.first as? PhysicallyBasedMaterial,
                   let authored=b.components[ModelComponent.self]?.materials.first as? PhysicallyBasedMaterial{
                    switch key{
                    case "tint":material.baseColor=authored.baseColor
                    case "program":material.program=authored.program
                    case "specular":material.specular=authored.specular
                    case "roughness":material.roughness=authored.roughness
                    case "emission":material.emissiveColor=authored.emissiveColor;material.emissiveIntensity=authored.emissiveIntensity
                    default:break
                    }
                    model.materials=[material];a.components.set(model)
                }
                for (a,b) in zip(a.children,b.children){visit(a,b)}
            }
            visit(result,original);return result
        }
        let missingEye=source.clone(recursive:true);missingEye.findEntity(named:"Eye_Left")?.isEnabled=false
        let missingJaw=source.clone(recursive:true);missingJaw.findEntity(named:"Jaw_Pivot")?.isEnabled=false
        let templates: [(String,Entity)] = [("usd",source),("native",native),("native-culling-back",backFaces),("native-source-tint",fieldVariant("tint")),("native-source-program",fieldVariant("program")),("native-source-specular",fieldVariant("specular")),("native-source-roughness",fieldVariant("roughness")),("native-source-emission",fieldVariant("emission")),("native-usd-materials",originalMaterials),("native-usd-meshes",originalMeshes),("native-usd-both",originalBoth),("negative-missing-eye",missingEye),("negative-missing-jaw",missingJaw)]
        let poses: [(String,MascotPose)] = [("neutral",MascotPose()),("articulated",MascotPose(open: 0.85,blink: 1)),("jaw",MascotPose(open: 0.85)),("lids",MascotPose(blink: 1)),("gaze",MascotPose(look: 1,tilt: 0.05))]
        var files: [[String: Any]] = []
        let audit = LifetimeProbe()
        for (name,template) in templates {
            for (poseName,pose) in poses {
                let engine = try Gate2Engine(template: template, device: device, configuration: EngineConfiguration(pixels: 96, offscreen: true), audit: audit)
                engine.rig?.apply(pose)
                for frame in 0..<5 {
                    let image = try await engine.poster()
                    guard let bitmap = image.representations.first as? NSBitmapImageRep,
                          let png = bitmap.representation(using: .png, properties: [:]) else { throw NSError(domain: "MissingPNG",code: 1) }
                    let filename = "\(name)-\(poseName)-\(frame).png"
                    try png.write(to: output.appendingPathComponent(filename))
                    files.append(["name": filename,"template": name,"pose":poseName,"repeat":frame,"bytes":png.count,
                                  "bitmapFormat":bitmap.bitmapFormat.rawValue,"colorSpace":bitmap.colorSpaceName.rawValue])
                }
                await engine.close()
            }
        }
        try JSONSerialization.data(withJSONObject: ["images":files,"live":audit.live,"windowsCreated":0,"rendererSource":"unchanged gate2/Engine.swift"],options:[.prettyPrinted,.sortedKeys]).write(to:output.appendingPathComponent("manifest.json"))
    }
}
