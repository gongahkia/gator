import Foundation
import Network

// Wraps NWPathMonitor into an @Observable so SwiftUI views can react
// to online / constrained / offline transitions. Used by the status
// header to render the network glyph and by the sync loop to suppress
// near-realtime polling when offline.
enum NetworkReachability: String, Sendable {
    case online
    case constrained // expensive / cellular / limited
    case offline
    case unknown

    var systemSymbol: String {
        switch self {
        case .online: "wifi"
        case .constrained: "wifi.exclamationmark"
        case .offline: "wifi.slash"
        case .unknown: "wifi.router"
        }
    }

    var displayTitle: String {
        switch self {
        case .online: "Online"
        case .constrained: "Constrained"
        case .offline: "Offline"
        case .unknown: "Checking"
        }
    }

    var isReachable: Bool { self == .online || self == .constrained }
}

@MainActor
@Observable
final class NetworkMonitor {
    private(set) var reachability: NetworkReachability = .unknown
    private let monitor = NWPathMonitor()
    private let queue = DispatchQueue(label: "com.gongahkia.hotcrossbuns.network")

    init() {
        monitor.pathUpdateHandler = { [weak self] path in
            let state = Self.classify(path)
            Task { @MainActor [weak self] in
                guard let self else { return }
                let prior = self.reachability
                self.reachability = state
                if prior != state {
                    AppLogger.info("network state changed", category: .sync, metadata: [
                        "from": prior.rawValue,
                        "to": state.rawValue
                    ])
                }
            }
        }
        monitor.start(queue: queue)
    }

    deinit {
        monitor.cancel()
    }

    nonisolated private static func classify(_ path: NWPath) -> NetworkReachability {
        switch path.status {
        case .unsatisfied: return .offline
        case .requiresConnection: return .constrained
        case .satisfied:
            if path.isConstrained || path.isExpensive { return .constrained }
            return .online
        @unknown default: return .unknown
        }
    }
}
