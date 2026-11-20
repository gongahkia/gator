import AppKit
import SwiftUI
import Testing
@testable import GatorMac
@testable import GatorCore

private let motionEvidence = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("motion/evidence/p04u-1")
@MainActor @Observable private final class MotionRenderInput {
    var text = "A short public answer."
    var active = true
    var visible = true
}
private struct MotionRenderFixture: View {
    let input: MotionRenderInput
    let baseline: Bool
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Gator · isolated native component").font(.headline)
            if baseline {
                HStack { ProgressView().controlSize(.small); Text("Working…").foregroundStyle(.secondary) }
                NativeMessageBody(text: input.text)
            } else {
                NativeThinkingStatus(text: "Working…", icon: "clock", owner: "fixture", pulsing: false)
                NativeStreamingTextView(entry: .init(id: "fixture:run:entry", text: input.text, active: input.active, hasExactIdentity: true), policy: .init(visible: input.visible))
            }
            Divider()
            Text("Composer and exact decisions are outside this component fixture.").font(.caption).foregroundStyle(.secondary)
        }.padding(20).background(Color(nsColor: .windowBackgroundColor))
    }
}
@Test @MainActor func streamingNativeComponentFitsFullCompactAndLongStatusWithoutFocus() throws {
    for width: CGFloat in [380, 540, 780] {
        let input = MotionRenderInput()
        input.text = "## Native Markdown\n\nFirst paragraph with **bold** and `code`.\n\nA plain paragraph with 👩🏽‍💻 and 日本語.\n\n```swift\nlet value = 42\n```"
        let host = NSHostingView(rootView: MotionRenderFixture(input: input, baseline: false).frame(width: width))
        host.frame = CGRect(origin: .zero, size: host.fittingSize)
        host.layoutSubtreeIfNeeded()
        #expect(host.fittingSize.width <= width + 1 && host.fittingSize.height.isFinite && host.fittingSize.height < 1000)
    }
    let status = NativePulseLabel.PulseField()
    status.configure(text: "Waiting for your exact review before an external action can continue.", owner: "A", active: false)
    let size = try #require(status.cell?.cellSize(forBounds: CGRect(x: 0, y: 0, width: 220, height: 1000)))
    #expect(size.width <= 220 && size.height >= 16 && size.height < 140)
    #expect(!status.acceptsFirstResponder)
}
@Test @MainActor func streamingNativeComponentEvidenceCapture() throws {
    guard ProcessInfo.processInfo.environment["GATOR_MOTION_CAPTURE"] == "1" else { return }
    for dark in [false, true] {
        for width: CGFloat in [400, 780] {
            for baseline in [false, true] {
                let input = MotionRenderInput()
                input.text = "## A useful answer\n\nKeep the public response accurate, readable and easy to copy.\n\nHere is a gentle plain-language continuation with cafe\u{301}, 👩🏽‍💻 and 中文.\n\n```swift\nlet scope = \"unchanged\"\n```\n\nThis plain prose tail uses the selectable streaming presentation."
                let root = MotionRenderFixture(input: input, baseline: baseline).frame(width: width).environment(\.colorScheme, dark ? .dark : .light)
                let host = NSHostingView(rootView: root)
                host.appearance = NSAppearance(named: dark ? .darkAqua : .aqua)
                host.frame = CGRect(origin: .zero, size: host.fittingSize); host.layoutSubtreeIfNeeded()
                let bitmap = try #require(host.bitmapImageRepForCachingDisplay(in: host.bounds))
                host.cacheDisplay(in: host.bounds, to: bitmap)
                let data = try #require(bitmap.representation(using: .png, properties: [:]))
                try data.write(to: motionEvidence.appendingPathComponent("component-\(baseline ? "before" : "after")-\(Int(width))-\(dark ? "dark" : "light").png"))
            }
        }
    }
}
@Test @MainActor func streamingHostedPresentationResourceTrial() async throws {
    guard let mode = ProcessInfo.processInfo.environment["GATOR_MOTION_BENCH"], ["before", "after"].contains(mode) else { return }
    let input = MotionRenderInput()
    let host = NSHostingView(rootView: MotionRenderFixture(input: input, baseline: mode == "before").frame(width: 540))
    host.frame = CGRect(x: 0, y: 0, width: 540, height: 650)
    func phase(_ name: String) throws {
        let value: [String: Any] = ["pid": ProcessInfo.processInfo.processIdentifier, "phase": name, "epoch": Date().timeIntervalSince1970]
        let line = try JSONSerialization.data(withJSONObject: value)
        let path = motionEvidence.appendingPathComponent("hosted-\(mode)-phases.jsonl")
        if !FileManager.default.fileExists(atPath: path.path) { try Data().write(to: path) }
        let handle = try FileHandle(forWritingTo: path); defer { try? handle.close() }
        try handle.seekToEnd(); try handle.write(contentsOf: line + Data([10]))
    }
    try phase("idle"); try await Task.sleep(for: .seconds(3))
    try phase("streaming")
    var latencies: [Double] = []
    for index in 0..<240 {
        let start = ContinuousClock.now
        if index.isMultiple(of: 2) { input.text += " Public words 👩🏽‍💻 中文." }
        await Task.yield(); host.layoutSubtreeIfNeeded()
        let duration = ContinuousClock.now - start
        latencies.append(Double(duration.components.seconds) * 1000 + Double(duration.components.attoseconds) / 1e15)
        try await Task.sleep(for: .milliseconds(42))
    }
    input.active = false; await Task.yield(); host.layoutSubtreeIfNeeded()
    try phase("settled"); try await Task.sleep(for: .seconds(3))
    input.visible = false; await Task.yield()
    try phase("hidden-component"); try await Task.sleep(for: .seconds(3))
    try phase("end")
    let result: [String: Any] = ["mode": mode, "iterations": 240, "fixtureLayoutCadenceMilliseconds": 42,
        "hostedLayoutMillisecondsMedian": latencies.sorted()[latencies.count / 2], "hostedLayoutMillisecondsMax": latencies.max() ?? 0,
        "scope": "headless NSHostingView; not window compositor, scrolling or whole-app responsiveness"]
    try JSONSerialization.data(withJSONObject: result, options: [.prettyPrinted, .sortedKeys]).write(to: motionEvidence.appendingPathComponent("hosted-\(mode)-layout.json"))
}
