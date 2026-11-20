import AppKit
struct NotchGeometry: Equatable {
    let frame: NSRect
    let cameraWidth: CGFloat
    let cameraHeight: CGFloat

    static func detect(frame: NSRect, safeTop: CGFloat, left: NSRect?, right: NSRect?) -> Self? {
        guard safeTop > 0, let left, let right, right.minX > left.maxX,
              left.minX >= frame.minX, right.maxX <= frame.maxX else { return nil }
        let gap = right.minX - left.maxX
        guard gap < frame.width / 2 else { return nil }
        return Self(frame: frame, cameraWidth: gap, cameraHeight: safeTop)
    }
    func panelFrame(expanded: Bool) -> NSRect {
        let width = min(frame.width - 24, expanded ? max(420, cameraWidth + 96) : cameraWidth + 64)
        let height = cameraHeight + (expanded ? 214 : 6)
        return NSRect(x: frame.midX - width / 2, y: frame.maxY - height, width: width, height: height)
    }
}

