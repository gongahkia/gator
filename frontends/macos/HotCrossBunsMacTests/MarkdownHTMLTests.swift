import XCTest
@testable import HotCrossBunsMac

final class MarkdownHTMLTests: XCTestCase {
    func testBoldTranspile() {
        let html = MarkdownHTML.markdownToCalendarHTML("This is **bold** text")
        XCTAssertEqual(html, "This is <b>bold</b> text")
    }

    func testItalicTranspile() {
        let html = MarkdownHTML.markdownToCalendarHTML("Emphasis with *italics*")
        XCTAssertEqual(html, "Emphasis with <i>italics</i>")
    }

    func testUnderlineTranspile() {
        let html = MarkdownHTML.markdownToCalendarHTML("Underline __here__")
        XCTAssertEqual(html, "Underline <u>here</u>")
    }

    func testLinkTranspile() {
        let html = MarkdownHTML.markdownToCalendarHTML("See [docs](https://example.com)")
        XCTAssertEqual(html, "See <a href=\"https://example.com\">docs</a>")
    }

    func testLiteralHTMLEscapesWhenTranspilingMarkdown() {
        let html = MarkdownHTML.markdownToCalendarHTML("Meet <script>alert(1)</script> & review")
        XCTAssertEqual(html, "Meet &lt;script&gt;alert(1)&lt;/script&gt; &amp; review")
    }

    func testLinkLabelAndAttributeEscapeWhenTranspilingMarkdown() {
        let html = MarkdownHTML.markdownToCalendarHTML("[<b>docs</b>](https://example.com?a=1&b=2)")
        XCTAssertEqual(html, #"<a href="https://example.com?a=1&amp;b=2">&lt;b&gt;docs&lt;/b&gt;</a>"#)
    }

    func testUnsafeLinkSchemeStaysPlainText() {
        let html = MarkdownHTML.markdownToCalendarHTML("See [bad](javascript:alert) later")
        XCTAssertEqual(html, "See [bad](javascript:alert) later")
    }

    func testLocalImagePointerTranspilesAsCalendarLink() {
        let url = URL(fileURLWithPath: "/Users/example/Pictures/launch plan (v1).png")
        let pointer = LocalFileAttachment.markdownPointer(for: url, kind: .image)
        let html = MarkdownHTML.markdownToCalendarHTML(pointer)

        XCTAssertEqual(
            html,
            #"<a href="file:///Users/example/Pictures/launch%20plan%20%28v1%29.png">Local image: launch plan (v1).png</a>"#
        )
        XCTAssertEqual(MarkdownHTML.calendarHTMLToMarkdown(html), pointer)
    }

    func testLocalFilePointerTranspilesAsCalendarLink() {
        let url = URL(fileURLWithPath: "/Users/example/Documents/spec.pdf")
        let pointer = LocalFileAttachment.markdownPointer(for: url, kind: .file)
        let html = MarkdownHTML.markdownToCalendarHTML(pointer)

        XCTAssertEqual(
            html,
            #"<a href="file:///Users/example/Documents/spec.pdf">Local file: spec.pdf</a>"#
        )
        XCTAssertEqual(MarkdownHTML.calendarHTMLToMarkdown(html), pointer)
    }

    func testBulletedList() {
        let html = MarkdownHTML.markdownToCalendarHTML("- first\n- second")
        XCTAssertEqual(html, "<ul><li>first</li><li>second</li></ul>")
    }

    func testNumberedList() {
        let html = MarkdownHTML.markdownToCalendarHTML("1. first\n2. second\n3. third")
        XCTAssertEqual(html, "<ol><li>first</li><li>second</li><li>third</li></ol>")
    }

    func testNewlineBecomesBreak() {
        let html = MarkdownHTML.markdownToCalendarHTML("line one\nline two")
        XCTAssertEqual(html, "line one<br>line two")
    }

    func testHTMLBoldToMarkdown() {
        let markdown = MarkdownHTML.calendarHTMLToMarkdown("<b>keep</b> it simple")
        XCTAssertEqual(markdown, "**keep** it simple")
    }

    func testHTMLItalicToMarkdown() {
        let markdown = MarkdownHTML.calendarHTMLToMarkdown("<i>see</i> and <em>also</em>")
        XCTAssertEqual(markdown, "*see* and *also*")
    }

    func testHTMLLinkToMarkdown() {
        let markdown = MarkdownHTML.calendarHTMLToMarkdown("Read <a href=\"https://example.com\">docs</a>")
        XCTAssertEqual(markdown, "Read [docs](https://example.com)")
    }

    func testHTMLListToMarkdown() {
        let markdown = MarkdownHTML.calendarHTMLToMarkdown("<ul><li>one</li><li>two</li></ul>")
        XCTAssertEqual(markdown.trimmingCharacters(in: .whitespacesAndNewlines), "- one\n- two")
    }

    func testHTMLOrderedListToMarkdown() {
        let markdown = MarkdownHTML.calendarHTMLToMarkdown("<ol><li>first</li><li>second</li></ol>")
        XCTAssertEqual(markdown.trimmingCharacters(in: .whitespacesAndNewlines), "1. first\n2. second")
    }

    func testHTMLBreakToNewline() {
        let markdown = MarkdownHTML.calendarHTMLToMarkdown("line one<br>line two")
        XCTAssertEqual(markdown, "line one\nline two")
    }

    func testPreservesUnsupportedHTML() {
        let markdown = MarkdownHTML.calendarHTMLToMarkdown("keep <span class=\"k\">this</span>")
        XCTAssertTrue(markdown.contains("<span"))
    }

    func testRoundTripBoldItalic() {
        let source = "**bold** and *italic* text"
        let html = MarkdownHTML.markdownToCalendarHTML(source)
        let back = MarkdownHTML.calendarHTMLToMarkdown(html)
        XCTAssertEqual(back, source)
    }

    func testRoundTripList() {
        let source = "- one\n- two"
        let html = MarkdownHTML.markdownToCalendarHTML(source)
        let back = MarkdownHTML.calendarHTMLToMarkdown(html).trimmingCharacters(in: .whitespacesAndNewlines)
        XCTAssertEqual(back, source)
    }

    func testEmptyStringRoundTrip() {
        XCTAssertEqual(MarkdownHTML.markdownToCalendarHTML(""), "")
        XCTAssertEqual(MarkdownHTML.calendarHTMLToMarkdown(""), "")
    }

    func testLocalImagePointerParsesFileURL() throws {
        let url = URL(fileURLWithPath: "/Users/example/Pictures/diagram.png")
        let pointer = LocalFileAttachment.markdownPointer(for: url, kind: .image)
        let parsed = try XCTUnwrap(LocalFileAttachment.parseMarkdownLine(pointer))

        XCTAssertEqual(parsed.kind, .image)
        XCTAssertEqual(parsed.displayName, "diagram.png")
        XCTAssertEqual(parsed.url.path, url.path)
    }

    func testLocalFilePointerParsesFileURL() throws {
        let url = URL(fileURLWithPath: "/Users/example/Documents/brief.pdf")
        let pointer = LocalFileAttachment.markdownPointer(for: url, kind: .file)
        let parsed = try XCTUnwrap(LocalFileAttachment.parseMarkdownLine(pointer))

        XCTAssertEqual(parsed.kind, .file)
        XCTAssertEqual(parsed.displayName, "brief.pdf")
        XCTAssertEqual(parsed.url.path, url.path)
    }

    func testMarkdownEditorLinkUsesSelectedTextAsLabel() {
        let result = MarkdownEditorTextMutation.link(
            text: "Read docs today",
            selection: NSRange(location: 5, length: 4)
        )

        XCTAssertEqual(result.text, "Read [docs](https://) today")
        XCTAssertEqual(result.selection.location, 12)
        XCTAssertEqual(result.selection.length, 8)
    }

    func testMarkdownEditorLinkInsertsAtCursorWhenSelectionIsEmpty() {
        let result = MarkdownEditorTextMutation.link(
            text: "Read ",
            selection: NSRange(location: 5, length: 0)
        )

        XCTAssertEqual(result.text, "Read [text](https://)")
        XCTAssertEqual(result.selection.location, 12)
        XCTAssertEqual(result.selection.length, 8)
    }
}
