// Isolated compatibility/import probe. No application integration.
import Foundation
import RealityKit
import SwiftUI
import Metal
import AppKit

@available(macOS 15.0, *)
struct CompatibilityView: View {
    let url: URL
    var body: some View {
        RealityView { content in
            if let character = try? await Entity(contentsOf: url) { content.add(character) }
        }.allowsHitTesting(false).accessibilityHidden(true)
    }
}

@main @MainActor struct RealityKitProbe {
    static func main() async throws {
        let root = URL(fileURLWithPath: CommandLine.arguments[1])
        for id in ["A", "B", "C"] {
            let start=ContinuousClock.now
            let entity=try await Entity(contentsOf:root.appendingPathComponent("Gator-\(id).usdz"))
            let loadDuration=start.duration(to:.now)
            let names=["Jaw_Pivot","Eye_Left","Eye_Right","Upper_Lid","Lower_Lid","Pupil"]
            guard names.allSatisfy({entity.findEntity(named:$0) != nil}) else {throw NSError(domain:"MissingNamedPart",code:1)}
            for eye in ["Eye_Left","Eye_Right"] {
                guard let parent=entity.findEntity(named:eye),["Upper_Lid","Lower_Lid","Pupil"].allSatisfy({parent.findEntity(named:$0) != nil}) else {throw NSError(domain:"EyeHierarchyMissing",code:2)}
            }
            if id == "B" {try await render(character:entity,to:root.deletingLastPathComponent().appendingPathComponent("prototype/realitykit-smoke.png"))}
            print("RealityKit imported \(id): independent jaw/eyes/lids/pupils present; load=\(loadDuration)")
        }
    }
    static func render(character:Entity,to url:URL) async throws {
        let renderer=try RealityRenderer()
        renderer.entities.append(character)
        let camera=PerspectiveCamera();camera.camera.fieldOfViewInDegrees=34
        camera.look(at:[0,0.03,0],from:[0,0.08,4.7],relativeTo:nil)
        renderer.entities.append(camera);renderer.activeCamera=camera
        let key=DirectionalLight();key.light.intensity=2400
        key.look(at:[0,0,0],from:[-3,4,5],relativeTo:nil);renderer.entities.append(key)
        let fill=DirectionalLight();fill.light.intensity=1000
        fill.look(at:[0,0,0],from:[3,1,4],relativeTo:nil);renderer.entities.append(fill)
        renderer.cameraSettings.colorBackground = .color(CGColor(gray:0,alpha:0))
        guard let device=MTLCreateSystemDefaultDevice() else {throw NSError(domain:"MetalUnavailable",code:1)}
        let desc=MTLTextureDescriptor.texture2DDescriptor(pixelFormat:.rgba8Unorm_srgb,width:256,height:256,mipmapped:false)
        desc.usage=[.renderTarget,.shaderRead];desc.storageMode = .shared
        guard let texture=device.makeTexture(descriptor:desc) else {throw NSError(domain:"TextureUnavailable",code:1)}
        let output=try RealityRenderer.CameraOutput(.singleProjection(colorTexture:texture))
        try await withCheckedThrowingContinuation { (completion:CheckedContinuation<Void,any Error>) in
            do {try renderer.updateAndRender(deltaTime:1/30,cameraOutput:output,onComplete:{_ in completion.resume()})}
            catch {completion.resume(throwing:error)}
        }
        var bytes=[UInt8](repeating:0,count:256*256*4)
        texture.getBytes(&bytes,bytesPerRow:256*4,from:MTLRegionMake2D(0,0,256,256),mipmapLevel:0)
        guard let rep=NSBitmapImageRep(bitmapDataPlanes:nil,pixelsWide:256,pixelsHigh:256,bitsPerSample:8,samplesPerPixel:4,hasAlpha:true,isPlanar:false,colorSpaceName:.deviceRGB,bytesPerRow:256*4,bitsPerPixel:32),let destination=rep.bitmapData else {throw NSError(domain:"PNGUnavailable",code:1)}
        bytes.withUnsafeBytes {source in destination.update(from:source.bindMemory(to:UInt8.self).baseAddress!,count:bytes.count)}
        guard let data=rep.representation(using:.png,properties:[:]) else {throw NSError(domain:"PNGUnavailable",code:2)}
        try data.write(to:url)
        let transparent=stride(from:3,to:bytes.count,by:4).filter{bytes[$0]==0}.count
        let visible=stride(from:3,to:bytes.count,by:4).filter{bytes[$0]>0}.count
        guard transparent>0,visible>0 else {throw NSError(domain:"AlphaSmokeFailed",code:1)}
        print("RealityRenderer macOS-15-target single-frame RGBA render: visible=\(visible) clear=\(transparent)")
    }

}
