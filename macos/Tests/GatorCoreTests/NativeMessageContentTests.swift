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
