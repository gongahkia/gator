import Foundation

// Applies an AdvancedSearchQuery to a QuickSwitcherEntity. Returns `true` when
// every structured filter passes. Free-text and regex paths do not filter
// here — those are handed to FuzzySearcher and NSRegularExpression by the
// caller. Keeping structural filtering separate from ranking lets the switcher
// short-circuit: if the structural pass returns 0, we skip the ranking.
//
// Filters scoped to a single entity kind are skipped (not failed) on
// inapplicable entities. Example: `attendee:alice` is an event-only filter,
// so a task is skipped — it doesn't match, but the filter doesn't
// incorrectly fail against it. Inspect-ability: power users can still mix
// "list:home completed" and get tasks back, because event-only fields only
// prune events.
enum AdvancedSearchMatcher {
    static func matches(
        _ entity: QuickSwitcherEntity,
        query: AdvancedSearchQuery,
        calendars: [CalendarListMirror],
        taskLists: [TaskListMirror],
        now: Date = Date(),
        calendar: Calendar = .current
    ) -> Bool {
        // kind: prefilter — when set, any entity of a different kind is
        // rejected outright. Notes are a client-side lens over standalone
        // undated tasks and tasks with note/details text.
        if let k = query.kind {
            switch (k, entity) {
            case (.task, .task(let t)) where t.dueDate != nil: break
            case (.note, .task(let t)) where t.appearsInNotesSurface: break
            case (.event, .event): break
            case (.list, .taskList): break
            case (.calendar, .calendar): break
            case (.filter, .customFilter): break
            default: return false
            }
        }

        switch entity {
        case .task(let task): return matchesTask(task, query: query, taskLists: taskLists, now: now, calendar: calendar)
        case .event(let event): return matchesEvent(event, query: query, calendars: calendars)
        case .taskList(let list): return matchesTaskList(list, query: query)
        case .calendar(let cal): return matchesCalendar(cal, query: query)
        case .customFilter(let f): return matchesCustomFilter(f, query: query)
        }
    }

    // MARK: - task

    private static func matchesTask(
        _ task: TaskMirror,
        query: AdvancedSearchQuery,
        taskLists: [TaskListMirror],
        now: Date,
        calendar: Calendar
    ) -> Bool {
        // Event-only filters — if any of these is set, tasks don't qualify.
        if query.calendarMatch != nil { return false }
        if query.attendeeMatch != nil { return false }
        if query.requireLocation { return false }

        // title:X — substring over title.
        if query.titleContains.isEmpty == false {
            for needle in query.titleContains {
                if task.title.localizedCaseInsensitiveContains(needle) == false { return false }
            }
        }

        // tag:X — every tag must be present (matches QueryDSL's tag semantics).
        if query.tagsAll.isEmpty == false {
            let actual = Set(TagExtractor.tags(in: task.title).map { $0.lowercased() })
            for needle in query.tagsAll where actual.contains(needle.lowercased()) == false {
                return false
            }
        }

        // list:X — match by id or case-insensitive title.
        if let listRef = query.listMatch {
            if task.taskListID == listRef {
                // pass
            } else if let list = taskLists.first(where: { $0.id == task.taskListID }),
                      list.title.localizedCaseInsensitiveCompare(listRef) == .orderedSame {
                // pass
            } else {
                return false
            }
        }

        if query.requireNotes, task.notes.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { return false }
        if query.requireDue, task.dueDate == nil { return false }
        if query.requireCompleted, task.isCompleted == false { return false }
        if query.requireOverdue {
            guard let due = task.dueDate else { return false }
            let startOfToday = calendar.startOfDay(for: now)
            if calendar.startOfDay(for: due) >= startOfToday { return false }
        }

        return true
    }

    // MARK: - event

    private static func matchesEvent(
        _ event: CalendarEventMirror,
        query: AdvancedSearchQuery,
        calendars: [CalendarListMirror]
    ) -> Bool {
        // Task-only filters — if any is set, events don't qualify.
        if query.listMatch != nil { return false }
        if query.requireOverdue { return false }
        if query.requireCompleted { return false }
        if query.requireDue { return false }

        if query.titleContains.isEmpty == false {
            for needle in query.titleContains
            where event.summary.localizedCaseInsensitiveContains(needle) == false {
                return false
            }
        }

        if query.tagsAll.isEmpty == false {
            // Events don't have tags; if any tag is required, no event matches.
            return false
        }

        if let calRef = query.calendarMatch {
            if event.calendarID == calRef {
                // pass
            } else if let cal = calendars.first(where: { $0.id == event.calendarID }),
                      cal.summary.localizedCaseInsensitiveCompare(calRef) == .orderedSame {
                // pass
            } else {
                return false
            }
        }

        if let attendeeRef = query.attendeeMatch {
            let lower = attendeeRef.lowercased()
            let attendees = event.attendeeEmails.map { $0.lowercased() }
                + event.attendeeResponses.map { $0.email.lowercased() }
                + event.attendeeResponses.compactMap { $0.displayName?.lowercased() }
            if attendees.contains(where: { $0.contains(lower) }) == false { return false }
        }

        if query.requireNotes, event.details.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { return false }
        if query.requireLocation, event.location.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { return false }

        return true
    }

    // MARK: - task list / calendar / custom filter

    private static func matchesTaskList(_ list: TaskListMirror, query: AdvancedSearchQuery) -> Bool {
        // Any task/event-only filter disqualifies these "container" entities
        // since the filter is asking about something only a task/event has.
        if query.tagsAll.isEmpty == false { return false }
        if query.listMatch != nil { return false }
        if query.calendarMatch != nil { return false }
        if query.attendeeMatch != nil { return false }
        if query.requireNotes || query.requireLocation || query.requireDue
            || query.requireCompleted || query.requireOverdue { return false }

        if query.titleContains.isEmpty == false {
            for needle in query.titleContains
            where list.title.localizedCaseInsensitiveContains(needle) == false {
                return false
            }
        }
        return true
    }

    private static func matchesCalendar(_ cal: CalendarListMirror, query: AdvancedSearchQuery) -> Bool {
        if query.tagsAll.isEmpty == false { return false }
        if query.listMatch != nil { return false }
        if query.calendarMatch != nil { return false }
        if query.attendeeMatch != nil { return false }
        if query.requireNotes || query.requireLocation || query.requireDue
            || query.requireCompleted || query.requireOverdue { return false }

        if query.titleContains.isEmpty == false {
            for needle in query.titleContains
            where cal.summary.localizedCaseInsensitiveContains(needle) == false {
                return false
            }
        }
        return true
    }

    private static func matchesCustomFilter(_ f: CustomFilterDefinition, query: AdvancedSearchQuery) -> Bool {
        if query.tagsAll.isEmpty == false { return false }
        if query.listMatch != nil { return false }
        if query.calendarMatch != nil { return false }
        if query.attendeeMatch != nil { return false }
        if query.requireNotes || query.requireLocation || query.requireDue
            || query.requireCompleted || query.requireOverdue { return false }

        if query.titleContains.isEmpty == false {
            for needle in query.titleContains
            where f.name.localizedCaseInsensitiveContains(needle) == false {
                return false
            }
        }
        return true
    }

    // MARK: - regex

    // Precompiled version: callers filtering thousands of entities should
    // compile the regex ONCE per query (see regexMatches(_:compiled:))
    // rather than paying the NSRegularExpression(pattern:) cost per entity.
    static func regexMatches(_ entity: QuickSwitcherEntity, regexPattern: String) -> Bool {
        guard let re = try? NSRegularExpression(pattern: regexPattern, options: [.caseInsensitive]) else { return false }
        return regexMatches(entity, compiled: re)
    }

    // Fast path used by the command palette for regex queries over large
    // entity sets. The caller builds the NSRegularExpression once; we reuse
    // it across every candidate.
    static func regexMatches(_ entity: QuickSwitcherEntity, compiled regex: NSRegularExpression) -> Bool {
        let candidates = regexCandidates(for: entity)
        for c in candidates where c.isEmpty == false {
            let range = NSRange(c.startIndex..., in: c)
            if regex.firstMatch(in: c, range: range) != nil { return true }
        }
        return false
    }

    private static func regexCandidates(for entity: QuickSwitcherEntity) -> [String] {
        switch entity {
        case .task(let t): return [t.title, t.notes]
        case .event(let e): return [e.summary, e.details, e.location] + e.attendeeEmails
        case .taskList(let l): return [l.title]
        case .calendar(let c): return [c.summary]
        case .customFilter(let f): return [f.name, f.queryExpression ?? ""]
        }
    }
}
