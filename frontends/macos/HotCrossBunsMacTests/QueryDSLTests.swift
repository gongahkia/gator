import XCTest
@testable import HotCrossBunsMac

final class QueryDSLTests: XCTestCase {
    // MARK: - fixtures

    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }()

    private var now: Date {
        calendar.date(from: DateComponents(year: 2026, month: 4, day: 18, hour: 10))!
    }

    private func day(_ offset: Int) -> Date {
        calendar.date(byAdding: .day, value: offset, to: calendar.startOfDay(for: now))!
    }

    private func ctx(taskLists: [TaskListMirror] = []) -> QueryContext {
        QueryContext(now: now, calendar: calendar, taskLists: taskLists)
    }

    private func task(
        id: String,
        title: String = "task",
        notes: String = "",
        list: String = "L1",
        due: Date? = nil,
        completed: Bool = false,
        deleted: Bool = false
    ) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: list,
            parentID: nil,
            title: title,
            notes: notes,
            status: completed ? .completed : .needsAction,
            dueDate: due,
            completedAt: nil,
            isDeleted: deleted,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: nil
        )
    }

    private func compile(_ s: String) -> CompiledQuery {
        switch QueryCompiler.compile(s) {
        case .success(let q): return q
        case .failure(let e): XCTFail("expected success, got error: \(e.message)"); return CompiledQuery(ast: .predicate(.titleContains("")))
        }
    }

    private func compileError(_ s: String) -> QueryCompileError? {
        if case .failure(let e) = QueryCompiler.compile(s) { return e }
        return nil
    }

    // MARK: - Lexer

    func testLexSingleField() throws {
        let toks = try QueryLexer.lex("tag:work")
        XCTAssertEqual(toks.count, 4) // identifier, colon, identifier, eof
        XCTAssertEqual(toks[0].kind, .identifier("tag"))
        XCTAssertEqual(toks[1].kind, .colon)
        XCTAssertEqual(toks[2].kind, .identifier("work"))
        XCTAssertEqual(toks[3].kind, .eof)
    }

    func testLexQuotedStringWithEscape() throws {
        let toks = try QueryLexer.lex("notes:\"hello \\\"world\\\"\"")
        XCTAssertEqual(toks.count, 4)
        if case .quotedString(let v) = toks[2].kind {
            XCTAssertEqual(v, "hello \"world\"")
        } else { XCTFail("expected quoted string") }
    }

    func testLexRelativeDatePositiveAndNegative() throws {
        let toks = try QueryLexer.lex("due<+7d due>-3w")
        // identifier, comparator, relativeDate, identifier, comparator, relativeDate, eof
        XCTAssertEqual(toks.count, 7)
        if case .relativeDate(let s, let a, let u) = toks[2].kind {
            XCTAssertEqual(s, 1); XCTAssertEqual(a, 7); XCTAssertEqual(u, "d")
        } else { XCTFail("expected relativeDate") }
        if case .relativeDate(let s, let a, let u) = toks[5].kind {
            XCTAssertEqual(s, -1); XCTAssertEqual(a, 3); XCTAssertEqual(u, "w")
        } else { XCTFail("expected relativeDate") }
    }

    func testLexAbsoluteDate() throws {
        let toks = try QueryLexer.lex("due:2026-01-15")
        XCTAssertEqual(toks.count, 4)
        if case .dateLiteral(let y, let m, let d) = toks[2].kind {
            XCTAssertEqual(y, 2026); XCTAssertEqual(m, 1); XCTAssertEqual(d, 15)
        } else { XCTFail("expected dateLiteral") }
    }

    func testLexUnterminatedStringThrows() {
        XCTAssertThrowsError(try QueryLexer.lex("notes:\"never closes")) { err in
            guard let e = err as? QueryCompileError else { return XCTFail("wrong error type") }
            XCTAssertTrue(e.message.lowercased().contains("unterminated"))
        }
    }

    func testLexStrayPlusThrows() {
        XCTAssertThrowsError(try QueryLexer.lex("+foo")) { err in
            guard let e = err as? QueryCompileError else { return XCTFail("wrong error type") }
            XCTAssertTrue(e.message.contains("+"))
        }
    }

    func testLexHashTagShorthand() throws {
        let toks = try QueryLexer.lex("#work")
        // identifier("tag"), colon, identifier("work"), eof
        XCTAssertEqual(toks.count, 4)
        XCTAssertEqual(toks[0].kind, .identifier("tag"))
        XCTAssertEqual(toks[1].kind, .colon)
        XCTAssertEqual(toks[2].kind, .identifier("work"))
    }

    func testLexEmptyHashTagThrows() {
        XCTAssertThrowsError(try QueryLexer.lex("#"))
    }

    func testLexSymbolicOperators() throws {
        let toks = try QueryLexer.lex("a && b || !c")
        XCTAssertEqual(toks.map(\.kind), [
            .identifier("a"), .and,
            .identifier("b"), .or,
            .not, .identifier("c"),
            .eof
        ])
    }

    func testLexKeywordsCaseInsensitive() throws {
        let toks = try QueryLexer.lex("a AND b Or c nOt d")
        XCTAssertEqual(toks.map(\.kind), [
            .identifier("a"), .and,
            .identifier("b"), .or,
            .identifier("c"), .not,
            .identifier("d"),
            .eof
        ])
    }

    // MARK: - Parser / AST

    func testParseSinglePredicate() {
        let q = compile("tag:work")
        XCTAssertEqual(q.ast, .predicate(.tag("work")))
    }

    func testParseImplicitAnd() {
        let q = compile("tag:work completed")
        XCTAssertEqual(q.ast, .and([.predicate(.tag("work")), .predicate(.completed)]))
    }

    func testParseExplicitAnd() {
        let q = compile("tag:work AND completed")
        XCTAssertEqual(q.ast, .and([.predicate(.tag("work")), .predicate(.completed)]))
    }

    func testParseOr() {
        let q = compile("tag:a OR tag:b")
        XCTAssertEqual(q.ast, .or([.predicate(.tag("a")), .predicate(.tag("b"))]))
    }

    func testAndBindsTighterThanOr() {
        let q = compile("tag:a AND tag:b OR tag:c")
        XCTAssertEqual(q.ast, .or([
            .and([.predicate(.tag("a")), .predicate(.tag("b"))]),
            .predicate(.tag("c"))
        ]))
    }

    func testParens() {
        let q = compile("(tag:a OR tag:b) AND tag:c")
        XCTAssertEqual(q.ast, .and([
            .or([.predicate(.tag("a")), .predicate(.tag("b"))]),
            .predicate(.tag("c"))
        ]))
    }

    func testUnaryMinusNegates() {
        let q = compile("-completed")
        XCTAssertEqual(q.ast, .not(.predicate(.completed)))
    }

    func testNotKeyword() {
        let q = compile("NOT completed")
        XCTAssertEqual(q.ast, .not(.predicate(.completed)))
    }

    func testBooleanValueFalseEquivalentToNegation() {
        let q = compile("completed:false")
        XCTAssertEqual(q.ast, .not(.predicate(.completed)))
    }

    func testBooleanValueTrueEquivalentToBare() {
        XCTAssertEqual(compile("completed:true").ast, compile("completed").ast)
    }

    func testHasNotesDueTag() {
        XCTAssertEqual(compile("has:notes").ast, .predicate(.hasNotes))
        XCTAssertEqual(compile("has:due").ast, .predicate(.hasDue))
        XCTAssertEqual(compile("has:tag").ast, .predicate(.hasTag))
        XCTAssertEqual(compile("has:tags").ast, .predicate(.hasTag))
    }

    func testDueComparators() {
        XCTAssertEqual(compile("due<+7d").ast, .predicate(.due(.lt, .relative(sign: 1, amount: 7, unit: .day))))
        XCTAssertEqual(compile("due<=+7d").ast, .predicate(.due(.le, .relative(sign: 1, amount: 7, unit: .day))))
        XCTAssertEqual(compile("due>today").ast, .predicate(.due(.gt, .today)))
        XCTAssertEqual(compile("due>=2026-01-15").ast, .predicate(.due(.ge, .absolute(year: 2026, month: 1, day: 15))))
        XCTAssertEqual(compile("due:today").ast, .predicate(.due(.eq, .today)))
    }

    func testBareStringIsTitleSubstring() {
        XCTAssertEqual(compile("hello").ast, .predicate(.titleContains("hello")))
        XCTAssertEqual(compile("\"some phrase\"").ast, .predicate(.titleContains("some phrase")))
    }

    func testHashTagShorthandInQuery() {
        XCTAssertEqual(compile("#work").ast, .predicate(.tag("work")))
        XCTAssertEqual(compile("#work AND -completed").ast, .and([
            .predicate(.tag("work")),
            .not(.predicate(.completed))
        ]))
    }

    // MARK: - Error messages

    func testEmptyQueryIsError() {
        XCTAssertNotNil(compileError(""))
        XCTAssertNotNil(compileError("    "))
    }

    func testUnknownFieldError() {
        let err = compileError("weight:5")
        XCTAssertNotNil(err)
        XCTAssertTrue(err!.message.lowercased().contains("unknown field"))
    }

    func testInequalityOnStringFieldError() {
        let err = compileError("title>foo")
        XCTAssertNotNil(err)
        XCTAssertTrue(err!.message.contains("'title'"))
    }

    func testMissingValueError() {
        let err = compileError("tag:")
        XCTAssertNotNil(err)
    }

    func testUnterminatedParenError() {
        XCTAssertNotNil(compileError("(tag:a"))
    }

    func testTrailingGarbageError() {
        XCTAssertNotNil(compileError("tag:a )"))
    }

    func testBadDateError() {
        let err = compileError("due:nope")
        XCTAssertNotNil(err)
    }

    // MARK: - Evaluator: individual predicates

    func testTitleSubstringCaseInsensitive() {
        let q = compile("HELLO")
        XCTAssertTrue(q.matches(task(id: "a", title: "hello world"), context: ctx()))
        XCTAssertTrue(q.matches(task(id: "b", title: "Say Hello"), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "c", title: "goodbye"), context: ctx()))
    }

    func testNotesSubstring() {
        let q = compile("notes:bank")
        XCTAssertTrue(q.matches(task(id: "a", notes: "call the bank"), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "b", notes: "call the store"), context: ctx()))
    }

    func testListMatchByIdExact() {
        let q = compile("list:L1")
        XCTAssertTrue(q.matches(task(id: "a", list: "L1"), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "b", list: "L2"), context: ctx()))
    }

    func testListMatchByTitleCaseInsensitive() {
        let lists = [TaskListMirror(id: "L1", title: "Work"), TaskListMirror(id: "L2", title: "Home")]
        let q = compile("list:work")
        XCTAssertTrue(q.matches(task(id: "a", list: "L1"), context: ctx(taskLists: lists)))
        XCTAssertFalse(q.matches(task(id: "b", list: "L2"), context: ctx(taskLists: lists)))
    }

    func testListQuotedWithSpaces() {
        let lists = [TaskListMirror(id: "L1", title: "Work Email")]
        let q = compile("list:\"Work Email\"")
        XCTAssertTrue(q.matches(task(id: "a", list: "L1"), context: ctx(taskLists: lists)))
    }

    func testTagExtraction() {
        let q = compile("tag:deep")
        XCTAssertTrue(q.matches(task(id: "a", title: "focus #deep"), context: ctx()))
        XCTAssertTrue(q.matches(task(id: "b", title: "#Deep focus"), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "c", title: "focus"), context: ctx()))
    }

    func testCompletedPredicate() {
        let q = compile("completed")
        XCTAssertTrue(q.matches(task(id: "a", completed: true), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "b", completed: false), context: ctx()))
    }

    func testOverduePredicate() {
        let q = compile("overdue")
        XCTAssertTrue(q.matches(task(id: "a", due: day(-1)), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "b", due: day(0)), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "c", due: day(1)), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "d", due: nil), context: ctx()))
    }

    func testHasNotesHasDueHasTag() {
        XCTAssertTrue(compile("has:notes").matches(task(id: "a", notes: "x"), context: ctx()))
        XCTAssertFalse(compile("has:notes").matches(task(id: "a", notes: "   "), context: ctx()))
        XCTAssertTrue(compile("has:due").matches(task(id: "a", due: day(0)), context: ctx()))
        XCTAssertFalse(compile("has:due").matches(task(id: "a", due: nil), context: ctx()))
        XCTAssertTrue(compile("has:tag").matches(task(id: "a", title: "x #foo"), context: ctx()))
        XCTAssertFalse(compile("has:tag").matches(task(id: "a", title: "x"), context: ctx()))
    }

    // MARK: - Evaluator: dates

    func testDueRelativeLt() {
        let q = compile("due<+7d")
        XCTAssertTrue(q.matches(task(id: "a", due: day(3)), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "b", due: day(7)), context: ctx())) // strictly less → not equal
        XCTAssertFalse(q.matches(task(id: "c", due: day(10)), context: ctx()))
    }

    func testDueRelativeLe() {
        let q = compile("due<=+7d")
        XCTAssertTrue(q.matches(task(id: "a", due: day(7)), context: ctx()))
    }

    func testDueAbsolute() {
        let q = compile("due:2026-04-20")
        XCTAssertTrue(q.matches(task(id: "a", due: calendar.date(from: DateComponents(year: 2026, month: 4, day: 20, hour: 15))!), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "b", due: calendar.date(from: DateComponents(year: 2026, month: 4, day: 21))!), context: ctx()))
    }

    func testDueToday() {
        let q = compile("due:today")
        XCTAssertTrue(q.matches(task(id: "a", due: day(0)), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "b", due: day(1)), context: ctx()))
    }

    func testDueYesterdayAndTomorrow() {
        XCTAssertTrue(compile("due:yesterday").matches(task(id: "a", due: day(-1)), context: ctx()))
        XCTAssertTrue(compile("due:tomorrow").matches(task(id: "a", due: day(1)), context: ctx()))
    }

    func testDueNilNeverMatchesComparisons() {
        XCTAssertFalse(compile("due<+7d").matches(task(id: "a", due: nil), context: ctx()))
        XCTAssertFalse(compile("due>=today").matches(task(id: "a", due: nil), context: ctx()))
        XCTAssertFalse(compile("due:today").matches(task(id: "a", due: nil), context: ctx()))
    }

    func testDueWeeksAndMonths() {
        let q1 = compile("due<+1w")
        XCTAssertTrue(q1.matches(task(id: "a", due: day(5)), context: ctx()))
        XCTAssertFalse(q1.matches(task(id: "b", due: day(7)), context: ctx()))
        // +1m — depends on calendar; just verify it parses and some future date matches
        let q2 = compile("due<+1m")
        XCTAssertTrue(q2.matches(task(id: "a", due: day(10)), context: ctx()))
    }

    // MARK: - Compound expressions

    func testNotFlipsResult() {
        let q = compile("-completed")
        XCTAssertTrue(q.matches(task(id: "a", completed: false), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "b", completed: true), context: ctx()))
    }

    func testAndRequiresAll() {
        let q = compile("tag:work AND completed")
        XCTAssertTrue(q.matches(task(id: "a", title: "x #work", completed: true), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "b", title: "x #work"), context: ctx())) // not completed
        XCTAssertFalse(q.matches(task(id: "c", completed: true), context: ctx()))  // no tag
    }

    func testOrShortCircuit() {
        let q = compile("overdue OR completed")
        XCTAssertTrue(q.matches(task(id: "a", due: day(-1)), context: ctx()))
        XCTAssertTrue(q.matches(task(id: "b", completed: true), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "c"), context: ctx()))
    }

    func testParenGroupsAffectEvaluation() {
        let q = compile("(overdue OR completed) AND tag:work")
        XCTAssertTrue(q.matches(task(id: "a", title: "x #work", due: day(-1)), context: ctx()))
        XCTAssertTrue(q.matches(task(id: "b", title: "x #work", completed: true), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "c", title: "x #work"), context: ctx()))
        XCTAssertFalse(q.matches(task(id: "d", due: day(-1)), context: ctx())) // no tag
    }

    func testRealisticQueryFromTodoSpec() {
        let lists = [TaskListMirror(id: "L1", title: "Home")]
        let q = compile("list:\"Home\" AND (tag:deep OR tag:focus) AND due<+7d AND -completed")
        // Match: list Home, tagged deep, due in 5 days, not completed
        let m = task(id: "a", title: "x #deep", list: "L1", due: day(5))
        XCTAssertTrue(q.matches(m, context: ctx(taskLists: lists)))

        // Fail: completed
        let f1 = task(id: "b", title: "x #deep", list: "L1", due: day(5), completed: true)
        XCTAssertFalse(q.matches(f1, context: ctx(taskLists: lists)))

        // Fail: due too far
        let f2 = task(id: "c", title: "x #deep", list: "L1", due: day(10))
        XCTAssertFalse(q.matches(f2, context: ctx(taskLists: lists)))

        // Fail: missing tag
        let f3 = task(id: "d", title: "x", list: "L1", due: day(5))
        XCTAssertFalse(q.matches(f3, context: ctx(taskLists: lists)))

        // Fail: wrong list
        let f4 = task(id: "e", title: "x #deep", list: "L2", due: day(5))
        XCTAssertFalse(q.matches(f4, context: ctx(taskLists: lists)))
    }

    // MARK: - Integration via CustomFilterDefinition

    func testCustomFilterUsesDSLWhenSet() {
        let lists = [TaskListMirror(id: "L1", title: "Work")]
        let def = CustomFilterDefinition(name: "q", queryExpression: "list:Work AND -completed")
        let t1 = task(id: "a", list: "L1")                      // matches
        let t2 = task(id: "b", list: "L1", completed: true)     // excluded by -completed
        let t3 = task(id: "c", list: "L2")                       // wrong list
        let results = def.filter([t1, t2, t3], now: now, calendar: calendar, taskLists: lists)
        XCTAssertEqual(results.map(\.id), ["a"])
    }

    func testCustomFilterMalformedDSLYieldsEmpty() {
        let def = CustomFilterDefinition(name: "q", queryExpression: "weight:5")
        let t1 = task(id: "a")
        let t2 = task(id: "b", title: "x")
        // Safety invariant: malformed DSL matches NOTHING, never everything.
        let results = def.filter([t1, t2], now: now, calendar: calendar)
        XCTAssertEqual(results, [])
        XCTAssertFalse(def.matches(t1, now: now, calendar: calendar))
    }

    func testCustomFilterFallsBackToStructuredWhenNoDSL() {
        // dueWindow: today — only tasks due today should match.
        let def = CustomFilterDefinition(name: "t", dueWindow: .today)
        XCTAssertTrue(def.matches(task(id: "a", due: day(0)), now: now, calendar: calendar))
        XCTAssertFalse(def.matches(task(id: "b", due: day(1)), now: now, calendar: calendar))
    }

    func testDeletedAlwaysExcluded() {
        let q = compile("has:due")
        // Evaluator itself doesn't exclude deleted — that's the caller's job.
        // CustomFilterDefinition.matches / .filter both exclude isDeleted.
        let def = CustomFilterDefinition(name: "q", queryExpression: "has:due")
        let t = task(id: "a", due: day(0), deleted: true)
        XCTAssertFalse(def.matches(t, now: now, calendar: calendar))
        XCTAssertEqual(def.filter([t], now: now, calendar: calendar), [])
        // But a direct evaluator call does match — document the boundary:
        XCTAssertTrue(q.matches(t, context: ctx()))
    }

    // MARK: - Codable backcompat

    func testLegacyCustomFilterJSONDecodesWithoutQueryField() throws {
        let json = """
        {
          "id": "\(UUID().uuidString)",
          "name": "Legacy",
          "systemImage": "star",
          "dueWindow": "today",
          "starredOnly": true,
          "includeCompleted": false,
          "taskListIDs": [],
          "tagsAny": []
        }
        """
        let data = Data(json.utf8)
        let decoded = try JSONDecoder().decode(CustomFilterDefinition.self, from: data)
        XCTAssertEqual(decoded.name, "Legacy")
        XCTAssertNil(decoded.queryExpression)
        XCTAssertFalse(decoded.isUsingQueryDSL)
    }

    func testRoundTripPreservesQueryExpression() throws {
        let original = CustomFilterDefinition(name: "q", queryExpression: "tag:deep AND -completed")
        let data = try JSONEncoder().encode(original)
        let decoded = try JSONDecoder().decode(CustomFilterDefinition.self, from: data)
        XCTAssertEqual(decoded.queryExpression, "tag:deep AND -completed")
        XCTAssertTrue(decoded.isUsingQueryDSL)
    }

    func testEmptyOrWhitespaceQueryTreatedAsNotUsingDSL() {
        let f1 = CustomFilterDefinition(name: "q", queryExpression: "")
        let f2 = CustomFilterDefinition(name: "q", queryExpression: "   ")
        XCTAssertFalse(f1.isUsingQueryDSL)
        XCTAssertFalse(f2.isUsingQueryDSL)
        // Falls back to structured (which defaults to match-everything-non-completed-non-deleted)
        XCTAssertTrue(f1.matches(task(id: "a"), now: now, calendar: calendar))
    }
}
