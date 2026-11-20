import SwiftUI
import GatorCore

enum NativeQuickReactionMode: String, CaseIterable {
    case off, subtle, expressive
    static let preference = "gator.messages.quickReactions"
    var label: String { rawValue.capitalized }
    static func resolve(_ raw: String?) -> Self { Self(rawValue: raw ?? "") ?? .subtle }
    func displays(_ reaction: NativeMessageReaction) -> Bool { self != .off && (self == .expressive || !reaction.expressive) }
    static func transcript(_ messages: [BridgeMessage], mode: Self) -> [BridgeMessage] {
        guard mode != .off else { return messages }
        let reacted = Set(messages.compactMap { message -> String? in
            guard let reaction = message.messageReaction, mode.displays(reaction) else { return nil }
            return message.id
        })
        return messages.filter { !reacted.contains($0.value["reaction_for"].string) }
    }
    static func position(_ saved: NativeReadingPositions.Position, raw: [BridgeMessage], mode: Self) -> NativeReadingPositions.Position {
        guard !saved.following, let anchor = saved.anchor, let response = raw.first(where: { $0.id == anchor }),
              let user = raw.first(where: { $0.id == response.value["reaction_for"].string }),
              let reaction = user.messageReaction, mode.displays(reaction) else { return saved }
        return .init(anchor: user.id, following: false)
    }
}

struct NativeMessageReactionChip: View {
    let reaction: NativeMessageReaction
    var body: some View {
        Text(verbatim: reaction.emoji).font(.system(size: 16))
            .padding(.horizontal, 7).padding(.vertical, 3)
            .background(Color(nsColor: .controlBackgroundColor), in: Capsule())
            .overlay(Capsule().strokeBorder(Color(nsColor: .separatorColor), lineWidth: 0.5))
            .accessibilityLabel(reaction.accessibilityLabel)
            .accessibilityHint("Conversational acknowledgement only; not approval or task completion")
            .allowsHitTesting(false)
            .transaction { $0.animation = nil; $0.disablesAnimations = true }
    }
}
