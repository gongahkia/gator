import AppKit
import CryptoKit
import Foundation
@main struct Validate {
    static func main()throws {
        let root=URL(fileURLWithPath:CommandLine.arguments[1])
        var results:[String:Any]=[:]
        for id in ["A","B","C"] {
            for s in [16,24,32,48,64,96] {
                let rep=NSBitmapImageRep(data:try Data(contentsOf:root.appendingPathComponent("\(id)/size-\(s).png")))!
                precondition(rep.pixelsWide==s && rep.pixelsHigh==s && rep.hasAlpha)
                precondition(rep.colorAt(x:0,y:0)!.alphaComponent==0)
            }
        }
        let frameURL=root.appendingPathComponent("prototype/frames")
        let urls=try FileManager.default.contentsOfDirectory(at:frameURL,includingPropertiesForKeys:nil).filter{$0.pathExtension=="png"}.sorted{$0.lastPathComponent<$1.lastPathComponent}
        precondition(urls.count==80)
        let unique=Set(try urls.map{SHA256.hash(data:try Data(contentsOf:$0)).description})
        precondition(unique.count>20)
        func bright(_ path:String)throws->Int {
            let rep=NSBitmapImageRep(data:try Data(contentsOf:root.appendingPathComponent(path)))!;var count=0
            for y in 0..<rep.pixelsHigh {for x in 0..<rep.pixelsWide {
                let c=rep.colorAt(x:x,y:y)!.usingColorSpace(.sRGB)!
                if c.alphaComponent>0.9 && min(c.redComponent,c.greenComponent,c.blueComponent)>0.75 {count+=1}
            }};return count
        }
        let open=try bright("prototype/frames/000.png"),closed=try bright("prototype/frames/013.png")
        precondition(closed < open/2)
        let fallback=try Data(contentsOf:root.appendingPathComponent("prototype/reduced-motion.png")),first=try Data(contentsOf:urls[0])
        precondition(fallback==first)
        let rep=NSBitmapImageRep(data:try Data(contentsOf:urls[34]))!
        var transparent=0,partial=0,opaque=0
        for y in 0..<rep.pixelsHigh {for x in 0..<rep.pixelsWide {let a=rep.colorAt(x:x,y:y)!.alphaComponent;if a==0 {transparent+=1}else if a<1{partial+=1}else{opaque+=1}}}
        precondition(transparent>0 && partial>0 && opaque>0)
        results=["native_small_sizes":"18 images have exact pixel dimensions and clear alpha corners","frame_count":urls.count,"distinct_animation_frames":unique.count,"bright_eye_pixels_neutral":open,"bright_eye_pixels_blink":closed,"reduced_motion_equals_neutral_frame":true,"sample_transparent_pixels":transparent,"sample_antialiased_alpha_pixels":partial,"sample_opaque_pixels":opaque]
        try JSONSerialization.data(withJSONObject:results,options:[.prettyPrinted,.sortedKeys]).write(to:root.appendingPathComponent("evidence/asset-validation.json"));print("Geometry animation, blink, neutral fallback and RGBA validation passed")
    }
}
