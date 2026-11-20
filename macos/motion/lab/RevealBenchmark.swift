import Foundation

@main struct RevealBenchmark {
    static func main() throws {
        var results: [[String: Any]] = []
        for strategy in NativeStreamingReveal.Strategy.allCases {
            for (name, chunks) in [
                ("sparse", Array(repeating: "a ", count: 40)),
                ("normal", Array(repeating: "small words ", count: 120)),
                ("fast", Array(repeating: "quick public output ", count: 100)),
                ("burst", [String(repeating: "burst words ", count: 140)]),
                ("CJK", Array(repeating: "你好世界。日本語の文章。", count: 90)),
                ("long-bypass", [String(repeating: "long ", count: 4000)])
            ] {
                let start = ContinuousClock.now
                var ticks = 0, largest = 0, pendingAtCompletion = 0
                var completionMicroseconds = 0.0
                for _ in 0..<10 {
                    var cursor = NativeStreamingReveal(), text = ""
                    cursor.receive("", identity: "owner:generation:run:entry", animate: true, now: 1)
                    for (index, chunk) in chunks.enumerated() {
                        text += chunk
                        cursor.receive(text, identity: "owner:generation:run:entry", animate: true, now: 1 + Double(index + 1) * 0.084)
                        largest = max(largest, cursor.pending)
                        if name == "sparse" || name == "normal" || name == "CJK" { cursor.tick(strategy: strategy); ticks += 1 }
                    }
                    while cursor.needsTick { cursor.tick(strategy: strategy); ticks += 1 }
                    let finishStart = ContinuousClock.now
                    cursor.receive(text, identity: "owner:generation:run:entry", animate: false, now: 100)
                    let finish = ContinuousClock.now - finishStart
                    completionMicroseconds = max(completionMicroseconds, Double(finish.components.seconds) * 1e6 + Double(finish.components.attoseconds) / 1e12)
                    pendingAtCompletion += cursor.pending
                    precondition(cursor.source.utf8.elementsEqual(text.utf8) && !cursor.needsTick)
                }
                let duration = ContinuousClock.now - start
                results.append(["strategy": strategy.rawValue, "fixture": name, "iterations": 10,
                    "cadenceMilliseconds": NativeStreamingReveal.cadenceMilliseconds, "ticks": ticks / 10, "largestBacklogGraphemes": largest,
                    "pendingAfterCompletion": pendingAtCompletion, "maximumCompletionMicroseconds": completionMicroseconds, "wallMilliseconds": Double(duration.components.seconds) * 1000 + Double(duration.components.attoseconds) / 1e15])
            }
        }
        print(String(decoding: try JSONSerialization.data(withJSONObject: results, options: [.prettyPrinted, .sortedKeys]), as: UTF8.self))
    }
}
