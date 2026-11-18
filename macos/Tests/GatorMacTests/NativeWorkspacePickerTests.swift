import AppKit
import Testing
@testable import GatorMac

@Test @MainActor func workspacePickerAllowsOneExistingDirectoryAndNeverCreatesFolders() {
    let panel = NativeWorkspacePicker.panel(source: "/tmp")
    #expect(panel.canChooseDirectories); #expect(!panel.canChooseFiles)
    #expect(!panel.allowsMultipleSelection); #expect(!panel.canCreateDirectories)
    #expect(panel.resolvesAliases); #expect(panel.prompt == "Choose Workspace")
    #expect(panel.directoryURL?.path == "/tmp")
    #expect(panel.message.contains("new conversation")); #expect(panel.message.contains("read-only"))
}
