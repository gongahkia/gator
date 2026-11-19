// captures only the isolated lab window; no microphone, system audio or desktop
import Foundation
import ScreenCaptureKit
import AVFoundation
import Synchronization
import AppKit

final class RecordingDelegate: NSObject, SCRecordingOutputDelegate {
    let outcome = Mutex<String?>(nil)
    func recordingOutputDidStartRecording(_ recordingOutput: SCRecordingOutput) { print("recording started") }
    func recordingOutputDidFinishRecording(_ recordingOutput: SCRecordingOutput) { outcome.withLock { $0 = "finished" } }
    func recordingOutput(_ recordingOutput: SCRecordingOutput, didFailWithError error: any Error) { outcome.withLock { $0 = "failed: \(error)" } }
}
@main struct RecordWindow {
    @MainActor static func main() async throws {
        _ = NSApplication.shared
        let content = try await SCShareableContent.excludingDesktopWindows(false, onScreenWindowsOnly: true)
        guard let window = content.windows.first(where: { $0.owningApplication?.bundleIdentifier == "dev.gator.mascot.gate2.validation" && $0.title == "Gator Concept B — Gate 2 only" }) else {
            throw NSError(domain: "LabWindowNotFound", code: 1)
        }
        let filter = SCContentFilter(desktopIndependentWindow: window)
        let config = SCStreamConfiguration()
        config.width = Int(window.frame.width) * 2; config.height = Int(window.frame.height) * 2
        config.minimumFrameInterval = CMTime(value: 1, timescale: 30)
        config.showsCursor = false; config.capturesAudio = false
        let recordingConfig = SCRecordingOutputConfiguration()
        recordingConfig.outputURL = URL(fileURLWithPath: CommandLine.arguments[1])
        recordingConfig.videoCodecType = .h264; recordingConfig.outputFileType = .mp4
        let delegate = RecordingDelegate(), recording = SCRecordingOutput(configuration: recordingConfig, delegate: delegate)
        let stream = SCStream(filter: filter, configuration: config, delegate: nil)
        try stream.addRecordingOutput(recording)
        try await stream.startCapture()
        if CommandLine.arguments.count > 3 { try Data().write(to: URL(fileURLWithPath: CommandLine.arguments[3])) }
        print("windowID=\(window.windowID); audio=false; desktop=false")
        try await Task.sleep(for: .seconds(Double(CommandLine.arguments[2]) ?? 20))
        do { try await stream.stopCapture() }
        catch { if delegate.outcome.withLock({ $0 }) != "finished" { throw error } }
        let deadline = ContinuousClock.now.advanced(by: .seconds(5))
        while delegate.outcome.withLock({ $0 }) == nil && ContinuousClock.now < deadline { try await Task.sleep(for: .milliseconds(50)) }
        guard delegate.outcome.withLock({ $0 }) == "finished" else { throw NSError(domain: "RecordingNotConfirmed", code: 1) }
        print("recording finished")
    }
}
