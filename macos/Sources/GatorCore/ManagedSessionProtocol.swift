import Foundation

// Host-private contracts. SwiftUI never constructs these frames or task IDs.
func validConversationID(_ id: String) -> Bool {
    id.range(of: "^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$", options: .regularExpression) != nil
}

struct SessionRequest: Sendable, Equatable {
    let sequence: Int
    let conversationID: String
    let goal: Bool
    let new: Bool
    let view: String
    let source: String
    let origin: String
    init(_ value: JSONValue) throws {
        sequence = value["sequence"].int; conversationID = value["conversation_id"].string
        goal = value["goal"].bool; new = value["new"].bool; view = value["view"].string
        source = value["source"].string; origin = value["origin_conversation_id"].string
        guard sequence > 0, conversationID.isEmpty || validConversationID(conversationID), source.utf8.count <= 8192 else { throw BridgeFailure("Invalid managed navigation request. Original chat retained.") }
    }
}

struct DialogueDispatch: Sendable, Equatable {
    enum Kind: String, Sendable { case task, guide, stop, revise_plan, revise_action }
    let sequence: Int
    let kind: Kind
    let turnID: String
    let childID: String
    let targetRun: String
    let instruction: String
    let ownerTask: String
    init(_ value: JSONValue) throws {
        sequence = value["sequence"].int
        guard let kind = Kind(rawValue: value["kind"].string) else { throw BridgeFailure("Unknown managed task request. Inspect the originating chat.") }
        self.kind = kind; turnID = value["turn_id"].string; childID = value["child_id"].string
        targetRun = value["target_run"].string; instruction = value["instruction"].string; ownerTask = value["owner_task"].string
        guard sequence > 0, validConversationID(turnID), validConversationID(childID), instruction.utf8.count <= 128 * 1024 else { throw BridgeFailure("Invalid managed task identity. No task was launched or controlled.") }
    }
}

struct DialogueControl: Sendable {
    let parentID: String
    let turnID: String
    let requestID: String
    let runID: String
    let action: DialogueDispatch.Kind
    let instruction: String
    var value: JSONValue { .object(["parent_id": .string(parentID), "turn_id": .string(turnID), "request_id": .string(requestID), "run_id": .string(runID), "action": .string(action.rawValue), "instruction": .string(instruction)]) }
}

enum HostCommand: Sendable {
    case focus(Bool)
    case configure(canStart: Bool, otherBusy: Bool, peers: [JSONValue])
    case bootstrap(parent: String, turn: String)
    case control(DialogueControl)
    case result(turn: String, child: String, error: String, cancelledPlan: Bool, revisingPlan: Bool)
    case refreshPreferences
    var fields: [String: JSONValue] {
        switch self {
        case .focus(let foreground): return ["type": .string("focus_session"), "foreground": .bool(foreground)]
        case let .configure(canStart, otherBusy, peers): return ["type": .string("dialogue_host"), "dialogue_host": .object(["can_start": .bool(canStart), "other_busy": .bool(otherBusy), "peers": .array(peers)])]
        case let .bootstrap(parent, turn): return ["type": .string("dialogue_bootstrap"), "dialogue_parent": .string(parent), "dialogue_turn": .string(turn)]
        case .control(let control): return ["type": .string("dialogue_control"), "dialogue_control": control.value]
        case let .result(turn, child, error, cancelled, revising): return ["type": .string("dialogue_result"), "dialogue_result": .object(["turn_id": .string(turn), "child_id": .string(child), "error": .string(error), "cancelled_plan": .bool(cancelled), "revising_plan": .bool(revising)])]
        case .refreshPreferences: return ["type": .string("refresh_preferences")]
        }
    }
}
