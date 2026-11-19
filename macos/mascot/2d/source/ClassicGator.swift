// original flat illustration; coordinates are editable vector authoring data
import CoreGraphics
import Foundation

enum ClassicGator {
    static func draw(_ context: CGContext, pixels: Int, pose: SpritePose) {
        context.saveGState()
        context.scaleBy(x: CGFloat(pixels) / 224, y: CGFloat(pixels) / 224)
        context.translateBy(x: 112, y: 112); context.rotate(by: CGFloat(pose.tilt)); context.translateBy(x: -112, y: -112)
        let ink: UInt32 = 0x173F32, skin: UInt32 = 0x46956C, snout: UInt32 = 0x83C79B, jaw: UInt32 = 0xD1E3AA
        func fill(_ color: UInt32) { context.setFillColor(CGColor(srgbRed: CGFloat((color >> 16) & 255) / 255, green: CGFloat((color >> 8) & 255) / 255, blue: CGFloat(color & 255) / 255, alpha: 1)) }
        func ellipse(_ rect: CGRect, _ color: UInt32) { fill(color); context.fillEllipse(in: rect) }
        func rounded(_ rect: CGRect, radius: CGFloat, _ color: UInt32) { fill(color); context.addPath(CGPath(roundedRect: rect, cornerWidth: radius, cornerHeight: radius, transform: nil)); context.fillPath() }
        let opening = CGFloat(pose.open) * 22
        // the short wide muzzle and tiny teeth distinguish the silhouette from a frog
        ellipse(CGRect(x: 38, y: 28, width: 148, height: 160), skin)
        for x in [82, 109, 136] { ellipse(CGRect(x: x, y: 177, width: 15, height: 17), skin) }
        rounded(CGRect(x: 39, y: 42 - opening, width: 146, height: 43), radius: 24, jaw)
        rounded(CGRect(x: 36, y: 72 - opening, width: 152, height: 20 + opening), radius: 10, ink)
        if opening > 3 { ellipse(CGRect(x: 94, y: 54 - opening * 0.38, width: 36, height: 9), 0xD58D87) }
        rounded(CGRect(x: 27, y: 76, width: 170, height: 55), radius: 25, snout)
        ellipse(CGRect(x: 78, y: 108, width: 7, height: 5), ink); ellipse(CGRect(x: 139, y: 108, width: 7, height: 5), ink)
        for x in [56, 160] { rounded(CGRect(x: x, y: 69, width: 8, height: 12), radius: 4, 0xFBFAE8) }
        // a simple mouth curve reads clearly at compact sizes
        context.setStrokeColor(CGColor(srgbRed: 0.09, green: 0.25, blue: 0.19, alpha: 1)); context.setLineWidth(3); context.setLineCap(.round)
        let smile = CGMutablePath(); smile.move(to: CGPoint(x: 57, y: 77)); smile.addQuadCurve(to: CGPoint(x: 167, y: 77), control: CGPoint(x: 112, y: 68 - max(0, -CGFloat(pose.concern)) * 5)); context.addPath(smile); context.strokePath()
        for x: CGFloat in [69, 129] {
            ellipse(CGRect(x: x - 4, y: 125, width: 34, height: 44), skin)
            let height = max(3, 36 * (1 - CGFloat(pose.blink)))
            ellipse(CGRect(x: x, y: 130 + (36 - height) / 2, width: 27, height: height), 0xFBFAE8)
            if pose.blink < 0.85 {
                context.saveGState(); context.addEllipse(in: CGRect(x: x, y: 130 + (36 - height) / 2, width: 27, height: height)); context.clip()
                ellipse(CGRect(x: x + 8 + CGFloat(pose.look) * 4, y: 148 - height * 0.29, width: 11, height: max(3, height * 0.58)), ink)
                ellipse(CGRect(x: x + 9 + CGFloat(pose.look) * 4, y: 148 + height * 0.14, width: 3, height: 4), 0xFFFFFF)
                context.restoreGState()
            } else {
                context.move(to: CGPoint(x: x + 4, y: 148)); context.addLine(to: CGPoint(x: x + 23, y: 148)); context.strokePath()
            }
            if pose.concern > 0.3 {
                context.move(to: CGPoint(x: x - 1, y: 175)); context.addLine(to: CGPoint(x: x + 24, y: 171 + CGFloat(pose.concern) * 5)); context.strokePath()
            }
        }
        context.restoreGState()
    }
}
