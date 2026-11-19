import AppKit
import RealityKit
import MetalKit
import QuartzCore

struct EngineConfiguration: Equatable, Codable {
    var pixels = 96
    var points: Double = 96
    var msaa = false
    var metalKit = false
    var offscreen = false
}

@MainActor final class DirectSurface: NSView {
    let metalLayer = CAMetalLayer()
    init(configuration: EngineConfiguration, device: any MTLDevice) {
        super.init(frame: NSRect(x: 0, y: 0, width: configuration.points, height: configuration.points))
        wantsLayer = true; layer = metalLayer
        metalLayer.device = device; metalLayer.pixelFormat = .bgra8Unorm_srgb
        metalLayer.framebufferOnly = true; metalLayer.isOpaque = false
        metalLayer.maximumDrawableCount = 2
        metalLayer.drawableSize = CGSize(width: configuration.pixels, height: configuration.pixels)
        setAccessibilityElement(false)
    }
    required init?(coder: NSCoder) { nil }
}

@MainActor final class ManualMetalKitSurface: NSObject, MTKViewDelegate {
    let view: MTKView
    var receive: ((any CAMetalDrawable) -> Void)?
    init(configuration: EngineConfiguration, device: any MTLDevice) {
        view = MTKView(frame: NSRect(x: 0, y: 0, width: configuration.points, height: configuration.points), device: device)
        super.init()
        view.isPaused = true; view.enableSetNeedsDisplay = false
        view.autoResizeDrawable = false
        view.drawableSize = CGSize(width: configuration.pixels, height: configuration.pixels)
        view.colorPixelFormat = .bgra8Unorm_srgb; view.framebufferOnly = true
        view.sampleCount = 1; view.depthStencilPixelFormat = .invalid
        view.layer?.isOpaque = false
        view.clearColor = MTLClearColorMake(0, 0, 0, 0)
        view.delegate = self; view.setAccessibilityElement(false)
    }
    func mtkView(_ view: MTKView, drawableSizeWillChange size: CGSize) {}
    func draw(in view: MTKView) {
        if let drawable = view.currentDrawable { receive?(drawable) }
    }
    func drawable() -> (any CAMetalDrawable)? {
        var result: (any CAMetalDrawable)?
        receive = { result = $0 }
        view.draw(); receive = nil
        return result
    }
    func release() { receive = nil; view.delegate = nil; view.releaseDrawables(); view.device = nil }
}

@MainActor final class Gate2Engine {
    private(set) var rig: MascotRig?
    private(set) var renderer: RealityRenderer?
    private(set) var surface: DirectSurface?
    private(set) var metalKitSurface: ManualMetalKitSurface?
    private(set) var configuration: EngineConfiguration
    private(set) var renders = 0
    private(set) var durations: [Double] = []
    private(set) var drawableBytes = 0
    private(set) var drawableDimensions: [Int] = []
    private var testTarget: (any MTLTexture)?
    private var inFlight = false
    private var stopping = false
    private var epoch = 0
    private var waiters: [CheckedContinuation<Void, Never>] = []
    let device: any MTLDevice
    let audit: LifetimeProbe
    var view: NSView? { surface ?? metalKitSurface?.view }
    init(template: Entity, device: any MTLDevice, configuration: EngineConfiguration, audit: LifetimeProbe) throws {
        self.device = device; self.configuration = configuration; self.audit = audit
        let character = template.clone(recursive: true)
        rig = try MascotRig(root: character)
        renderer = try RealityRenderer()
        guard let renderer else { throw NSError(domain: "MissingRenderer", code: 1) }
        renderer.entities.append(character)
        let camera = PerspectiveCamera(); camera.camera.fieldOfViewInDegrees = 34
        camera.look(at: [0, 0.03, 0], from: [0, 0.08, 4.7], relativeTo: nil)
        renderer.entities.append(camera); renderer.activeCamera = camera
        let key = DirectionalLight(); key.light.intensity = 2400
        key.look(at: [0, 0, 0], from: [-3, 4, 5], relativeTo: nil)
        let fill = DirectionalLight(); fill.light.intensity = 1000
        fill.look(at: [0, 0, 0], from: [3, 1, 4], relativeTo: nil)
        renderer.entities.append(key); renderer.entities.append(fill)
        renderer.cameraSettings.colorBackground = .color(CGColor(gray: 0, alpha: 0))
        renderer.cameraSettings.antialiasing = configuration.msaa ? .multisample4X : .none
        audit.track(self, kind: "engine"); audit.track(renderer, kind: "RealityRenderer")
        audit.track(character, kind: "sceneRoot")
        installSurface()
    }
    private func installSurface() {
        if configuration.offscreen {
            let descriptor = MTLTextureDescriptor.texture2DDescriptor(pixelFormat: .bgra8Unorm_srgb, width: configuration.pixels, height: configuration.pixels, mipmapped: false)
            descriptor.storageMode = .shared; descriptor.usage = [.renderTarget, .shaderRead]
            testTarget = device.makeTexture(descriptor: descriptor)
            if let testTarget { audit.track(testTarget as AnyObject, kind: "headlessTarget") }
        } else if configuration.metalKit {
            metalKitSurface = ManualMetalKitSurface(configuration: configuration, device: device)
            if let view = metalKitSurface?.view { audit.track(view, kind: "surface") }
        } else {
            surface = DirectSurface(configuration: configuration, device: device)
            if let surface { audit.track(surface, kind: "surface"); audit.track(surface.metalLayer, kind: "layer") }
        }
    }
    private func releaseSurface() {
        surface?.removeFromSuperview(); surface?.metalLayer.device = nil; surface?.layer = nil; surface = nil
        metalKitSurface?.view.removeFromSuperview(); metalKitSurface?.release(); metalKitSurface = nil
        drawableBytes = 0; drawableDimensions = []
        testTarget = nil
    }
    func resize(_ configuration: EngineConfiguration) async throws {
        epoch += 1
        await drained()
        guard !stopping else { return }
        releaseSurface(); self.configuration = configuration; installSurface()
        renderer?.cameraSettings.antialiasing = configuration.msaa ? .multisample4X : .none
    }
    func pause() async {
        epoch += 1
        await drained(); releaseSurface()
        rig?.apply(MascotPose())
    }
    func resume() {
        guard !stopping, view == nil, testTarget == nil else { return }
        installSurface()
    }
    private func drained() async {
        if inFlight { await withCheckedContinuation { waiters.append($0) } }
    }
    func stopTransition() { epoch += 1 }
    private func render(_ texture: any MTLTexture) async throws {
        guard !stopping, !inFlight, let renderer else { throw NSError(domain: "RendererUnavailableOrBusy", code: 1) }
        inFlight = true
        defer {
            inFlight = false
            let pending = waiters; waiters.removeAll()
            for continuation in pending { continuation.resume() }
        }
        let output = try RealityRenderer.CameraOutput(.singleProjection(colorTexture: texture))
        let start = ContinuousClock.now
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, any Error>) in
            do { try renderer.updateAndRender(deltaTime: 1 / 30, cameraOutput: output, onComplete: { _ in continuation.resume() }) }
            catch { continuation.resume(throwing: error) }
        }
        let duration = start.duration(to: .now)
        durations.append(Double(duration.components.seconds) * 1000 + Double(duration.components.attoseconds) / 1e15)
        if durations.count > 256 { durations.removeFirst() }
        renders += 1
    }
    func draw() async throws {
        if let testTarget, !stopping {
            drawableBytes = testTarget.allocatedSize; drawableDimensions = [testTarget.width, testTarget.height]
            try await render(testTarget); return
        }
        guard !stopping, view != nil else { return }
        let operation = epoch
        let drawable = surface?.metalLayer.nextDrawable() ?? metalKitSurface?.drawable()
        guard let drawable else { throw NSError(domain: "DrawableUnavailable", code: 1) }
        drawableBytes = drawable.texture.allocatedSize
        drawableDimensions = [drawable.texture.width, drawable.texture.height]
        try await render(drawable.texture)
        if !stopping && operation == epoch { drawable.present() }
    }
    func transition(to pose: MascotPose, steps: Int = 6, onFrame: ((Int) -> Void)? = nil) async throws {
        epoch += 1; let operation = epoch
        await drained()
        guard !stopping, let from = rig?.pose, view != nil || testTarget != nil else { return }
        for step in 1...max(1, steps) {
            try Task.checkCancellation()
            guard !stopping, epoch == operation, view != nil || testTarget != nil else { return }
            rig?.apply(step == max(1, steps) ? pose : from.interpolated(to: pose, fraction: Float(step) / Float(max(1, steps))))
            try await draw(); onFrame?(step)
            if step < steps { try await Task.sleep(for: .milliseconds(33)) }
        }
    }
    func poster() async throws -> NSImage {
        await drained()
        let pixels = configuration.pixels
        let descriptor = MTLTextureDescriptor.texture2DDescriptor(pixelFormat: .bgra8Unorm_srgb, width: pixels, height: pixels, mipmapped: false)
        descriptor.storageMode = .shared; descriptor.usage = [.renderTarget, .shaderRead]
        guard let texture = device.makeTexture(descriptor: descriptor) else { throw NSError(domain: "PosterTextureUnavailable", code: 1) }
        audit.track(texture as AnyObject, kind: "posterTexture")
        try await render(texture)
        var bytes = [UInt8](repeating: 0, count: pixels * pixels * 4)
        texture.getBytes(&bytes, bytesPerRow: pixels * 4, from: MTLRegionMake2D(0, 0, pixels, pixels), mipmapLevel: 0)
        for i in stride(from: 0, to: bytes.count, by: 4) { bytes.swapAt(i, i + 2) }
        guard let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: pixels, pixelsHigh: pixels, bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: pixels * 4, bitsPerPixel: 32), let destination = rep.bitmapData else { throw NSError(domain: "PosterBitmapUnavailable", code: 1) }
        bytes.withUnsafeBytes { destination.update(from: $0.bindMemory(to: UInt8.self).baseAddress!, count: bytes.count) }
        let image = NSImage(size: NSSize(width: pixels, height: pixels)); image.addRepresentation(rep)
        return image
    }
    func close() async {
        stopping = true; epoch += 1
        await drained()
        renderer?.entities.removeAll(); renderer?.activeCamera = nil
        rig = nil; renderer = nil; releaseSurface()
    }
}
