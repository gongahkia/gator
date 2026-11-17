import AppKit
import Testing
@testable import GatorMac

@Test func cameraGapRequiresActualDisplayEvidence() throws {
    let frame = NSRect(x: 0, y: 0, width: 1470, height: 956)
    let left = NSRect(x: 0, y: 924, width: 646, height: 32)
    let right = NSRect(x: 825, y: 924, width: 645, height: 32)
    #expect(NotchGeometry.detect(frame: frame, safeTop: 0, left: left, right: right) == nil)
    #expect(NotchGeometry.detect(frame: frame, safeTop: 32, left: nil, right: right) == nil)
    #expect(NotchGeometry.detect(frame: frame, safeTop: 32, left: right, right: left) == nil)
    let geometry = try #require(NotchGeometry.detect(frame: frame, safeTop: 32, left: left, right: right))
    #expect(geometry.cameraWidth == 179)
    #expect(geometry.panelFrame(expanded: false) == NSRect(x: 613.5, y: 918, width: 243, height: 38))
    #expect(geometry.panelFrame(expanded: true) == NSRect(x: 525, y: 710, width: 420, height: 246))
}

@Test func notchPanelTracksNegativeDisplayOriginsAndStaysWithinScreen() throws {
    let frame = NSRect(x: -1470, y: 300, width: 1470, height: 956)
    let left = NSRect(x: -1470, y: 1224, width: 646, height: 32)
    let right = NSRect(x: -645, y: 1224, width: 645, height: 32)
    let geometry = try #require(NotchGeometry.detect(frame: frame, safeTop: 32, left: left, right: right))
    for expanded in [false, true] {
        let panel = geometry.panelFrame(expanded: expanded)
        #expect(panel.midX == frame.midX)
        #expect(panel.maxY == frame.maxY)
        #expect(frame.contains(panel))
    }
}
