import AppKit
import SwiftUI
import Testing
@testable import GatorMac
@testable import GatorCore

private func motionEntry(_ text: String, owner: String = "owner:generation:run:entry", active: Bool = true, exact: Bool = true) -> NativePublicText {
    .init(id: owner, text: text, active: active, hasExactIdentity: exact)
}
private func motionSnapshot(_ text: String = "Hello", state: String = "running", id: String = "activity-1", extra: [String: JSONValue] = [:]) -> BridgeSnapshot {
    var fields: [String: JSONValue] = ["running": .bool(true), "activity": .object([
        "started_at": .string("run-start"), "entries": .array([.object([
            "id": .string(id), "started_at": .string("entry-start"), "kind": .string("text"), "state": .string(state), "text": .string(text)
        ])])])]
    fields.merge(extra) { _, new in new }; return BridgeSnapshot(.object(fields))
}
private func shown(_ cursor: NativeStreamingReveal) -> String { String(cursor.source[..<cursor.visibleIndex]) }

@Test func streamingGraphemesNeverSplitDuringReveal() {
    for value in ["👩🏽‍💻🇸🇬e\u{301}你好世界", "مرحبا بالعالم", "日本語の文章です", "नमस्ते दुनिया", "한글 문장", "a" + String(repeating: "👨‍👩‍👧‍👦", count: 20)] {
        var cursor = NativeStreamingReveal(); cursor.receive("", identity: "A", animate: true, now: 1)
        cursor.receive(value, identity: "A", animate: true, now: 2)
        var steps = 0
        while cursor.needsTick {
            cursor.tick(); steps += 1
            #expect(value.indices.contains(cursor.visibleIndex) || cursor.visibleIndex == value.endIndex)
            #expect(value.utf8.starts(with: shown(cursor).utf8)); #expect(steps < 100)
        }
        #expect(shown(cursor).utf8.elementsEqual(value.utf8))
    }
}
@Test func streamingProviderDeltaCanExtendPreviousGrapheme() {
    for (before, after) in [("Cafe", "Cafe\u{301} today"), ("👩", "👩🏽‍💻 together"), ("🇸", "🇸🇬 flag")] {
        var cursor = NativeStreamingReveal(); cursor.receive(before, identity: "A", animate: true, now: 1)
        cursor.receive(after, identity: "A", animate: true, now: 2)
        #expect(cursor.characters == Array(after))
        while cursor.needsTick { cursor.tick() }
        #expect(shown(cursor).utf8.elementsEqual(after.utf8))
    }
}
@Test func streamingAdaptiveBatchingBoundsBacklogAndAcceleratesBursts() {
    func steps(_ strategy: NativeStreamingReveal.Strategy) -> Int {
        var cursor = NativeStreamingReveal(); cursor.receive("", identity: "A", animate: true, now: 1)
        cursor.receive(String(repeating: "natural words ", count: 80), identity: "A", animate: true, now: 2)
        var ticks = 0
        while cursor.needsTick { cursor.tick(strategy: strategy); ticks += 1 }
        return ticks
    }
    #expect(steps(.adaptive) < steps(.word)); #expect(steps(.word) < steps(.character))
    var cursor = NativeStreamingReveal(); cursor.receive("Start", identity: "A", animate: true, now: 1)
    let large = "Start" + String(repeating: "a", count: 3_000)
    cursor.receive(large, identity: "A", animate: true, now: 2)
    #expect(!cursor.needsTick && shown(cursor) == large)
    cursor.receive(String(repeating: "a", count: 20_000), identity: "A", animate: true, now: 3)
    #expect(cursor.characters.isEmpty && !cursor.needsTick && shown(cursor).count == 20_000)
}
@Test func streamingReplacementTruncationClearAndRawUnicodeAreAuthoritative() {
    var cursor = NativeStreamingReveal()
    for text in ["Initial", "Initial append", "Revised", "R", "", "café", "cafe\u{301}"] {
        let previous = cursor.source
        cursor.receive(text, identity: "A", animate: true, now: 2)
        if !text.utf8.starts(with: previous.utf8) || text.isEmpty { #expect(shown(cursor).utf8.elementsEqual(text.utf8)) }
    }
    #expect(cursor.source.utf8.elementsEqual("cafe\u{301}".utf8))
}
@Test func streamingHistoryOwnerGenerationReconnectionAndCompletionBypassReplay() {
    var cursor = NativeStreamingReveal()
    cursor.receive("History", identity: "A:1:run", animate: true, now: 1)
    #expect(!cursor.needsTick && shown(cursor) == "History")
    cursor.receive("History arriving", identity: "A:1:run", animate: true, now: 2)
    #expect(cursor.needsTick)
    cursor.receive("History arriving", identity: "A:1:run", animate: false, now: 3)
    #expect(!cursor.needsTick && shown(cursor) == "History arriving")
    for identity in ["B:1:run", "A:2:run", "A:2:new-run"] {
        cursor.receive("Already confirmed", identity: identity, animate: true, now: 4)
        #expect(!cursor.needsTick && shown(cursor) == "Already confirmed")
    }
}
@Test func streamingProjectionRequiresExactPublicIdentityAndSelectedGeneration() throws {
    let a = try #require(NativeMotionPresentation.publicText(snapshot: motionSnapshot(), owner: "A:1").first)
    let b = try #require(NativeMotionPresentation.publicText(snapshot: motionSnapshot(), owner: "A:2").first)
    #expect(a.active && a.hasExactIdentity && a.id != b.id)
    for snapshot in [motionSnapshot(state: "completed"), motionSnapshot(state: "failed"), motionSnapshot(state: "stopped"), motionSnapshot(id: ""), motionSnapshot(extra: ["cancelling": .bool(true)]), motionSnapshot(extra: ["pending_approval": .object(["id": .number(1), "token": .string("exact")])])] {
        #expect(NativeMotionPresentation.publicText(snapshot: snapshot, owner: "A").allSatisfy { !$0.active })
    }
    let privateEntries: JSONValue = .object(["started_at": .string("run"), "entries": .array([
        .object(["kind": .string("reasoning"), "text": .string("private")]), .object(["kind": .string("tool"), "arguments": .string("private")])])])
    #expect(NativeMotionPresentation.publicText(snapshot: motionSnapshot(extra: ["activity": privateEntries]), owner: "A").isEmpty)
}
@Test func streamingAmbiguousMultipleTextEntriesStayStaticWithUniqueRowIDs() {
    let entry: JSONValue = .object(["id": .string("duplicate"), "started_at": .string("start"), "kind": .string("text"), "state": .string("running"), "text": .string("Public")])
    let activity: JSONValue = .object(["started_at": .string("run"), "entries": .array([entry, entry])])
    let values = NativeMotionPresentation.publicText(snapshot: motionSnapshot(extra: ["activity": activity]), owner: "A")
    #expect(values.count == 2 && Set(values.map(\.id)).count == 2)
    #expect(values.allSatisfy { !$0.active && !$0.hasExactIdentity })
}
@Test func streamingPolicyDisablesMotionForEverySafetyAndVisibilityBoundary() {
    #expect(NativeMotionPolicy().allows)
    for policy in [NativeMotionPolicy(enabled: false), .init(reduceMotion: true), .init(visible: false), .init(reading: true), .init(reviewing: true), .init(closing: true), .init(connected: false)] { #expect(!policy.allows) }
    #expect(NativeMotionPolicy.followsLayout(following: true, scrolling: false, searching: false))
    #expect(!NativeMotionPolicy.followsLayout(following: false, scrolling: false, searching: false))
    #expect(!NativeMotionPolicy.followsLayout(following: true, scrolling: true, searching: false))
    #expect(!NativeMotionPolicy.followsLayout(following: true, scrolling: false, searching: true))
}
@Test @MainActor func streamingPreferencePersistsIndependentlyWithoutReenablingPresence() throws {
    let domain = "gator.motion.test." + UUID().uuidString
    let defaults = try #require(UserDefaults(suiteName: domain)); defer { defaults.removePersistentDomain(forName: domain) }
    #expect(NativeMotionPreference.enabled(in: defaults))
    for key in [NativeMascotPreference.shown, NativeMascotPreference.animated, NativeDesktopPetPreference.shown] { defaults.set(false, forKey: key) }
    for value in [false, true, false] {
        defaults.set(value, forKey: NativeMotionPreference.streaming)
        #expect(NativeMotionPreference.enabled(in: try #require(UserDefaults(suiteName: domain))) == value)
        #expect(!defaults.bool(forKey: NativeMascotPreference.shown) && !defaults.bool(forKey: NativeMascotPreference.animated) && !defaults.bool(forKey: NativeDesktopPetPreference.shown))
    }
}
@Test @MainActor func streamingMarkdownBlocksRemainNativeAndFormattedContentBypassesCadence() {
    let samples = ["# Heading\n\nParagraph", "**Bold** and *italic*", "`inline`", "1. Item\n   - Nested", "```swift\nlet x = 1", "```swift\nlet x = 1\n```", "[link](https://example.invalid)", "First paragraph\n\nSecond paragraph"]
    for source in samples {
        let presentation = NativeStreamingPresentation()
        presentation.receive(motionEntry(source), policy: .init())
        #expect(presentation.blocks == NativeMessageContent.blocks(source))
        #expect(!presentation.scheduled)
        if !NativeMotionPresentation.plainProse(presentation.blocks.last?.text ?? "") { #expect(!presentation.revealsTail) }
        presentation.hide()
    }
}
@Test @MainActor func streamingCopyPreservesExactConfirmedMarkdownAndUnicode() {
    let raw = "## Title\n\n**cafe\u{301}** 👩🏽‍💻\n```swift\nlet x = 1\n```\n"
    let pasteboard = NSPasteboard.withUniqueName(); defer { pasteboard.releaseGlobally() }
    NativeStreamingCopy.write(raw, to: pasteboard)
    #expect(pasteboard.string(forType: .string)?.utf8.elementsEqual(raw.utf8) == true)
}
@Test @MainActor func streamingCompletionAndHiddenCancellationFlushWithoutRetainingCadence() {
    let model = NativeStreamingPresentation()
    model.receive(motionEntry("One"), policy: .init(), now: 1)
    model.receive(motionEntry("One two three four five"), policy: .init(), now: 2)
    #expect(model.scheduled && model.cursor.pending > 0)
    model.advance()
    model.receive(motionEntry("One two three four five", active: false), policy: .init(), now: 3)
    #expect(!model.scheduled && shown(model.cursor) == "One two three four five")
    model.hide(); #expect(!model.scheduled && model.blocks.isEmpty && model.cursor.characters.isEmpty)
    model.receive(motionEntry("One two three four five"), policy: .init(), now: 4)
    #expect(!model.scheduled && shown(model.cursor) == "One two three four five")
}
@Test @MainActor func streamingCadenceDoesNotRetainReleasedViewModelOrReadingAnchors() async {
    let positions = NativeReadingPositions(); positions.remember("A", anchor: "row-9", following: false)
    var model: NativeStreamingPresentation? = NativeStreamingPresentation(); weak var probe = model
    model?.receive(motionEntry("One"), policy: .init(), now: 1)
    model?.receive(motionEntry("One two three four five"), policy: .init(), now: 2)
    #expect(model?.scheduled == true); model = nil
    for _ in 0..<20 { await Task.yield() }
    #expect(probe == nil)
    #expect(positions.restored("A", available: ["row-9"]).anchor == "row-9")
}
@Test @MainActor func streamingRepeatedIndependentPresentationsCancelAllTasks() {
    for _ in 0..<100 {
        let full = NativeStreamingPresentation(), compact = NativeStreamingPresentation()
        for view in [full, compact] {
            view.receive(motionEntry("A"), policy: .init(), now: 1)
            view.receive(motionEntry("A fresh public response"), policy: .init(), now: 2)
            view.receive(motionEntry("A fresh public response"), policy: .init(visible: false), now: 3)
            #expect(!view.scheduled && view.cursor.characters.isEmpty)
            view.hide()
        }
    }
}
@Test func streamingStatusPrecedenceNeverPulsesAnExactReviewOrClaimsSuccess() throws {
    let pending = motionSnapshot(extra: ["pending_approval": .object(["id": .number(1), "token": .string("exact")]), "dialogue_busy": .bool(true)])
    let status = try #require(NativeMotionPresentation.status(snapshot: pending))
    #expect(!status.active && status.text == "Waiting for your review")
    #expect(NativeMotionPresentation.status(snapshot: motionSnapshot(extra: ["cancelling": .bool(true)]))?.active == false)
    #expect(NativeMotionPresentation.status(snapshot: motionSnapshot(extra: ["running": .bool(false)])) == nil)
    #expect(NativeMotionPresentation.status(snapshot: motionSnapshot(extra: ["running": .bool(false), "dialogue_busy": .bool(true)]))?.text == "Considering your message…")
}
@Test @MainActor func streamingPulseHasBoundedOpacityAndTerminatesOnOwnerOrPolicyChanges() {
    let field = NativePulseLabel.PulseField(); var installed: [CABasicAnimation] = []
    field.install = { _, animation in installed.append(animation) }
    field.configure(text: "Working…", owner: "A", active: true)
    field.configure(text: "Working…", owner: "A", active: true)
    #expect(installed.count == 1 && field.pulsing && !field.acceptsFirstResponder && !field.isSelectable)
    #expect(installed[0].fromValue as? Double == 0.82 && installed[0].toValue as? Double == 1)
    field.configure(text: "Working…", owner: "B", active: true); #expect(installed.count == 2)
    field.configure(text: "Waiting for your review", owner: "B", active: false)
    #expect(!field.pulsing && field.layer?.opacity == 1)
    #expect(field.stringValue == "Waiting for your review")
    field.stop(); #expect(!field.pulsing)
}
@Test @MainActor func streamingVisibilityProbeHasNoFocusAndReleasesObservers() {
    var probe: NativeMotionVisibility.Probe? = .init(update: { _ in })
    weak var weakProbe = probe
    probe?.viewDidMoveToWindow(); #expect(probe?.acceptsFirstResponder == false)
    probe?.detach(); probe = nil; #expect(weakProbe == nil)
}
@Test func streamingWelcomeAndReadinessRemediationReflectExistingNativeWorkflow() {
    #expect(NativeMotionPresentation.showsWelcome(empty: true, running: false, considering: false))
    #expect(!NativeMotionPresentation.showsWelcome(empty: true, running: true, considering: false))
    #expect(!NativeMotionPresentation.showsWelcome(empty: true, running: false, considering: true))
    let message = NativeMotionPresentation.readinessMessage(.object(["issue": .string("Missing credential.")]))
    #expect(message.contains("Models and Providers") && !message.contains("terminal"))
    #expect(message.contains("does not verify a live provider request"))
}
@Test @MainActor func streamingPulseContrastRemainsReadableInNativeAppearances() throws {
    for name in [NSAppearance.Name.aqua, .darkAqua, .accessibilityHighContrastAqua, .accessibilityHighContrastDarkAqua] {
        let appearance = try #require(NSAppearance(named: name))
        appearance.performAsCurrentDrawingAppearance {
            let background = resolvedRGB(.windowBackgroundColor, over: [1,1,1])
            #expect(textContrast(resolvedRGB(NSColor.labelColor.withAlphaComponent(0.82), over: background), background) >= 4.5)
        }
    }
}

@Test @MainActor func streamingNativeProsePreservesSelectionAndExactUnicodeThroughAppends() {
    let view = NativeStreamingProse.ProseView()
    view.configure(source: "Public 👩🏽‍💻", visible: "Public 👩🏽‍💻".utf16.count, entering: 0, response: "Public 👩🏽‍💻")
    view.setSelectedRange(NSRange(location: 0, length: 6))
    let next = "Public 👩🏽‍💻 中文 cafe\u{301}"
    view.configure(source: next, visible: next.utf16.count, entering: "Public 👩🏽‍💻".utf16.count, response: next)
    #expect(view.string.utf8.elementsEqual(next.utf8))
    #expect(view.selectedRange() == NSRange(location: 0, length: 6))
    #expect(view.measured(width: 220).height > 0 && view.measured(width: 220).height < 300)
    view.configure(source: "Revised", visible: 7, entering: 7, response: "Revised")
    #expect(view.string == "Revised" && view.isSelectable && !view.isEditable)
}

@Test @MainActor func streamingOwnerReplacementCancelsPreviousCadenceImmediately() {
    let model = NativeStreamingPresentation()
    model.receive(motionEntry("A"), policy: .init(), now: 1)
    model.receive(motionEntry("A public append"), policy: .init(), now: 2)
    #expect(model.scheduled)
    model.receive(motionEntry("Other owner's already confirmed response", owner: "B:new-generation:run:entry"), policy: .init(), now: 3)
    #expect(!model.scheduled && !model.cursor.needsTick)
    #expect(shown(model.cursor) == "Other owner's already confirmed response")
}
@Test @MainActor func streamingEquatableMarkdownDoesNotHideMeaningfulUnicodeReplacement() {
    let composed = NativeMessageContent.blocks("café")[0], decomposed = NativeMessageContent.blocks("cafe\u{301}")[0]
    #expect(NativeMessageBlockView(block: composed) != NativeMessageBlockView(block: decomposed))
    #expect(NativeMessageBody(text: composed.text) != NativeMessageBody(text: decomposed.text))
}
@Test func streamingEmptyActivityDisclosureIsSuppressedWithoutHidingTools() {
    #expect(!NativeMotionPresentation.hasPublicTools(motionSnapshot()))
    let tools: JSONValue = .object(["entries": .array([.object(["kind": .string("tool"), "tool": .string("read_file")])])])
    #expect(NativeMotionPresentation.hasPublicTools(motionSnapshot(extra: ["activity": tools])))
}
@Test @MainActor func streamingTwoPresentationsCannotCreateGoOwnersOrAdmitWork() {
    let root = FileManager.default.temporaryDirectory.appendingPathComponent("gator-motion-unstarted-" + UUID().uuidString)
    let sessions = ManagedSessionCoordinator(configuration: .init(runtime: root.appendingPathComponent("not-a-runtime"), workspace: root), nativeDraftDirectory: root.appendingPathComponent("drafts"), profile: "test")
    let generation = sessions.current.generation
    let full = NativeStreamingPresentation(), compact = NativeStreamingPresentation()
    for presentation in [full, compact] {
        presentation.receive(motionEntry("One"), policy: .init(), now: 1)
        presentation.receive(motionEntry("One two three"), policy: .init(), now: 2)
        presentation.hide()
    }
    #expect(sessions.ownerCount == 1 && sessions.current.generation == generation)
    #expect(!sessions.current.started && sessions.current.pendingSubmission == nil && sessions.current.draftText.isEmpty)
    #expect(!FileManager.default.fileExists(atPath: root.path))
}
@MainActor private final class WeakStreamingText {
    weak var view: NativeStreamingProse.ProseView?
    weak var storage: NSTextStorage?
    init(_ view: NativeStreamingProse.ProseView) { self.view = view; storage = view.textStorage }
}
@Test @MainActor func streamingNativeTextViewLifetimesReleaseTheirTextStores() async {
    var probes: [WeakStreamingText] = []
    for _ in 0..<100 {
        autoreleasepool {
            let view = NativeStreamingProse.ProseView(); probes.append(WeakStreamingText(view))
            view.configure(source: "Public text", visible: 11, entering: 0, response: "Public text")
            _ = view.measured(width: 380)
        }
    }
    // AppKit retains even an unconfigured NSTextView briefly. Observe eventual
    // release without retaining the subjects, rather than asserting synchronous ARC.
    let deadline = ContinuousClock.now + .seconds(2)
    while probes.contains(where: { $0.view != nil || $0.storage != nil }) && ContinuousClock.now < deadline {
        try? await Task.sleep(for: .milliseconds(10))
    }
    #expect(probes.allSatisfy { $0.view == nil && $0.storage == nil })
}

@Test @MainActor func streamingEarlierBlockReplacementFlushesTheWholeConfirmedTail() {
    let model = NativeStreamingPresentation()
    model.receive(motionEntry("First paragraph\n\nTail"), policy: .init(), now: 1)
    model.receive(motionEntry("First paragraph\n\nTail arriving words"), policy: .init(), now: 2)
    #expect(model.scheduled)
    model.receive(motionEntry("Corrected first paragraph\n\nTail arriving words plus correction"), policy: .init(), now: 3)
    #expect(!model.scheduled && shown(model.cursor) == "Tail arriving words plus correction")
}

