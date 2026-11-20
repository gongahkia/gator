// window-only evidence; verifies the exact fixture PID and bundle path
import AppKit
import ScreenCaptureKit
import AVFoundation
import Synchronization

final class FixtureRecordingDelegate:NSObject,SCRecordingOutputDelegate{
    let outcome=Mutex<String?>(nil)
    func recordingOutputDidStartRecording(_ recordingOutput:SCRecordingOutput){print("recording started")}
    func recordingOutputDidFinishRecording(_ recordingOutput:SCRecordingOutput){outcome.withLock{$0="finished"}}
    func recordingOutput(_ recordingOutput:SCRecordingOutput,didFailWithError error:any Error){outcome.withLock{$0=String(describing:error)}}
}
@main struct RecordFixture{
    @MainActor static func main() async{
        do{
            _=NSApplication.shared
            guard CommandLine.arguments.count==4,let pid=Int32(CommandLine.arguments[1]),
                  let app=NSRunningApplication(processIdentifier:pid),
                  app.bundleIdentifier=="dev.gator.presence.acceptance",
                  app.bundleURL?.resolvingSymlinksInPath().path.hasPrefix(URL(fileURLWithPath:"/tmp/gator-native-test-presence-").resolvingSymlinksInPath().path)==true else{throw NSError(domain:"RefuseNonfixture",code:1)}
            print("fixturePID=\(pid) bundle=\(app.bundleIdentifier ?? "nil") path=\(app.bundleURL!.path) screenCapture=\(CGPreflightScreenCaptureAccess())")
            let content=try await SCShareableContent.excludingDesktopWindows(false,onScreenWindowsOnly:false)
            let candidates=content.windows.filter{$0.owningApplication?.processID==pid}
            for w in candidates{print("candidate=\(w.windowID) title=\(w.title ?? "nil") onScreen=\(w.isOnScreen) bundle=\(w.owningApplication?.bundleIdentifier ?? "nil")")}
            let mainWindows=candidates.filter{$0.isOnScreen && (($0.title?.hasPrefix("Gator") == true) && $0.frame.width>=540)}
            guard mainWindows.count==1,let window=mainWindows.first else{throw NSError(domain:"NoUniqueVisibleFixtureWindow",code:1)}
            let config=SCStreamConfiguration();config.width=Int(window.frame.width)*2;config.height=Int(window.frame.height)*2
            config.minimumFrameInterval=CMTime(value:1,timescale:30);config.showsCursor=false;config.capturesAudio=false
            let settings=SCRecordingOutputConfiguration();settings.outputURL=URL(fileURLWithPath:CommandLine.arguments[2]);settings.videoCodecType = .h264;settings.outputFileType = .mp4
            let delegate=FixtureRecordingDelegate(),recording=SCRecordingOutput(configuration:settings,delegate:delegate)
            let stream=SCStream(filter:SCContentFilter(desktopIndependentWindow:window),configuration:config,delegate:nil)
            try stream.addRecordingOutput(recording);try await stream.startCapture()
            try await Task.sleep(for:.seconds(Double(CommandLine.arguments[3]) ?? 28))
            do{try await stream.stopCapture()}catch{if delegate.outcome.withLock({$0}) != "finished"{throw error}}
            let deadline=ContinuousClock.now.advanced(by:.seconds(5))
            while delegate.outcome.withLock({$0})==nil && ContinuousClock.now<deadline{try await Task.sleep(for:.milliseconds(50))}
            guard delegate.outcome.withLock({$0})=="finished" else{throw NSError(domain:"RecordingNotConfirmed",code:1)}
            print("recording finished; audio=false; desktop=false")
        }catch{print("recording failed: \(error)");exit(1)}
    }
}
