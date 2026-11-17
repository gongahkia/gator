import SwiftUI
import AppKit
import GatorCore
import UniformTypeIdentifiers

struct ComposerView: View {
    @Environment(GatorApplicationState.self) private var application
    @Environment(ManagedSessionCoordinator.self) private var sessions
    @State private var dropTargeted = false
    @State private var contentHeight: CGFloat = 0
    var compact: Bool
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let target = application.correctionTarget {
                HStack {
                    Text(target.label).font(.caption.weight(.medium))
                    Spacer()
                    Button("Cancel Correction") { application.cancelCorrection() }.disabled(application.pendingSubmission != nil)
                }
                if let issue = sessions.correctionUnavailableReason { NativeStatusMessage(text: issue).font(.caption) }
                Button("Inspect Correction Target") { sessions.inspectTask(target.ownerID) }.font(.caption)
            }
            AttachmentSelectionView(compact: compact)
            if !application.attachmentStatus.isEmpty { Label(application.attachmentStatus, systemImage: "arrow.down.doc").font(.caption) }
            if !application.attachmentIssue.isEmpty { NativeStatusMessage(text: application.attachmentIssue, severity: .error).font(.caption) }
            ZStack(alignment: .topLeading) {
                if application.draftText.isEmpty {
                    Text("Message Gator…").font(.body).foregroundStyle(.tertiary).padding(.top, 4).padding(.leading, 2).allowsHitTesting(false)
                }
                NativeComposer(text: application.draftText, editable: application.canEditDraft, focusRequest: application.focusRequest, onEdit: application.editDraft, onDropFiles: { urls in
                    let owner = application
                    guard let request = owner.beginAttachmentImport() else { return false }
                    Task { await owner.importFiles(urls, request: request) }; return true
                }, onDragTarget: { dropTargeted = $0 }, onPasteImage: { NativeAttachmentActions.pasteImage(owner: application, explicitly: false) }, onContentHeight: { contentHeight = $0 }) {
                    if sessions.canSubmit { let owner = application; Task { await sessions.submit(from: owner) } }
                }
                // A native input context/undo stack must never be reused for a
                // different conversation or replacement execution owner.
                .id(application.context + ":" + application.generation.uuidString)
            }
            .frame(height: NativeComposerPresentation.editorHeight(contentHeight: contentHeight, compact: compact))
            if let reason = NativeComposerPresentation.slashFeedback(application.draftText) {
                NativeStatusMessage(text: reason, severity: .information).font(.caption)
                    .accessibilityIdentifier("gator.composer.slash-feedback")
                    .background(NativeAccessibilityStatus(status: "Messages cannot start with a slash here. Add a word before the path, or use native conversation controls.").frame(width: 0, height: 0))
            }
            HStack {
                Button { NativeAttachmentActions.pick(owner: application, sessions: sessions) } label: { Image(systemName: "paperclip") }
                    .focusable().accessibilityLabel("Attach Files").help(NativeUploadLimits.hint).disabled(!application.canImportAttachments)
                Menu {
                    Button("Paste Image") {
                        if !NativeAttachmentActions.pasteImage(owner: application, explicitly: true), let request = application.beginAttachmentImport() { application.failAttachmentImport(request, message: "Copy an image first. Clipboard file paths and HTML are not imported as pixels.") }
                    }
                } label: { Image(systemName: "photo") }.menuStyle(.borderlessButton).fixedSize().accessibilityLabel("Image attachment options").disabled(!application.canImportAttachments)
                if application.correctionTarget == nil {
                    Toggle("Plan first", isOn: Binding(get: { application.planFirst }, set: { value in application.setPlanFirst(value) })).toggleStyle(.checkbox).font(.caption).focusable()
                        .help("Request a proposed plan for review before any implementation")
                }
                Text(compact ? "⇧↩ newline" : "⇧ Return: new line")
                    .font(.caption).foregroundStyle(.secondary)
                Spacer()
                Button {
                    let owner = application; Task { await sessions.submit(from: owner) }
                } label: { Image(systemName: "arrow.up").font(.system(size: 15, weight: .semibold)).frame(width: 30, height: 28) }
                .buttonStyle(.borderedProminent).clipShape(RoundedRectangle(cornerRadius: 9))
                .disabled(!sessions.canSubmit).help(application.sendUnavailableReason ?? "Send message")
                .accessibilityLabel("Send message").focusable()
            }
        }
        .task {
            // A newly inserted composer must request focus after SwiftUI restores
            // its focus hierarchy. Snapshot updates do not restart this task.
            await Task.yield()
            application.requestComposerFocus()
        }
        .padding(12)
        .background(Color(nsColor: .textBackgroundColor), in: RoundedRectangle(cornerRadius: 12))
        .onDrop(of: [UTType.fileURL.identifier], isTargeted: $dropTargeted) { providers in NativeAttachmentActions.drop(providers, owner: application) }
        .overlay(alignment: .top) { if dropTargeted { Label("Drop files to attach · no automatic send", systemImage: "arrow.down.doc").font(.caption).padding(8).background(.regularMaterial, in: RoundedRectangle(cornerRadius: 8)).allowsHitTesting(false) } }
        .overlay(RoundedRectangle(cornerRadius: 12).stroke(Color(nsColor: .separatorColor), lineWidth: 0.5))
    }
}
