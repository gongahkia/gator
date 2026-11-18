// Standalone concept studio. Fixture states only; no production app integration.
import AppKit
import SceneKit
@main @MainActor final class Preview: NSObject, NSApplicationDelegate, NSWindowDelegate {
    let stage=Stage(directions[1])
    let view=SCNView(frame:NSRect(x:0,y:0,width:360,height:360))
    var window:NSWindow!
    var pending:Task<Void,Never>?
    var reduced=NSWorkspace.shared.accessibilityDisplayShouldReduceMotion
    let label=NSTextField(labelWithString:"Fixture states — not connected to Gator")
    static func main(){let app=NSApplication.shared;let delegate=Preview();app.delegate=delegate;app.setActivationPolicy(.regular);app.run();withExtendedLifetime(delegate){}}
    func applicationDidFinishLaunching(_ note:Notification){
        window=NSWindow(contentRect:NSRect(x:0,y:0,width:470,height:470),styleMask:[.titled,.closable,.miniaturizable],backing:.buffered,defer:false)
        window.title="Gator B — isolated 3D concept";window.delegate=self;window.isReleasedWhenClosed=false
        NSLayoutConstraint.activate([view.widthAnchor.constraint(equalToConstant:400),view.heightAnchor.constraint(equalToConstant:340)])
        view.scene=stage.scene;view.pointOfView=stage.camera;view.backgroundColor = .clear;view.rendersContinuously=false;view.preferredFramesPerSecond=30;view.antialiasingMode = .multisampling4X
        let box=NSStackView();box.orientation = .vertical;box.spacing=12;box.edgeInsets=NSEdgeInsets(top:12,left:12,bottom:12,right:12);box.addArrangedSubview(view);box.addArrangedSubview(label)
        let controls=NSStackView();controls.spacing=8
        for (name,action) in [("Blink",#selector(blink)),("Jaw",#selector(jaw)),("Attention",#selector(attention)),("Neutral",#selector(neutral))]{let b=NSButton(title:name,target:self,action:action);b.bezelStyle = .rounded;controls.addArrangedSubview(b)}
        box.addArrangedSubview(controls)
        let reducedButton=NSButton(checkboxWithTitle:"Reduced motion / static pose",target:self,action:#selector(reduce(_:)));reducedButton.state=reduced ? .on:.off;box.addArrangedSubview(reducedButton)
        window.contentView=box;window.center();window.makeKeyAndOrderFront(nil);NSApp.activate(ignoringOtherApps:true)
    }
    func pose(_ open:CGFloat=0,_ blink:CGFloat=0,_ tilt:CGFloat=0){
        SCNTransaction.begin();SCNTransaction.animationDuration=reduced ? 0:0.18;stage.character.pose(open:open,blink:blink,tilt:reduced ? 0:tilt);SCNTransaction.commit()
    }
    @objc func blink(){
        pending?.cancel();guard !reduced else{return};pose(0,1)
        pending=Task{@MainActor [weak self] in
            do{try await Task.sleep(for:.milliseconds(180))}catch{return}
            guard let self,self.window.isVisible,!self.window.isMiniaturized else{return};self.pose()
        }
    }
    @objc func jaw(){pending?.cancel();pose(0.75);label.stringValue="Jaw inspection — no audio or speech synchronization"}
    @objc func attention(){pending?.cancel();pose(0.12,0,-0.06);label.stringValue="Attention fixture — grants no authority"}
    @objc func neutral(){pending?.cancel();pose();label.stringValue="Neutral fixture — no continuous animation"}
    @objc func reduce(_ sender:NSButton){reduced=sender.state == .on;neutral()}
    func applicationDidResignActive(_ notification:Notification){
        pending?.cancel();SCNTransaction.begin();SCNTransaction.disableActions=true;stage.character.pose(open:0,blink:0);SCNTransaction.commit();stage.scene.isPaused=true
    }
    func applicationDidBecomeActive(_ notification:Notification){stage.scene.isPaused=false}
    func windowWillMiniaturize(_ notification:Notification){pending?.cancel();stage.scene.isPaused=true}
    func windowDidDeminiaturize(_ notification:Notification){stage.scene.isPaused=false;neutral()}
    func windowWillClose(_ notification:Notification){pending?.cancel();view.scene=nil;NSApp.terminate(nil)}
    func applicationShouldTerminateAfterLastWindowClosed(_ sender:NSApplication)->Bool{true}
}
