import AppKit
import Darwin

@MainActor private final class WeakCanvas {
    weak var value: NativeMascotCanvas?
    init(_ value: NativeMascotCanvas?) { self.value = value }
}

@main @MainActor final class PlaybackLab: NSObject, NSApplicationDelegate {
    var window: NSWindow!
    let slot = NSView()
    var canvas: NativeMascotCanvas?
    var second: NativeMascotCanvas?
    let start = Date()
    var results: [[String: Any]] = []
    var samples: FileHandle!
    var task: Task<Void, Never>?
    var starts = 0
    private var retired: [WeakCanvas] = []
    private var animatedStyles: Set<String> = []
    var output: URL!
    var assets: URL!
    var phase = "starting"
    var assertions = 0
    static func main() {
        let app = NSApplication.shared, delegate = PlaybackLab()
        app.delegate = delegate; app.setActivationPolicy(.regular); app.run(); withExtendedLifetime(delegate) {}
    }
    func applicationDidFinishLaunching(_ notification: Notification) {
        let args = CommandLine.arguments
        func arg(_ name: String, _ fallback: String) -> String { guard let i = args.firstIndex(of: name), i + 1 < args.count else { return fallback }; return args[i + 1] }
        output = URL(fileURLWithPath: arg("--output", "/tmp/gator-2d-lab")); assets = Bundle.main.resourceURL!.appendingPathComponent("Mascot")
        do {
            try FileManager.default.createDirectory(at: output, withIntermediateDirectories: true)
            let file = output.appendingPathComponent("samples.ndjson"); FileManager.default.createFile(atPath: file.path, contents: nil); samples = try FileHandle(forWritingTo: file)
            window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 650, height: 470), styleMask: [.titled,.closable,.resizable,.miniaturizable], backing: .buffered, defer: false)
            window.title = "Gator 2D Playback — isolated"; window.isReleasedWhenClosed = false
            let stack = NSStackView(); stack.orientation = .vertical; stack.spacing = 12
            stack.addArrangedSubview(NSTextField(labelWithString: "Native PNG / Core Animation · no live 3D or backend"))
            slot.widthAnchor.constraint(equalToConstant: 280).isActive = true; slot.heightAnchor.constraint(equalToConstant: 120).isActive = true; stack.addArrangedSubview(slot)
            let text = NSTextView(frame: NSRect(x: 0,y:0,width:620,height:1200)); text.string = (1...60).map { "Playback fixture paragraph \($0). Editable native text." }.joined(separator:"\n\n")
            let scroll = NSScrollView(); scroll.hasVerticalScroller = true; scroll.documentView = text; stack.addArrangedSubview(scroll)
            window.contentView = stack; window.center(); window.makeKeyAndOrderFront(nil); NSApp.activate(ignoringOtherApps: true)
            task = Task { [weak self] in
                guard let self else { return }
                do { try await run() } catch { results.append(["error":String(describing:error)]) }
                canvas?.detach(); second?.detach(); canvas = nil; second = nil
                DispatchQueue.main.async { [weak self] in
                    guard let self else { return }; task = nil
                    RunLoop.main.perform { [weak self] in MainActor.assumeIsolated { self?.finish() } }
                }
            }
        } catch { print(error); NSApp.terminate(nil) }
    }
    func metric() throws -> [String: Any] {
        var usage = rusage(); getrusage(RUSAGE_SELF, &usage)
        var info = task_vm_info_data_t(), count = mach_msg_type_number_t(MemoryLayout<task_vm_info_data_t>.size / MemoryLayout<integer_t>.size)
        let status = withUnsafeMutablePointer(to: &info) { p in p.withMemoryRebound(to: integer_t.self, capacity: Int(count)) { task_info(mach_task_self_, task_flavor_t(TASK_VM_INFO), $0, &count) } }
        guard status == KERN_SUCCESS else { throw CocoaError(.coderReadCorrupt) }
        return ["time":Date().timeIntervalSince(start),"phase":phase,"rss":info.resident_size,"footprint":info.phys_footprint,"graphics":info.ledger_tag_graphics_footprint,"peakFootprint":info.ledger_phys_footprint_peak,"peakRSS":usage.ru_maxrss,"cpu":Double(usage.ru_utime.tv_sec + usage.ru_stime.tv_sec) + Double(usage.ru_utime.tv_usec + usage.ru_stime.tv_usec)/1e6]
    }
    func record() async throws -> [String: Any] {
        var row = try metric(); row["cacheBytes"] = await canvas?.player.store.bytes ?? 0; row["activeFrameBytes"] = canvas?.player.activeFrameBytes ?? 0
        row["animationStarts"] = (canvas?.player.animationStarts ?? 0) + (second?.player.animationStarts ?? 0)
        row["activeAnimations"] = canvas?.player.layer.animationKeys()?.count ?? 0
        row["retiredViewsAlive"] = retired.filter { $0.value != nil }.count
        var data = try JSONSerialization.data(withJSONObject: row); data.append(10); try samples.write(contentsOf:data); return row
    }
    func phase(_ name: String, seconds: Int, active: NativeMascotStyle? = nil) async throws {
        phase = name; try write()
        let first = try await record(); var rows: [[String:Any]] = []
        for index in 0..<seconds {
            if let active { try await show(active,.responding,revision:index) }
            try await Task.sleep(for:.seconds(1)); rows.append(try await record())
        }
        let last = try await record()
        func median(_ key: String)->Double { let a = rows.compactMap { ($0[key] as? NSNumber)?.doubleValue }.sorted(); return a[a.count/2] }
        results.append(["phase":name,"rss":median("rss"),"footprint":median("footprint"),"peakFootprint":last["peakFootprint"]!,"cpu":((last["cpu"] as! Double)-(first["cpu"] as! Double))/((last["time"] as! Double)-(first["time"] as! Double))*100,"cacheBytes":last["cacheBytes"]!,"activeAnimations":last["activeAnimations"]!]); try write()
    }
    func show(_ style: NativeMascotStyle, _ expression: NativeMascotExpression, revision: Int = 0, animate: Bool = true) async throws {
        if canvas == nil {
            let view = NativeMascotCanvas(root: assets); view.frame = NSRect(x:0,y:0,width:112,height:112); slot.addSubview(view); canvas = view
        }
        let began = Date()
        canvas?.request = NativeMascotRequest(owner:"fixture",style:style,expression:expression,revision:revision,animate:animate)
        await canvas?.player.settled()
        if animate, expression != .neutral, canvas?.player.current?.visible == true {
            guard canvas?.player.layer.animation(forKey:"gator.expression") is CAKeyframeAnimation else { throw NSError(domain:"MissingLiveAnimation",code:1) }
            animatedStyles.insert(style.rawValue); assertions += 1
        }
        results.append(["loadMilliseconds":Date().timeIntervalSince(began)*1000,"style":style.rawValue,"expression":expression.rawValue,"activeFrameBytes":canvas?.player.activeFrameBytes ?? 0])
    }
    func run() async throws {
        try await phase("host-baseline",seconds:12)
        try await show(.rendered,.neutral,animate:false); try await phase("rendered-static",seconds:10)
        try await show(.rendered,.working); try await phase("rendered-active",seconds:12,active:.rendered)
        try await show(.rendered,.neutral); try await phase("rendered-idle",seconds:10)
        try await show(.classic,.neutral,animate:false); try await phase("classic-static",seconds:8)
        try await phase("classic-active",seconds:12,active:.classic)
        for cycle in 0..<10 {
            try await show(cycle.isMultiple(of:2) ? .rendered:.classic,.attention)
            try await Task.sleep(for:.seconds(1))
            try await show(cycle.isMultiple(of:2) ? .classic:.rendered,.disconnected)
            try await Task.sleep(for:.seconds(1))
            canvas?.request?.reduceMotion = true; await canvas?.player.settled()
            try await Task.sleep(for:.seconds(1))
            if cycle.isMultiple(of:3) {
                let view = NativeMascotCanvas(root: assets); view.frame = NSRect(x:145,y:20,width:56,height:56); slot.addSubview(view); second = view
                view.request = NativeMascotRequest(owner:"other-surface",style:.classic,expression:.working); await view.player.settled()
            }
            canvas?.request?.visible = false; second?.request?.visible = false
            window.orderOut(nil); await canvas?.player.settled(); await second?.player.settled()
            try await phase("hidden-\(cycle)",seconds:3)
            guard canvas?.player.layer.animationKeys() == nil, await canvas?.player.store.bytes == 0 else { throw CocoaError(.validationMissingMandatoryProperty) }
            assertions += 1
            retired.append(WeakCanvas(canvas)); if let second { retired.append(WeakCanvas(second)) }
            canvas?.detach(); canvas?.removeFromSuperview(); canvas = nil; second?.detach(); second?.removeFromSuperview(); second = nil
            window.makeKeyAndOrderFront(nil)
        }
        window.orderOut(nil); try await phase("all-released",seconds:15)
    }
    func finish() {
        let alive = retired.filter { $0.value != nil }.count
        if alive > 0 { results.append(["error":"Retired native views remain after owning task/run-loop teardown", "count":alive]) }
        else { assertions += 1 }
        if animatedStyles.count != 2 { results.append(["error":"Both styles need observed animations in an actual layer context", "observed":Array(animatedStyles)]) }
        else { assertions += 1 }
        results.append(["retiredViewsAliveAfterTask":alive,"owningTaskReleased":task == nil])
        try? write(done:true); try? samples.close(); NSApp.terminate(nil)
    }
    func write(done: Bool = false) throws {
        try JSONSerialization.data(withJSONObject:["pid":ProcessInfo.processInfo.processIdentifier,"phase":phase,"done":done,"elapsed":Date().timeIntervalSince(start),"assertions":assertions,"results":results],options:[.prettyPrinted,.sortedKeys]).write(to:output.appendingPathComponent("results.json"),options:.atomic)
    }
}
