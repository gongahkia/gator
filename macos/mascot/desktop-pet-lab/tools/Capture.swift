// evidence-only capture: includes only explicitly allowlisted fixture windows
import AppKit
import ScreenCaptureKit
import AVFoundation
import Synchronization
import ImageIO
import UniformTypeIdentifiers
final class PetRecordingDelegate:NSObject,SCRecordingOutputDelegate {
    let result=Mutex<String?>(nil)
    func recordingOutputDidStartRecording(_ recordingOutput:SCRecordingOutput){print("started")}
    func recordingOutputDidFinishRecording(_ recordingOutput:SCRecordingOutput){result.withLock{$0="finished"}}
    func recordingOutput(_ recordingOutput:SCRecordingOutput,didFailWithError error:any Error){result.withLock{$0=String(describing:error)}}
}
@main struct CapturePet {
    @MainActor static func main() async {
        do{
            _=NSApplication.shared
            let args=CommandLine.arguments;guard args.count>=3,CGPreflightScreenCaptureAccess() else{throw NSError(domain:"ExistingScreenPermissionRequired",code:1)}
            let destination=URL(fileURLWithPath:args[1]);let pids=args.dropFirst(2).prefix(while:{$0 != "--seconds"}).compactMap(Int32.init)
            guard !pids.isEmpty else{throw NSError(domain:"NoFixturePIDs",code:1)}
            let allowed=["dev.gator.pet.b1.lab","dev.gator.pet.focus-fixture","dev.gator.pet.b1.gator-fixture"]
            for pid in pids{guard let app=NSRunningApplication(processIdentifier:pid),let id=app.bundleIdentifier,allowed.contains(id),app.bundleURL?.resolvingSymlinksInPath().path.hasPrefix(URL(fileURLWithPath:"/tmp/").resolvingSymlinksInPath().path)==true else{throw NSError(domain:"RefuseNonfixture",code:1)}}
            let content=try await SCShareableContent.excludingDesktopWindows(true,onScreenWindowsOnly:true)
            let windows=content.windows.filter{pids.contains($0.owningApplication?.processID ?? -1) && (["Gator Desktop Pet Lab","Gator Desktop Pet — isolated","Inert Focus Fixture","Gator"].contains($0.title ?? "") || ($0.owningApplication?.bundleIdentifier=="dev.gator.pet.b1.gator-fixture" && $0.windowLayer==25))}
            guard let display=content.displays.first(where:{CGDisplayIsBuiltin($0.displayID) != 0}) ?? content.displays.first else{throw NSError(domain:"NoDisplay",code:1)}
            let filter=SCContentFilter(display:display,including:windows);filter.includeMenuBar=args.contains("--menu")
            if filter.includeMenuBar{guard let front=NSWorkspace.shared.frontmostApplication?.bundleIdentifier,allowed.contains(front) else{throw NSError(domain:"RefusePrivateMenuCapture",code:1)}}
            let config=SCStreamConfiguration();config.width=display.width;config.height=display.height;config.showsCursor=false;config.capturesAudio=false;let background=CGColor(gray:0.12,alpha:1);config.backgroundColor=background;defer{withExtendedLifetime(background){}};config.minimumFrameInterval=CMTime(value:1,timescale:30)
            print("allowedPIDs=\(pids) windows=\(windows.map{$0.windowID}) display=\(display.displayID) dimensions=\(display.width)x\(display.height) menuBar=false unrelatedWindows=false audio=false")
            if let index=args.firstIndex(of:"--seconds"),index+1<args.count,let seconds=Double(args[index+1]){
                guard seconds>0,seconds<=60 else{throw NSError(domain:"BoundedRecordingRequired",code:1)}
                let settings=SCRecordingOutputConfiguration();settings.outputURL=destination;settings.videoCodecType = .h264;settings.outputFileType = .mp4
                let delegate=PetRecordingDelegate();let recording=SCRecordingOutput(configuration:settings,delegate:delegate);let stream=SCStream(filter:filter,configuration:config,delegate:nil)
                try stream.addRecordingOutput(recording);try await stream.startCapture();try await Task.sleep(for:.seconds(seconds));try await stream.stopCapture()
                let deadline=ContinuousClock.now.advanced(by:.seconds(5))
                while delegate.result.withLock({$0})==nil && ContinuousClock.now<deadline{try await Task.sleep(for:.milliseconds(50))}
                guard delegate.result.withLock({$0})=="finished" else{throw NSError(domain:"RecordingUnconfirmed",code:1)}
                print("finished")
            }else{
                let image=try await SCScreenshotManager.captureImage(contentFilter:filter,configuration:config)
                guard let writer=CGImageDestinationCreateWithURL(destination as CFURL,UTType.png.identifier as CFString,1,nil) else{throw CocoaError(.fileWriteUnknown)}
                CGImageDestinationAddImage(writer,image,nil);guard CGImageDestinationFinalize(writer) else{throw CocoaError(.fileWriteUnknown)};print("saved")
            }
        }catch{print("capture failed: \(error)");exit(1)}
    }
}
