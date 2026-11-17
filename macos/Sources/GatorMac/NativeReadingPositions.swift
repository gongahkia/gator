import Observation

/// Ephemeral reading intent only. It neither retains an owner nor changes Go state.
@MainActor @Observable final class NativeReadingPositions {
    struct Position: Equatable {
        var anchor: String?
        var following = true
    }
    @ObservationIgnored private var positions: [String: Position] = [:]
    @ObservationIgnored private var order: [String] = []
    private let capacity: Int
    init(capacity: Int = 32) { self.capacity = max(1, capacity) }
    var count: Int { positions.count }
    func remember(_ context: String, anchor: String?, following: Bool) {
        guard !context.isEmpty else { return }
        positions[context] = Position(anchor: following ? nil : anchor, following: following)
        order.removeAll { $0 == context }; order.append(context)
        if order.count > capacity { positions.removeValue(forKey: order.removeFirst()) }
    }
    func restored(_ context: String, available: [String]) -> Position {
        guard var position = positions[context], !position.following else { return Position() }
        if let anchor = position.anchor, available.contains(anchor) { return position }
        position.anchor = available.first
        return position
    }
}
