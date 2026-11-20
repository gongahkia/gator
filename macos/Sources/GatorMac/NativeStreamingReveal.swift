import Foundation
import NaturalLanguage

/// Bounded presentation cursor. It never owns a transport, draft or execution.
struct NativeStreamingReveal {
    enum Strategy: String, CaseIterable { case character, word, adaptive }
    static let cadenceMilliseconds = 66
    static let cadence: Duration = .milliseconds(cadenceMilliseconds)
    static let maximumCharacters = 8_192
    static let maximumBacklog = 2_048
    private(set) var source = ""
    private(set) var identity = ""
    private(set) var characters: [Character] = []
    private(set) var boundaries: [Int] = [0]
    private(set) var visible = 0
    private(set) var entering = 0
    private(set) var baseline = false
    private(set) var arrivalRate = 0.0
    private var lastArrival = 0.0
    var pending: Int { characters.count - visible }
    var needsTick: Bool { pending > 0 || entering < visible }
    var visibleIndex: String.Index { String.Index(utf16Offset: boundaries[min(visible, boundaries.count - 1)], in: source) }
    var enteringIndex: String.Index { String.Index(utf16Offset: boundaries[min(entering, boundaries.count - 1)], in: source) }

    mutating func receive(_ text: String, identity: String, animate: Bool, now: Double) {
        let same = source.utf8.elementsEqual(text.utf8)
        if same, self.identity == identity, animate, baseline { return }
        let append = text.utf8.starts(with: source.utf8)
        let continuing = baseline && self.identity == identity && append
        let oldCount = characters.count
        let oldVisible = visible
        let oldSource = source
        self.identity = identity; source = text
        guard animate, text.utf8.count <= Self.maximumCharacters * 4 else { settle(release: true); return }
        // Rebuild only the last grapheme and appended bytes: a combining mark or
        // ZWJ can extend the previous last cluster across a provider delta.
        if continuing, let last = characters.last {
            let byteOffset = oldSource.utf8.count - String(last).utf8.count
            let suffix = String(decoding: text.utf8.dropFirst(byteOffset), as: UTF8.self)
            characters.removeLast(); boundaries.removeLast()
            for character in suffix { characters.append(character); boundaries.append(boundaries.last! + String(character).utf16.count) }
        } else {
            characters = Array(text); boundaries = [0]
            for character in characters { boundaries.append(boundaries.last! + String(character).utf16.count) }
        }
        guard characters.count <= Self.maximumCharacters else { settle(release: true); return }
        baseline = true
        if !continuing { visible = characters.count; entering = visible }
        else {
            visible = min(oldVisible, characters.count); entering = visible
            let delta = max(0, characters.count - oldCount)
            if lastArrival > 0, now > lastArrival {
                arrivalRate = arrivalRate * 0.6 + Double(delta) / max(0.016, now - lastArrival) * 0.4
            }
            if pending > Self.maximumBacklog { visible = characters.count; entering = visible }
        }
        lastArrival = now
    }
    mutating func tick(strategy: Strategy = .adaptive) {
        guard pending > 0 else { entering = visible; return }
        entering = visible
        var budget: Int
        switch strategy {
        case .character: budget = 1
        case .word: budget = 1
        case .adaptive: budget = max(2, (pending + 4) / 5, Int(min(512, arrivalRate * Double(Self.cadenceMilliseconds) / 1000 * 1.5)))
        }
        budget = min(pending, budget)
        if strategy != .character {
            let end = min(characters.count, visible + budget + 20)
            let window = String(characters[visible..<end])
            let tokenizer = NLTokenizer(unit: .word); tokenizer.string = window
            let ends = tokenizer.tokens(for: window.startIndex..<window.endIndex).map { window.distance(from: window.startIndex, to: $0.upperBound) }
            if let boundary = ends.first(where: { $0 >= budget && $0 <= budget + 16 }) { budget = boundary }
        }
        visible = min(characters.count, visible + budget)
    }
    mutating func settle(release: Bool = false) {
        if release {
            characters = []; boundaries = [source.utf16.count]; visible = 0; entering = 0; baseline = false; arrivalRate = 0; lastArrival = 0
        } else { visible = characters.count; entering = visible }
    }
}
