import AppKit
import SwiftUI
import GatorCore

@MainActor enum NativeWorkspacePicker {
    static func panel(source: String) -> NSOpenPanel {
        let panel = NSOpenPanel()
        panel.title = "Choose Workspace"
        panel.prompt = "Choose Workspace"
        panel.message = "A different workspace opens a new conversation. Existing chats and background work keep their workspace. Work uses read-only snapshots."
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.canCreateDirectories = false
        panel.resolvesAliases = true
        if !source.isEmpty { panel.directoryURL = URL(fileURLWithPath: source, isDirectory: true) }
        return panel
    }
    static func choose(_ sessions: ManagedSessionCoordinator) {
        guard let token = sessions.beginWorkspaceChoice() else { return }
        let panel = panel(source: sessions.workspaceSource)
        // AppKit owns this asynchronous panel until completion. No modal loop
        // blocks the main actor or the other managed owners.
        panel.begin { response in
            let url = response == .OK ? panel.url : nil
            Task { @MainActor in
                let scoped = url?.startAccessingSecurityScopedResource() == true
                defer { if scoped { url?.stopAccessingSecurityScopedResource() } }
                await sessions.finishWorkspaceChoice(token, path: url?.path)
            }
        }
    }
}

struct ChooseWorkspaceButton: View {
    let sessions: ManagedSessionCoordinator
    var body: some View {
        Button("Choose Workspace…", systemImage: "folder") { NativeWorkspacePicker.choose(sessions) }
            .disabled(!sessions.canChooseWorkspace)
            .help("A different folder opens a new conversation; existing work keeps its workspace.")
    }
}

struct NativeWorkspaceContext: View {
    let sessions: ManagedSessionCoordinator
    let compact: Bool
    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack(spacing: 6) {
                Image(systemName: "folder").accessibilityHidden(true)
                Text(sessions.workspaceSource.isEmpty ? "Workspace not confirmed" : sessions.workspaceSource)
                    .lineLimit(1).truncationMode(.middle).textSelection(.enabled)
                    .help(sessions.workspaceSource).accessibilityLabel("Current workspace: " + sessions.workspaceSource)
                Spacer(minLength: 0)
                if sessions.workspaceChoosing { ProgressView().controlSize(.small).accessibilityLabel("Choosing workspace") }
                Text("Read-only in Work").foregroundStyle(.secondary).fixedSize()
            }
            if !sessions.workspaceStatus.isEmpty { Text(sessions.workspaceStatus).foregroundStyle(.secondary).lineLimit(2) }
            if !sessions.workspaceIssue.isEmpty {
                Label(sessions.workspaceIssue, systemImage: "exclamationmark.triangle").foregroundStyle(.primary).textSelection(.enabled).lineLimit(3).help(sessions.workspaceIssue)
                if sessions.workspaceNeedsReview { AllConversationsButton(sessions: sessions) }
            }
        }.font(.caption).padding(.horizontal, 14).padding(.vertical, compact ? 4 : 6)
    }
}
