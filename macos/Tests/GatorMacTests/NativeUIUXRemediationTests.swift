import AppKit
import SwiftUI
import Testing
@testable import GatorCore
@testable import GatorMac

@MainActor func resolvedRGB(_ color: NSColor, over background: [Double]) -> [Double] {
    let color = color.usingColorSpace(.sRGB)!
    let alpha = Double(color.alphaComponent)
    return zip([Double(color.redComponent), Double(color.greenComponent), Double(color.blueComponent)], background).map { $0 * alpha + $1 * (1 - alpha) }
}
func textContrast(_ first: [Double], _ second: [Double]) -> Double {
    func luminance(_ rgb: [Double]) -> Double {
        let values = rgb.map { $0 <= 0.04045 ? $0 / 12.92 : pow(($0 + 0.055) / 1.055, 2.4) }
        return values[0] * 0.2126 + values[1] * 0.7152 + values[2] * 0.0722
    }
    let a = luminance(first), b = luminance(second)
    return (max(a, b) + 0.05) / (min(a, b) + 0.05)
}
@Test @MainActor func uiuxStatusTextMeetsAAOnResolvedNativeSurfaces() throws {
    for name in [NSAppearance.Name.aqua, .darkAqua, .accessibilityHighContrastAqua, .accessibilityHighContrastDarkAqua] {
        let appearance = try #require(NSAppearance(named: name))
        appearance.performAsCurrentDrawingAppearance {
            for (label, color) in [("window", NSColor.windowBackgroundColor), ("text", .textBackgroundColor), ("control", .controlBackgroundColor)] {
                let background = resolvedRGB(color, over: [1, 1, 1]), foreground = resolvedRGB(.labelColor, over: background)
                let contrast = textContrast(foreground, background)
                #expect(contrast >= 4.5)
                print("GATOR_UIUX_CONTRAST appearance=\(name.rawValue) surface=\(label) foreground=\(foreground) background=\(background) ratio=\(contrast)")
            }
        }
    }
}

@Test @MainActor func uiuxSlashFeedbackOnlyAppearsForLeadingSlashAndDoesNotRewriteText() {
    #expect(NativeComposerPresentation.slashFeedback("") == nil)
    #expect(NativeComposerPresentation.slashFeedback(" \n ") == nil)
    #expect(NativeComposerPresentation.slashFeedback("Read /Users/example/notes.txt") == nil)
    for input in ["/help", "  /Users/example/notes.txt", "\n/ literal"] {
        #expect(NativeComposerPresentation.slashFeedback(input)?.contains("Messages cannot start with /") == true)
    }
}

@Test @MainActor func uiuxDuplicateAttachmentNamesAreExplainedWithoutChangingUploadIdentity() {
    let files = [NativeUpload(id: "a/report.md", name: "report.md", size: 1, sha256: "one"), NativeUpload(id: "b/report.md", name: "report.md", size: 2, sha256: "two"), NativeUpload(id: "c/other.md", name: "other.md", size: 1, sha256: "three")]
    #expect(NativeComposerPresentation.duplicateNames(files) == ["report.md"])
    #expect(NativeComposerPresentation.duplicateNames(Array(files.prefix(1))) == [])
    #expect(files.map(\.id) == ["a/report.md", "b/report.md", "c/other.md"])
}

@Test @MainActor func uiuxReadingPositionsRetainIndependentAnchorsAndExplicitFollowIntent() {
    let positions = NativeReadingPositions()
    positions.remember("conversation:A", anchor: "A:13", following: false)
    positions.remember("conversation:B", anchor: "B:5", following: false)
    #expect(positions.restored("conversation:A", available: ["A:12", "A:13", "A:14"]) == .init(anchor: "A:13", following: false))
    #expect(positions.restored("conversation:B", available: ["B:5"]).anchor == "B:5")
    // A background append does not change the reader's intent or anchor.
    #expect(positions.restored("conversation:A", available: ["A:12", "A:13", "A:14", "A:15"]).anchor == "A:13")
    positions.remember("conversation:A", anchor: nil, following: true)
    #expect(positions.restored("conversation:A", available: ["A:15"]).following)
}

@Test @MainActor func uiuxReadingPositionsBoundRetentionAndRecoverMissingAnchors() {
    let positions = NativeReadingPositions(capacity: 2)
    positions.remember("A", anchor: "deleted", following: false)
    #expect(positions.restored("A", available: ["first", "last"]) == .init(anchor: "first", following: false))
    #expect(positions.restored("A", available: []).anchor == nil)
    positions.remember("B", anchor: "B", following: false)
    positions.remember("C", anchor: "C", following: false)
    #expect(positions.count == 2)
    #expect(positions.restored("A", available: ["A"]).following)
    positions.remember("", anchor: "ignored", following: false)
    #expect(positions.count == 2)
}

@Test @MainActor func uiuxRecentConversationProjectionPreservesGoOrderAndSelection() throws {
    let data = Data(#"[{"id":"B","title":"Second"},{"id":"A","title":"First"},{"id":"A","title":"Duplicate"},{"title":"No identity"}]"#.utf8)
    let recent = try JSONDecoder().decode([JSONValue].self, from: data)
    let result = NativeRecentConversation.items(recent, activity: [], current: "A")
    #expect(result.map(\.id) == ["B", "A"])
    #expect(result.map(\.title) == ["Second", "First"])
    #expect(result.map(\.current) == [false, true])
}

@Test @MainActor func uiuxListInlineRenderingPreservesEmphasisAndCode() {
    let content = NativeMessageBody.inlineContent("**Bold** _italic_ `code` and ordinary text")
    #expect(String(content.characters) == "Bold italic code and ordinary text")
    #expect(content.runs.contains { $0.inlinePresentationIntent?.contains(.stronglyEmphasized) == true })
    #expect(content.runs.contains { $0.inlinePresentationIntent?.contains(.emphasized) == true })
    #expect(content.runs.contains { $0.inlinePresentationIntent?.contains(.code) == true })
}

@Test @MainActor func uiuxNestedListRenderingWrapsAtCompactAndFullWidths() {
    let text = "7. Parent\n  - **Nested** " + String(repeating: "long wrapped content ", count: 12) + "\n    12. Grandchild"
    func height(_ width: CGFloat) -> CGFloat {
        let host = NSHostingView(rootView: NativeMessageBody(text: text).frame(width: width).fixedSize(horizontal: false, vertical: true))
        host.layoutSubtreeIfNeeded()
        return host.fittingSize.height
    }
    let compact = height(260), full = height(620)
    #expect(compact > full)
    #expect(full > 60)
}

@Test @MainActor func uiuxEditorGrowthIsMeasuredAndBounded() {
    #expect(NativeComposerPresentation.editorHeight(contentHeight: 15, compact: false) == 76)
    #expect(NativeComposerPresentation.editorHeight(contentHeight: 130, compact: false) == 130)
    #expect(NativeComposerPresentation.editorHeight(contentHeight: 1000, compact: false) == 160)
    #expect(NativeComposerPresentation.editorHeight(contentHeight: 1000, compact: true) == 100)
    let editor = ComposerTextView(frame: NSRect(x: 0, y: 0, width: 260, height: 76))
    editor.isVerticallyResizable = true; editor.textContainer?.widthTracksTextView = true
    editor.textContainer?.containerSize = NSSize(width: 260, height: CGFloat.greatestFiniteMagnitude)
    var heights: [CGFloat] = []
    editor.onContentHeight = { heights.append($0) }
    editor.string = "Short"; editor.reportContentHeight()
    editor.string = (1...8).map { "Line \($0)" }.joined(separator: "\n"); editor.reportContentHeight()
    #expect(heights.count == 2)
    #expect(heights[1] > heights[0])
}
