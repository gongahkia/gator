import SwiftUI
import AppKit
import GatorCore

@MainActor @Observable final class NativeStreamingPresentation {
    private(set) var blocks: [NativeMessageBlock] = []
    private(set) var cursor = NativeStreamingReveal()
    private(set) var ticks = 0
    @ObservationIgnored private var task: Task<Void, Never>?
    @ObservationIgnored private var text = ""
    @ObservationIgnored private var ticket = 0
    var scheduled: Bool { task != nil }
    var revealsTail: Bool { cursor.baseline && blocks.last?.kind == .paragraph }
    func receive(_ entry: NativePublicText, policy: NativeMotionPolicy, now: Double = ProcessInfo.processInfo.systemUptime) {
        let append = entry.text.utf8.starts(with: text.utf8)
        if !text.utf8.elementsEqual(entry.text.utf8) { text = entry.text; blocks = NativeMessageContent.blocks(entry.text) }
        let tail = blocks.last
        let prose = tail?.kind == .paragraph && NativeMotionPresentation.plainProse(tail?.text ?? "")
        let animate = append && policy.allows && entry.active && entry.hasExactIdentity && prose
        cursor.receive(tail?.text ?? "", identity: entry.id + ":block:\(tail?.id ?? -1)", animate: animate, now: now)
        if !animate { cancel(); return }
        if !cursor.needsTick { cancel(); return }
        guard task == nil else { return }
        ticket += 1; let epoch = ticket
        task = Task { [weak self] in
            while !Task.isCancelled {
                do { try await Task.sleep(for: NativeStreamingReveal.cadence) } catch { return }
                guard let self, self.ticket == epoch else { return }
                self.advance()
                if !self.cursor.needsTick { self.task = nil; return }
            }
        }
    }
    func advance() { cursor.tick(); ticks += 1 }
    func hide() { cancel(); cursor.settle(release: true); blocks = []; text = "" }
    private func cancel() { ticket += 1; task?.cancel(); task = nil }
    deinit { task?.cancel() }
}

struct NativeStreamingTextView: View {
    let entry: NativePublicText
    let policy: NativeMotionPolicy
    @State private var presentation = NativeStreamingPresentation()
    private struct Request: Equatable { let entry: NativePublicText; let policy: NativeMotionPolicy }
    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            if presentation.blocks.isEmpty {
                NativeMessageBody(text: entry.text).equatable()
            } else {
                ForEach(presentation.blocks) { block in
                    if presentation.revealsTail && block.id == presentation.blocks.last?.id {
                        let cursor = presentation.cursor
                        (Text(cursor.source[..<cursor.enteringIndex]) +
                         Text(cursor.source[cursor.enteringIndex..<cursor.visibleIndex]).foregroundColor(.primary.opacity(0.88)))
                            .fixedSize(horizontal: false, vertical: true)
                            .accessibilityLabel(Text(verbatim: block.text))
                    } else { NativeMessageBlockView(block: block).equatable() }
                }
            }
        }.textSelection(.enabled).lineSpacing(3).frame(maxWidth: .infinity, alignment: .leading)
            .contextMenu { Button("Copy Response") { NativeStreamingCopy.write(entry.text, to: .general) } }
            .onChange(of: Request(entry: entry, policy: policy), initial: true) { _, request in presentation.receive(request.entry, policy: request.policy) }
            .onAppear { presentation.receive(entry, policy: policy) }
            .onDisappear { presentation.hide() }
    }
}

@MainActor enum NativeStreamingCopy {
    static func write(_ text: String, to pasteboard: NSPasteboard) {
        pasteboard.clearContents(); pasteboard.setString(text, forType: .string)
    }
}
