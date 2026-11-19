import AppKit
import RealityKit
import Metal
import Darwin

@main @MainActor final class DiagnosticLab: NSObject, NSApplicationDelegate {
    var window: NSWindow!
    let slot = NSView(frame: NSRect(x: 0,y: 0,width: 112,height: 112))
    let label = NSTextField(labelWithString: "Isolated final diagnostic — no Gator backend")
    let start = Date()
    let audit = LifetimeProbe()
    var device: (any MTLDevice)?
    var emptyRenderer: RealityRenderer?
    var template: Entity?
    var engine: Gate2Engine?
    var presenter: Gate2Presentation?
    var deliberatelyHeldEngine: Gate2Engine?
    var operation: Task<Void,Never>?
    var sampler: Task<Void,Never>?
    var phase = "launch"
    var framesRetired = 0
    var file: FileHandle!
    var assertionCount = 0
    var decodeCount = 0
    var retainedPoster: NSImage?
    var events: [[String:Any]] = []
    let output: URL
    let mode: String
    let policy: String
    override init() {
        func arg(_ key: String,_ fallback: String) -> String {
            guard let i = CommandLine.arguments.firstIndex(of:key),i+1<CommandLine.arguments.count else{return fallback}
            return CommandLine.arguments[i+1]
        }
        output = URL(fileURLWithPath:arg("--output","/tmp/gator-final-diagnostic"))
        mode = arg("--mode","memory");policy = arg("--policy","retained")
        super.init()
    }
    static func main() {
        let app = NSApplication.shared, delegate = DiagnosticLab()
        app.delegate = delegate;app.setActivationPolicy(.regular);app.run()
        withExtendedLifetime(delegate) {}
    }
    func applicationDidFinishLaunching(_ notification: Notification) {
        do {
            ProcessInfo.processInfo.disableAutomaticTermination("Finite isolated renderer diagnostic")
            try FileManager.default.createDirectory(at:output,withIntermediateDirectories:true)
            let path=output.appendingPathComponent("samples.ndjson")
            guard FileManager.default.createFile(atPath:path.path,contents:nil) else{throw NSError(domain:"CreateSamples",code:1)}
            file=try FileHandle(forWritingTo:path)
            window=NSWindow(contentRect:NSRect(x:0,y:0,width:540,height:250),styleMask:[.titled,.closable],backing:.buffered,defer:false)
            window.title="Gator Concept B — final diagnostic only";window.isReleasedWhenClosed=false
            let stack=NSStackView();stack.orientation = .vertical;stack.spacing=12
            stack.edgeInsets=NSEdgeInsets(top:12,left:12,bottom:12,right:12)
            stack.addArrangedSubview(label);slot.widthAnchor.constraint(equalToConstant:112).isActive=true
            slot.heightAnchor.constraint(equalToConstant:112).isActive=true;stack.addArrangedSubview(slot)
            window.contentView=stack;window.center();window.makeKeyAndOrderFront(nil)
            operation=Task{[weak self] in
                guard let self else{return}
                do {
                    sampler=Task{[weak self] in
                        while !Task.isCancelled {
                            guard let self else{return}
                            do{try sample();try await Task.sleep(for:.milliseconds(50))}catch{return}
                        }
                    }
                    if mode == "ownership"{try await ownership()}
                    else if mode == "policies"{try await policies()}
                    else{try await memory()}
                    sampler?.cancel();await sampler?.value;sampler=nil
                    try sample()
                    DispatchQueue.main.async{[weak self] in
                        guard let self else{return}
                        operation=nil
                        RunLoop.main.perform{[weak self] in
                            MainActor.assumeIsolated{self?.finishAfterOwnerTask()}
                        }
                    }
                } catch {
                    events.append(["error":String(describing:error)]);sampler?.cancel()
                    await sampler?.value;sampler=nil
                    try? write(done:true);NSApp.terminate(nil)
                }
            }
        }catch{fputs("Diagnostic startup failed: \(error)\n",stderr);NSApp.terminate(nil)}
    }
    var frames: Int{framesRetired+(engine?.renders ?? 0)+(presenter?.frames ?? 0)}
    func sample(_ event: String? = nil) throws {
        var info=task_vm_info_data_t();var count=mach_msg_type_number_t(MemoryLayout.size(ofValue:info)/MemoryLayout<integer_t>.size)
        let result=withUnsafeMutablePointer(to:&info){pointer in pointer.withMemoryRebound(to:integer_t.self,capacity:Int(count)){
            task_info(mach_task_self_,task_flavor_t(TASK_VM_INFO),$0,&count)
        }}
        guard result == KERN_SUCCESS else{throw NSError(domain:"VMInfo",code:Int(result))}
        let base=ResourceSample.take(start:start,device:device)
        var row=try JSONSerialization.jsonObject(with:JSONEncoder().encode(base)) as! [String:Any]
        row["phase"]=phase;row["frames"]=frames;row["live"]=audit.live;row["event"]=event
        row["internalBytes"]=info.internal;row["externalBytes"]=info.external;row["compressedBytes"]=info.compressed
        row["deviceBytes"]=info.device;row["graphicsFootprintBytes"]=info.ledger_tag_graphics_footprint
        row["graphicsCompressedBytes"]=info.ledger_tag_graphics_footprint_compressed
        row["graphicsNoFootprintBytes"]=info.ledger_tag_graphics_nofootprint
        row["purgeableNonvolatileBytes"]=info.ledger_purgeable_nonvolatile
        row["purgeableVolatileBytes"]=info.ledger_purgeable_volatile
        row["physicalFootprintPeakBytes"]=info.ledger_phys_footprint_peak
        row["drawableBytes"]=engine?.drawableBytes ?? presenter?.engine?.drawableBytes ?? 0
        row["pendingWork"]=presenter?.pendingWork ?? false
        var data=try JSONSerialization.data(withJSONObject:row,options:[.sortedKeys]);data.append(10);try file.write(contentsOf:data)
    }
    func checkpoint(_ name:String,seconds:Double=3) async throws {
        phase=name;label.stringValue=name
        events.append(["event":"phase","phase":name,"elapsed":Date().timeIntervalSince(start),"live":audit.live,"frames":frames])
        try sample("start");try write()
        if seconds>0{try await Task.sleep(for:.seconds(seconds))}
        try sample("end")
    }
    func loadTemplate() async throws {
        if device == nil{device=MTLCreateSystemDefaultDevice()}
        let archive=try await NativeMeshArchive.read(Bundle.main.resourceURL!.appendingPathComponent("Assets/B-native-meshes.json"))
        let t=Date();template=try await archive.entity();decodeCount+=1
        if let template{audit.track(template,kind:"immutableTemplate")}
        events.append(["event":"model-load","milliseconds":Date().timeIntervalSince(t)*1000])
    }
    func create() throws {
        guard let template,let device else{throw NSError(domain:"ResourcesUnavailable",code:1)}
        let t=Date();engine=try Gate2Engine(template:template,device:device,configuration:EngineConfiguration(pixels:96,points:96,offscreen:policy == "offscreen"),audit:audit)
        if let view=engine?.view{view.frame=slot.bounds.insetBy(dx:8,dy:8);slot.addSubview(view)}
        events.append(["event":"engine-create","milliseconds":Date().timeIntervalSince(t)*1000])
    }
    func retire() async {
        if let engine{framesRetired+=engine.renders;await engine.close()};engine=nil
    }
    func memory() async throws {
        try await checkpoint("00-before-RealityKit",seconds:12)
        device=MTLCreateSystemDefaultDevice();emptyRenderer=try RealityRenderer()
        try await checkpoint("01-empty-renderer");emptyRenderer=nil
        try await loadTemplate();try await checkpoint("02-mesh-loaded")
        for cycle in 0..<3 {
            try create();try await checkpoint("c\(cycle)-03-before-first-render",seconds:1)
            try await engine?.draw();try await checkpoint("c\(cycle)-04-after-first-render")
            try await engine?.transition(to:MascotPose(open:0.4,blink:1),steps:4)
            try await checkpoint("c\(cycle)-05-after-first-animation")
            phase="c\(cycle)-06-sustained-jaw";label.stringValue=phase;try write()
            try sample("start")
            for step in 0..<60 {
                engine?.rig?.apply(MascotPose(open:step.isMultiple(of:2) ? 0.4 : 0))
                try await engine?.draw()
                if step.isMultiple(of:15){try sample("GPU-completed-frame")}
                try await Task.sleep(for:.milliseconds(33))
            }
            try sample("end")
            try await checkpoint("c\(cycle)-07-GPU-completed",seconds:6)
            engine?.rig?.apply(MascotPose());try await engine?.draw()
            if policy == "poster"{retainedPoster=try await engine?.poster()}
            try await checkpoint("c\(cycle)-08-static-idle",seconds:6)
            await engine?.pause();try await checkpoint("c\(cycle)-09-drawables-released",seconds:4)
            await retire();try await checkpoint("c\(cycle)-10-renderer-destroyed",seconds:4)
        }
        template=nil;retainedPoster=nil;device=nil
        try await checkpoint("11-all-owned-references-released",seconds:8)
    }
    func ownership() async throws {
        try await checkpoint("00-before-RealityKit",seconds:4);try await loadTemplate();try create()
        deliberatelyHeldEngine=engine
        try await engine?.draw();retainedPoster=try await engine?.poster()
        try await checkpoint("owned-wrapper-open",seconds:0)
        await retire();try await checkpoint("closed-wrapper-still-held-by-test",seconds:0)
        guard audit.live["engine",default:0] == 1 && audit.live["RealityRenderer",default:0] == 0 else{throw NSError(domain:"ClosedWrapperAudit",code:1)}
        assertionCount+=1;deliberatelyHeldEngine=nil
        try await checkpoint("wrapper-handle-released",seconds:0)
        guard audit.live["engine",default:0] == 0 else{throw NSError(domain:"WrapperStillOwned",code:1)}
        assertionCount+=1
        guard let template,let device,let retainedPoster else{throw NSError(domain:"MissingFixture",code:1)}
        presenter=Gate2Presentation(template:template,device:device,poster:retainedPoster,audit:audit,systemReduceMotion:{false},releaseBetweenEvents:true,showPoster:{_ in},mount:{_,_,_ in {}})
        if let presenter{audit.track(presenter,kind:"presentation")}
        try await presenter?.request(.responding,configuration:EngineConfiguration(offscreen:true))
        try await presenter?.request(.hidden)
        guard presenter?.pendingWork == false else{throw NSError(domain:"PendingCompletionTask",code:1)}
        assertionCount+=1
        framesRetired+=presenter?.frames ?? 0;presenter=nil
        self.template=nil;self.device=nil;self.retainedPoster=nil
        try await checkpoint("all-subjects-released-before-test-task-return",seconds:0)
    }
    func policies() async throws {
        try await checkpoint("00-before-RealityKit",seconds:12);try await loadTemplate();try create()
        retainedPoster=try await engine?.poster();await retire()
        guard let template,let device,let poster=retainedPoster else{throw NSError(domain:"MissingPolicyResources",code:1)}
        try await checkpoint("01-warmed-static-poster",seconds:5)
        for (index,burst) in [false,true,true,false].enumerated(){
            let name="policy-\(index)-"+(burst ? "release-between-gestures":"reuse-until-idle")
            presenter=Gate2Presentation(template:template,device:device,poster:poster,audit:audit,systemReduceMotion:{false},
                releaseBetweenEvents:burst,showPoster:{[weak self] image in
                    guard let self else{return}
                    slot.subviews.forEach{$0.removeFromSuperview()}
                    let view=NSImageView(image:image);view.imageScaling = .scaleProportionallyUpOrDown;view.frame=slot.bounds;slot.addSubview(view)
                },mount:{[weak self] view,_,_ in
                    guard let self else{return {}}
                    view.frame=slot.bounds;view.alphaValue=0;slot.addSubview(view)
                    return{[weak view] in view?.alphaValue=1}
                })
            phase=name+"-active";label.stringValue=phase;try write();try sample("start")
            for _ in 0..<8{
                let started=ContinuousClock.now
                try await presenter?.request(.responding,configuration:EngineConfiguration(pixels:96,points:96))
                let deadline=started.advanced(by:.seconds(1))
                if deadline>ContinuousClock.now{try await Task.sleep(until:deadline,clock:.continuous)}
            }
            try sample("end")
            events.append(["event":"policy","policy":name,"resumeMilliseconds":presenter?.resumeMilliseconds ?? [],"created":audit.created,"frames":presenter?.frames ?? 0])
            try await presenter?.request(.idle)
            try await checkpoint(name+"-returned-static",seconds:6)
            framesRetired+=presenter?.frames ?? 0;presenter=nil
        }
        self.template=nil;self.device=nil;retainedPoster=nil
        try await checkpoint("11-all-owned-references-released",seconds:5)
    }
    func finishAfterOwnerTask() {
        do {
            phase="12-after-owner-task-and-runloop-return";try sample("ownership-resample")
            for kind in ["engine","RealityRenderer","sceneRoot","surface","layer","immutableTemplate","presentation"]{
                guard audit.live[kind,default:0] == 0 else{throw NSError(domain:"SurvivingApplicationOwner",code:1,userInfo:[NSLocalizedDescriptionKey:kind])}
                assertionCount+=1
            }
            events.append(["event":"final-after-task-release","live":audit.live,"ownedOperationTask":operation != nil,"ownedSamplerTask":sampler != nil])
            try write(done:true);try file.close();NSApp.terminate(nil)
        }catch{events.append(["error":String(describing:error)]);try? write(done:true);NSApp.terminate(nil)}
    }
    func write(done:Bool=false) throws {
        let report:[String:Any]=["pid":ProcessInfo.processInfo.processIdentifier,"mode":mode,"policy":policy,"phase":phase,"done":done,
                               "elapsed":Date().timeIntervalSince(start),"decodeCount":decodeCount,"frames":frames,"live":audit.live,
                               "created":audit.created,"assertions":assertionCount,"events":events]
        try JSONSerialization.data(withJSONObject:report,options:[.prettyPrinted,.sortedKeys]).write(to:output.appendingPathComponent("results.json"),options:.atomic)
    }
}
