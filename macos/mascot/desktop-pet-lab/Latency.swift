import AppKit
@main @MainActor final class PetLatency:NSObject,NSApplicationDelegate {
    var owner:PetPanelController?
    var output:URL!
    var task:Task<Void,Never>?
    static func main(){let app=NSApplication.shared,delegate=PetLatency();app.delegate=delegate;app.setActivationPolicy(.accessory);app.run();withExtendedLifetime(delegate){}}
    func applicationDidFinishLaunching(_ notification:Notification){
        let args=CommandLine.arguments;guard args.count==3 else{NSApp.terminate(nil);return};output=URL(fileURLWithPath:args[2])
        owner=PetPanelController(assets:URL(fileURLWithPath:args[1]));owner?.screen=PetScreens.read().first(where:{$0.builtIn}) ?? PetScreens.read().first
        task=Task{[weak self] in guard let self else{return};var results:[[String:Any]]=[]
            do{
                guard !NSWorkspace.shared.accessibilityDisplayShouldReduceMotion else{throw CocoaError(.userCancelled)}
                let start=ContinuousClock.now;owner?.show(.init(owner:"latency-fixture",style:.rendered,expression:.neutral,animate:false));try await readyImage()
                results.append(["phase":"first-show-through-image-ready","milliseconds":milliseconds(start.duration(to:.now))])
                for style in NativeMascotStyle.allCases{
                    for index in 0..<10{
                        owner?.update(.init(owner:"latency-fixture",style:style,expression:.neutral,animate:false));await owner?.canvas?.player.settled()
                        let start=ContinuousClock.now;owner?.update(.init(owner:"latency-fixture",style:style,expression:.working,revision:index));await owner?.canvas?.player.settled()
                        guard owner?.canvas?.player.layer.animationKeys()?.isEmpty==false else{throw CocoaError(.coderReadCorrupt)}
                        results.append(["phase":"gesture-decode-and-submit","style":style.rawValue,"milliseconds":milliseconds(start.duration(to:.now))])
                    }
                }
                try JSONSerialization.data(withJSONObject:["results":results,"OSReduceMotion":NSWorkspace.shared.accessibilityDisplayShouldReduceMotion],options:[.prettyPrinted,.sortedKeys]).write(to:output)
            }catch{try? JSONSerialization.data(withJSONObject:["error":String(describing:error)]).write(to:output)}
            await owner?.destroy();owner=nil;NSApp.terminate(nil)
        }
    }
    func readyImage() async throws{
        let deadline=ContinuousClock.now.advanced(by:.seconds(4))
        while owner?.canvas?.player.layer.contents==nil && ContinuousClock.now<deadline{try await Task.sleep(for:.milliseconds(10))}
        guard owner?.canvas?.player.layer.contents != nil else{throw CocoaError(.coderReadCorrupt)}
    }
    func milliseconds(_ d:Duration)->Double{Double(d.components.seconds)*1000+Double(d.components.attoseconds)/1e15}
}
