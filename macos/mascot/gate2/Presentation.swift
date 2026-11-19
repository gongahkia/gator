import AppKit
import RealityKit
import Metal

enum PresentationEvent { case idle, blink, working, responding, attention, disconnected, hidden, reducedMotion }

// cosmetic resource ownership only; there is no GatorCore or execution path
@MainActor final class Gate2Presentation {
    private(set) var engine: Gate2Engine?
    private(set) var poster: NSImage
    private(set) var framesRetired = 0
    private(set) var generation = 0
    private(set) var activeSlot: Int?
    private(set) var hasWork = false
    private(set) var visibleSlots: Set<Int> = [0, 1]
    private(set) var resumeMilliseconds: [Double] = []
    private var work: Task<Void, any Error>?
    private var lastResponding: ContinuousClock.Instant?
    private var lastRespondingSlot: Int?
    private var lastRespondingConfiguration: EngineConfiguration?
    private var neutralPoster: NSImage
    private let releaseBetweenEvents: Bool
    private let template: Entity
    private let device: any MTLDevice
    private let audit: LifetimeProbe
    private let now: () -> ContinuousClock.Instant
    private let systemReduceMotion: () -> Bool
    private let showPoster: (NSImage) -> Void
    private let mount: (NSView, Int, Double) -> (() -> Void)
    var frames: Int { framesRetired + (engine?.renders ?? 0) }
    var pendingWork: Bool { work != nil }
    func setVisibleSlots(_ slots: Set<Int>) { visibleSlots = slots }
    init(template: Entity, device: any MTLDevice, poster: NSImage, audit: LifetimeProbe,
         now: @escaping () -> ContinuousClock.Instant = { .now },
         systemReduceMotion: @escaping () -> Bool = { NSWorkspace.shared.accessibilityDisplayShouldReduceMotion },
         releaseBetweenEvents: Bool = false,
         showPoster: @escaping (NSImage) -> Void, mount: @escaping (NSView, Int, Double) -> (() -> Void)) {
        self.template = template; self.device = device; self.poster = poster; self.audit = audit
        self.showPoster = showPoster; self.mount = mount
        self.now = now
        self.systemReduceMotion = systemReduceMotion
        self.releaseBetweenEvents = releaseBetweenEvents; neutralPoster = poster
    }
    func request(_ event: PresentationEvent, slot: Int = 0, configuration: EngineConfiguration = EngineConfiguration(),
                 motion: Bool = true, onFrame: ((Int) -> Void)? = nil) async throws {
        if case .responding = event, lastRespondingSlot == slot, lastRespondingConfiguration == configuration,
           let lastResponding, lastResponding.duration(to: now()) < .milliseconds(250) { return }
        if case .responding = event { lastResponding = now(); lastRespondingSlot = slot; lastRespondingConfiguration = configuration }
        generation += 1; let ticket = generation
        let previous = work; previous?.cancel()
        if let previous {
            do { try await previous.value } catch is CancellationError {} catch {
                await releaseEngine(); showPoster(poster); throw error
            }
        }
        guard generation == ticket else { return }
        let operation = Task { @MainActor [weak self] in
            guard let self else { return }
            try Task.checkCancellation()
            hasWork = true
            defer { hasWork = false }
            switch event {
            case .idle, .hidden, .reducedMotion:
                lastResponding = nil
                poster = neutralPoster
                if case .hidden = event { visibleSlots.removeAll() }
                await releaseEngine(); activeSlot = nil; showPoster(poster); return
            default: break
            }
            guard motion, !systemReduceMotion(), visibleSlots.contains(slot) else { await releaseEngine(); activeSlot = nil; showPoster(poster); return }
            let start = Date()
            if engine == nil { engine = try Gate2Engine(template: template, device: device, configuration: configuration, audit: audit) }
            guard let engine else { throw NSError(domain: "PresentationEngineMissing", code: 1) }
            if engine.configuration != configuration { try await engine.resize(configuration) }
            guard generation == ticket else { return }
            showPoster(poster)
            let reveal = engine.view.map { mount($0, slot, configuration.points) }
            activeSlot = slot
            try await engine.draw()
            guard generation == ticket else { return }
            reveal?()
            resumeMilliseconds.append(Date().timeIntervalSince(start) * 1000)
            if resumeMilliseconds.count > 64 { resumeMilliseconds.removeFirst() }
            switch event {
            case .blink:
                try await engine.transition(to: MascotPose(blink: 1), steps: 3, onFrame: onFrame)
                try await engine.transition(to: MascotPose(), steps: 3)
                await releaseEngine(); activeSlot = nil; showPoster(poster)
            case .responding:
                try await engine.transition(to: MascotPose(open: 0.35, look: 0.1), steps: 3, onFrame: onFrame)
                try await engine.transition(to: MascotPose(), steps: 3)
            case .working: try await engine.transition(to: MascotPose(look: 0.5, tilt: -0.055), onFrame: onFrame)
            case .attention: try await engine.transition(to: MascotPose(look: 0.2, tilt: -0.025, concern: -0.5), onFrame: onFrame)
            case .disconnected: try await engine.transition(to: MascotPose(tilt: 0.035, concern: 0.85), onFrame: onFrame)
            default: break
            }
            if releaseBetweenEvents && self.engine != nil {
                switch event {
                case .working, .attention, .disconnected: poster = try await engine.poster()
                default: poster = neutralPoster
                }
                await releaseEngine(); activeSlot = nil; showPoster(poster)
            }
        }
        work = operation
        defer { if generation == ticket { work = nil } }
        try await operation.value
    }
    func captureNeutral() async throws {
        if engine == nil {
            engine = try Gate2Engine(template: template, device: device,
                                     configuration: EngineConfiguration(pixels: 192, points: 104), audit: audit)
        }
        if let engine {
            engine.rig?.apply(MascotPose()); poster = try await engine.poster(); neutralPoster = poster
            showPoster(poster)
        }
        if releaseBetweenEvents { await releaseEngine(); activeSlot = nil }
    }
    private func releaseEngine() async {
        if let engine { framesRetired += engine.renders; await engine.close() }
        engine = nil
    }
}
