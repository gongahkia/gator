// draw-on-demand RealityKit pilot, deliberately outside production targets
import AppKit
import MetalKit
import RealityKit
import QuartzCore

@MainActor final class MascotMetalSurface: NSView {
    let metalLayer: CAMetalLayer
    init(frame: NSRect, device: any MTLDevice) {
        metalLayer = CAMetalLayer()
        super.init(frame: frame)
        wantsLayer = true
        metalLayer.device = device
        metalLayer.pixelFormat = .bgra8Unorm_srgb
        metalLayer.isOpaque = false
        metalLayer.framebufferOnly = true
        metalLayer.drawableSize = CGSize(width: 224, height: 224)
        layer = metalLayer
        setAccessibilityElement(false)
    }
    required init?(coder: NSCoder) { nil }
}

@MainActor final class RealtimeRenderer {
    let rig: MascotRig
    let renderer: RealityRenderer
    let texture: any MTLTexture
    let output: RealityRenderer.CameraOutput
    let queue: any MTLCommandQueue
    let view: MascotMetalSurface
    private(set) var renders = 0
    private(set) var transitionMilliseconds: [Double] = []
    private(set) var loadMilliseconds: Double
    private(set) var surfaceFacts: [String: Int] = [:]
    private var closed = false
    private var rendering = false
    init(asset: URL, points: CGFloat = 112) async throws {
        let start = Date()
        let character = try await Entity(contentsOf: asset)
        try Task.checkCancellation()
        rig = try MascotRig(root: character)
        renderer = try RealityRenderer()
        renderer.entities.append(character)
        let camera = PerspectiveCamera()
        camera.camera.fieldOfViewInDegrees = 34
        camera.look(at: [0, 0.03, 0], from: [0, 0.08, 4.7], relativeTo: nil)
        renderer.entities.append(camera); renderer.activeCamera = camera
        let key = DirectionalLight(); key.light.intensity = 2400
        key.look(at: [0, 0, 0], from: [-3, 4, 5], relativeTo: nil)
        let fill = DirectionalLight(); fill.light.intensity = 1000
        fill.look(at: [0, 0, 0], from: [3, 1, 4], relativeTo: nil)
        renderer.entities.append(key); renderer.entities.append(fill)
        renderer.cameraSettings.colorBackground = .color(CGColor(gray: 0, alpha: 0))
        renderer.cameraSettings.antialiasing = .none
        guard let device = MTLCreateSystemDefaultDevice(), let commandQueue = device.makeCommandQueue() else {
            throw NSError(domain: "MascotMetalUnavailable", code: 1)
        }
        queue = commandQueue
        let descriptor = MTLTextureDescriptor.texture2DDescriptor(pixelFormat: .bgra8Unorm_srgb, width: 224, height: 224, mipmapped: false)
        descriptor.usage = [.renderTarget, .shaderRead]; descriptor.storageMode = .shared
        guard let madeTexture = device.makeTexture(descriptor: descriptor) else { throw NSError(domain: "MascotTextureUnavailable", code: 1) }
        texture = madeTexture
        output = try RealityRenderer.CameraOutput(.singleProjection(colorTexture: texture))
        view = MascotMetalSurface(frame: NSRect(x: 0, y: 0, width: points, height: points), device: device)
        loadMilliseconds = Date().timeIntervalSince(start) * 1000
    }
    func draw() async throws {
        guard !closed, !rendering else { return }
        rendering = true
        defer { rendering = false }
        let start = Date()
        try await withCheckedThrowingContinuation { (completion: CheckedContinuation<Void, any Error>) in
            do {
                try renderer.updateAndRender(deltaTime: 1 / 30, cameraOutput: output, onComplete: { _ in completion.resume() })
            } catch { completion.resume(throwing: error) }
        }
        guard !closed else { return }
        renders += 1
        transitionMilliseconds.append(Date().timeIntervalSince(start) * 1000)
        if transitionMilliseconds.count > 256 { transitionMilliseconds.removeFirst() }
        view.metalLayer.drawableSize = CGSize(width: 224, height: 224)
        guard let drawable = view.metalLayer.nextDrawable(), let buffer = queue.makeCommandBuffer(), let blit = buffer.makeBlitCommandEncoder() else { return }
        surfaceFacts = ["drawableWidth": drawable.texture.width, "drawableHeight": drawable.texture.height, "viewWidth": Int(view.bounds.width), "viewHeight": Int(view.bounds.height)]
        guard drawable.texture.width == 224 && drawable.texture.height == 224 else { throw NSError(domain: "MascotDrawableSizeMismatch", code: 1) }
        blit.copy(from: texture, sourceSlice: 0, sourceLevel: 0, sourceOrigin: MTLOrigin(x: 0, y: 0, z: 0), sourceSize: MTLSize(width: 224, height: 224, depth: 1), to: drawable.texture, destinationSlice: 0, destinationLevel: 0, destinationOrigin: MTLOrigin(x: 0, y: 0, z: 0))
        blit.endEncoding(); buffer.present(drawable); buffer.commit()
    }
    func transition(to pose: MascotPose, steps: Int = 6, onFrame: ((Int) -> Void)? = nil) async throws {
        let from = rig.pose
        for step in 1...max(1, steps) {
            try Task.checkCancellation()
            guard !closed else { return }
            rig.apply(from.interpolated(to: pose, fraction: Float(step) / Float(max(1, steps))))
            try await draw()
            onFrame?(step)
            if step < steps { try await Task.sleep(for: .milliseconds(33)) }
        }
    }
    func close() {
        closed = true
        renderer.entities.removeAll()
        view.metalLayer.device = nil
        view.removeFromSuperview()
    }
    func savePNG(_ url: URL) throws -> [String: Int] {
        var bytes = [UInt8](repeating: 0, count: 224 * 224 * 4)
        texture.getBytes(&bytes, bytesPerRow: 224 * 4, from: MTLRegionMake2D(0, 0, 224, 224), mipmapLevel: 0)
        for offset in stride(from: 0, to: bytes.count, by: 4) { bytes.swapAt(offset, offset + 2) }
        guard let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: 224, pixelsHigh: 224, bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 224 * 4, bitsPerPixel: 32), let destination = rep.bitmapData else { throw NSError(domain: "MascotPNGUnavailable", code: 1) }
        bytes.withUnsafeBytes { source in destination.update(from: source.bindMemory(to: UInt8.self).baseAddress!, count: bytes.count) }
        guard let data = rep.representation(using: .png, properties: [:]) else { throw NSError(domain: "MascotPNGUnavailable", code: 2) }
        try data.write(to: url)
        return ["transparent": stride(from: 3, to: bytes.count, by: 4).filter { bytes[$0] == 0 }.count, "visible": stride(from: 3, to: bytes.count, by: 4).filter { bytes[$0] > 0 }.count]
    }
}
