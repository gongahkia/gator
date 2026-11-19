import AppKit
import RealityKit

@main @MainActor struct CheckMaterialParity {
    static func main() async throws {
        let source=try await Entity(contentsOf:URL(fileURLWithPath:CommandLine.arguments[1]))
        let archive=try await NativeMeshArchive.read(URL(fileURLWithPath:CommandLine.arguments[2]))
        let native=try await archive.entity()
        guard let original=source.findEntity(named:"Gator_B") else{throw NSError(domain:"MissingOriginal",code:1)}
        var count=0;var failures:[String]=[]
        func visit(_ a:Entity,_ b:Entity,_ path:String){
            let path=path+"/"+a.name
            if let left=a.components[ModelComponent.self]?.materials.first as? PhysicallyBasedMaterial,
               let right=b.components[ModelComponent.self]?.materials.first as? PhysicallyBasedMaterial{
                count+=1
                if left.faceCulling != right.faceCulling{failures.append(path+": imported \(left.faceCulling), native \(right.faceCulling)")}
            }
            for (left,right) in zip(a.children,b.children){visit(left,right,path)}
        }
        visit(original,native,"")
        if count != 26{failures.append("Expected 26 material pairs, found \(count)")}
        FileHandle.standardOutput.write(try JSONSerialization.data(withJSONObject:["pairs":count,"failures":failures,"passed":failures.isEmpty],options:[.sortedKeys,.prettyPrinted]))
        if !failures.isEmpty{exit(1)}
    }
}
