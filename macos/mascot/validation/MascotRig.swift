// isolated P04M-A2 validation; no application or execution dependencies
import Foundation
import RealityKit
import simd

enum MascotExpression: String, CaseIterable {
    case idle, working, responding, completed, needsAttention, disconnected, reducedMotion
    var pose: MascotPose {
        switch self {
        case .idle, .reducedMotion: MascotPose()
        case .working: MascotPose(look: 0.6, tilt: -0.055)
        case .responding: MascotPose(open: 0.35, look: 0.12)
        case .completed: MascotPose(open: 0.12, tilt: 0.035)
        case .needsAttention: MascotPose(look: 0.2, tilt: -0.025, concern: -0.5)
        case .disconnected: MascotPose(tilt: 0.035, concern: 0.85)
        }
    }
}

struct MascotPose: Equatable {
    var open: Float = 0
    var blink: Float = 0
    var look: Float = 0
    var tilt: Float = 0
    var concern: Float = 0
    func interpolated(to other: Self, fraction: Float) -> Self {
        let t = min(1, max(0, fraction))
        func mix(_ a: Float, _ b: Float) -> Float { a + (b - a) * t }
        return Self(open: mix(open, other.open), blink: mix(blink, other.blink),
                    look: mix(look, other.look), tilt: mix(tilt, other.tilt), concern: mix(concern, other.concern))
    }
}

@MainActor final class MascotRig {
    let root: Entity
    let head: Entity
    let jaw: Entity
    let cavity: Entity
    let tongue: Entity
    let eyes: [Entity]
    let pupils: [Entity]
    let upper: [Entity]
    let lower: [Entity]
    let seams: [Entity]
    private(set) var pose = MascotPose()
    init(root: Entity) throws {
        func require(_ name: String, in parent: Entity) throws -> Entity {
            guard let part = parent.findEntity(named: name) else {
                throw NSError(domain: "MascotRigMissingPart", code: 1, userInfo: [NSLocalizedDescriptionKey: name])
            }
            return part
        }
        self.root = root
        let resolvedHead = try require("Head_Pivot", in: root)
        head = resolvedHead
        jaw = try require("Jaw_Pivot", in: resolvedHead)
        cavity = try require("Mouth_Interior", in: resolvedHead)
        tongue = try require("Tongue", in: resolvedHead)
        eyes = try ["Eye_Left", "Eye_Right"].map { try require($0, in: resolvedHead) }
        pupils = try eyes.map { try require("Pupil", in: $0) }
        upper = try eyes.map { try require("Upper_Lid", in: $0) }
        lower = try eyes.map { try require("Lower_Lid", in: $0) }
        seams = try eyes.map { try require("Closed_Eye_Line", in: $0) }
        apply(MascotPose())
    }
    func apply(_ value: MascotPose) {
        pose = value
        let open = min(1, max(0, value.open)), blink = min(1, max(0, value.blink))
        jaw.orientation = simd_quatf(angle: open * 0.5, axis: [1, 0, 0])
        cavity.scale.y = 0.022 + open * 0.21
        cavity.position.y = -0.36 - open * 0.075
        tongue.isEnabled = open >= 0.35
        tongue.position.y = -0.43 - open * 0.065
        head.orientation = simd_quatf(angle: value.tilt, axis: [0, 0, 1])
        for i in eyes.indices {
            pupils[i].position.x = min(1, max(-1, value.look)) * 0.035
            upper[i].orientation = simd_quatf(angle: -(1 - blink) * (.pi / 2 - 0.04) + value.concern * (i == 0 ? 0.14 : 0.04), axis: [1, 0, 0])
            lower[i].orientation = simd_quatf(angle: (1 - blink) * (.pi / 2 - 0.04), axis: [1, 0, 0])
            seams[i].isEnabled = blink > 0.9
        }
    }
    func verify() throws -> [String: Any] {
        var assertions = 0
        func check(_ condition: Bool, _ name: String) throws {
            guard condition else { throw NSError(domain: "MascotRigAssertion", code: assertions, userInfo: [NSLocalizedDescriptionKey: name]) }
            assertions += 1
        }
        let originalHead = head.transform, originalJaw = jaw.transform
        let originalEyes = eyes.map(\.transform), originalPupils = pupils.map(\.transform)
        apply(MascotPose(open: 0.85))
        try check(jaw.transform != originalJaw, "jaw changes")
        try check(head.transform == originalHead, "jaw does not move head")
        try check(eyes.map(\.transform) == originalEyes, "jaw does not move eye roots")
        let openJaw = jaw.transform
        apply(MascotPose(open: 0.85, blink: 1))
        try check(jaw.transform == openJaw, "blink does not move jaw")
        try check(upper.allSatisfy { abs($0.orientation.angle) < 0.001 }, "upper lids close")
        try check(lower.allSatisfy { abs($0.orientation.angle) < 0.001 }, "lower lids close")
        try check(seams.allSatisfy(\.isEnabled), "blink seam visible")
        apply(MascotPose(look: 1))
        try check(pupils.map(\.transform) != originalPupils, "pupils move")
        try check(eyes.map(\.transform) == originalEyes, "gaze does not move eye roots")
        let neutralLids = upper.map(\.transform)
        for _ in 0..<1000 {
            for expression in MascotExpression.allCases { apply(expression.pose) }
            apply(MascotPose(blink: 1)); apply(MascotPose())
        }
        try check(head.transform == originalHead, "head neutral does not accumulate")
        try check(jaw.transform == originalJaw, "jaw neutral does not accumulate")
        try check(pupils.map(\.transform) == originalPupils, "pupils neutral does not accumulate")
        try check(upper.map(\.transform) == neutralLids, "lids neutral do not accumulate")
        try check(!tongue.isEnabled && seams.allSatisfy { !$0.isEnabled }, "neutral hidden parts restored")
        for expression in MascotExpression.allCases {
            apply(expression.pose)
            try check(pose == expression.pose, "expression applied \(expression.rawValue)")
        }
        apply(MascotPose())
        return ["assertions": assertions, "cycles": 1000, "independentParts": ["Jaw_Pivot", "Eye_Left/Pupil", "Eye_Right/Pupil", "Eye_Left/Upper_Lid", "Eye_Right/Upper_Lid", "Eye_Left/Lower_Lid", "Eye_Right/Lower_Lid"], "jawPivot": [jaw.position.x, jaw.position.y, jaw.position.z]]
    }
}
