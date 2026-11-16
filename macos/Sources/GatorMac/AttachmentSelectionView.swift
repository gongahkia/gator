import SwiftUI
import AppKit
import GatorCore

struct AttachmentSelectionView: View {
    @Environment(GatorApplicationState.self) private var owner
    let compact: Bool
    var body: some View {
        if !owner.draftUploads.isEmpty {
            VStack(alignment: .leading, spacing: 5) {
                Text("Attachments · selected, not sent").font(.caption.weight(.medium))
                ScrollView {
                    LazyVStack(spacing: 6) {
                        ForEach(owner.draftUploads) { file in
                            HStack(alignment: .center, spacing: 8) {
                                AttachmentThumbnail(file: file).frame(width: 38, height: 38).accessibilityHidden(true)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(verbatim: file.name).font(.callout).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                                    Text(file.typeLabel + " · " + ByteCountFormatter.string(fromByteCount: Int64(file.size), countStyle: .file)).font(.caption2).foregroundStyle(.secondary)
                                }.frame(maxWidth: .infinity, alignment: .leading)
                                Button { owner.removeAttachment(file.id); owner.requestComposerFocus() } label: { Image(systemName: "xmark.circle") }
                                    .focusable().buttonStyle(.borderless).accessibilityLabel("Remove attachment " + file.name)
                                    .help("Remove from this message").disabled(owner.attachmentImport != nil)
                            }.padding(7).background(.background.opacity(0.6), in: RoundedRectangle(cornerRadius: 8))
                                .accessibilityElement(children: .contain)
                        }
                    }
                }.frame(height: min(CGFloat(owner.draftUploads.count) * 62, compact ? 118 : 150))
            }.task(id: owner.draftUploads) { await owner.validateDraftAttachments() }
        }
    }
}
private struct AttachmentThumbnail: View {
    @Environment(GatorApplicationState.self) private var owner
    let file: NativeUpload
    @State private var image: NSImage?
    var body: some View {
        Group {
            if let image { Image(nsImage: image).resizable().scaledToFit() }
            else { Image(systemName: file.isImage ? "photo" : "doc.text").font(.title2).foregroundStyle(.secondary) }
        }.task(id: file.id) { if file.isImage, let data = await owner.attachmentThumbnail(file) { image = NSImage(data: data) } }
    }
}
