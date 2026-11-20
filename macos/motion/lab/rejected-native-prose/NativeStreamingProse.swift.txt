import SwiftUI
import AppKit

/// The active plain paragraph uses one selectable native text store. Earlier
/// Markdown blocks keep their existing SwiftUI renderer and selection behavior.
struct NativeStreamingProse: NSViewRepresentable {
    let cursor: NativeStreamingReveal
    let response: String
    func makeNSView(context: Context) -> ProseView { ProseView() }
    func updateNSView(_ view: ProseView, context: Context) {
        view.configure(source: cursor.source, visible: cursor.boundaries[min(cursor.visible, cursor.boundaries.count - 1)],
                       entering: cursor.boundaries[min(cursor.entering, cursor.boundaries.count - 1)], response: response)
    }
    func sizeThatFits(_ proposal: ProposedViewSize, nsView: ProseView, context: Context) -> CGSize? {
        nsView.measured(width: proposal.width ?? 400)
    }
    @MainActor final class ProseView: NSTextView {
        private var source = ""
        private var response = ""
        private var length = 0
        private var tail = NSRange(location: 0, length: 0)
        init() {
            let storage = NSTextStorage(), layout = NSLayoutManager(), container = NSTextContainer(containerSize: .init(width: 400, height: CGFloat.greatestFiniteMagnitude))
            storage.addLayoutManager(layout); layout.addTextContainer(container)
            super.init(frame: .zero, textContainer: container)
            drawsBackground = false; isEditable = false; isSelectable = true; isRichText = false
            textContainerInset = .zero; container.lineFragmentPadding = 0
            container.widthTracksTextView = false; container.heightTracksTextView = false
            isHorizontallyResizable = false; isVerticallyResizable = true
            font = .preferredFont(forTextStyle: .body); textColor = .labelColor
            setAccessibilityLabel("Assistant response")
        }
        required init?(coder: NSCoder) { nil }
        private var attributes: [NSAttributedString.Key: Any] {
            let paragraph = NSMutableParagraphStyle(); paragraph.lineSpacing = 3
            return [.font: font ?? NSFont.systemFont(ofSize: NSFont.systemFontSize), .foregroundColor: NSColor.labelColor, .paragraphStyle: paragraph]
        }
        func configure(source next: String, visible: Int, entering: Int, response: String) {
            guard let storage = textStorage else { return }
            let end = min(visible, next.utf16.count), fade = min(entering, end)
            let same = next.utf8.elementsEqual(source.utf8)
            self.response = response
            if same && length == end && tail == NSRange(location: fade, length: end - fade) { return }
            let append = (same || next.utf8.starts(with: source.utf8)) && end >= length
            let layoutChanged = !append || end != length
            let selection = selectedRanges
            storage.beginEditing()
            if append {
                if tail.length > 0 { storage.addAttribute(.foregroundColor, value: NSColor.labelColor, range: tail) }
                if end > length {
                    storage.append(NSAttributedString(string: (next as NSString).substring(with: NSRange(location: length, length: end - length)), attributes: attributes))
                }
            } else {
                storage.setAttributedString(NSAttributedString(string: (next as NSString).substring(to: end), attributes: attributes))
            }
            tail = NSRange(location: fade, length: end - fade)
            if tail.length > 0 { storage.addAttribute(.foregroundColor, value: NSColor.labelColor.withAlphaComponent(0.88), range: tail) }
            storage.endEditing()
            if append && layoutChanged {
                let rendered = storage.string as NSString
                selectedRanges = selection.map { value in
                    let range = value.rangeValue
                    guard range.location + range.length <= end else { return NSValue(range: NSRange(location: end, length: 0)) }
                    return NSValue(range: range.length == 0 ? range : rendered.rangeOfComposedCharacterSequences(for: range))
                }
            }
            source = next; self.response = response; length = end
            if !same { setAccessibilityValue(next) }
            if layoutChanged { invalidateIntrinsicContentSize() }
        }
        func measured(width: CGFloat) -> CGSize {
            guard let container = textContainer, let layout = layoutManager else { return .zero }
            let width = max(1, width)
            if container.containerSize.width != width { container.containerSize = CGSize(width: width, height: CGFloat.greatestFiniteMagnitude) }
            layout.ensureLayout(for: container)
            return CGSize(width: width, height: max(1, ceil(layout.usedRect(for: container).height)))
        }
        override func menu(for event: NSEvent) -> NSMenu? {
            let menu = super.menu(for: event) ?? NSMenu()
            menu.addItem(.separator())
            let item = NSMenuItem(title: "Copy Response", action: #selector(copyResponse), keyEquivalent: "")
            item.target = self; menu.addItem(item); return menu
        }
        @objc private func copyResponse() { NativeStreamingCopy.write(response, to: .general) }
    }
}
