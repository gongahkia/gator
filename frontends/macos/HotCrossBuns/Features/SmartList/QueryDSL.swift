import Foundation

// Query DSL for custom sidebar filters.
//
// Grammar (EBNF-ish):
//
//   Query      := OrExpr EOF
//   OrExpr     := AndExpr ( (OR) AndExpr )*
//   AndExpr    := NotExpr ( (AND | implicit) NotExpr )*
//   NotExpr    := (NOT | '-' | '!') NotExpr | Primary
//   Primary    := '(' OrExpr ')' | Predicate
//   Predicate  := Field Comparator Value
//               | Field ':' Value
//               | BooleanShorthand                // completed / done / overdue
//               | Value                           // bare string → title substring
//   Field      := title | notes | list | tag | completed | done | due | has | overdue
//   Comparator := '<' | '<=' | '>' | '>=' | '=' | ':'
//   Value      := QuotedString | Identifier | Number | DateLiteral | RelativeDate
//   DateLiteral:= YYYY-MM-DD
//   RelativeDate := today | tomorrow | yesterday | ('+'|'-') Number ('d'|'w'|'m'|'y')
//
// Keywords AND / OR / NOT are case-insensitive. Symbolic equivalents: &&, ||, !, - (leading).
//
// Safety invariants:
//  - Compiler is pure, never throws an uncaught error: the Result return value always
//    carries either a success or a QueryCompileError.
//  - Evaluator is pure: no mutation of TaskMirror, no Google I/O, no disk I/O.
//  - On compile failure, callers must treat the filter as matching nothing — never
//    as matching everything — so users see an empty list + error message, not
//    an accidentally-unbounded list.

// MARK: - Public API

struct QueryContext: Sendable {
    let now: Date
    let calendar: Calendar
    let taskLists: [TaskListMirror]

    init(now: Date = Date(), calendar: Calendar = .current, taskLists: [TaskListMirror] = []) {
        self.now = now
        self.calendar = calendar
        self.taskLists = taskLists
    }
}

struct QueryCompileError: Error, Equatable, Sendable {
    let message: String
    let position: Int // char offset into the source, or -1 when unknown
    let length: Int   // span length at `position`, or 0 when unknown
}

struct CompiledQuery: Equatable, Sendable {
    let ast: QueryNode // internal for test inspection

    func matches(_ task: TaskMirror, context: QueryContext) -> Bool {
        QueryEvaluator.evaluate(ast, on: task, context: context)
    }
}

enum QueryCompiler {
    static func compile(_ source: String) -> Result<CompiledQuery, QueryCompileError> {
        let trimmed = source.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else {
            return .failure(QueryCompileError(message: "Query is empty.", position: 0, length: 0))
        }
        do {
            let tokens = try QueryLexer.lex(source)
            let ast = try QueryParser.parse(tokens: tokens)
            return .success(CompiledQuery(ast: ast))
        } catch let err as QueryCompileError {
            return .failure(err)
        } catch {
            // Defensive — lex/parse throw only QueryCompileError, but we never want
            // to crash the sidebar if a future change regresses the invariant.
            return .failure(QueryCompileError(message: "Unexpected error: \(error)", position: -1, length: 0))
        }
    }
}

// MARK: - AST

enum Comparator: Equatable, Sendable {
    case lt, gt, le, ge, eq
}

indirect enum QueryNode: Equatable, Sendable {
    case and([QueryNode])
    case or([QueryNode])
    case not(QueryNode)
    case predicate(Predicate)
}

enum Predicate: Equatable, Sendable {
    case titleContains(String)
    case notesContains(String)
    case listMatches(String)
    case tag(String)
    case completed
    case overdue
    case hasNotes
    case hasDue
    case hasTag
    case due(Comparator, DateExpr)
}

enum DateExpr: Equatable, Sendable {
    case today
    case tomorrow
    case yesterday
    case relative(sign: Int, amount: Int, unit: DateUnit)
    case absolute(year: Int, month: Int, day: Int)
}

enum DateUnit: Equatable, Sendable {
    case day, week, month, year
}

// MARK: - Tokens

enum QueryTokenKind: Equatable {
    case identifier(String)
    case quotedString(String)
    case number(String)
    case dateLiteral(year: Int, month: Int, day: Int)
    case relativeDate(sign: Int, amount: Int, unit: Character) // unit ∈ {d,w,m,y}
    case comparator(Comparator)
    case colon
    case and
    case or
    case not
    case minus
    case leftParen
    case rightParen
    case eof
}

struct QueryToken: Equatable {
    let kind: QueryTokenKind
    let position: Int
    let length: Int
}

// MARK: - Lexer

enum QueryLexer {
    static func lex(_ source: String) throws -> [QueryToken] {
        var tokens: [QueryToken] = []
        let chars = Array(source)
        var i = 0
        while i < chars.count {
            let start = i
            let c = chars[i]
            if c.isWhitespace { i += 1; continue }

            // Parens
            if c == "(" { tokens.append(QueryToken(kind: .leftParen, position: start, length: 1)); i += 1; continue }
            if c == ")" { tokens.append(QueryToken(kind: .rightParen, position: start, length: 1)); i += 1; continue }

            // Comparators (multi-char first)
            if c == "<" {
                if i + 1 < chars.count, chars[i + 1] == "=" {
                    tokens.append(QueryToken(kind: .comparator(.le), position: start, length: 2)); i += 2; continue
                }
                tokens.append(QueryToken(kind: .comparator(.lt), position: start, length: 1)); i += 1; continue
            }
            if c == ">" {
                if i + 1 < chars.count, chars[i + 1] == "=" {
                    tokens.append(QueryToken(kind: .comparator(.ge), position: start, length: 2)); i += 2; continue
                }
                tokens.append(QueryToken(kind: .comparator(.gt), position: start, length: 1)); i += 1; continue
            }
            if c == "=" { tokens.append(QueryToken(kind: .comparator(.eq), position: start, length: 1)); i += 1; continue }
            if c == ":" { tokens.append(QueryToken(kind: .colon, position: start, length: 1)); i += 1; continue }

            // Symbolic boolean ops
            if c == "&", i + 1 < chars.count, chars[i + 1] == "&" {
                tokens.append(QueryToken(kind: .and, position: start, length: 2)); i += 2; continue
            }
            if c == "|", i + 1 < chars.count, chars[i + 1] == "|" {
                tokens.append(QueryToken(kind: .or, position: start, length: 2)); i += 2; continue
            }
            if c == "!" { tokens.append(QueryToken(kind: .not, position: start, length: 1)); i += 1; continue }

            // Quoted string — supports \", \\, \n, \t escapes
            if c == "\"" {
                var j = i + 1
                var buf = ""
                var closed = false
                while j < chars.count {
                    if chars[j] == "\\", j + 1 < chars.count {
                        let esc = chars[j + 1]
                        switch esc {
                        case "\"": buf.append("\"")
                        case "\\": buf.append("\\")
                        case "n": buf.append("\n")
                        case "t": buf.append("\t")
                        default: buf.append(esc)
                        }
                        j += 2
                    } else if chars[j] == "\"" {
                        closed = true; j += 1; break
                    } else {
                        buf.append(chars[j]); j += 1
                    }
                }
                guard closed else {
                    throw QueryCompileError(message: "Unterminated string literal.", position: start, length: j - start)
                }
                tokens.append(QueryToken(kind: .quotedString(buf), position: start, length: j - start))
                i = j
                continue
            }

            // '+' and '-' — check for relative date first, else unary NOT (only '-').
            if c == "+" || c == "-" {
                if i + 1 < chars.count, chars[i + 1].isNumber {
                    var j = i + 1
                    while j < chars.count, chars[j].isNumber { j += 1 }
                    if j < chars.count, "dwmy".contains(chars[j]) {
                        guard let amount = Int(String(chars[i + 1 ..< j])) else {
                            throw QueryCompileError(message: "Invalid number in relative date.", position: start, length: j - start + 1)
                        }
                        let sign = c == "+" ? 1 : -1
                        tokens.append(QueryToken(kind: .relativeDate(sign: sign, amount: amount, unit: chars[j]),
                                                 position: start, length: j - start + 1))
                        i = j + 1
                        continue
                    }
                    // digit follows but not a unit letter — fall through so number is emitted separately
                }
                if c == "+" {
                    throw QueryCompileError(message: "Stray '+' — did you mean a relative date like '+7d'?",
                                            position: start, length: 1)
                }
                // c == "-"
                tokens.append(QueryToken(kind: .minus, position: start, length: 1))
                i += 1
                continue
            }

            // Number or date literal
            if c.isNumber {
                var j = i
                while j < chars.count, chars[j].isNumber { j += 1 }
                // YYYY-MM-DD date literal
                if j - i == 4, j + 5 < chars.count + 1 {
                    if j + 5 <= chars.count,
                       j < chars.count, chars[j] == "-",
                       chars[j + 1].isNumber, chars[j + 2].isNumber,
                       chars[j + 3] == "-",
                       chars[j + 4].isNumber, chars[j + 5].isNumber {
                        let year = Int(String(chars[i ..< j]))!
                        let month = Int(String(chars[j + 1 ... j + 2]))!
                        let day = Int(String(chars[j + 4 ... j + 5]))!
                        tokens.append(QueryToken(kind: .dateLiteral(year: year, month: month, day: day),
                                                 position: start, length: 10))
                        i = j + 6
                        continue
                    }
                }
                tokens.append(QueryToken(kind: .number(String(chars[i ..< j])), position: start, length: j - i))
                i = j
                continue
            }

            // '#' tag shorthand: #work → identifier("tag"), colon, identifier("work")
            if c == "#" {
                let tagStart = i + 1
                var j = tagStart
                while j < chars.count, chars[j].isLetter || chars[j].isNumber || chars[j] == "_" || chars[j] == "-" {
                    j += 1
                }
                guard j > tagStart else {
                    throw QueryCompileError(message: "Empty tag after '#'.", position: start, length: 1)
                }
                let name = String(chars[tagStart ..< j])
                tokens.append(QueryToken(kind: .identifier("tag"), position: start, length: j - start))
                tokens.append(QueryToken(kind: .colon, position: start, length: 0))
                tokens.append(QueryToken(kind: .identifier(name), position: start, length: 0))
                i = j
                continue
            }

            // Identifier — letters, digits (not leading), underscore, dot
            if c.isLetter || c == "_" {
                var j = i
                while j < chars.count, (chars[j].isLetter || chars[j].isNumber || chars[j] == "_" || chars[j] == ".") {
                    j += 1
                }
                let ident = String(chars[i ..< j])
                switch ident.lowercased() {
                case "and": tokens.append(QueryToken(kind: .and, position: start, length: j - i))
                case "or": tokens.append(QueryToken(kind: .or, position: start, length: j - i))
                case "not": tokens.append(QueryToken(kind: .not, position: start, length: j - i))
                default: tokens.append(QueryToken(kind: .identifier(ident), position: start, length: j - i))
                }
                i = j
                continue
            }

            throw QueryCompileError(message: "Unexpected character '\(c)'.", position: start, length: 1)
        }
        tokens.append(QueryToken(kind: .eof, position: chars.count, length: 0))
        return tokens
    }
}

// MARK: - Parser

struct QueryParser {
    private let tokens: [QueryToken]
    private var index: Int = 0

    private init(tokens: [QueryToken]) {
        self.tokens = tokens
    }

    static func parse(tokens: [QueryToken]) throws -> QueryNode {
        var p = QueryParser(tokens: tokens)
        let node = try p.parseOr()
        let last = p.peek()
        if case .eof = last.kind {} else {
            throw QueryCompileError(message: "Unexpected token after query.", position: last.position, length: last.length)
        }
        return node
    }

    private func peek() -> QueryToken { tokens[index] }

    private mutating func advance() -> QueryToken {
        let t = tokens[index]
        if index + 1 < tokens.count { index += 1 }
        return t
    }

    private mutating func parseOr() throws -> QueryNode {
        var children: [QueryNode] = [try parseAnd()]
        while case .or = peek().kind {
            _ = advance()
            children.append(try parseAnd())
        }
        return children.count == 1 ? children[0] : .or(children)
    }

    private mutating func parseAnd() throws -> QueryNode {
        var children: [QueryNode] = [try parseNot()]
        while canStartImplicitAndRhs() {
            if case .and = peek().kind { _ = advance() }
            children.append(try parseNot())
        }
        return children.count == 1 ? children[0] : .and(children)
    }

    // true iff the next token can begin another AND operand; false for EOF, ')', OR.
    // Explicit AND is also consumed by this path (callee checks and advances).
    private func canStartImplicitAndRhs() -> Bool {
        switch peek().kind {
        case .eof, .rightParen, .or: return false
        case .and: return true
        case .identifier, .quotedString, .number, .leftParen, .not, .minus, .dateLiteral, .relativeDate: return true
        default: return false
        }
    }

    private mutating func parseNot() throws -> QueryNode {
        let tok = peek()
        if case .not = tok.kind { _ = advance(); return .not(try parseNot()) }
        if case .minus = tok.kind { _ = advance(); return .not(try parseNot()) }
        return try parsePrimary()
    }

    private mutating func parsePrimary() throws -> QueryNode {
        if case .leftParen = peek().kind {
            _ = advance()
            let inner = try parseOr()
            let closing = peek()
            guard case .rightParen = closing.kind else {
                throw QueryCompileError(message: "Expected ')' to close parenthesis.", position: closing.position, length: closing.length)
            }
            _ = advance()
            return inner
        }
        return try parsePredicate()
    }

    private mutating func parsePredicate() throws -> QueryNode {
        let tok = peek()
        switch tok.kind {
        case .identifier(let name):
            _ = advance()
            let next = peek()
            if case .colon = next.kind {
                _ = advance()
                return try parseFieldValue(field: name, fieldToken: tok, cmp: .eq)
            }
            if case .comparator(let cmp) = next.kind {
                _ = advance()
                return try parseFieldValue(field: name, fieldToken: tok, cmp: cmp)
            }
            if let node = booleanShorthand(name: name) { return node }
            // Bare identifier → title substring
            return .predicate(.titleContains(name))
        case .quotedString(let s):
            _ = advance()
            return .predicate(.titleContains(s))
        case .number(let s):
            _ = advance()
            return .predicate(.titleContains(s))
        default:
            throw QueryCompileError(message: "Expected field, keyword, or search term.", position: tok.position, length: tok.length)
        }
    }

    private func booleanShorthand(name: String) -> QueryNode? {
        switch name.lowercased() {
        case "completed", "done": return .predicate(.completed)
        case "overdue": return .predicate(.overdue)
        default: return nil
        }
    }

    private mutating func parseFieldValue(field: String, fieldToken: QueryToken, cmp: Comparator) throws -> QueryNode {
        let lower = field.lowercased()
        switch lower {
        case "title":
            try requireEq(cmp: cmp, field: lower, tok: fieldToken)
            return .predicate(.titleContains(try readStringValue(field: lower)))
        case "notes":
            try requireEq(cmp: cmp, field: lower, tok: fieldToken)
            return .predicate(.notesContains(try readStringValue(field: lower)))
        case "list":
            try requireEq(cmp: cmp, field: lower, tok: fieldToken)
            return .predicate(.listMatches(try readStringValue(field: lower)))
        case "tag":
            try requireEq(cmp: cmp, field: lower, tok: fieldToken)
            return .predicate(.tag(try readStringValue(field: lower)))
        case "completed", "done":
            try requireEq(cmp: cmp, field: lower, tok: fieldToken)
            let b = try readBoolValue(field: lower)
            return b ? .predicate(.completed) : .not(.predicate(.completed))
        case "overdue":
            try requireEq(cmp: cmp, field: lower, tok: fieldToken)
            let b = try readBoolValue(field: lower)
            return b ? .predicate(.overdue) : .not(.predicate(.overdue))
        case "has":
            try requireEq(cmp: cmp, field: lower, tok: fieldToken)
            let targetTok = peek()
            guard case .identifier(let t) = targetTok.kind else {
                throw QueryCompileError(message: "Expected 'has:notes', 'has:due', or 'has:tag'.",
                                        position: targetTok.position, length: targetTok.length)
            }
            _ = advance()
            switch t.lowercased() {
            case "notes": return .predicate(.hasNotes)
            case "due": return .predicate(.hasDue)
            case "tag", "tags": return .predicate(.hasTag)
            default:
                throw QueryCompileError(message: "Unknown 'has' target '\(t)'. Expected notes, due, or tag.",
                                        position: targetTok.position, length: targetTok.length)
            }
        case "due":
            let expr = try readDateValue(field: lower)
            return .predicate(.due(cmp, expr))
        default:
            throw QueryCompileError(message: "Unknown field '\(field)'.", position: fieldToken.position, length: fieldToken.length)
        }
    }

    private func requireEq(cmp: Comparator, field: String, tok: QueryToken) throws {
        guard cmp == .eq else {
            throw QueryCompileError(message: "Field '\(field)' only supports ':' or '='.",
                                    position: tok.position, length: tok.length)
        }
    }

    private mutating func readStringValue(field: String) throws -> String {
        let tok = peek()
        switch tok.kind {
        case .quotedString(let s): _ = advance(); return s
        case .identifier(let s): _ = advance(); return s
        case .number(let s): _ = advance(); return s
        default:
            throw QueryCompileError(message: "Expected a value after '\(field)'.", position: tok.position, length: tok.length)
        }
    }

    private mutating func readBoolValue(field: String) throws -> Bool {
        let tok = peek()
        switch tok.kind {
        case .identifier(let s):
            switch s.lowercased() {
            case "true", "yes", "on": _ = advance(); return true
            case "false", "no", "off": _ = advance(); return false
            default:
                throw QueryCompileError(message: "Expected true/false after '\(field)'.", position: tok.position, length: tok.length)
            }
        case .number(let s):
            if s == "1" { _ = advance(); return true }
            if s == "0" { _ = advance(); return false }
            throw QueryCompileError(message: "Expected true/false after '\(field)'.", position: tok.position, length: tok.length)
        default:
            throw QueryCompileError(message: "Expected true/false after '\(field)'.", position: tok.position, length: tok.length)
        }
    }

    private mutating func readDateValue(field: String) throws -> DateExpr {
        let tok = peek()
        switch tok.kind {
        case .identifier(let s):
            switch s.lowercased() {
            case "today": _ = advance(); return .today
            case "tomorrow": _ = advance(); return .tomorrow
            case "yesterday": _ = advance(); return .yesterday
            default:
                throw QueryCompileError(message: "Expected date value after '\(field)'. Use YYYY-MM-DD, +Nd/-Nd, or today/tomorrow/yesterday.",
                                        position: tok.position, length: tok.length)
            }
        case .relativeDate(let sign, let amount, let unit):
            _ = advance()
            let du: DateUnit
            switch unit {
            case "d": du = .day
            case "w": du = .week
            case "m": du = .month
            case "y": du = .year
            default:
                throw QueryCompileError(message: "Unknown relative-date unit '\(unit)'.", position: tok.position, length: tok.length)
            }
            return .relative(sign: sign, amount: amount, unit: du)
        case .dateLiteral(let y, let m, let d):
            _ = advance()
            return .absolute(year: y, month: m, day: d)
        default:
            throw QueryCompileError(message: "Expected date value after '\(field)'. Use YYYY-MM-DD, +Nd/-Nd, or today/tomorrow/yesterday.",
                                    position: tok.position, length: tok.length)
        }
    }
}

// MARK: - Evaluator

enum QueryEvaluator {
    static func evaluate(_ node: QueryNode, on task: TaskMirror, context: QueryContext) -> Bool {
        switch node {
        case .and(let children): return children.allSatisfy { evaluate($0, on: task, context: context) }
        case .or(let children): return children.contains { evaluate($0, on: task, context: context) }
        case .not(let child): return evaluate(child, on: task, context: context) == false
        case .predicate(let p): return match(p, on: task, context: context)
        }
    }

    static func match(_ p: Predicate, on task: TaskMirror, context: QueryContext) -> Bool {
        switch p {
        case .titleContains(let s):
            return task.title.localizedCaseInsensitiveContains(s)
        case .notesContains(let s):
            return task.notes.localizedCaseInsensitiveContains(s)
        case .listMatches(let s):
            if task.taskListID == s { return true }
            if let list = context.taskLists.first(where: { $0.id == task.taskListID }),
               list.title.localizedCaseInsensitiveCompare(s) == .orderedSame {
                return true
            }
            return false
        case .tag(let s):
            let tags = TagExtractor.tags(in: task.title).map { $0.lowercased() }
            return tags.contains(s.lowercased())
        case .completed:
            return task.isCompleted
        case .overdue:
            guard let due = task.dueDate else { return false }
            let startOfToday = context.calendar.startOfDay(for: context.now)
            return context.calendar.startOfDay(for: due) < startOfToday
        case .hasNotes:
            return task.notes.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
        case .hasDue:
            return task.dueDate != nil
        case .hasTag:
            return TagExtractor.tags(in: task.title).isEmpty == false
        case .due(let cmp, let expr):
            return compareDue(task: task, cmp: cmp, expr: expr, context: context)
        }
    }

    static func resolveDate(_ expr: DateExpr, context: QueryContext) -> Date? {
        let cal = context.calendar
        let startOfToday = cal.startOfDay(for: context.now)
        switch expr {
        case .today: return startOfToday
        case .tomorrow: return cal.date(byAdding: .day, value: 1, to: startOfToday)
        case .yesterday: return cal.date(byAdding: .day, value: -1, to: startOfToday)
        case .relative(let sign, let amount, let unit):
            let component: Calendar.Component
            switch unit {
            case .day: component = .day
            case .week: component = .weekOfYear
            case .month: component = .month
            case .year: component = .year
            }
            return cal.date(byAdding: component, value: sign * amount, to: startOfToday)
        case .absolute(let y, let m, let d):
            var comps = DateComponents()
            comps.year = y
            comps.month = m
            comps.day = d
            return cal.date(from: comps).map { cal.startOfDay(for: $0) }
        }
    }

    static func compareDue(task: TaskMirror, cmp: Comparator, expr: DateExpr, context: QueryContext) -> Bool {
        guard let due = task.dueDate, let target = resolveDate(expr, context: context) else { return false }
        let cal = context.calendar
        let lhs = cal.startOfDay(for: due)
        let rhs = cal.startOfDay(for: target)
        switch cmp {
        case .lt: return lhs < rhs
        case .le: return lhs <= rhs
        case .gt: return lhs > rhs
        case .ge: return lhs >= rhs
        case .eq: return lhs == rhs
        }
    }
}
