// original geometric accents; no mascot raster, downloaded artwork or emoji font rasterization.
import Foundation
import CoreGraphics
import ImageIO
import UniformTypeIdentifiers
let root = URL(fileURLWithPath: CommandLine.arguments[1], isDirectory: true)
try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
for name in ["completed", "attention"] {
    guard let destination = CGImageDestinationCreateWithURL(root.appendingPathComponent(name + ".gif") as CFURL, UTType.gif.identifier as CFString, 12, nil) else { fatalError("GIF destination unavailable") }
    for frame in 0..<12 {
        let progress = Double(frame) / 11
        let context = CGContext(data: nil, width: 96, height: 96, bitsPerComponent: 8, bytesPerRow: 384,
                                space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
        // a quiet expanding ring of eight accents leaves native emoji text unobstructed.
        if frame > 0 && frame < 11 {
            let color = name == "completed" ? CGColor(red: 0.18, green: 0.58, blue: 0.39, alpha: 1) : CGColor(red: 0.66, green: 0.43, blue: 0.12, alpha: 1)
            context.setFillColor(color)
            for index in 0..<8 {
                let angle = Double(index) * .pi / 4
                let radius = 32 + 9 * progress
                let size = 2 + 2 * sin(progress * .pi)
                let x = 48 + cos(angle) * radius, y = 48 + sin(angle) * radius
                context.fillEllipse(in: CGRect(x: x-size/2, y: y-size/2, width: size, height: size))
            }
        }
        CGImageDestinationAddImage(destination, context.makeImage()!, [kCGImagePropertyGIFDictionary: [kCGImagePropertyGIFDelayTime: 0.06]] as CFDictionary)
    }
    guard CGImageDestinationFinalize(destination) else { fatalError("GIF export failed") }
}
