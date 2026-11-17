import AppKit

// Original alligator silhouette, shared by the PNG template and application icon.
let root = URL(fileURLWithPath: CommandLine.arguments[1], isDirectory: true)
try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)

func head() -> NSBezierPath {
    let path = NSBezierPath()
    path.move(to: NSPoint(x: 9, y: 59))
    path.curve(to: NSPoint(x: 48, y: 67), controlPoint1: NSPoint(x: 15, y: 66), controlPoint2: NSPoint(x: 35, y: 62))
    path.curve(to: NSPoint(x: 64, y: 79), controlPoint1: NSPoint(x: 51, y: 80), controlPoint2: NSPoint(x: 59, y: 86))
    path.curve(to: NSPoint(x: 85, y: 67), controlPoint1: NSPoint(x: 76, y: 83), controlPoint2: NSPoint(x: 87, y: 77))
    path.line(to: NSPoint(x: 94, y: 24))
    path.line(to: NSPoint(x: 70, y: 18))
    path.line(to: NSPoint(x: 60, y: 35))
    path.curve(to: NSPoint(x: 17, y: 40), controlPoint1: NSPoint(x: 39, y: 29), controlPoint2: NSPoint(x: 25, y: 32))
    path.line(to: NSPoint(x: 10, y: 48))
    path.line(to: NSPoint(x: 46, y: 46))
    path.line(to: NSPoint(x: 53, y: 50))
    path.line(to: NSPoint(x: 12, y: 53))
    path.curve(to: NSPoint(x: 9, y: 59), controlPoint1: NSPoint(x: 7, y: 53), controlPoint2: NSPoint(x: 6, y: 56))
    path.close()
    return path
}

func png(pixels: Int, appIcon: Bool) throws -> Data {
    let bitmap = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: pixels, pixelsHigh: pixels, bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: pixels * 4, bitsPerPixel: 32)!
    NSGraphicsContext.saveGraphicsState()
    defer { NSGraphicsContext.restoreGraphicsState() }
    NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: bitmap)
    let transform = NSAffineTransform()
    transform.scale(by: CGFloat(pixels) / 100)
    transform.concat()
    if appIcon {
        NSColor.black.setFill()
        NSBezierPath(roundedRect: NSRect(x: 6, y: 6, width: 88, height: 88), xRadius: 20, yRadius: 20).fill()
        let inset = NSAffineTransform()
        inset.translateX(by: 10, yBy: 10)
        inset.scale(by: 0.8)
        inset.concat()
    }
    (appIcon ? NSColor.white : NSColor.black).setFill()
    head().fill()
    for x in [26.0, 39.0] {
        let tooth = NSBezierPath()
        tooth.move(to: NSPoint(x: x, y: 52))
        tooth.line(to: NSPoint(x: x + 3, y: 47))
        tooth.line(to: NSPoint(x: x + 6, y: 52))
        tooth.close()
        tooth.fill()
    }
    // Clear eye and nostril keep the monochrome template legible in either appearance.
    NSGraphicsContext.current?.compositingOperation = appIcon ? .sourceOver : .copy
    (appIcon ? NSColor.black : NSColor.clear).setFill()
    NSBezierPath(ovalIn: NSRect(x: 58, y: 67, width: 6, height: 6)).fill()
    NSBezierPath(ovalIn: NSRect(x: 16, y: 56, width: 4, height: 3)).fill()
    return bitmap.representation(using: .png, properties: [:])!
}

for size in [16, 32, 128, 256, 512] {
    for scale in [1, 2] {
        let name = "icon_\(size)x\(size)" + (scale == 2 ? "@2x" : "") + ".png"
        try png(pixels: size * scale, appIcon: true).write(to: root.appendingPathComponent(name))
    }
}
let resources = root.deletingLastPathComponent()
try png(pixels: 1024, appIcon: false).write(to: resources.appendingPathComponent("GatorHead.png"))
try png(pixels: 36, appIcon: false).write(to: resources.appendingPathComponent("GatorMenuBar.png"))
