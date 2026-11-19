// headless RealityKit/offscreen lifecycle tests; no window or execution backend
import AppKit
import RealityKit
import Metal

@main @MainActor struct CheckLifecycle {
    static func main() async throws {
        let archive = try await NativeMeshArchive.read(URL(fileURLWithPath: CommandLine.arguments[1]))
        let template = try await archive.entity()
        guard let device = MTLCreateSystemDefaultDevice() else { throw NSError(domain: "NoMetalDevice", code: 1) }
        let audit = LifetimeProbe()
        var assertions = 0
        func check(_ condition: Bool, _ description: String) throws {
            guard condition else { throw NSError(domain: "HeadlessLifecycleAssertion", code: assertions, userInfo: [NSLocalizedDescriptionKey: description]) }
            assertions += 1
        }
        let image = NSImage(size: NSSize(width: 96, height: 96))
        var instant = ContinuousClock.now
        var systemReduced = false
        let presentation = Gate2Presentation(template: template, device: device, poster: image, audit: audit, now: { instant }, systemReduceMotion: { systemReduced }, showPoster: { _ in }, mount: { _,_,_ in {} })
        let small = EngineConfiguration(pixels: 96, points: 96, offscreen: true)
        let full = EngineConfiguration(pixels: 192, points: 104, offscreen: true)
        try await presentation.request(.working, configuration: small)
        try check(presentation.engine?.rig?.pose == MascotPose(look: 0.5, tilt: -0.055), "working actual transforms applied")
        let identity = presentation.engine
        try await presentation.request(.responding, slot: 1, configuration: full)
        try check(presentation.engine === identity && presentation.activeSlot == 1, "one engine reused across presentation handoff")
        try check(presentation.engine?.drawableDimensions == [192,192], "handoff target resized")
        try check(presentation.engine?.rig?.pose == MascotPose(), "responding returns jaw to neutral")
        let beforeThrottled = presentation.frames
        try await presentation.request(.responding, slot: 1, configuration: full)
        try check(presentation.frames == beforeThrottled, "rapid text deltas throttled with controlled clock")
        instant = instant.advanced(by: .seconds(1))
        await presentation.engine?.pause()
        try check(presentation.engine?.drawableBytes == 0, "pause releases owned render target")
        let pausedFrames = presentation.frames
        try await presentation.engine?.transition(to: MascotPose(open: 1), steps: 1)
        try check(presentation.frames == pausedFrames, "paused geometry updates do not draw")
        presentation.engine?.resume()
        try await presentation.engine?.draw()
        try check(presentation.frames == pausedFrames + 1, "resume renders again")
        var interrupted: Task<Void, any Error>?
        await withCheckedContinuation { (ready: CheckedContinuation<Void, Never>) in
            interrupted = Task {
                try await presentation.request(.responding, configuration: small) { step in if step == 3 { ready.resume() } }
            }
        }
        try await presentation.request(.hidden)
        if let interrupted { do { try await interrupted.value } catch is CancellationError {} }
        try check(presentation.engine == nil && !presentation.pendingWork && !presentation.hasWork, "hide cancels open-jaw animation and destroys engine")
        let hiddenFrames = presentation.frames
        try await presentation.request(.working)
        try await presentation.request(.responding)
        try check(presentation.frames == hiddenFrames && presentation.engine == nil, "state changes while hidden do not recreate renderer")
        presentation.setVisibleSlots([0,1])
        try await presentation.request(.attention, configuration: small)
        try check(presentation.engine != nil, "visible resume works")
        try await presentation.request(.reducedMotion)
        try check(presentation.engine == nil, "reduced motion keeps static surface only")
        try await presentation.request(.responding, configuration: small, motion: false)
        try check(presentation.engine == nil, "motion disabled suppresses live renderer")
        systemReduced = true
        try await presentation.request(.working, configuration: small)
        try check(presentation.engine == nil, "system Reduce Motion takes precedence")
        systemReduced = false
        for _ in 0..<12 {
            try await presentation.request(.blink, configuration: small)
            try check(presentation.engine == nil && !presentation.pendingWork, "bounded blink tears down renderer")
        }
        try await presentation.request(.working, configuration: full)
        var superseded: Task<Void, any Error>?
        await withCheckedContinuation { (ready: CheckedContinuation<Void, Never>) in
            superseded = Task {
                try await presentation.request(.working, configuration: small) { step in if step == 1 { ready.resume() } }
            }
        }
        try await presentation.request(.disconnected, configuration: small)
        if let superseded { do { try await superseded.value } catch is CancellationError {} }
        try check(presentation.engine?.rig?.pose == MascotPose(tilt: 0.035, concern: 0.85), "latest concurrent state wins: \(String(describing: presentation.engine?.rig?.pose))")
        try await presentation.request(.idle)
        try check(presentation.engine == nil, "idle releases renderer")
        try check(audit.live["RealityRenderer", default: 0] == 0 && audit.live["sceneRoot", default: 0] == 0, "no retained renderer/scene after idle")
        let burst = Gate2Presentation(template: template, device: device, poster: image, audit: audit, now: { instant }, systemReduceMotion: { false }, releaseBetweenEvents: true, showPoster: { _ in }, mount: { _,_,_ in {} })
        try await burst.request(.working, configuration: small)
        try check(burst.engine == nil && !burst.pendingWork, "working burst releases renderer")
        try await burst.request(.responding, configuration: small)
        try check(burst.engine == nil && !burst.pendingWork, "responding burst releases renderer")
        let burstFrames = burst.frames
        try await burst.request(.responding, configuration: small)
        try check(burst.frames == burstFrames, "burst throttle works after engine destruction")
        try await burst.request(.hidden)
        instant = instant.advanced(by: .seconds(1))
        try await burst.request(.attention, configuration: small)
        try check(burst.frames == burstFrames, "hidden burst updates do not draw")
        let rig = try MascotRig(root: template.clone(recursive: true))
        let rigResult = try rig.verify()
        let result: [String: Any] = ["assertions": assertions, "rig": rigResult, "live": audit.live, "created": audit.created,
                                   "closedWrapperIntentionallyRetainedByTest": identity != nil,
                                   "frames": presentation.frames, "windowsCreated": 0, "subscriptionsCreated": 0, "pendingTask": presentation.pendingWork]
        FileHandle.standardOutput.write(try JSONSerialization.data(withJSONObject: result, options: [.prettyPrinted, .sortedKeys]))
    }
}
