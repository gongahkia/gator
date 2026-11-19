import AppKit
import Darwin

private final class ObserverBag {
    var entries:[(NotificationCenter,NSObjectProtocol)]=[]
    deinit{for (center,token) in entries{center.removeObserver(token)}}
}
private final class WeakProbe {
    weak var value:AnyObject?
    let kind:String
    init(_ value:AnyObject?,_ kind:String){self.value=value;self.kind=kind}
}
@objc(GatorPetLabApplication) @MainActor final class PetLabApplication:NSApplication {
    var nativeEventCompleted:(()->Void)?
    override func sendEvent(_ event:NSEvent){
        super.sendEvent(event)
        if (event.type == .applicationDefined && event.subtype.rawValue == 177) || (event.type == .mouseMoved && event.eventNumber == 177) {
            let callback=nativeEventCompleted;nativeEventCompleted=nil
            DispatchQueue.main.async{callback?()}
        }
    }
}
@main @MainActor final class PetLab:NSObject,NSApplicationDelegate,NSWindowDelegate {
    var control:NSWindow!
    let slot=NSView()
    var pet:PetPanelController?
    var inChat:NativeMascotCanvas?
    var policy=PetPolicy()
    var reducer=NativeMascotStateMachine()
    var signal=NativeMascotSignal(owner:"fixture-selected-A",connected:true)
    var style=NativeMascotStyle.rendered
    var mode=PetInteraction.passive
    var placement=PetPlacement.reserved
    var preferred:String?
    var screens:[PetScreen]=[]
    private var observers:ObserverBag?
    var statusItem:NSStatusItem?
    var ownedTask:Task<Void,Never>?
    var trial=false,terminating=false,mayTerminate=false
    var phase="interactive"
    var output:URL!
    var stream:FileHandle!
    var events:[[String:Any]]=[]
    var summaries:[[String:Any]]=[]
    private var retired:[WeakProbe]=[]
    var assertions=0
    let began=Date()
    var desktopCheck:NSButton!,chatCheck:NSButton!,animateCheck:NSButton!,reduceCheck:NSButton!,sleepCheck:NSButton!
    var styles:NSPopUpButton!,modes:NSPopUpButton!,positions:NSPopUpButton!,displays:NSPopUpButton!
    var peerPID:Int32?
    var request:NativeMascotRequest{.init(owner:signal.owner,style:style,expression:reducer.expression,revision:reducer.revision,animate:policy.animated,reduceMotion:policy.reduceMotion || NSWorkspace.shared.accessibilityDisplayShouldReduceMotion,visible:policy.shouldShow)}
    var assets:URL{Bundle.main.resourceURL!.appendingPathComponent("Mascot")}
    static func main(){let app=PetLabApplication.shared,lab=PetLab();app.delegate=lab;app.setActivationPolicy(.regular);app.run();withExtendedLifetime(lab){}}
    func arg(_ key:String)->String?{let a=CommandLine.arguments;guard let i=a.firstIndex(of:key),i+1<a.count else{return nil};return a[i+1]}
    func applicationDidFinishLaunching(_ notification:Notification){
        do{
            output=URL(fileURLWithPath:arg("--output") ?? "/tmp/gator-pet-b1-interactive")
            guard output.path.hasPrefix("/tmp/gator-") || output.path.hasPrefix("/private/tmp/gator-") else{throw CocoaError(.fileWriteNoPermission)}
            try FileManager.default.createDirectory(at:output,withIntermediateDirectories:true)
            let samples=output.appendingPathComponent("samples.ndjson");FileManager.default.createFile(atPath:samples.path,contents:nil);stream=try FileHandle(forWritingTo:samples)
            peerPID=arg("--peer-pid").flatMap(Int32.init)
            style=NativeMascotStyle(rawValue:arg("--style") ?? "rendered") ?? .rendered
            mode=arg("--mode")=="interactive" ? .interactive:.passive
            placement=arg("--placement")=="A" ? .menu:arg("--placement")=="C" ? .offset:.reserved
            policy.enabled=CommandLine.arguments.contains("--show");policy.animated = !CommandLine.arguments.contains("--static");policy.reduceMotion=CommandLine.arguments.contains("--reduce-motion")
            screens=PetScreens.read();preferred=screens.first(where:{$0.builtIn})?.id ?? screens.first?.id
            reducer.receive(signal);buildControls();buildMenus();installObservers();syncControls()
            trial=CommandLine.arguments.contains("--trial")
            if trial || CommandLine.arguments.contains("--locked"){lockTestControls(control.contentView)}
            if trial{ownedTask=Task{[weak self] in
                guard let self else{return}
                do{try await runTrial()}catch{events.append(["error":String(describing:error),"phase":phase])}
                await teardown()
                guard !terminating else{return}
                DispatchQueue.main.async{[weak self] in
                    guard let self else{return};ownedTask=nil
                    RunLoop.main.perform{[weak self] in MainActor.assumeIsolated{self?.finishTrial()}}
                }
            }}
            else{
                try write()
                if CommandLine.arguments.contains("--demo"){ownedTask=Task{[weak self] in
                    guard let self else{return}
                    do{idle();try await Task.sleep(for:.seconds(3));working();try await Task.sleep(for:.seconds(2));for _ in 0..<9{responding();try await Task.sleep(for:.seconds(1))};completed();try await Task.sleep(for:.seconds(2));attention();try await Task.sleep(for:.seconds(2));disconnected()}catch is CancellationError{}catch{event("demo-error")}
                }}
            }
        }catch{print(error);NSApp.terminate(nil)}
    }
    func row(_ views:[NSView])->NSStackView{let r=NSStackView(views:views);r.orientation = .horizontal;r.spacing=10;r.alignment = .centerY;return r}
    func button(_ title:String,_ action:Selector)->NSButton{let b=NSButton(title:title,target:self,action:action);b.bezelStyle = .rounded;return b}
    func check(_ title:String,_ id:String)->NSButton{let b=NSButton(checkboxWithTitle:title,target:self,action:#selector(change));b.identifier=NSUserInterfaceItemIdentifier(id);b.setAccessibilityIdentifier(id);return b}
    func popup(_ values:[String],_ id:String)->NSPopUpButton{let b=NSPopUpButton();b.addItems(withTitles:values);b.target=self;b.action=#selector(change);b.setAccessibilityIdentifier(id);return b}
    func buildControls(){
        control=NSWindow(contentRect:CGRect(x:0,y:0,width:780,height:540),styleMask:[.titled,.closable,.miniaturizable,.resizable],backing:.buffered,defer:false)
        control.title="Gator Desktop Pet Lab";control.identifier=NSUserInterfaceItemIdentifier("controls");control.isReleasedWhenClosed=false;control.delegate=self
        let stack=NSStackView();stack.orientation = .vertical;stack.spacing=14;stack.edgeInsets=NSEdgeInsets(top:20,left:20,bottom:20,right:20)
        stack.addArrangedSubview(NSTextField(labelWithString:"ISOLATED PROTOTYPE · fixture signals only · no Go process or execution authority"))
        desktopCheck=check("Show Desktop Pet","lab.desktop");chatCheck=check("Show in-chat fixture","lab.inchat");animateCheck=check("Animate","lab.animate");reduceCheck=check("Simulate Reduce Motion","lab.reduce");sleepCheck=check("Simulate sleeping display","lab.sleep")
        stack.addArrangedSubview(row([desktopCheck,chatCheck,animateCheck,button("Tuck Away",#selector(tuck))]))
        styles=popup(NativeMascotStyle.allCases.map(\.label),"lab.style");modes=popup(PetInteraction.allCases.map(\.rawValue),"lab.mode");positions=popup(PetPlacement.allCases.map(\.rawValue),"lab.placement")
        stack.addArrangedSubview(row([styles,modes,positions]));displays=popup([],"lab.display");refreshDisplays();stack.addArrangedSubview(row([NSTextField(labelWithString:"Fixed display:"),displays,reduceCheck,sleepCheck]))
        let states=row([button("Idle",#selector(idle)),button("Working",#selector(working)),button("Responding",#selector(responding)),button("Completed",#selector(completed)),button("Attention",#selector(attention)),button("Disconnected",#selector(disconnected)),button("Other owner",#selector(otherOwner))]);stack.addArrangedSubview(states)
        slot.widthAnchor.constraint(equalToConstant:112).isActive=true;slot.heightAnchor.constraint(equalToConstant:112).isActive=true;stack.addArrangedSubview(slot)
        let input=NSTextField(string:"");input.placeholderString="Keyboard fixture — nothing is sent";input.setAccessibilityIdentifier("lab.input");input.widthAnchor.constraint(equalToConstant:640).isActive=true;stack.addArrangedSubview(input)
        let note=NSTextField(wrappingLabelWithString:"Passive passes all pointer events through. Interactive uses the entire 72×72 pt panel; click opens this fixture, drag does not activate it. Menu-bar Tuck Away remains available. Desktop preference is memory-only and defaults OFF every lab launch.");note.widthAnchor.constraint(equalToConstant:700).isActive=true;stack.addArrangedSubview(note)
        control.contentView=stack;control.center()
        if CommandLine.arguments.contains("--background"){control.orderFrontRegardless()}else{control.makeKeyAndOrderFront(nil);NSApp.activate(ignoringOtherApps:true)}
        apply()
    }
    func lockTestControls(_ view:NSView?){guard let view else{return};if let field=view as? NSControl{field.isEnabled=false};for child in view.subviews{lockTestControls(child)}}
    func menuItem(_ title:String,_ selector:Selector,_ key:String="")->NSMenuItem{let item=NSMenuItem(title:title,action:selector,keyEquivalent:key);item.target=self;return item}
    func buildMenus(){
        let menu=NSMenu(),appItem=NSMenuItem(),appMenu=NSMenu();appMenu.addItem(menuItem("Show Controls",#selector(openControls),"0"));appMenu.addItem(menuItem("Tuck Away Desktop Pet",#selector(tuck),"H"));appMenu.addItem(menuItem("Quit Pet Lab",#selector(quit),"q"));appItem.submenu=appMenu;menu.addItem(appItem);NSApp.mainMenu=menu
        let item=NSStatusBar.system.statusItem(withLength:NSStatusItem.squareLength);item.button?.image=NSImage(contentsOf:Bundle.main.resourceURL!.appendingPathComponent("GatorMenuBar.png"));item.button?.image?.size=NSSize(width:18,height:18);item.button?.image?.isTemplate=true;item.button?.setAccessibilityLabel("Gator pet lab controls")
        let controls=NSMenu();controls.addItem(menuItem("Show Controls",#selector(openControls)));controls.addItem(menuItem("Enable Desktop Pet",#selector(enable)));controls.addItem(menuItem("Tuck Away Desktop Pet",#selector(tuck)));controls.addItem(menuItem("Quit Pet Lab",#selector(quit)));item.menu=controls;statusItem=item
    }
    func installObservers(){
        let bag=ObserverBag()
        func observe(_ center:NotificationCenter,_ name:Notification.Name,_ callback:@escaping @MainActor ()->Void){
            let token=center.addObserver(forName:name,object:nil,queue:.main){_ in MainActor.assumeIsolated{callback()}};bag.entries.append((center,token))
        }
        observe(.default,NSApplication.didChangeScreenParametersNotification){[weak self] in
            guard let self else{return};screens=PetScreens.read();refreshDisplays();apply(reposition:true);event("screen-parameters")
        }
        let workspace=NSWorkspace.shared.notificationCenter
        observe(workspace,NSWorkspace.accessibilityDisplayOptionsDidChangeNotification){[weak self] in self?.apply();self?.event("system-motion-options")}
        observe(workspace,NSWorkspace.activeSpaceDidChangeNotification){[weak self] in self?.pet?.canvas?.refreshVisibility();self?.event("space-changed")}
        observe(workspace,NSWorkspace.screensDidSleepNotification){[weak self] in self?.policy.temporarilySuppressed=true;self?.apply();self?.event("screens-sleep")}
        observe(workspace,NSWorkspace.screensDidWakeNotification){[weak self] in self?.policy.temporarilySuppressed=false;self?.apply(reposition:true);self?.event("screens-wake")}
        observers=bag
    }
    func refreshDisplays(){displays?.removeAllItems();displays?.addItems(withTitles:screens.enumerated().map{"\($0.offset+1): \($0.element.builtIn ? "Built-in":"External") · \(Int($0.element.frame.width))×\(Int($0.element.frame.height))"});if let index=screens.firstIndex(where:{$0.id==preferred}){displays?.selectItem(at:index)}}
    func syncControls(){desktopCheck.state=policy.enabled ? .on:.off;chatCheck.state=policy.inChat ? .on:.off;animateCheck.state=policy.animated ? .on:.off;reduceCheck.state=policy.reduceMotion ? .on:.off;sleepCheck.state=policy.temporarilySuppressed ? .on:.off;styles.selectItem(at:style == .rendered ? 0:1);modes.selectItem(at:mode == .passive ? 0:1);positions.selectItem(at:PetPlacement.allCases.firstIndex(of:placement) ?? 1)}
    @objc func change(){
        policy.enabled=desktopCheck.state == .on;policy.inChat=chatCheck.state == .on;policy.animated=animateCheck.state == .on;policy.reduceMotion=reduceCheck.state == .on;policy.temporarilySuppressed=sleepCheck.state == .on
        style=styles.indexOfSelectedItem==0 ? .rendered:.classic;mode=modes.indexOfSelectedItem==0 ? .passive:.interactive;placement=PetPlacement.allCases[max(0,positions.indexOfSelectedItem)]
        if displays.indexOfSelectedItem>=0{preferred=screens[displays.indexOfSelectedItem].id};apply(reposition:true);event("settings")
    }
    func apply(reposition:Bool=false){
        if policy.shouldShow{
            if pet == nil{let owner=PetPanelController(assets:assets);owner.open={ [weak self] in self?.openControls() };owner.tuck={ [weak self] in self?.tuck() };owner.record={ [weak self] name in self?.event(name) };pet=owner}
            pet?.screen=PetGeometry.select(screens,preferred:preferred);pet?.placement=placement;pet?.interaction=mode
            if reposition || pet?.panel?.isVisible != true{let front=NSWorkspace.shared.frontmostApplication?.processIdentifier;pet?.show(request);event("show-without-activation",extra:["frontUnchanged":front==NSWorkspace.shared.frontmostApplication?.processIdentifier])}else{pet?.update(request)}
        }else{pet?.hide()}
        if policy.inChat{
            if inChat == nil{let canvas=NativeMascotCanvas(root:assets);canvas.frame=CGRect(x:0,y:0,width:112,height:112);slot.addSubview(canvas);inChat=canvas}
            var chat=request;chat.visible=true;inChat?.request=chat
        }else{if let inChat{retired.append(WeakProbe(inChat,"chatCanvas"));inChat.detach();inChat.removeFromSuperview()};inChat=nil}
    }
    func setSignal(_ new:NativeMascotSignal){signal=new;reducer.receive(new);apply();event("fixture-signal")}
    @objc func idle(){setSignal(.init(owner:signal.owner,connected:true))}
    @objc func working(){setSignal(.init(owner:signal.owner,connected:true,working:true))}
    @objc func responding(){setSignal(.init(owner:signal.owner,connected:true,working:true,publicTextRevision:signal.publicTextRevision+1))}
    @objc func completed(){setSignal(.init(owner:signal.owner,connected:true,completedRun:UUID().uuidString))}
    @objc func attention(){setSignal(.init(owner:signal.owner,connected:true,working:true,attention:true))}
    @objc func disconnected(){setSignal(.init(owner:signal.owner,connected:false))}
    @objc func otherOwner(){setSignal(.init(owner:signal.owner.hasSuffix("A") ? "fixture-selected-B":"fixture-selected-A",connected:true))}
    @objc func enable(){policy.enabled=true;syncControls();apply();event("explicit-enable")}
    @objc func tuck(){policy.tuck();syncControls();apply();event("explicit-tuck")}
    @objc func openControls(){control.makeKeyAndOrderFront(nil);NSApp.activate(ignoringOtherApps:true)}
    @objc func quit(){NSApp.terminate(nil)}
    func rect(_ r:CGRect)->[String:Double]{["x":r.minX,"y":r.minY,"width":r.width,"height":r.height]}
    func event(_ name:String,extra:[String:Any]=[:]){var row: [String:Any]=["event":name,"elapsed":Date().timeIntervalSince(began),"phase":phase,"petEnabled":policy.enabled,"petVisible":pet?.panel?.isVisible == true,"petKey":pet?.panel?.isKeyWindow == true,"labActive":NSApp.isActive,"expression":reducer.expression.rawValue];for(k,v)in extra{row[k]=v};events.append(row);try? write()}
    func metric() async throws->[String:Any]{
        var usage=rusage();getrusage(RUSAGE_SELF,&usage);var info=task_vm_info_data_t();var count=mach_msg_type_number_t(MemoryLayout<task_vm_info_data_t>.size/MemoryLayout<integer_t>.size)
        let code=withUnsafeMutablePointer(to:&info){p in p.withMemoryRebound(to:integer_t.self,capacity:Int(count)){task_info(mach_task_self_,task_flavor_t(TASK_VM_INFO),$0,&count)}};guard code==KERN_SUCCESS else{throw CocoaError(.coderReadCorrupt)}
        let row:[String:Any]=["elapsed":Date().timeIntervalSince(began),"phase":phase,"RSS":info.resident_size,"footprint":info.phys_footprint,"peakFootprint":info.ledger_phys_footprint_peak,"graphics":info.ledger_tag_graphics_footprint,"cpu":Double(usage.ru_utime.tv_sec+usage.ru_stime.tv_sec)+Double(usage.ru_utime.tv_usec+usage.ru_stime.tv_usec)/1e6,"cacheBytes":await pet?.canvas?.player.store.bytes ?? 0,"activeFrameBytes":pet?.canvas?.player.activeFrameBytes ?? 0,"animationKeys":pet?.canvas?.player.layer.animationKeys()?.count ?? 0,"starts":pet?.canvas?.player.animationStarts ?? 0,"petVisible":pet?.panel?.isVisible == true,"petOccluded":pet?.panel?.occlusionState.contains(.visible) != true,"labActive":NSApp.isActive,"retiredAlive":retired.filter{$0.value != nil}.count]
        var data=try JSONSerialization.data(withJSONObject:row);data.append(10);try stream.write(contentsOf:data);return row
    }
    func samplePhase(_ name:String,_ seconds:Int,pulses:Bool=false) async throws{
        phase=name;event("phase");let first=try await metric();var rows:[[String:Any]]=[]
        for _ in 0..<seconds{try Task.checkCancellation();if pulses{responding()};try await Task.sleep(for:.seconds(1));rows.append(try await metric())}
        let last=try await metric();func median(_ key:String)->Double{let values=rows.compactMap{($0[key] as? NSNumber)?.doubleValue}.sorted();return values[values.count/2]}
        summaries.append(["phase":name,"RSS":median("RSS"),"footprint":median("footprint"),"peakFootprint":last["peakFootprint"]!,"CPUPercent":((last["cpu"] as! Double)-(first["cpu"] as! Double))/((last["elapsed"] as! Double)-(first["elapsed"] as! Double))*100]);try write()
    }
    func verify(_ condition:Bool,_ text:String)throws{guard condition else{throw NSError(domain:"PetLab",code:assertions,userInfo:[NSLocalizedDescriptionKey:text])};assertions+=1}
    func releasePet() async{
        guard let pet else{return};retired += [WeakProbe(pet,"controller"),WeakProbe(pet.panel,"panel"),WeakProbe(pet.canvas,"canvas"),WeakProbe(pet.canvas?.player,"player"),WeakProbe(pet.hit,"hit")]
        await pet.destroy();self.pet=nil
    }
    func runTrial() async throws{
        if CommandLine.arguments.contains("--lifetime-only"){
            policy.inChat=false;apply()
            if let peerPID,let app=NSRunningApplication(processIdentifier:peerPID),app.bundleIdentifier=="dev.gator.pet.focus-fixture"{app.activate(options:[.activateAllWindows])}
            enable();await pet?.canvas?.player.settled();try await lifetimeCycles();return
        }
        policy.inChat=false;policy.enabled=false;syncControls();apply();try await samplePhase("hidden-host-baseline",12)
        policy.enabled=true;policy.animated=false;apply();await pet?.canvas?.player.settled();try await samplePhase("visible-static",10)
        policy.animated=true;idle();try await samplePhase("visible-idle-including-blink",14)
        working();await pet?.canvas?.player.settled();try verify(pet?.canvas?.player.layer.animationKeys()?.isEmpty==false,"visible working clip attached");try await samplePhase("working-finite",8)
        try await samplePhase("rendered-responding",12,pulses:true)
        style = .classic;apply();try await samplePhase("classic-responding",12,pulses:true)
        policy.animated=false;attention();await pet?.canvas?.player.settled();try verify(pet?.canvas?.player.layer.animationKeys()==nil,"animation disabled");try await samplePhase("animation-disabled",6)
        policy.animated=true;policy.reduceMotion=true;working();await pet?.canvas?.player.settled();try verify(pet?.canvas?.player.layer.animationKeys()==nil,"Reduce Motion");try await samplePhase("reduced-motion",6)
        tuck();await pet?.canvas?.player.settled();let starts=pet?.canvas?.player.animationStarts ?? 0
        try await samplePhase("tucked-away",12,pulses:true);await pet?.canvas?.player.settled()
        let tuckedBytes=await pet?.canvas?.player.store.bytes ?? 0
        try verify(pet?.canvas?.player.layer.animationKeys()==nil && tuckedBytes==0,"hidden target and cache cleared")
        try verify(pet?.canvas?.player.animationStarts==starts && !policy.enabled,"signals cannot wake tucked pet")
        enable();policy.reduceMotion=false;policy.inChat=true;apply();try await samplePhase("pet-and-inchat",10,pulses:true)
        policy.inChat=false;apply()
        if let peerPID,let app=NSRunningApplication(processIdentifier:peerPID),app.bundleIdentifier=="dev.gator.pet.focus-fixture"{
            app.activate(options:[.activateAllWindows]);try await samplePhase("inactive-pet",8,pulses:true)
            try verify(!NSApp.isActive && pet?.panel?.isKeyWindow==false,"pet never activates on animation")
        }
        try await lifetimeCycles()
    }
    func lifetimeCycles() async throws{
        for cycle in 0..<12{
            phase="lifetime-\(cycle)";tuck();await pet?.canvas?.player.settled();try verify(await pet?.canvas?.player.store.bytes==0,"tuck releases cache")
            await releasePet();enable();style=cycle.isMultiple(of:2) ? .rendered:.classic;working();await pet?.canvas?.player.settled()
            try verify(pet?.panel?.canBecomeKey==false && pet?.panel?.canBecomeMain==false,"panel cannot take focus")
            if let screen=PetGeometry.select(screens,preferred:preferred){for item in PetPlacement.allCases{placement=item;apply(reposition:true);try verify(screen.usable.contains(pet!.panel!.frame),"geometry remains within usable bounds")}}
            try await Task.sleep(for:.seconds(1));_=try await metric()
        }
        tuck();await releasePet();try await samplePhase("all-pet-owners-released",12)
    }
    func teardown() async{policy.enabled=false;policy.inChat=false;apply();await releasePet();observers=nil}
    func finishTrial(){
        let before=retired.filter{$0.value != nil}.map(\.kind)
        events.append(["event":"after-owning-task-release-before-native-event","survivors":before,"currentEventWindow":NSApp.currentEvent?.windowNumber ?? -1,"applicationWindows":NSApp.windows.map(\.windowNumber)])
        // dispatch one marked event through the native application loop before
        // probing AppKit's last-event window reference; this never intercepts global input.
        guard let app=NSApp as? PetLabApplication else{events.append(["error":"native event probe app class missing"]);finishAfterNativeEvent();return}
        app.nativeEventCompleted={ [weak self] in self?.finishAfterNativeEvent() }
        if let event=NSEvent.mouseEvent(with:.mouseMoved,location:.zero,modifierFlags:[],timestamp:ProcessInfo.processInfo.systemUptime,windowNumber:control.windowNumber,context:nil,eventNumber:177,clickCount:0,pressure:0){app.postEvent(event,atStart:false)}
    }
    func finishAfterNativeEvent(){
        let survivors=retired.filter{$0.value != nil}.map(\.kind)
        events.append(["event":"after-native-event-turn","survivors":survivors,"currentEventWindow":NSApp.currentEvent?.windowNumber ?? -1])
        if !survivors.isEmpty{events.append(["error":"retained test subjects"])}
        try? write(done:true);try? stream.close();NSApp.terminate(nil)
    }
    func write(done:Bool=false)throws{
        let values:[String:Any]=["pid":getpid(),"done":done,"elapsed":Date().timeIntervalSince(began),"phase":phase,"assertions":assertions,"events":events,"summaries":summaries,"screens":screens.map{["id":$0.id,"builtIn":$0.builtIn,"frame":rect($0.frame),"visible":rect($0.visible),"safeTop":$0.safeTop,"scale":$0.scale,"cameraGap":$0.cameraGap.map{ $0 as Any } ?? NSNull(),"quickAccess":$0.quickAccess.map{rect($0) as Any} ?? NSNull()]},"petFrame":pet?.panel.map{rect($0.frame) as Any} ?? NSNull(),"panelLevel":pet?.panel.map{$0.level.rawValue as Any} ?? NSNull(),"desktopEnabled":policy.enabled,"style":style.rawValue,"mode":mode.rawValue,"placement":placement.rawValue]
        // JSON nulls replace absent optional diagnostics, never raw application data.
        let data=try JSONSerialization.data(withJSONObject:values,options:[.prettyPrinted,.sortedKeys]);try data.write(to:output.appendingPathComponent("state.json"),options:.atomic)
    }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender:NSApplication)->Bool{false}
    func applicationShouldTerminate(_ sender:NSApplication)->NSApplication.TerminateReply{
        if mayTerminate{return .terminateNow}
        guard !terminating else{return .terminateCancel}
        terminating=true;event("quit-cleanup-start")
        let activity=ownedTask;activity?.cancel();ownedTask=nil
        Task{[weak self] in
            guard let self else{return};await activity?.value;await teardown()
            if let statusItem{NSStatusBar.system.removeStatusItem(statusItem)};self.statusItem=nil
            event("quit-cleanup-finished");try? write(done:true);try? stream.close()
            mayTerminate=true;NSApp.terminate(nil)
        }
        return .terminateCancel
    }
}
