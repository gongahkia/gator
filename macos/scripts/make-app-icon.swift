import AppKit
// Build-time native vector drawing, no downloaded artwork or runtime dependency.
let root = URL(fileURLWithPath: CommandLine.arguments[1], isDirectory: true)
try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
for size in [16, 32, 128, 256, 512] {
    for scale in [1, 2] {
        let pixels = size * scale
        let bitmap = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: pixels, pixelsHigh: pixels, bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: pixels * 4, bitsPerPixel: 32)!
        NSGraphicsContext.saveGraphicsState(); NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: bitmap)
        let inset = CGFloat(pixels) * 0.08, side = CGFloat(pixels) - inset * 2
        NSColor(srgbRed: 0.14, green: 0.40, blue: 0.29, alpha: 1).setFill()
        NSBezierPath(roundedRect: NSRect(x: inset, y: inset, width: side, height: side), xRadius: side * 0.23, yRadius: side * 0.23).fill()
        if let symbol = NSImage(systemSymbolName: "bubble.left.and.bubble.right.fill", accessibilityDescription: nil)?.withSymbolConfiguration(.init(pointSize: CGFloat(pixels) * 0.44, weight: .regular)) {
            let rect = NSRect(x: CGFloat(pixels) * 0.23, y: CGFloat(pixels) * 0.24, width: CGFloat(pixels) * 0.54, height: CGFloat(pixels) * 0.51)
            let glyph = NSImage(size: rect.size)
            glyph.lockFocus()
            symbol.draw(in: NSRect(origin: .zero, size: rect.size))
            NSColor.white.setFill(); NSRect(origin: .zero, size: rect.size).fill(using: .sourceIn)
            glyph.unlockFocus(); glyph.draw(in: rect)
        }
        NSGraphicsContext.restoreGraphicsState()
        let name = "icon_\(size)x\(size)" + (scale == 2 ? "@2x" : "") + ".png"
        try bitmap.representation(using: .png, properties: [:])!.write(to: root.appendingPathComponent(name))
    }
}
