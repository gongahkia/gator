import Foundation

struct SpritePose {
    var open: Float = 0
    var blink: Float = 0
    var look: Float = 0
    var tilt: Float = 0
    var concern: Float = 0
    func mixed(_ other: Self, _ amount: Float) -> Self {
        func mix(_ a: Float, _ b: Float) -> Float { a + (b - a) * amount }
        return Self(open: mix(open, other.open), blink: mix(blink, other.blink), look: mix(look, other.look), tilt: mix(tilt, other.tilt), concern: mix(concern, other.concern))
    }
}
struct SpriteRecipe {
    let name: String
    let duration: Double
    let poses: [SpritePose]
    var staticPose: SpritePose { poses.last ?? SpritePose() }
    static func path(_ a: SpritePose, _ b: SpritePose, count: Int) -> [SpritePose] {
        (0..<count).map { a.mixed(b, Float($0) / Float(count - 1)) }
    }
    static let all: [Self] = {
        let idle = SpritePose(), thinking = SpritePose(look: 0.55, tilt: -0.055)
        let happy = SpritePose(open: 0.12, tilt: 0.035, concern: -0.25)
        return [
            Self(name: "neutral", duration: 0, poses: [idle]),
            Self(name: "blink", duration: 0.30, poses: path(idle, SpritePose(blink: 1), count: 4) + path(SpritePose(blink: 1), idle, count: 4)),
            Self(name: "working", duration: 0.80, poses: path(idle, SpritePose(look: -0.4, tilt: 0.025), count: 5) + path(SpritePose(look: -0.4, tilt: 0.025), thinking, count: 7)),
            Self(name: "responding", duration: 0.55, poses: path(idle, SpritePose(open: 0.40, look: 0.12), count: 4) + path(SpritePose(open: 0.40, look: 0.12), idle, count: 5)),
            Self(name: "completed", duration: 0.65, poses: path(idle, SpritePose(open: 0.22, tilt: -0.04, concern: -0.35), count: 5) + path(SpritePose(open: 0.22, tilt: -0.04, concern: -0.35), happy, count: 6)),
            Self(name: "attention", duration: 0.55, poses: path(idle, SpritePose(look: 0.2, tilt: -0.025, concern: -0.5), count: 9)),
            Self(name: "disconnected", duration: 0.60, poses: path(idle, SpritePose(tilt: 0.035, concern: 0.85), count: 9))
        ]
    }()
}
