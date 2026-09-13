import Foundation

struct BackoffPolicy: Sendable {
    var baseDelay: Duration
    var maxDelay: Duration
    var jitter: Duration
    var maxAttempts: Int

    static let nearRealtime = BackoffPolicy(
        baseDelay: .seconds(90),
        maxDelay: .seconds(600),
        jitter: .seconds(15),
        maxAttempts: 6
    )

    func delay(forAttempt attempt: Int, randomSource: @Sendable () -> Double = { Double.random(in: 0...1) }) -> Duration {
        let clampedAttempt = max(0, min(attempt, maxAttempts))
        let base = baseDelay * Int(pow(2.0, Double(clampedAttempt)).rounded(.down))
        let capped = base > maxDelay ? maxDelay : base
        let jitterFraction = randomSource()
        let jitterComponent = jitter * jitterFraction
        return capped + jitterComponent
    }

    func shouldBackoff(from error: Error) -> Bool {
        guard let apiError = error as? GoogleAPIError else {
            return false
        }

        if case .httpStatus(let status, let body) = apiError {
            if GoogleAPIError.isQuotaBody(body) {
                return false
            }
            return status == 429 || (500...599).contains(status)
        }

        return false
    }
}

enum NearRealtimePollingCadence {
    private static let backgroundedWindowMultiplier = 2
    private static let energySavingMultiplier = 4

    static func delay(
        policy: BackoffPolicy = .nearRealtime,
        attempt: Int,
        isNetworkConstrained: Bool,
        isLowPowerModeEnabled: Bool,
        isMainWindowFocused: Bool,
        randomSource: @Sendable () -> Double = { Double.random(in: 0...1) }
    ) -> Duration {
        let baseDelay = attempt == 0
            ? policy.baseDelay
            : policy.delay(forAttempt: attempt, randomSource: randomSource)

        var multiplier = 1
        if isMainWindowFocused == false {
            multiplier = max(multiplier, backgroundedWindowMultiplier)
        }
        if isNetworkConstrained || isLowPowerModeEnabled {
            multiplier = max(multiplier, energySavingMultiplier)
        }

        let adjusted = baseDelay * multiplier
        return adjusted > policy.maxDelay ? policy.maxDelay : adjusted
    }
}

private extension Duration {
    static func * (lhs: Duration, rhs: Int) -> Duration {
        guard rhs > 0 else { return .zero }
        var result = lhs
        for _ in 1..<rhs {
            result += lhs
        }
        return result
    }

    static func * (lhs: Duration, rhs: Double) -> Duration {
        let seconds = Double(lhs.components.seconds) + Double(lhs.components.attoseconds) / 1e18
        let scaled = seconds * rhs
        let scaledSeconds = Int64(scaled.rounded(.down))
        let scaledAttoseconds = Int64((scaled - Double(scaledSeconds)) * 1e18)
        return Duration(secondsComponent: scaledSeconds, attosecondsComponent: scaledAttoseconds)
    }
}
