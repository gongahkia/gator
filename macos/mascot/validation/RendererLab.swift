import AppKit
import RealityKit
import Darwin

@MainActor private final class WeakRenderer {
    weak var value: RealtimeRenderer?
    init(_ value: RealtimeRenderer) { self.value = value }
}

@main @MainActor final class RendererLab: NSObject, NSApplicationDelegate, NSWindowDelegate {
    let slot = NSStackView()
    let status = NSTextField(labelWithString: "Static baseline — no RealityKit scene initialized")
    var window: NSWindow!
    var renderers: [RealtimeRenderer] = []
    private var retired: [WeakRenderer] = []
    var task: Task<Void, Never>?
    var results: [[String: Any]] = []
    var frameCount = 0
    var modelLoads = 0
    var events: [[String: Any]] = []
    var loadHeartbeatGaps: [Double] = []
    let started = Date()
    let mode: String
    let output: URL
    let assets: URL
    let recordReady: URL?
    override init() {
        func argument(_ name: String, fallback: String) -> String {
            guard let index = CommandLine.arguments.firstIndex(of: name), index + 1 < CommandLine.arguments.count else { return fallback }
            return CommandLine.arguments[index + 1]
        }
        mode = argument("--mode", fallback: "interactive")
        output = URL(fileURLWithPath: argument("--output", fallback: "/tmp/gator-mascot-a2-interactive"), isDirectory: true)
        assets = Bundle.main.resourceURL!.appendingPathComponent("Assets", isDirectory: true)
        let readyPath = argument("--record-ready", fallback: "")
        recordReady = readyPath.isEmpty ? nil : URL(fileURLWithPath: readyPath)
        super.init()
    }
    static func main() {
        let app = NSApplication.shared, delegate = RendererLab()
        app.delegate = delegate; app.setActivationPolicy(.regular); app.run()
        withExtendedLifetime(delegate) {}
    }
    func applicationDidFinishLaunching(_ notification: Notification) {
        do { try FileManager.default.createDirectory(at: output, withIntermediateDirectories: true) }
        catch { fputs("Cannot create validation output: \(error)\n", stderr); NSApp.terminate(nil); return }
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 660, height: 470), styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
        window.title = "Gator B — RealityKit validation only"
        window.identifier = NSUserInterfaceItemIdentifier("mascot-validation")
        window.isReleasedWhenClosed = false; window.delegate = self
        let stack = NSStackView(); stack.orientation = .vertical; stack.spacing = 10
        stack.edgeInsets = NSEdgeInsets(top: 12, left: 18, bottom: 12, right: 18)
        slot.orientation = .horizontal; slot.spacing = 12
        slot.heightAnchor.constraint(equalToConstant: 130).isActive = true
        stack.addArrangedSubview(slot); stack.addArrangedSubview(status)
        let buttons = NSStackView(); buttons.spacing = 4
        for expression in MascotExpression.allCases {
            let button = NSButton(title: expression.rawValue, target: self, action: #selector(selectExpression(_:)))
            button.identifier = NSUserInterfaceItemIdentifier(expression.rawValue)
            button.isEnabled = mode == "interactive"
            button.bezelStyle = .rounded; buttons.addArrangedSubview(button)
        }
        stack.addArrangedSubview(buttons)
        let scroll = NSScrollView(); scroll.hasVerticalScroller = true
        let text = NSTextView(frame: NSRect(x: 0, y: 0, width: 610, height: 1000))
        text.string = "ISOLATED RENDERER FIXTURE — not Gator chat\nNo backend, credentials or task authority.\n\n" + (1...70).map { "Fixture paragraph \($0): native scrolling and text selection remain available." }.joined(separator: "\n\n")
        text.isEditable = true; text.isVerticallyResizable = true
        scroll.documentView = text; stack.addArrangedSubview(scroll)
        stack.addArrangedSubview(NSTextField(labelWithString: "No model requests. Genuine named 3D parts; draw-on-demand Metal surface."))
        window.contentView = stack; window.center(); showStatic(); window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        task = Task { [weak self] in
            guard let self else { return }
            do {
                switch mode {
                case "quick": try await quick()
                case "lifecycle": try await lifecycle()
                case "demo": try await demo()
                case "checks": try await checks()
                default: return
                }
                removeRenderers(); try write(); NSApp.terminate(nil)
            } catch {
                results.append(["error": String(describing: error)])
                removeRenderers()
                do { try write() } catch { fputs("Validation report write failed\n", stderr) }
                NSApp.terminate(nil)
            }
        }
    }
    func showStatic() {
        for view in slot.arrangedSubviews { slot.removeArrangedSubview(view); view.removeFromSuperview() }
        let image = NSImageView(image: NSImage(contentsOf: assets.appendingPathComponent("B-static.png"))!)
        image.imageScaling = .scaleProportionallyUpOrDown
        image.widthAnchor.constraint(equalToConstant: 112).isActive = true
        image.heightAnchor.constraint(equalToConstant: 112).isActive = true
        slot.addArrangedSubview(image)
    }
    func removeRenderers() {
        for renderer in renderers {
            frameCount += renderer.renders
            retired.append(WeakRenderer(renderer)); renderer.close()
        }
        renderers.removeAll()
        showStatic()
    }
    func load(_ count: Int = 1, points: CGFloat = 112) async throws {
        removeRenderers()
        for view in slot.arrangedSubviews { slot.removeArrangedSubview(view); view.removeFromSuperview() }
        for _ in 0..<count {
            let monitor = Task { [weak self] in
                var last = ContinuousClock.now
                while !Task.isCancelled {
                    do { try await Task.sleep(for: .milliseconds(10)) } catch { return }
                    let now = ContinuousClock.now, duration = last.duration(to: now)
                    self?.loadHeartbeatGaps.append(Double(duration.components.seconds) * 1000 + Double(duration.components.attoseconds) / 1e15)
                    last = now
                }
            }
            defer { monitor.cancel() }
            let renderer = try await RealtimeRenderer(asset: assets.appendingPathComponent("Gator-B.usdz"), points: points)
            renderer.view.widthAnchor.constraint(equalToConstant: points).isActive = true
            renderer.view.heightAnchor.constraint(equalToConstant: points).isActive = true
            slot.addArrangedSubview(renderer.view); renderers.append(renderer); modelLoads += 1
            try await renderer.draw()
            monitor.cancel(); await monitor.value
            results.append(["loadMilliseconds": renderer.loadMilliseconds, "firstFrameMilliseconds": renderer.transitionMilliseconds.last ?? -1, "points": points, "surface": renderer.surfaceFacts, "loadHeartbeatMaximumGapMs": loadHeartbeatGaps.max() ?? 0])
            loadHeartbeatGaps.removeAll(keepingCapacity: true)
        }
    }
    @objc func selectExpression(_ sender: NSButton) {
        guard mode == "interactive" else { return }
        guard let expression = MascotExpression(rawValue: sender.identifier?.rawValue ?? "") else { return }
        task?.cancel()
        task = Task { [weak self] in
            guard let self else { return }
            do {
                if expression == .reducedMotion { removeRenderers(); status.stringValue = "Reduced Motion: static cached Concept B"; return }
                if renderers.isEmpty { try await load() }
                status.stringValue = "Fixture state: " + expression.rawValue
                for renderer in renderers { try await renderer.transition(to: expression.pose) }
            } catch is CancellationError {} catch { status.stringValue = "Renderer failed; static fallback"; removeRenderers() }
        }
    }
    func metrics() -> (cpu: Double, rss: UInt64) {
        var usage = rusage(); getrusage(RUSAGE_SELF, &usage)
        let cpu = Double(usage.ru_utime.tv_sec + usage.ru_stime.tv_sec) + Double(usage.ru_utime.tv_usec + usage.ru_stime.tv_usec) / 1_000_000
        var info = mach_task_basic_info(), count = mach_msg_type_number_t(MemoryLayout<mach_task_basic_info>.size / MemoryLayout<natural_t>.size)
        let result = withUnsafeMutablePointer(to: &info) { pointer in
            pointer.withMemoryRebound(to: integer_t.self, capacity: Int(count)) { task_info(mach_task_self_, task_flavor_t(MACH_TASK_BASIC_INFO), $0, &count) }
        }
        return (cpu, result == KERN_SUCCESS ? info.resident_size : 0)
    }
    func phase(_ name: String, seconds: Int, responding: Bool = false, blink: Bool = false) async throws {
        status.stringValue = "Measurement: " + name
        let initial = metrics(), start = Date(), initialFrames = frameCount + renderers.reduce(0) { $0 + $1.renders }
        var samples: [UInt64] = []
        for second in 0..<seconds {
            try Task.checkCancellation()
            if responding {
                for renderer in renderers { try await renderer.transition(to: MascotPose(open: second.isMultiple(of: 2) ? 0.4 : 0.12), steps: 4) }
            } else if blink && second > 0 && second.isMultiple(of: 7) {
                for renderer in renderers {
                    try await renderer.transition(to: MascotPose(blink: 1), steps: 3)
                    try await renderer.transition(to: MascotPose(), steps: 3)
                }
            }
            samples.append(metrics().rss)
            try await Task.sleep(for: .seconds(1))
        }
        let final = metrics(), elapsed = Date().timeIntervalSince(start)
        let warmFrames = renderers.flatMap(\.transitionMilliseconds).sorted()
        let totalFrames = frameCount + renderers.reduce(0) { $0 + $1.renders }
        let phaseRecord: [String: Any] = ["phase": name, "seconds": elapsed, "cpuPercentOneCore": (final.cpu - initial.cpu) / elapsed * 100,
                        "rssMiBSamples": samples.map { Double($0) / 1_048_576 }, "rssFinalMiB": Double(final.rss) / 1_048_576,
                        "renders": totalFrames - initialFrames,
                        "renderMillisecondsP50": warmFrames.isEmpty ? 0 : warmFrames[warmFrames.count / 2],
                        "renderMillisecondsP95": warmFrames.isEmpty ? 0 : warmFrames[min(warmFrames.count - 1, Int(Double(warmFrames.count) * 0.95))],
                        "liveRenderers": renderers.count, "retiredStillAlive": retired.filter { $0.value != nil }.count]
        results.append(phaseRecord)
        try write()
    }
    func quick() async throws {
        try await phase("static-cold", seconds: 12)
        try await load()
        try await validateCurrent()
        try await phase("one-idle-no-draw", seconds: 15)
        try await phase("one-idle-blink", seconds: 15, blink: true)
        try await phase("one-responding", seconds: 12, responding: true)
        try await remainingQuick()
    }
    func validateCurrent() async throws {
        guard let renderer = renderers.first else { throw NSError(domain: "LabNoRenderer", code: 1) }
        results.append(["rigValidation": try renderer.rig.verify()])
        try await renderer.draw()
        results.append(["alpha": try renderer.savePNG(output.appendingPathComponent("neutral.png"))])
    }
    func remainingQuick() async throws {
        try await load(2)
        try await phase("two-idle", seconds: 12)
        try await phase("two-responding", seconds: 10, responding: true)
        try await load(points: 40)
        try await phase("collapsed-idle", seconds: 12)
        removeRenderers(); window.miniaturize(nil)
        try await phase("hidden-teardown", seconds: 15)
        window.deminiaturize(nil)
        try await phase("reduced-static-after-engine", seconds: 12)
    }
    func lifecycle() async throws {
        try await phase("static-cold", seconds: 15)
        for cycle in 0..<10 {
            try await load(points: cycle.isMultiple(of: 2) ? 112 : 40)
            try await phase("cycle-\(cycle)-visible", seconds: 20, blink: true)
            for expression in MascotExpression.allCases where expression != .reducedMotion {
                for renderer in renderers { try await renderer.transition(to: expression.pose, steps: 4) }
            }
            try await phase("cycle-\(cycle)-responding", seconds: 8, responding: true)
            removeRenderers(); window.orderOut(nil)
            try await phase("cycle-\(cycle)-hidden", seconds: 30)
            window.makeKeyAndOrderFront(nil)
        }
    }
    func checks() async throws {
        var assertions = 0
        func check(_ condition: Bool, _ label: String) throws {
            guard condition else { throw NSError(domain: "RendererLifecycleAssertion", code: assertions, userInfo: [NSLocalizedDescriptionKey: label]) }
            assertions += 1
        }
        var rejectedMissing = false
        do { _ = try await RealtimeRenderer(asset: assets.appendingPathComponent("missing.usdz")) }
        catch { rejectedMissing = true; showStatic() }
        try check(rejectedMissing && renderers.isEmpty, "missing resource retains static UI")
        try await load()
        try await validateCurrent()
        let retirement = try await cancelCurrentAnimation()
        try check(retirement.value == nil, "renderer released after cancelled transition")
        try check(renderers.isEmpty, "no renderer after removal")
        try await load(2)
        try check(renderers.count == 2 && renderers[0].rig.root !== renderers[1].rig.root, "independent roots for two views")
        let otherJaw = renderers[1].rig.jaw.transform
        renderers[0].rig.apply(MascotPose(open: 0.85, blink: 1))
        try check(renderers[1].rig.jaw.transform == otherJaw, "one presentation does not change the other")
        removeRenderers()
        try check(retired.allSatisfy { $0.value == nil }, "all retired renderer owners released")
        results.append(["lifecycleAssertions": assertions, "cancellation": "after confirmed first frame, before transition completion", "missingResource": "static fallback; chat-independent"])
    }
    private func cancelCurrentAnimation() async throws -> WeakRenderer {
        guard let renderer = renderers.first else { throw NSError(domain: "MissingRenderer", code: 1) }
        let weakReference = WeakRenderer(renderer)
        var animation: Task<Void, any Error>?
        await withCheckedContinuation { (ready: CheckedContinuation<Void, Never>) in
            animation = Task {
                try await renderer.transition(to: MascotPose(open: 0.85, blink: 1), steps: 120) { step in
                    if step == 1 { ready.resume() }
                }
            }
        }
        animation?.cancel(); removeRenderers()
        let framesAtCancellation = renderer.renders
        if let animation {
            do { try await animation.value; throw NSError(domain: "ExpectedCancellation", code: 1) }
            catch is CancellationError {}
        }
        guard renderer.renders == framesAtCancellation else { throw NSError(domain: "RenderedAfterCancellation", code: 1) }
        return weakReference
    }
    func demo() async throws {
        if let recordReady {
            let deadline = ContinuousClock.now.advanced(by: .seconds(15))
            while !FileManager.default.fileExists(atPath: recordReady.path) {
                guard ContinuousClock.now < deadline else { throw NSError(domain: "RecordingReadinessTimedOut", code: 1) }
                try await Task.sleep(for: .milliseconds(50))
            }
        }
        try await load()
        guard let renderer = renderers.first else { return }
        results.append(["rigValidation": try renderer.rig.verify()])
        for expression in MascotExpression.allCases where expression != .reducedMotion {
            status.stringValue = "LIVE 3D fixture: " + expression.rawValue
            try await renderer.transition(to: expression.pose, steps: 12)
            try await renderer.draw()
            results.append(["state": expression.rawValue, "alpha": try renderer.savePNG(output.appendingPathComponent(expression.rawValue + ".png"))])
            if expression == .idle {
                try await renderer.transition(to: MascotPose(blink: 1), steps: 8)
                _ = try renderer.savePNG(output.appendingPathComponent("blink.png"))
                try await renderer.transition(to: MascotPose(), steps: 8)
            }
            try await Task.sleep(for: .seconds(2))
        }
        renderer.rig.root.orientation = simd_quatf(angle: -0.48, axis: [0, 1, 0])
        try await renderer.draw(); _ = try renderer.savePNG(output.appendingPathComponent("quarter.png"))
        renderer.rig.root.orientation = simd_quatf()
        status.stringValue = "Reduced Motion: cached static poster (2D)"
        removeRenderers(); try await Task.sleep(for: .seconds(3))
    }
    func write() throws {
        let document: [String: Any] = ["mode": mode, "pid": getpid(), "elapsed": Date().timeIntervalSince(started), "modelLoads": modelLoads, "events": events,
                                      "renders": frameCount + renderers.reduce(0) { $0 + $1.renders }, "results": results,
                                      "macOS": ProcessInfo.processInfo.operatingSystemVersionString, "processors": ProcessInfo.processInfo.processorCount]
        try JSONSerialization.data(withJSONObject: document, options: [.prettyPrinted, .sortedKeys]).write(to: output.appendingPathComponent("measurements.json"), options: .atomic)
    }
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        events.append(["event": "applicationShouldTerminate", "elapsed": Date().timeIntervalSince(started), "frontBundle": NSWorkspace.shared.frontmostApplication?.bundleIdentifier ?? "unknown"])
        task?.cancel(); removeRenderers(); return .terminateNow
    }
    func windowWillClose(_ notification: Notification) {
        events.append(["event": "windowWillClose", "elapsed": Date().timeIntervalSince(started)])
        NSApp.terminate(nil)
    }
}
