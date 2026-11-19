// an inert second application for pointer/focus/fullscreen checks, never an agent
import AppKit

@main @MainActor final class FocusFixture:NSObject,NSApplicationDelegate,NSWindowDelegate {
    var window:NSWindow!,root:NSView!,target:NSButton!,input:NSTextField!,secure:NSSecureTextField!
    var output:URL!
    var clicks=0,fullScreen=false
    var tokens:[NSObjectProtocol]=[]
    var targetFrame:CGRect?
    static func main(){let app=NSApplication.shared,owner=FocusFixture();app.delegate=owner;app.setActivationPolicy(.regular);app.run();withExtendedLifetime(owner){}}
    func applicationDidFinishLaunching(_ notification:Notification){
        let args=CommandLine.arguments;guard args.count==2,args[1].hasPrefix("/tmp/gator-") else{NSApp.terminate(nil);return}
        output=URL(fileURLWithPath:args[1]);try? FileManager.default.createDirectory(at:output,withIntermediateDirectories:true)
        guard let screen=PetScreens.read().first(where:{$0.builtIn}) ?? PetScreens.read().first,let pet=PetGeometry.position(on:screen,placement:.reserved) else{NSApp.terminate(nil);return}
        targetFrame=pet.insetBy(dx:-12,dy:-12)
        window=NSWindow(contentRect:CGRect(x:pet.midX-240,y:max(screen.usable.minY+20,pet.midY-180),width:480,height:340),styleMask:[.titled,.closable,.resizable,.miniaturizable],backing:.buffered,defer:false)
        window.title="Inert Focus Fixture";window.identifier=NSUserInterfaceItemIdentifier("peer");window.isReleasedWhenClosed=false;window.delegate=self;window.collectionBehavior=[.fullScreenPrimary]
        root=NSView(frame:CGRect(x:0,y:0,width:480,height:340));window.contentView=root
        target=NSButton(title:"Pointer target",target:self,action:#selector(clicked));target.bezelStyle = .regularSquare;target.setAccessibilityIdentifier("peer.target");root.addSubview(target)
        input=NSTextField(string:"");input.placeholderString="Inert text input";input.setAccessibilityIdentifier("peer.input");input.frame=CGRect(x:20,y:76,width:440,height:28);root.addSubview(input)
        secure=NSSecureTextField(string:"");secure.placeholderString="Empty secure field — do not enter a real secret";secure.setAccessibilityIdentifier("peer.secure");secure.frame=CGRect(x:20,y:40,width:440,height:28);root.addSubview(secure)
        for (i,pair) in [("Focus text",#selector(focusText)),("Focus secure",#selector(focusSecure)),("Toggle Fullscreen",#selector(toggle))].enumerated(){let b=NSButton(title:pair.0,target:self,action:pair.1);b.bezelStyle = .rounded;b.frame=CGRect(x:20+i*148,y:4,width:142,height:28);root.addSubview(b)}
        for name in [NSWindow.didEnterFullScreenNotification,NSWindow.didExitFullScreenNotification]{tokens.append(NotificationCenter.default.addObserver(forName:name,object:window,queue:.main){[weak self] notification in let entered=notification.name==NSWindow.didEnterFullScreenNotification;MainActor.assumeIsolated{self?.fullScreen=entered;self?.write()}})}
        placeTarget();window.makeKeyAndOrderFront(nil);NSApp.activate(ignoringOtherApps:true);write()
    }
    func placeTarget(){if let targetFrame{target.frame=root.convert(window.convertFromScreen(targetFrame),from:nil)}}
    func windowDidResize(_ notification:Notification){placeTarget()}
    @objc func clicked(){clicks+=1;write()}
    @objc func focusText(){window.makeFirstResponder(input);write()}
    @objc func focusSecure(){window.makeFirstResponder(secure);write()}
    @objc func toggle(){window.toggleFullScreen(nil)}
    func windowDidBecomeKey(_ notification:Notification){write()}
    func write(){guard let output,let window,let target else{return};let r=window.convertToScreen(root.convert(target.frame,to:nil));let data:[String:Any]=["pid":getpid(),"clicks":clicks,"fullScreen":fullScreen,"key":window.isKeyWindow,"active":NSApp.isActive,"target":["x":r.minX,"y":r.minY,"width":r.width,"height":r.height]];try? JSONSerialization.data(withJSONObject:data,options:[.prettyPrinted,.sortedKeys]).write(to:output.appendingPathComponent("peer.json"),options:.atomic)}
    func applicationWillTerminate(_ notification:Notification){for token in tokens{NotificationCenter.default.removeObserver(token)}}
    func applicationShouldTerminateAfterLastWindowClosed(_ sender:NSApplication)->Bool{true}
}
