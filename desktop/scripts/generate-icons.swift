// Run on macOS: swift desktop/scripts/generate-icons.swift
// Rasterize the same 🐊 symbol used in Gator's UI, with no tile or wordmark.
// Generated assets are checked in; normal builds need neither Swift nor macOS.
import AppKit
import CoreText

let script = URL(fileURLWithPath: #filePath)
let assets = script.deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("assets")
let manager = FileManager.default
try manager.createDirectory(at: assets, withIntermediateDirectories: true)

let size = 1024
let bitmap = NSBitmapImageRep(
    bitmapDataPlanes: nil, pixelsWide: size, pixelsHigh: size,
    bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true,
    isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0
)!
let graphics = NSGraphicsContext(bitmapImageRep: bitmap)!
NSGraphicsContext.saveGraphicsState()
NSGraphicsContext.current = graphics
let context = graphics.cgContext
context.clear(CGRect(x: 0, y: 0, width: size, height: size))
let font = NSFont(name: "Apple Color Emoji", size: 880)!
let symbol = NSAttributedString(string: "🐊", attributes: [.font: font])
let line = CTLineCreateWithAttributedString(symbol)
let bounds = CTLineGetImageBounds(line, context)
let inset: CGFloat = 80
let scale = min((CGFloat(size) - inset * 2) / bounds.width, (CGFloat(size) - inset * 2) / bounds.height)
context.translateBy(x: CGFloat(size) / 2, y: CGFloat(size) / 2)
context.scaleBy(x: scale, y: scale)
context.textPosition = CGPoint(x: -bounds.midX, y: -bounds.midY)
CTLineDraw(line, context)
NSGraphicsContext.restoreGraphicsState()
let png = bitmap.representation(using: .png, properties: [:])!
try png.write(to: assets.appendingPathComponent("icon.png"))

let temporary = manager.temporaryDirectory.appendingPathComponent(UUID().uuidString)
let iconset = temporary.appendingPathComponent("Gator.iconset")
try manager.createDirectory(at: iconset, withIntermediateDirectories: true)
defer { try? manager.removeItem(at: temporary) }
for points in [16, 32, 128, 256, 512] {
    for density in [1, 2] {
        let pixels = points * density
        let target = iconset.appendingPathComponent("icon_\(points)x\(points)\(density == 2 ? "@2x" : "").png")
        let resize = Process()
        resize.executableURL = URL(fileURLWithPath: "/usr/bin/sips")
        resize.arguments = ["-z", String(pixels), String(pixels), assets.appendingPathComponent("icon.png").path, "--out", target.path]
        resize.standardOutput = FileHandle.nullDevice
        try resize.run()
        resize.waitUntilExit()
        guard resize.terminationStatus == 0 else { fatalError("Icon resize failed") }
    }
}
let convert = Process()
convert.executableURL = URL(fileURLWithPath: "/usr/bin/iconutil")
convert.arguments = ["-c", "icns", iconset.path, "-o", assets.appendingPathComponent("icon.icns").path]
try convert.run()
convert.waitUntilExit()
guard convert.terminationStatus == 0 else { fatalError("Icon conversion failed") }
print("Generated alligator app icons in \(assets.path)")
