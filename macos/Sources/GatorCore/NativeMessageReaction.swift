import Foundation

/// A confirmed Go acknowledgement bound to one exact durable dialogue turn.
/// This validates presentation metadata; it does not classify user intent.
public struct NativeMessageReaction: Equatable, Sendable, Identifiable {
    public let conversation: String
    public let turn: String
    public let revision: Int
    public let kind: String
    public let emoji: String
    public let expressive: Bool
    public let accessibilityLabel: String
    public var id: String { conversation + ":" + turn + ":" + String(revision) }
    public init?(turn value: JSONValue, conversation: String) {
        let reaction = value["reaction"]
        let revision = reaction["revision"].int
        guard !conversation.isEmpty, !value["id"].string.isEmpty,
              value["state"].string == "answered", !value["response"].string.isEmpty,
              value["issue"].string.isEmpty, value["task_chat"].string.isEmpty,
              value["input_ids"].array.isEmpty, value["reply_to_update_id"].string.isEmpty,
              reaction["version"] == .number(1), reaction["conversation_id"].string.utf8.elementsEqual(conversation.utf8),
              reaction["turn_id"].string.utf8.elementsEqual(value["id"].string.utf8), revision > 0,
              reaction["revision"] == .number(Decimal(revision)), value["revision"] == reaction["revision"] else { return nil }
        let kind = reaction["kind"].string
        let expected: (String, String, String, String)
        switch kind {
        case "appreciation": expected = ("❤️", "subtle", "Gator reacted with a heart: appreciation", "You're welcome.")
        case "laughter": expected = ("😂", "subtle", "Gator reacted with laughter", "Glad that made you smile.")
        case "praise": expected = ("🙌", "subtle", "Gator reacted with raised hands: appreciation", "Thanks for the kind words.")
        case "farewell": expected = ("👋", "subtle", "Gator reacted with a farewell wave", "Take care!")
        case "greeting": expected = ("👋", "expressive", "Gator reacted with a greeting wave", "Hello!")
        default: return nil
        }
        guard reaction["emoji"].string.utf8.elementsEqual(expected.0.utf8), reaction["level"].string == expected.1, value["response"].string.utf8.elementsEqual(expected.3.utf8) else { return nil }
        self.conversation = conversation; turn = value["id"].string; self.revision = revision
        self.kind = kind; emoji = expected.0; expressive = expected.1 == "expressive"; accessibilityLabel = expected.2
    }
}

public extension BridgeMessage {
    var messageReaction: NativeMessageReaction? {
        guard isUser, let reaction = NativeMessageReaction(turn: value["reaction_turn"], conversation: value["reaction_conversation"].string),
              id.utf8.elementsEqual(("conversation:" + reaction.conversation + ":dialogue:" + reaction.turn + ":user").utf8) else { return nil }
        return reaction
    }
}
