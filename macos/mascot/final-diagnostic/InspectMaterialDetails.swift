import AppKit
import RealityKit

@main @MainActor struct InspectMaterialDetails {
    static func main() async throws {
        let usd=try await Entity(contentsOf:URL(fileURLWithPath:CommandLine.arguments[1]))
        let native=try await NativeMeshArchive.read(URL(fileURLWithPath:CommandLine.arguments[2])).entity()
        func collect(_ root:Entity)->[[String:Any]]{
            var rows:[[String:Any]]=[]
            func visit(_ e:Entity,_ path:String){
                let path=path+"/"+e.name
                if let m=e.components[ModelComponent.self]?.materials.first as? PhysicallyBasedMaterial{
                    rows.append(["path":path,"faceCulling":String(describing:m.faceCulling),"blending":String(describing:m.blending),
                                 "opacityThreshold":m.opacityThreshold as Any? ?? NSNull(),"writesDepth":m.writesDepth,"readsDepth":m.readsDepth,
                                 "roughness":m.roughness.scale,"metallic":m.metallic.scale,"specular":m.specular.scale,
                                 "clearcoat":m.clearcoat.scale,"clearcoatRoughness":m.clearcoatRoughness.scale,
                                 "emissiveIntensity":m.emissiveIntensity,"anisotropyLevel":m.anisotropyLevel.scale,
                                 "anisotropyAngle":m.anisotropyAngle.scale,"sheen":String(describing:m.sheen),
                                 "program":String(describing:m.program),"triangleFillMode":String(describing:m.triangleFillMode),
                                 "baseColor":String(describing:m.baseColor),"emissiveColor":String(describing:m.emissiveColor),
                                 "normal":String(describing:m.normal),"ambientOcclusion":String(describing:m.ambientOcclusion),
                                 "textureTransform":String(describing:m.textureCoordinateTransform),
                                 "parameterSpecular":String(describing:m.__parameterBlock.get(parameter:PhysicallyBasedMaterial.Specular.__scaleKey)),
                                 "parameterRoughness":String(describing:m.__parameterBlock.get(parameter:PhysicallyBasedMaterial.Roughness.__scaleKey)),
                                 "parameterClearcoat":String(describing:m.__parameterBlock.get(parameter:PhysicallyBasedMaterial.Clearcoat.__scaleKey)),
                                 "parameterClearcoatRoughness":String(describing:m.__parameterBlock.get(parameter:PhysicallyBasedMaterial.ClearcoatRoughness.__scaleKey)),
                                 "parameterAnisotropy":String(describing:m.__parameterBlock.get(parameter:PhysicallyBasedMaterial.AnisotropyLevel.__scaleKey)),
                                 "cgBaseColorSpace":String(describing:m.baseColor.tint.cgColor.colorSpace?.name),
                                 "cgBaseColorComponents":m.baseColor.tint.cgColor.components ?? []])
                }
                for child in e.children{visit(child,path)}
            };visit(root,"");return rows
        }
        guard let source=usd.findEntity(named:"Gator_B") else{throw NSError(domain:"MissingRoot",code:1)}
        FileHandle.standardOutput.write(try JSONSerialization.data(withJSONObject:["USD":collect(source),"native":collect(native)],options:[.prettyPrinted,.sortedKeys]))
    }
}
