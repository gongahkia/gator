import Testing
@testable import GatorCore

@Test func nativeMarkdownPreservesCodeWhitespaceAndSeparatesReadableBlocks() {
    let blocks = NativeMessageContent.blocks("# Title\n\nHello **friend**.\n\n- First\n- Second\n\n```swift\n  let value = 1\n\n  print(value)\n```\n\n1. Last")
    #expect(blocks.map(\.kind) == [.heading(1), .paragraph, .list, .list, .code("swift"), .list])
    #expect(blocks[4].text == "  let value = 1\n\n  print(value)")
    #expect(blocks[5].text == "1. Last")
    #expect(NativeMessageContent.blocks("```\nstreaming code")[0].text == "streaming code")
    #expect(NativeMessageContent.blocks("A /command and <html> are text")[0].text == "A /command and <html> are text")
}

@Test func nestedMarkdownRetainsUnorderedOrderedAndMixedHierarchy() {
    let blocks = NativeMessageContent.blocks("- Parent\n  - Child\n    + Grandchild\n  4. Ordered child\n3. Next root\n   8) Nested ordered\n       * Deep mixed")
    #expect(blocks.map(\.kind) == Array(repeating: .list, count: 7))
    #expect(blocks.compactMap(\.listItem).map(\.depth) == [0, 1, 2, 1, 0, 1, 2])
    #expect(blocks.compactMap(\.listItem).map(\.marker) == ["•", "•", "•", "4.", "3.", "8)", "•"])
    #expect(blocks.compactMap(\.listItem).map(\.text) == ["Parent", "Child", "Grandchild", "Ordered child", "Next root", "Nested ordered", "Deep mixed"])
}

@Test func markdownBlankLinesAndWrappedItemsKeepTheirParent() {
    let blocks = NativeMessageContent.blocks("## Steps\n\n1. **Read** the _first_ `file`\n   and continue the long item on another line.\n\n   A second paragraph in this item.\n\n   - Child\n\n2. Second\n\nA separate paragraph.\n\n# Next\n- New root")
    #expect(blocks.map(\.kind) == [.heading(2), .list, .list, .list, .paragraph, .heading(1), .list])
    #expect(blocks[1].listItem?.text == "**Read** the _first_ `file`\nand continue the long item on another line.\n\nA second paragraph in this item.")
    #expect(blocks[2].listItem?.depth == 1)
    #expect(blocks[3].listItem?.marker == "2.")
    #expect(blocks[6].listItem?.depth == 0)
}

@Test func markdownTabsAndMultipleDepthsDoNotFlattenIntoRootItems() {
    let blocks = NativeMessageContent.blocks("- zero\n\t1. one\n\t\t- two\n\t\t\t3. three\n\t- one again\n- root again")
    #expect(blocks.compactMap(\.listItem).map(\.depth) == [0, 1, 2, 3, 1, 0])
    #expect(Set(blocks.map(\.id)).count == 6)
}

@Test func markdownFencesNeverParseListLikeCodeAsLists() {
    let input = "- Before\n```text\n- literal\n  9. literal\n\n# literal\n```\n\n7. After\n~~~\n  - streaming code"
    let blocks = NativeMessageContent.blocks(input)
    #expect(blocks.map(\.kind) == [.list, .code("text"), .list, .code("")])
    #expect(blocks[1].text == "- literal\n  9. literal\n\n# literal")
    #expect(blocks[1].listItem == nil)
    #expect(blocks[2].listItem?.marker == "7.")
    #expect(blocks[3].text == "  - streaming code")
}

@Test func markdownOrdinalsAndNonListPunctuationStayLiteral() {
    let blocks = NativeMessageContent.blocks("04. Retain the ordinal\n  12) Retain the delimiter\n\n-plain\n1.25 is a number\n1234567890. is not a supported list marker")
    #expect(blocks[0].listItem?.marker == "04.")
    #expect(blocks[1].listItem?.marker == "12)")
    #expect(blocks[2].kind == .paragraph)
    #expect(blocks[2].text == "-plain\n1.25 is a number\n1234567890. is not a supported list marker")
}
