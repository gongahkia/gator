// Design-only procedural geometry. No GatorCore, bridge, credentials or live state.
import AppKit
import SceneKit
import Metal
import Foundation
import Darwin

func color(_ hex: UInt32) -> NSColor {
    NSColor(srgbRed: Double((hex >> 16) & 255)/255, green: Double((hex >> 8) & 255)/255, blue: Double(hex & 255)/255, alpha: 1)
}
struct Direction {
    let id, title: String
    let skin, snout, jaw: UInt32
    let head: SCNVector3
    let muzzle: SCNVector3
    let eyeX, eyeY, eyeWidth, eyeHeight: CGFloat
    let muzzleZ: CGFloat
    let ridgeCount: Int
}
let directions = [
    Direction(id:"A",title:"Round Gator",skin:0x4B946A,snout:0x75B084,jaw:0xB4CF9D,head:SCNVector3(1,1,0.86),muzzle:SCNVector3(0.79,0.26,0.39),eyeX:0.34,eyeY:0.49,eyeWidth:0.21,eyeHeight:0.26,muzzleZ:0.77,ridgeCount:2),
    Direction(id:"B",title:"Soft 3D Gator",skin:0x3E835F,snout:0x67A47B,jaw:0xADCC94,head:SCNVector3(1.03,0.94,0.84),muzzle:SCNVector3(0.88,0.29,0.49),eyeX:0.40,eyeY:0.51,eyeWidth:0.22,eyeHeight:0.26,muzzleZ:0.79,ridgeCount:3),
    Direction(id:"C",title:"Expressive Mini",skin:0x5EAE92,snout:0x83C2A7,jaw:0xC8DDB7,head:SCNVector3(0.99,1.02,0.85),muzzle:SCNVector3(0.74,0.24,0.36),eyeX:0.29,eyeY:0.47,eyeWidth:0.23,eyeHeight:0.31,muzzleZ:0.78,ridgeCount:1)
]
func material(_ hex: UInt32, roughness: CGFloat = 0.72, unlit: Bool = false) -> SCNMaterial {
    let m=SCNMaterial();m.diffuse.contents=color(hex);m.lightingModel=unlit ? .constant : .physicallyBased
    m.roughness.contents=roughness;m.metalness.contents=0;m.isDoubleSided=true
    return m
}
@discardableResult func ball(_ name:String,_ at:SCNVector3,_ size:SCNVector3,_ mat:SCNMaterial,_ parent:SCNNode, segments:Int=20)->SCNNode {
    let shape=SCNSphere(radius:1);shape.segmentCount=segments;shape.materials=[mat]
    let n=SCNNode(geometry:shape);n.name=name;n.position=at;n.scale=size;parent.addChildNode(n);return n
}
// Rigid upper/lower hemisphere shells rotate behind the sclera when open.
// Blink never rebuilds a mesh and is portable as named transform animation.
func lidGeometry(upper:Bool,mat:SCNMaterial)->SCNGeometry {
    let rows=8,cols=24
    var v:[SCNVector3]=[],normals:[SCNVector3]=[],indices:[Int32]=[]
    for row in 0...rows {
        let phi=(upper ? 0 : CGFloat.pi/2)+CGFloat(row)/CGFloat(rows)*CGFloat.pi/2
        for col in 0...cols {
            let a=CGFloat(col)/CGFloat(cols)*2*CGFloat.pi
            let n=SCNVector3(sin(phi)*cos(a),cos(phi),sin(phi)*sin(a));v.append(n);normals.append(n)
        }
    }
    for row in 0..<rows {for col in 0..<cols {
        let i=Int32(row*(cols+1)+col),j=i+Int32(cols+1)
        indices += [i,i+1,j,i+1,j+1,j]
    }}
    let g=SCNGeometry(sources:[SCNGeometrySource(vertices:v),SCNGeometrySource(normals:normals)],elements:[SCNGeometryElement(indices:indices,primitiveType:.triangles)])
    g.materials=[mat];return g
}
final class Character {
    let root=SCNNode(),head=SCNNode(),jaw=SCNNode()
    let spec:Direction
    var upper:[SCNNode]=[],lower:[SCNNode]=[],pupils:[SCNNode]=[],closedLines:[SCNNode]=[]
    var cavity:SCNNode!,tongue:SCNNode!
    init(_ d:Direction) {
        spec=d;root.name="Gator_\(d.id)";head.name="Head_Pivot";root.addChildNode(head)
        let skin=material(d.skin),snout=material(d.snout),lowerSkin=material(d.jaw),ink=material(0x173C2F,unlit:true),white=material(0xFAF9EB,roughness:0.35)
        ball("Round_Head",SCNVector3(0,0.03,0),d.head,skin,head,segments:36)
        // The short broad muzzle, low nostrils and two tiny rounded teeth carry
        // alligator identity without long anatomy or a body.
        if d.id == "B" {
            let shape=SCNBox(width:d.muzzle.x*2,height:d.muzzle.y*2,length:d.muzzle.z*2,chamferRadius:0.24)
            shape.chamferSegmentCount=8;shape.materials=[snout]
            let n=SCNNode(geometry:shape);n.name="Upper_Snout";n.position=SCNVector3(0,-0.18,d.muzzleZ);head.addChildNode(n)
        } else {ball("Upper_Snout",SCNVector3(0,-0.18,d.muzzleZ),d.muzzle,snout,head)}
        cavity=ball("Mouth_Interior",SCNVector3(0,-0.37,1.14),SCNVector3(d.muzzle.x*(d.id == "B" ? 0.84:0.65),0.025,0.045),ink,head)
        jaw.name="Jaw_Pivot";jaw.position=SCNVector3(0,-0.30,0.24);head.addChildNode(jaw)
        ball("Lower_Jaw",SCNVector3(0,-0.15,0.51),SCNVector3(d.muzzle.x*0.94,0.18,d.muzzle.z*0.92),lowerSkin,jaw)
        tongue=ball("Tongue",SCNVector3(0,-0.44,1.12),SCNVector3(0.19,0.025,0.045),material(0xB87473),head);tongue.isHidden=true
        for sign:CGFloat in [-1,1] {
            let x=sign*0.23
            ball(sign<0 ? "Nostril_Left":"Nostril_Right",SCNVector3(x,-0.085,d.muzzleZ+d.muzzle.z*(d.id == "B" ? 1.04:0.95)),SCNVector3(0.039,0.023,0.018),ink,head,segments:12)
            ball(sign<0 ? "Tooth_Left":"Tooth_Right",SCNVector3(sign*d.muzzle.x*0.66,d.id == "B" ? -0.46:-0.395,d.id == "B" ? 1.29:1.075),SCNVector3(0.034,0.048,0.029),white,head,segments:12)
            let eye=SCNNode();eye.name=sign<0 ? "Eye_Left":"Eye_Right";eye.position=SCNVector3(sign*d.eyeX,d.eyeY,0.69);head.addChildNode(eye)
            if d.id=="B" {ball("Eye_Cushion",SCNVector3(0,0.035,-0.055),SCNVector3(d.eyeWidth*1.2,d.eyeHeight*1.1,0.16),skin,eye)}
            ball("Sclera",SCNVector3Zero,SCNVector3(d.eyeWidth,d.eyeHeight,0.15),white,eye,segments:20)
            let pupil=ball("Pupil",SCNVector3(0,-0.015,0.151),SCNVector3(d.id=="C" ? 0.10:0.085,d.id=="C" ? 0.16:0.12,0.035),material(0x153B31,roughness:0.28),eye,segments:20)
            ball("Catchlight",SCNVector3(-0.025,0.035,0.03),SCNVector3(0.022,0.026,0.008),material(0xFFFFFF,unlit:true),pupil,segments:8)
            // Counteract pupil scaling for the catchlight's small world size.
            pupil.childNodes.first?.scale=SCNVector3(0.20,0.18,0.25)
            pupils.append(pupil)
            let seam=ball("Closed_Eye_Line",SCNVector3(0,0,0.309),SCNVector3(d.eyeWidth*0.75,0.008,0.008),ink,eye,segments:8)
            closedLines.append(seam)
            for isUpper in [true,false] {
                let lid=SCNNode(geometry:lidGeometry(upper:isUpper,mat:skin));lid.name=isUpper ? "Upper_Lid":"Lower_Lid";lid.scale=SCNVector3(d.eyeWidth*1.035,d.eyeHeight*1.035,0.30);eye.addChildNode(lid)
                if isUpper {upper.append(lid)} else {lower.append(lid)}
            }
        }
        for index in 0..<d.ridgeCount {
            let z=CGFloat(index)*0.21-0.34
            ball("Crown_Ridge_\(index)",SCNVector3(0,0.94,z),SCNVector3(d.id=="C" ? 0.16:0.13,0.13,0.15),skin,head,segments:12)
        }
        pose(open:0,blink:0)
    }
    func pose(open:CGFloat,blink:CGFloat,look:CGFloat=0,tilt:CGFloat=0,concern:CGFloat=0) {
        let o=max(0,min(1,open)),b=max(0,min(1,blink))
        jaw.eulerAngles.x=o*0.50
        cavity.scale.y=0.022+o*0.21;cavity.position.y = -0.36-o*0.075
        tongue.isHidden=o<0.35;tongue.position.y = -0.43-o*0.065
        for (i,lid) in upper.enumerated() {lid.eulerAngles.x = -(1-b)*(CGFloat.pi/2-(spec.id == "C" ? (i==0 ? 0.72:0.50):0.04)) + concern*(i==0 ? 0.14:0.04)}
        for lid in lower {lid.eulerAngles.x = (1-b)*(CGFloat.pi/2-0.04)}
        for p in pupils {p.position.x=look*0.035}
        for line in closedLines {line.opacity=pow(b,8)}
        head.eulerAngles.z=tilt
    }
}
final class Stage {
    let scene=SCNScene(),camera=SCNNode()
    let renderer=SCNRenderer(device:MTLCreateSystemDefaultDevice(),options:nil)
    let character:Character
    init(_ d:Direction) {
        character=Character(d);scene.rootNode.addChildNode(character.root)
        scene.background.contents=NSColor.clear
        camera.camera=SCNCamera();camera.camera?.usesOrthographicProjection=true;camera.camera?.orthographicScale=1.42;camera.camera?.zNear=0.01;camera.camera?.zFar=30
        camera.camera?.wantsHDR=true;camera.camera?.exposureOffset = -0.12
        camera.position=SCNVector3(0,0.05,6);scene.rootNode.addChildNode(camera)
        func light(_ name:String,_ type:SCNLight.LightType,_ power:CGFloat,_ pos:SCNVector3) {
            let n=SCNNode();n.name=name;n.light=SCNLight();n.light?.type=type;n.light?.intensity=power;n.position=pos
            if type == .directional {n.look(at:SCNVector3(0,0,0))};scene.rootNode.addChildNode(n)
        }
        light("Key",.directional,1100,SCNVector3(-3,4,5));light("Fill",.directional,380,SCNVector3(3,1,4));light("Ambient",.ambient,240,SCNVector3Zero)
        renderer.scene=scene;renderer.pointOfView=camera
    }
    func image(quarter:Bool=false,size:Int=512)->NSImage {
        character.root.eulerAngles.y=quarter ? -0.48:0
        return renderer.snapshot(atTime:0,with:CGSize(width:size,height:size),antialiasingMode:.multisampling4X)
    }
}
func writePNG(_ image:NSImage,_ path:URL)throws {
    guard let tiff=image.tiffRepresentation,let b=NSBitmapImageRep(data:tiff),let data=b.representation(using:.png,properties:[:]) else {throw NSError(domain:"MascotLab",code:1)}
    try data.write(to:path)
}
func numbers(_ src:SCNGeometrySource)->[SCNVector3] {
    (0..<src.vectorCount).map { i in
        let start=src.dataOffset+i*src.dataStride
        return src.data.withUnsafeBytes { data in SCNVector3(data.loadUnaligned(fromByteOffset:start,as:Float.self),data.loadUnaligned(fromByteOffset:start+4,as:Float.self),data.loadUnaligned(fromByteOffset:start+8,as:Float.self)) }
    }
}
func triangleIndices(_ e:SCNGeometryElement)->[Int] {
    let count=e.data.count/e.bytesPerIndex
    let values=(0..<count).map { i in e.data.withUnsafeBytes { p -> Int in
        switch e.bytesPerIndex {case 1:return Int(p.loadUnaligned(fromByteOffset:i,as:UInt8.self));case 2:return Int(p.loadUnaligned(fromByteOffset:i*2,as:UInt16.self));default:return Int(p.loadUnaligned(fromByteOffset:i*4,as:UInt32.self))}
    }}
    if e.primitiveType == .triangles {return values}
    if e.primitiveType == .triangleStrip {var out:[Int]=[];for i in 0..<max(0,values.count-2) {let a=values[i],b=values[i+1],c=values[i+2];if a != b && b != c && a != c {out += i%2==0 ? [a,b,c]:[b,a,c]}};return out}
    return []
}
func vector(_ v:SCNVector3)->String {String(format:"(%.6f, %.6f, %.6f)",v.x,v.y,v.z)}
func exportUSD(_ character:Character,_ url:URL)throws->[String:Int] {
    var lines=["#usda 1.0","(","    defaultPrim = \"Gator_\(character.spec.id)\"","    upAxis = \"Y\"","    metersPerUnit = 1", ")"]
    var meshes=0,triangles=0,vertices=0
    func emit(_ node:SCNNode,_ depth:Int) {
        let p=String(repeating:"    ",count:depth),name=node.name ?? "Part"
        lines += ["\(p)def Xform \"\(name)\" {","\(p)    double3 xformOp:translate = \(vector(node.position))","\(p)    float3 xformOp:rotateXYZ = \(vector(SCNVector3(node.eulerAngles.x*180/CGFloat.pi,node.eulerAngles.y*180/CGFloat.pi,node.eulerAngles.z*180/CGFloat.pi)))","\(p)    float3 xformOp:scale = \(vector(node.scale))","\(p)    uniform token[] xformOpOrder = [\"xformOp:translate\", \"xformOp:rotateXYZ\", \"xformOp:scale\"]"]
        if node.isHidden || node.opacity == 0 {lines.append("\(p)    token visibility = \"invisible\"")}
        if let geometry=node.geometry,let source=geometry.sources(for:.vertex).first {
            let points=numbers(source),normals=geometry.sources(for:.normal).first.map(numbers) ?? []
            let indices=geometry.elements.flatMap(triangleIndices);meshes+=1;vertices+=points.count;triangles+=indices.count/3
            let diffuse=(geometry.firstMaterial?.diffuse.contents as? NSColor)?.usingColorSpace(.sRGB) ?? .green
            let finish=geometry.firstMaterial
            let roughness=(finish?.roughness.contents as? NSNumber)?.doubleValue ?? 0.72
            let unlit=finish?.lightingModel == .constant
            func linear(_ x:CGFloat)->CGFloat {x <= 0.04045 ? x/12.92:pow((x+0.055)/1.055,2.4)}
            let rgb="(\(linear(diffuse.redComponent)), \(linear(diffuse.greenComponent)), \(linear(diffuse.blueComponent)))"
            let baseColor=unlit ? "(0, 0, 0)":rgb
            let emission=unlit ? rgb:"(0, 0, 0)"
            lines += ["\(p)    def Mesh \"Surface\" (prepend apiSchemas = [\"MaterialBindingAPI\"]) {","\(p)        uniform token subdivisionScheme = \"none\"","\(p)        bool doubleSided = true","\(p)        point3f[] points = [\(points.map(vector).joined(separator:", "))]","\(p)        int[] faceVertexCounts = [\(Array(repeating:"3",count:indices.count/3).joined(separator:", "))]","\(p)        int[] faceVertexIndices = [\(indices.map(String.init).joined(separator:", "))]","\(p)        normal3f[] normals = [\(normals.map(vector).joined(separator:", "))] (interpolation = \"vertex\")","\(p)        color3f[] primvars:displayColor = [\(rgb)]","\(p)        rel material:binding = <\(nodePath(node))/Surface/Finish>","\(p)        def Material \"Finish\" {","\(p)            token outputs:surface.connect = <\(nodePath(node))/Surface/Finish/Shader.outputs:surface>","\(p)            def Shader \"Shader\" {","\(p)                uniform token info:id = \"UsdPreviewSurface\"","\(p)                color3f inputs:diffuseColor = \(baseColor)","\(p)                color3f inputs:emissiveColor = \(emission)","\(p)                float inputs:roughness = \(roughness)","\(p)                float inputs:metallic = 0","\(p)                token outputs:surface","\(p)            }","\(p)        }","\(p)    }"]
        }
        for child in node.childNodes {emit(child,depth+1)};lines += ["\(p)}"]
    }
    emit(character.root,0);try lines.joined(separator:"\n").write(to:url,atomically:true,encoding:.utf8)
    return ["meshes":meshes,"triangles":triangles,"vertices":vertices]
}
func nodePath(_ node:SCNNode)->String {
    var parts:[String]=[];var n:SCNNode?=node
    while let current=n,current.name != nil {parts.insert(current.name!,at:0);n=current.parent}
    return "/"+parts.joined(separator:"/")
}
#if !MASCOT_PREVIEW && !MASCOT_BENCHMARK
@main struct Lab {
    static func main() throws {
        let output=URL(fileURLWithPath:CommandLine.arguments.count>1 ? CommandLine.arguments[1]:"macos/mascot/concepts",isDirectory:true)
        var stats:[String:Any]=[:]
        for d in directions {
            let stage=Stage(d),dir=output.appendingPathComponent(d.id)
            try FileManager.default.createDirectory(at:dir,withIntermediateDirectories:true)
            for (name,o,b,tilt,concern):(String,CGFloat,CGFloat,CGFloat,CGFloat) in [("neutral",0,0,0,0),("open",0.85,0,0,0),("blink",0,1,0,0),("attention",0.12,0,-0.055,-0.10),("concern",0,0,0.05,0.7)] {
                stage.character.pose(open:o,blink:b,tilt:tilt,concern:concern)
                try writePNG(stage.image(),dir.appendingPathComponent("front-\(name).png"))
                if name=="neutral" || name=="open" {try writePNG(stage.image(quarter:true),dir.appendingPathComponent("quarter-\(name).png"))}
            }
            stage.character.pose(open:0,blink:0);stage.character.root.eulerAngles.y=0
            for pixels in [16,24,32,48,64,96] {try writePNG(stage.image(size:pixels),dir.appendingPathComponent("size-\(pixels).png"))}
            let counts=try exportUSD(stage.character,output.appendingPathComponent("models/Gator-\(d.id).usda"));stats[d.id]=counts
            _ = stage.scene.write(to:output.appendingPathComponent("models/Gator-\(d.id).scn"),options:nil,delegate:nil,progressHandler:nil)
        }
        // One deterministic 4-second geometry animation, with a separate static
        // reduced-motion frame. These PNGs/GIF are previews, never the model.
        let stage=Stage(directions[1]),frames=output.appendingPathComponent("prototype/frames")
        try FileManager.default.createDirectory(at:frames,withIntermediateDirectories:true)
        func smooth(_ x:CGFloat)->CGFloat {let t=max(0,min(1,x));return t*t*(3-2*t)}
        let start=ContinuousClock.now
        for i in 0..<80 {
            let t=CGFloat(i)/20
            let blink=max(0,1-abs(t-0.65)/0.12)
            let open=0.78*(smooth((t-1.2)/0.5)-smooth((t-2.25)/0.45))
            let attention=smooth((t-2.7)/0.35)-smooth((t-3.55)/0.35)
            stage.character.pose(open:open,blink:blink,look:attention,tilt:attention * -0.06,concern:attention * -0.1)
            try writePNG(stage.image(quarter:true,size:256),frames.appendingPathComponent(String(format:"%03d.png",i)))
        }
        // The reduced-motion poster is exactly the cached neutral render,
        // rather than a later independently rendered approximation of it.
        try Data(contentsOf:frames.appendingPathComponent("000.png")).write(to:output.appendingPathComponent("prototype/reduced-motion.png"))
        stats["offline_preview_seconds"]=String(describing:start.duration(to:.now));stats["preview_frames"]=80
        try JSONSerialization.data(withJSONObject:stats,options:[.prettyPrinted,.sortedKeys]).write(to:output.appendingPathComponent("evidence/geometry.json"))
        print("Generated A/B/C geometry and 80 B prototype frames")
    }
}

#endif
