import AppKit
import UniformTypeIdentifiers

// Headless share extension: reads the shared payload, serialises it
// into the App Group's UserDefaults, then dismisses. The main app
// consumes SharedInboxItem on next activation (see
// AppModel.consumePendingSharedItems).
final class ShareViewController: NSViewController {
    override var nibName: NSNib.Name? { nil }

    override func loadView() {
        view = NSView(frame: NSRect(x: 0, y: 0, width: 10, height: 10))
    }

    override func viewDidAppear() {
        super.viewDidAppear()
        Task { await processInput() }
    }

    @MainActor
    private func processInput() async {
        guard let context = extensionContext else {
            completeRequest(success: false)
            return
        }
        let items = (context.inputItems as? [NSExtensionItem]) ?? []
        let payload = await extractPayload(from: items)
        guard let payload else {
            completeRequest(success: false)
            return
        }
        SharedInboxDefaults.append(payload)
        completeRequest(success: true)
    }

    private func extractPayload(from items: [NSExtensionItem]) async -> SharedInboxItem? {
        var title: String?
        var url: String?
        for item in items {
            if let text = item.attributedContentText?.string, text.isEmpty == false, title == nil {
                title = text
            }
            for provider in item.attachments ?? [] {
                if provider.hasItemConformingToTypeIdentifier(UTType.url.identifier), url == nil {
                    if let loaded = try? await provider.loadItem(forTypeIdentifier: UTType.url.identifier),
                       let candidate = (loaded as? URL)?.absoluteString ?? loaded as? String {
                        url = candidate
                    }
                }
                if provider.hasItemConformingToTypeIdentifier(UTType.plainText.identifier), title == nil {
                    if let loaded = try? await provider.loadItem(forTypeIdentifier: UTType.plainText.identifier),
                       let candidate = loaded as? String {
                        title = candidate
                    }
                }
            }
        }
        let source = Bundle.main.bundleIdentifier
        if let link = url, let label = title {
            return SharedInboxItem(text: "\(label) \(link)", createdAt: Date(), source: source)
        }
        if let link = url {
            return SharedInboxItem(text: link, createdAt: Date(), source: source)
        }
        if let label = title {
            return SharedInboxItem(text: label, createdAt: Date(), source: source)
        }
        return nil
    }

    private func completeRequest(success: Bool) {
        extensionContext?.completeRequest(returningItems: nil, completionHandler: nil)
    }
}
