import AppKit
import RealityKit
import Metal

@main @MainActor final class Gate2Lab: NSObject, NSApplicationDelegate, NSWindowDelegate {
    let experiment: String
    let output: URL
    let asset: URL
    let start = Date()
    let audit = LifetimeProbe()
    var window: NSWindow!
    let slots = [NSStackView(), NSStackView()]
    let status = NSTextField(labelWithString: "Isolated Gate 2 resource study — no Go backend")
    let text = NSTextView()
    var device: (any MTLDevice)?
    var template: Entity?
    var frameworkObject: Entity?
    var emptyRenderer: RealityRenderer?
    var engine: Gate2Engine?
    var secondEngine: Gate2Engine?
    var original: RealtimeRenderer?
    var poster: NSImage!
    var task: Task<Void, Never>?
    var log: FileHandle?
    var phaseName = "starting"
    var records: [[String: Any]] = []
    var framesRetired = 0
    var assertionCount = 0
    var decodeCount = 0
    var activeSlot = 0
    var finished = false
    var events: [[String: Any]] = []
    var presentation: Gate2Presentation?
    var compactWindow: NSWindow?
    var controlledClose = false
    override init() {
        func arg(_ name: String, _ fallback: String) -> String {
            guard let i = CommandLine.arguments.firstIndex(of: name), i + 1 < CommandLine.arguments.count else { return fallback }
            return CommandLine.arguments[i + 1]
        }
        experiment = arg("--experiment", "direct96")
        output = URL(fileURLWithPath: arg("--output", "/tmp/gator-gate2-run"))
        asset = Bundle.main.resourceURL!.appendingPathComponent("Assets/Gator-B.usdz")
        super.init()
    }
    static func main() {
        let app = NSApplication.shared, delegate = Gate2Lab()
        app.delegate = delegate; app.setActivationPolicy(.regular); app.run()
        withExtendedLifetime(delegate) {}
    }
    func applicationDidFinishLaunching(_ notification: Notification) {
        do {
            ProcessInfo.processInfo.disableAutomaticTermination("Isolated renderer resource trial")
            try FileManager.default.createDirectory(at: output, withIntermediateDirectories: true)
            let logURL = output.appendingPathComponent("samples.ndjson")
            guard FileManager.default.createFile(atPath: logURL.path, contents: nil) else { throw NSError(domain: "LogCreateFailed", code: 1) }
            log = try FileHandle(forWritingTo: logURL)
            poster = NSImage(contentsOf: Bundle.main.resourceURL!.appendingPathComponent("Assets/B-static.png"))
            guard poster != nil else { throw NSError(domain: "MissingStaticPoster", code: 1) }
            makeWindow()
            task = Task { [weak self] in
                guard let self else { return }
                do {
                    switch experiment {
                    case "attribution", "attribution-native": try await attribution()
                    case "checks": try await checks()
                    case "endurance": try await endurance()
                    case "hybrid": try await hybridComparison()
                    default: try await comparison()
                    }
                    await destroy()
                    try write(done: true)
                } catch {
                    records.append(["error": String(describing: error)])
                    await destroy()
                    do { try write(done: true) } catch { fputs("Cannot write gate report\n", stderr) }
                }
                do { try log?.close() } catch { fputs("Cannot close metrics log\n", stderr) }
                NSApp.terminate(nil)
            }
        } catch { fputs("Lab startup failed: \(error)\n", stderr); NSApp.terminate(nil) }
    }
    func makeWindow() {
        _ = NSWorkspace.shared.accessibilityDisplayShouldReduceMotion
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 680, height: 570), styleMask: [.titled, .closable, .miniaturizable], backing: .buffered, defer: false)
        window.title = "Gator Concept B — Gate 2 only"
        window.delegate = self; window.isReleasedWhenClosed = false
        window.appearance = NSAppearance(named: .aqua)
        let layout = NSStackView(); layout.orientation = .vertical; layout.spacing = 12
        layout.edgeInsets = NSEdgeInsets(top: 14, left: 16, bottom: 14, right: 16)
        let row = NSStackView(); row.spacing = 20
        for (index, slot) in slots.enumerated() {
            slot.orientation = .vertical
            slot.widthAnchor.constraint(equalToConstant: 290).isActive = true
            slot.heightAnchor.constraint(equalToConstant: 280).isActive = true
            slot.addArrangedSubview(NSTextField(labelWithString: index == 0 ? "Full-window fixture" : "Compact fixture"))
            row.addArrangedSubview(slot)
        }
        layout.addArrangedSubview(row); layout.addArrangedSubview(status)
        let scroll = NSScrollView(); scroll.hasVerticalScroller = true
        text.frame = NSRect(x: 0, y: 0, width: 630, height: 3000)
        text.string = (1...90).map { "Fixture paragraph \($0). Editable native text; no message is sent." }.joined(separator: "\n\n")
        text.isVerticallyResizable = true; text.isEditable = true
        scroll.documentView = text; layout.addArrangedSubview(scroll)
        window.contentView = layout; window.center(); showPosters()
        window.makeKeyAndOrderFront(nil); NSApp.activate(ignoringOtherApps: true)
    }
    func showPosters() {
        for slot in slots {
            for view in slot.arrangedSubviews.dropFirst() { slot.removeArrangedSubview(view); view.removeFromSuperview() }
            let image = NSImageView(image: poster)
            image.imageScaling = .scaleProportionallyUpOrDown
            image.widthAnchor.constraint(equalToConstant: 104).isActive = true
            image.heightAnchor.constraint(equalToConstant: 104).isActive = true
            image.setAccessibilityElement(false); slot.addArrangedSubview(image)
        }
    }
    func attach(_ view: NSView, slot: Int = 0, points: Double) {
        for old in slots[slot].arrangedSubviews.dropFirst() { slots[slot].removeArrangedSubview(old); old.removeFromSuperview() }
        view.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([view.widthAnchor.constraint(equalToConstant: points), view.heightAnchor.constraint(equalToConstant: points)])
        slots[slot].addArrangedSubview(view)
    }
    func prepareTemplate() async throws {
        if device == nil { device = MTLCreateSystemDefaultDevice() }
        guard device != nil else { throw NSError(domain: "MetalUnavailable", code: 1) }
        if template == nil {
            if experiment.contains("native") || experiment == "checks" || experiment == "endurance" || experiment == "hybrid" {
                let archive = try await NativeMeshArchive.read(Bundle.main.resourceURL!.appendingPathComponent("Assets/B-native-meshes.json"))
                template = try await archive.entity()
            } else { template = try await Entity(contentsOf: asset) }
            decodeCount += 1
            if let template { audit.track(template, kind: "immutableTemplate") }
        }
    }
    func create(_ configuration: EngineConfiguration, slot: Int = 0) async throws {
        try await prepareTemplate()
        guard let template, let device else { throw NSError(domain: "MissingTemplate", code: 1) }
        let loadStart = Date()
        let created = try Gate2Engine(template: template, device: device, configuration: configuration, audit: audit)
        if slot == 0 { engine = created } else { secondEngine = created }
        if let view = created.view { attach(view, slot: slot, points: configuration.points) }
        try await created.draw()
        records.append(["event": "create", "configuration": try object(configuration), "firstVisibleMilliseconds": Date().timeIntervalSince(loadStart) * 1000])
    }
    func sceneWithoutDraw() throws {
        guard let template, let device else { throw NSError(domain: "MissingTemplate", code: 1) }
        engine = try Gate2Engine(template: template, device: device, configuration: EngineConfiguration(), audit: audit)
        if let view = engine?.view { attach(view, points: 96) }
    }
    func destroy() async {
        if let presentation {
            do { try await presentation.request(.hidden) } catch { records.append(["shutdownError": String(describing: error)]) }
            records.append(["event": "presentationTotals", "resumeMilliseconds": presentation.resumeMilliseconds, "frames": presentation.frames])
            framesRetired += presentation.frames; self.presentation = nil
        }
        if let engine { framesRetired += engine.renders; await engine.close() }
        if let secondEngine { framesRetired += secondEngine.renders; await secondEngine.close() }
        if let original { framesRetired += original.renders; original.close() }
        engine = nil; secondEngine = nil; original = nil
        showPosters()
    }
    var frames: Int { framesRetired + (engine?.renders ?? 0) + (secondEngine?.renders ?? 0) + (original?.renders ?? 0) + (presentation?.frames ?? 0) }
    func object<T: Encodable>(_ value: T) throws -> Any { try JSONSerialization.jsonObject(with: JSONEncoder().encode(value)) }
    func sample() throws -> ResourceSample {
        let value = ResourceSample.take(start: start, device: device)
        var data = try JSONEncoder().encode(value); data.append(10); try log?.write(contentsOf: data)
        return value
    }
    func phase(_ name: String, seconds: Int = 10, settle: Int = 4, activity: Bool = false) async throws {
        phaseName = name; status.stringValue = name
        try write()
        if settle > 0 { try await Task.sleep(for: .seconds(settle)) }
        let initial = try sample(), initialFrames = frames
        var samples: [ResourceSample] = []
        for i in 0..<seconds {
            if activity {
                let pose = MascotPose(open: i.isMultiple(of: 2) ? 0.4 : 0, blink: i.isMultiple(of: 5) ? 1 : 0, look: 0.2)
                if let engine { try await engine.transition(to: pose, steps: 4) }
                if let secondEngine { try await secondEngine.transition(to: pose, steps: 4) }
                if let original { try await original.transition(to: pose, steps: 4) }
                if let presentation { try await presentation.request(.responding, slot: activeSlot, configuration: configuration(for: activeSlot)) }
            }
            try await Task.sleep(for: .seconds(1))
            samples.append(try sample())
        }
        let final = try sample(), elapsed = final.elapsed - initial.elapsed
        let record: [String: Any] = ["phase": name, "seconds": elapsed, "cpuPercentOneCore": (final.cpuSeconds - initial.cpuSeconds) / elapsed * 100,
                                     "samples": try object(samples), "frames": frames - initialFrames, "live": audit.live,
                                     "configuration": engine.map { try? object($0.configuration) } as Any,
                                     "drawableBytes": engine?.drawableBytes ?? 0, "drawableDimensions": engine?.drawableDimensions ?? [],
                                     "decodeCount": decodeCount]
        // optional configuration is represented explicitly as null in JSON
        var clean = record; clean["configuration"] = (engine ?? presentation?.engine).flatMap { try? object($0.configuration) } ?? NSNull()
        clean["drawableBytes"] = (engine ?? presentation?.engine)?.drawableBytes ?? 0
        clean["drawableDimensions"] = (engine ?? presentation?.engine)?.drawableDimensions ?? []
        clean["pendingAnimationTask"] = presentation?.pendingWork ?? false
        clean["hasAnimationWork"] = presentation?.hasWork ?? false
        func countNodes(_ entity: Entity) -> Int { 1 + entity.children.reduce(0) { $0 + countNodes($1) } }
        clean["templateNodeCount"] = template.map(countNodes) ?? 0
        clean["sceneNodeCount"] = (engine ?? presentation?.engine)?.renderer?.entities.reduce(0) { $0 + countNodes($1) } ?? 0
        clean["resumeLastMilliseconds"] = presentation?.resumeMilliseconds.last ?? 0
        let file = String(format: "%03d-%@.json", records.count, name)
        try JSONSerialization.data(withJSONObject: clean, options: [.sortedKeys, .prettyPrinted]).write(to: output.appendingPathComponent(file), options: .atomic)
        clean.removeValue(forKey: "samples"); clean["sampleFile"] = file
        func median(_ values: [UInt64]) -> Double {
            let values = values.sorted(); guard !values.isEmpty else { return 0 }
            return values.count.isMultiple(of: 2) ? (Double(values[values.count / 2 - 1]) + Double(values[values.count / 2])) / 2 : Double(values[values.count / 2])
        }
        clean["medianRSSBytes"] = median(samples.map(\.rssBytes))
        clean["medianFootprintBytes"] = median(samples.map(\.footprintBytes))
        clean["medianHeapBytes"] = median(samples.map(\.defaultHeapInUseBytes))
        clean["lastSample"] = try object(final)
        records.append(clean); try write()
    }
    func baseline() async throws {
        // initialize both native fixture windows before attributing renderer costs
        openCompact(); compactWindow?.orderOut(nil); window.makeKeyAndOrderFront(nil)
        try await phase("A0-host-no-RealityKit-initialization", seconds: 10, settle: 20)
    }
    func attribution() async throws {
        try await baseline()
        frameworkObject = Entity()
        try await phase("A1-empty-entity-no-model-scene")
        device = MTLCreateSystemDefaultDevice()
        try await phase("A1-Metal-device-only")
        emptyRenderer = try RealityRenderer()
        try await phase("A1-empty-RealityRenderer")
        emptyRenderer = nil; frameworkObject = nil
        try await phase("A1-empty-renderer-released")
        try await prepareTemplate()
        try await phase("A2-static-model-template-only")
        try sceneWithoutDraw()
        try await phase("A2-scene-attached-without-draw")
        try await engine?.draw()
        try await phase("A3-live96-static-pose")
        try await phase("A3-live96-jaw-blink", activity: true)
        try await engine?.transition(to: MascotPose())
        try await phase("A5-returned-to-idle")
        await engine?.pause(); window.orderOut(nil)
        try await phase("A6-hidden-drawables-released")
        await destroy()
        try await phase("A7-renderer-destroyed")
        self.template = nil
        try await phase("A7-template-release-requested")
    }
    func comparison() async throws {
        try await baseline()
        if experiment == "original" {
            original = try await RealtimeRenderer(asset: asset)
            device = original?.texture.device
            if let original {
                audit.track(original, kind: "originalOwner"); audit.track(original.renderer, kind: "RealityRenderer")
                audit.track(original.rig.root, kind: "sceneRoot"); audit.track(original.view, kind: "surface")
                audit.track(original.texture as AnyObject, kind: "originalOffscreenTexture")
                attach(original.view, points: 112); try await original.draw()
            }
        } else {
            let pixels = experiment.contains("256") || experiment == "msaa" ? 256 : 96
            try await create(EngineConfiguration(pixels: pixels, points: pixels == 256 ? 112 : 96, msaa: experiment == "msaa", metalKit: experiment == "mtk96"))
            if experiment == "native-two" { try await create(EngineConfiguration(), slot: 1) }
        }
        try await phase("A3-or-A4-visible-idle")
        try await phase("A3-or-A4-visible-active", activity: true)
        if let engine {
            engine.rig?.apply(MascotPose()); try await engine.draw()
            poster = try await engine.poster(); try savePoster("neutral-\(experiment).png")
            engine.rig?.apply(MascotPose(open: 0.85, blink: 1)); poster = try await engine.poster(); try savePoster("articulated-\(experiment).png")
            engine.rig?.apply(MascotPose()); try await engine.draw()
        }
        try await phase("A5-returned-to-idle")
        if let engine { await engine.pause() }
        if let original { original.view.removeFromSuperview(); original.view.metalLayer.device = nil }
        window.orderOut(nil)
        try await phase("A6-hidden")
        await destroy()
        try await phase("A7-destroyed")
    }
    func savePoster(_ name: String) throws {
        guard let bitmap = poster.representations.first as? NSBitmapImageRep, let data = bitmap.representation(using: .png, properties: [:]) else { throw NSError(domain: "MissingPoster", code: 1) }
        try data.write(to: output.appendingPathComponent(name))
    }
    func check(_ condition: Bool, _ name: String) throws {
        guard condition else { throw NSError(domain: "Gate2Assertion", code: assertionCount, userInfo: [NSLocalizedDescriptionKey: name]) }
        assertionCount += 1
    }
    func checks() async throws {
        try await create(EngineConfiguration())
        guard let engine else { throw NSError(domain: "MissingEngine", code: 1) }
        records.append(["rig": try engine.rig!.verify()])
        try await engine.transition(to: MascotPose(open: 0.8), steps: 1)
        await engine.pause()
        try check(engine.view == nil && engine.drawableBytes == 0, "pause releases surface/target")
        try check(engine.rig?.pose == MascotPose(), "pause restores neutral jaw")
        let count = engine.renders
        try await engine.transition(to: MascotPose(open: 1), steps: 1)
        try check(engine.renders == count, "hidden state does not draw")
        engine.resume(); if let view = engine.view { attach(view, points: 96) }
        try await engine.draw()
        try check(engine.renders == count + 1, "resume draws")
        try await engine.resize(EngineConfiguration(pixels: 256, points: 112))
        if let view = engine.view { attach(view, points: 112) }
        try await engine.draw()
        try check(engine.drawableDimensions == [256, 256], "resolution respected")
        await engine.close()
        try check(engine.rig == nil && engine.renderer == nil && engine.view == nil, "close releases resources even while owner retained")
        self.engine = nil
    }
    func configuration(for slot: Int) -> EngineConfiguration {
        EngineConfiguration(pixels: slot == 0 ? 192 : 96, points: slot == 0 ? 104 : 56)
    }
    func preparePresentation() async throws {
        try await prepareTemplate()
        guard let template, let device else { throw NSError(domain: "MissingTemplate", code: 1) }
        presentation = Gate2Presentation(template: template, device: device, poster: poster, audit: audit, releaseBetweenEvents: true,
                                         showPoster: { [weak self] image in self?.poster = image; self?.showPosters() },
                                         mount: { [weak self] view, slot, points in self?.mountOverlay(view, slot: slot, points: points) ?? {} })
    }
    func mountOverlay(_ view: NSView, slot: Int, points: Double) -> () -> Void {
        for old in slots[slot].arrangedSubviews.dropFirst() { slots[slot].removeArrangedSubview(old); old.removeFromSuperview() }
        let container = NSView(frame: NSRect(x: 0, y: 0, width: points, height: points))
        let cached = NSImageView(image: poster); cached.imageScaling = .scaleProportionallyUpOrDown
        cached.frame = container.bounds; view.frame = container.bounds
        view.translatesAutoresizingMaskIntoConstraints = true; view.alphaValue = 0
        container.addSubview(cached); container.addSubview(view)
        container.widthAnchor.constraint(equalToConstant: points).isActive = true
        container.heightAnchor.constraint(equalToConstant: points).isActive = true
        slots[slot].addArrangedSubview(container)
        return { [weak cached, weak view] in cached?.removeFromSuperview(); view?.alphaValue = 1 }
    }
    func openCompact() {
        if compactWindow == nil {
            let panel = NSWindow(contentRect: NSRect(x: 60, y: 80, width: 440, height: 340), styleMask: [.titled, .closable], backing: .buffered, defer: false)
            panel.title = "Gator compact renderer fixture"; panel.isReleasedWhenClosed = false
            if let row = slots[1].superview as? NSStackView { row.removeArrangedSubview(slots[1]) }
            slots[1].removeFromSuperview()
            let container = NSView(frame: NSRect(x: 0, y: 0, width: 440, height: 340)); panel.contentView = container
            slots[1].translatesAutoresizingMaskIntoConstraints = false; container.addSubview(slots[1])
            NSLayoutConstraint.activate([slots[1].centerXAnchor.constraint(equalTo: container.centerXAnchor), slots[1].centerYAnchor.constraint(equalTo: container.centerYAnchor)])
            compactWindow = panel
        }
        compactWindow?.makeKeyAndOrderFront(nil)
    }
    func hybridComparison() async throws {
        try await baseline(); try await preparePresentation()
        try await presentation?.request(.working, configuration: configuration(for: 0))
        try await presentation?.captureNeutral()
        try await presentation?.request(.idle)
        try await phase("hybrid-static-idle")
        try await presentation?.request(.blink, configuration: configuration(for: 0))
        try await phase("hybrid-after-blink-static")
        try await phase("hybrid-responding", activity: true)
        try await presentation?.request(.idle)
        try await phase("hybrid-returned-idle")
        try await presentation?.request(.hidden); window.orderOut(nil)
        try await presentation?.request(.responding)
        try await phase("hybrid-hidden-state-update")
        await destroy(); try await phase("hybrid-all-destroyed")
    }
    func endurance() async throws {
        try await baseline(); try await preparePresentation()
        try await presentation?.request(.working, configuration: configuration(for: 0))
        try await presentation?.captureNeutral(); try savePoster("neutral-final.png")
        try await presentation?.request(.idle)
        try await phase("initialized-static", seconds: 10)
        for cycle in 0..<10 {
            presentation?.setVisibleSlots([0]); activeSlot = 0
            window.makeKeyAndOrderFront(nil)
            try await presentation?.request(.blink, configuration: configuration(for: 0))
            try await phase("cycle-\(cycle)-static-idle", seconds: 5, settle: 0)
            try await presentation?.request(.working, configuration: configuration(for: 0))
            try await phase("cycle-\(cycle)-working", seconds: 3, settle: 0)
            try await phase("cycle-\(cycle)-responding", seconds: 4, settle: 0, activity: true)
            openCompact(); presentation?.setVisibleSlots([0,1]); activeSlot = 1
            try await presentation?.request(.responding, slot: 1, configuration: configuration(for: 1))
            try await phase("cycle-\(cycle)-compact", seconds: 5, settle: 0)
            if cycle.isMultiple(of: 2) { controlledClose = true; window.close(); controlledClose = false }
            else { window.orderOut(nil) }
            presentation?.setVisibleSlots([1])
            try await phase("cycle-\(cycle)-full-hidden", seconds: 4, settle: 0)
            window.makeKeyAndOrderFront(nil); activeSlot = 0; presentation?.setVisibleSlots([0])
            compactWindow?.orderOut(nil)
            try await presentation?.request(.working, configuration: configuration(for: 0))
            try await phase("cycle-\(cycle)-full-returned", seconds: 4, settle: 0)
            try await presentation?.request(.idle)
            try await phase("cycle-\(cycle)-settled-idle", seconds: 6, settle: 0)
            try await presentation?.request(.hidden); window.orderOut(nil)
            try await presentation?.request(.responding)
            try await phase("cycle-\(cycle)-hidden", seconds: 20, settle: 0)
            window.makeKeyAndOrderFront(nil); presentation?.setVisibleSlots([0])
            try await presentation?.request(.reducedMotion)
            try await phase("cycle-\(cycle)-reduced-motion", seconds: 5, settle: 0)
        }
        try await presentation?.request(.idle)
        try await phase("final-settled-idle", seconds: 15)
        await destroy(); template = nil
        try await phase("final-all-destroyed", seconds: 15)
        try check(Date().timeIntervalSince(start) >= 600, "final trial at least ten minutes")
    }
    func write(done: Bool = false) throws {
        finished = finished || done
        let document: [String: Any] = ["experiment": experiment, "pid": getpid(), "phaseReady": phaseName, "done": done,
                                      "elapsed": Date().timeIntervalSince(start), "records": records, "live": audit.live,
                                      "created": audit.created, "frames": frames, "decodeCount": decodeCount, "assertions": assertionCount,
                                      "backingScale": window?.backingScaleFactor ?? 0, "events": events]
        try JSONSerialization.data(withJSONObject: document, options: [.sortedKeys, .prettyPrinted]).write(to: output.appendingPathComponent("results.json"), options: .atomic)
    }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        events.append(["event": "applicationShouldTerminate", "phase": phaseName, "finished": finished, "front": NSWorkspace.shared.frontmostApplication?.bundleIdentifier ?? "unknown"])
        do { try write(done: finished) } catch { fputs("Cannot record termination\n", stderr) }
        return .terminateNow
    }
    func windowWillClose(_ notification: Notification) {
        events.append(["event": "windowWillClose", "phase": phaseName, "controlled": controlledClose])
        if !controlledClose { task?.cancel() }
    }
}
