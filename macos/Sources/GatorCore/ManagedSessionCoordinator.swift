import Foundation
import Observation
import CryptoKit

public struct NativeChatActivity: Identifiable, Equatable {
    public let id: String
    public let title: String
    public let status: String
    public let summary: String
    public let needsAttention: Bool
}

/// One application-wide coordinator, shared by both presentations. An owner
/// never changes conversation when selection changes; its stream and tokens stay put.
@MainActor @Observable
public final class ManagedSessionCoordinator {
    public private(set) var current: GatorApplicationState
    public private(set) var switching = false
    public private(set) var issue = ""
    public private(set) var started = false
    public private(set) var stopping = false
    private var inspections: [String: NativeTaskInspection] = [:]
    private var records: [UUID: GatorApplicationState] = [:]
    private var order: [UUID] = []
    private var attention: [String: NativeChatActivity] = [:]
    private var uncertain: Set<UUID> = []
    @ObservationIgnored private let configuration: BridgeLaunchConfiguration
    @ObservationIgnored private let store: NativeDraftStore
    @ObservationIgnored private let factory: @Sendable () -> any BridgeTransport
    @ObservationIgnored private let operationTimeout: Duration
    @ObservationIgnored private let admissionTimeout: Duration
    @ObservationIgnored private var transition: Task<Void, Never>?
    @ObservationIgnored private var seenSessions: [UUID: Int] = [:]
    @ObservationIgnored private var seenDispatches: [UUID: Int] = [:]
    @ObservationIgnored private var hostSignatures: [UUID: JSONValue] = [:]
    @ObservationIgnored private var hostTask: Task<Void, Never>?
    @ObservationIgnored private var hostDirty = false
    @ObservationIgnored private var expected: [UUID: String] = [:]

    public convenience init(configuration: BridgeLaunchConfiguration, nativeDraftDirectory: URL, profile: String) {
        self.init(configuration: configuration, store: NativeDraftStore(directory: nativeDraftDirectory, profile: profile), factory: { GoBridgeConnection() })
    }
    init(configuration: BridgeLaunchConfiguration, store: NativeDraftStore, operationTimeout: Duration = .seconds(5), admissionTimeout: Duration = .seconds(20), factory: @escaping @Sendable () -> any BridgeTransport) {
        var config = configuration; config.managed = true
        self.configuration = config; self.store = store; self.factory = factory; self.operationTimeout = operationTimeout; self.admissionTimeout = admissionTimeout
        let owner = GatorApplicationState(configuration: config, store: store, responseTimeout: admissionTimeout, factory: factory)
        current = owner
        let key = UUID(); records[key] = owner; order = [key]; expected[key] = config.conversationID
        observe(owner)
    }

    public var owners: [String: GatorApplicationState] {
        var result: [String: GatorApplicationState] = [:]
        for owner in records.values where !owner.conversationID.isEmpty { result[owner.conversationID] = owner }
        return result
    }
    public var ownerCount: Int { records.count }
    public var workingCount: Int { records.values.filter { $0.connectionStatus != .closed && ($0.snapshot?.running == true || $0.reservesPlanCapacity || $0.reservesRecoveryCapacity) }.count }
    public var canNavigate: Bool { !switching && !stopping && started }
    public var persistenceIssue: String { records.values.map(\.persistenceIssue).first(where: { !$0.isEmpty }) ?? "" }
    public var quitGuard: String? {
        if switching || !uncertain.isEmpty { return "A chat transition or task-control outcome is unconfirmed. Quit stops all Go owners and preserves retained work; nothing is replayed on restart." }
        let held = records.values.filter { $0.quitGuard != nil }
        return held.isEmpty ? nil : "\(held.count) chat(s) have work, review, queued input or an unconfirmed submission. Quit stops all Go owners. History and drafts are kept; work and approvals will not be replayed."
    }
    public var activity: [NativeChatActivity] {
        var rows = attention
        for update in current.snapshot?.value["update_conversations"].array ?? [] where validConversationID(update["id"].string) {
            rows[update["id"].string] = NativeChatActivity(id: update["id"].string, title: update["title"].string, status: "New update", summary: "", needsAttention: true)
        }
        for key in order {
            guard let owner = records[key], validConversationID(owner.conversationID) else { continue }
            let state = owner.snapshot
            let taskOutcome = state?.value["dialogue_task"]["state"].string ?? ""
            let outcome = taskOutcome.isEmpty ? state?.messages.last(where: { $0.value["activity"] != .null })?.value["activity"]["state"].string ?? "" : taskOutcome
            let status: String
            if owner.connectionStatus == .disconnected { status = "Disconnected" }
            else if state?.hasApproval == true { status = "Needs review" }
            else if uncertain.contains(key) { status = "Control outcome unconfirmed · inspect" }
            else if state?.running == true { status = state?.cancelling == true ? "Stopping" : "Working" }
            else if state?.value["dialogue_busy"].bool == true { status = "Considering your message" }
            else if state?.value["queued"].int ?? 0 > 0 { status = "Paused / queued" }
            else if let unread = attention[owner.conversationID] { status = unread.status }
            else if ["completed", "failed", "stopped", "interrupted", "review", "unconfirmed"].contains(outcome) { status = outcome.capitalized }
            else { status = "Idle" }
            rows[owner.conversationID] = NativeChatActivity(id: owner.conversationID, title: owner.title, status: status,
                summary: String((state?.value["dialogue_task"]["summary"].string ?? "").prefix(300)),
                needsAttention: attention[owner.conversationID] != nil || state?.hasApproval == true || owner.connectionStatus == .disconnected)
        }
        return rows.values.sorted { $0.id < $1.id }
    }
    public var canSubmit: Bool { !switching && !stopping && correctionUnavailableReason == nil && current.canSend && (current.snapshot?.value["dialogue_active"].bool == true || current.snapshot?.running == true || workingCount < 3) }

    private func key(for owner: GatorApplicationState) -> UUID? { records.first(where: { $0.value === owner })?.key }
    private func observe(_ owner: GatorApplicationState) {
        owner.changed = { [weak self] owner, before in self?.changed(owner, before: before) }
    }
    @discardableResult private func enqueue(_ action: @escaping @MainActor () async throws -> Void) -> Task<Void, Never> {
        let previous = transition
        let next = Task { [weak self] in
            await previous?.value
            guard let self, !self.stopping else { return }
            do { try await action() } catch { self.issue = error.localizedDescription }
        }
        transition = next; return next
    }
    func settled() async { await transition?.value }

    public func start() async {
        guard !started else { return }; started = true
        await enqueue { [self] in
            await current.start(); try await current.awaitReady()
            try validateIdentity(current)
            try await focus(current, true)
            try await store.selectConversation(current.conversationID)
            scheduleHostProjection()
        }.value
    }
    private func validateIdentity(_ owner: GatorApplicationState) throws {
        guard let key = key(for: owner), validConversationID(owner.conversationID) else { throw BridgeFailure("Go did not project a valid conversation identity.") }
        if let id = expected[key], !id.isEmpty, id != owner.conversationID { throw BridgeFailure("The requested conversation identity was not confirmed. Original selection retained.") }
        guard !records.contains(where: { $0.key != key && $0.value.conversationID == owner.conversationID && $0.value.connectionStatus != .closed }) else { throw BridgeFailure("This conversation already has a native owner. No competing owner will be selected.") }
        expected[key] = owner.conversationID
    }
    private func focus(_ owner: GatorApplicationState, _ foreground: Bool) async throws {
        try await owner.host(.focus(foreground), timeout: operationTimeout) { $0.value["foreground"].bool == foreground && $0.value["session_request"] == .null }
    }
    public func retainedOwnerExplanation(_ owner: GatorApplicationState) -> String? {
        if let key = key(for: owner), uncertain.contains(key) { return "This original owner retains an unconfirmed host bootstrap or task control. Its slot is kept because timeout does not prove non-execution. Open its chat to inspect; no control will be retried automatically." }
        if owner.receiptOperation?.unresolved == true { return "An unconfirmed read-only receipt check is retained with this owner. Its result has not been established; no check or external send will be replayed." }
        if owner.recovery?.unresolved == true { return "Unresolved retained work or action evidence keeps this cached owner available for inspection. It will not be evicted to make room for another chat." }
        return nil
    }
    private func disposable(_ owner: GatorApplicationState) -> Bool {
        guard let key = key(for: owner), !uncertain.contains(key), owner !== current, owner.pendingSubmission == nil, owner.operation.isEmpty, !owner.approvalSending, owner.planOperation?.blocksAuthorization != true, owner.receiptOperation?.unresolved != true, owner.recoveryPreparation?.blocks != true, owner.recovery?.unresolved != true else { return false }
        // Even a disconnected owner can retain uncertain work or an exact review.
        return owner.snapshot?.unsafeToNavigate == false && owner.snapshot?.unsupportedNotice == nil
    }
    private func create(id: String = "", new: Bool = false, source: String = "") async throws -> GatorApplicationState {
        if !id.isEmpty, let existing = owners[id] {
            guard existing.isConnected else { throw BridgeFailure("The original task owner is disconnected. Open it and reconnect explicitly; no second owner was started.") }
            return existing
        }
        if records.count >= 4 {
            guard let retiringKey = order.first(where: { records[$0].map(disposable) == true }), let retiring = records[retiringKey] else { throw BridgeFailure("Four chats are retained. Finish work, resolve reviews or inspect uncertain work before opening another chat.") }
            guard await retiring.shutdown() else { throw BridgeFailure("The cached chat changed while retiring. Its work was kept.") }
            records.removeValue(forKey: retiringKey); order.removeAll { $0 == retiringKey }; hostSignatures.removeValue(forKey: retiringKey); seenSessions.removeValue(forKey: retiringKey); seenDispatches.removeValue(forKey: retiringKey); expected.removeValue(forKey: retiringKey)
        }
        guard !stopping else { throw BridgeFailure("Gator is closing.") }
        var config = configuration; config.conversationID = id; config.bootMode = new ? "new" : ""; config.startSource = source
        let owner = GatorApplicationState(configuration: config, store: store, responseTimeout: admissionTimeout, factory: factory), key = UUID()
        records[key] = owner; order.append(key); expected[key] = id; observe(owner)
        await owner.start()
        do { try await owner.awaitReady(); try validateIdentity(owner) }
        catch { _ = await owner.shutdown(confirmed: true); records.removeValue(forKey: key); order.removeAll { $0 == key }; expected.removeValue(forKey: key); seenSessions.removeValue(forKey: key); seenDispatches.removeValue(forKey: key); hostSignatures.removeValue(forKey: key); throw error }
        scheduleHostProjection(); return owner
    }
    private func select(id: String = "", new: Bool = false, source: String = "") async throws {
        switching = true; issue = ""; defer { switching = false }
        let previous = current
        var next: GatorApplicationState?
        do {
            if previous.isConnected { try await focus(previous, false) }
            if !id.isEmpty, let retained = owners[id], retained.connectionStatus == .disconnected {
                try await store.selectConversation(retained.conversationID)
                current = retained; attention.removeValue(forKey: retained.conversationID)
                issue = "This owner disconnected. Reconnect explicitly to inspect its durable state; nothing is replayed."
                return
            }
            let owner = try await create(id: id, new: new, source: source); next = owner
            try await focus(owner, true)
            guard !stopping else { throw BridgeFailure("Gator is closing.") }
            try await store.selectConversation(owner.conversationID)
            current = owner; attention.removeValue(forKey: owner.conversationID)
        } catch {
            if let next, next !== previous, next.isConnected { try? await focus(next, false) }
            if previous.isConnected { try? await focus(previous, true) }
            current = previous
            throw error
        }
    }
    public func newConversation() async {
        let origin = current
        await enqueue { [self] in
            guard origin === current, origin.isConnected else { throw BridgeFailure("The selected chat changed. Choose New Conversation again.") }
            try await origin.requestManagedNew()
        }.value
        await settled()
    }
    public func openConversation(_ id: String) async {
        guard validConversationID(id), started, !stopping else { return }
        let origin = current, generation = origin.generation
        if canOpenActivity(id, from: origin) {
            await enqueue { [self] in
                guard generation == origin.generation else { throw BridgeFailure("The requesting owner changed. Choose the chat again.") }
                try await select(id: id)
            }.value
        } else {
            await enqueue { [self] in
                guard origin === current, origin.generation == generation else { throw BridgeFailure("The selected conversation changed. Choose it again from Recents.") }
                try await origin.requestManagedConversation(id)
            }.value
            await settled()
        }
    }
    private func canOpenActivity(_ id: String, from owner: GatorApplicationState) -> Bool {
        activity.contains(where: { $0.id == id }) || owner.snapshot?.value["branch"]["conversation_id"].string == id || owner.snapshot?.value["dialogue_task"]["parent_id"].string == id || owner.snapshot?.value["dialogue"].array.contains(where: { $0["task_chat"].string == id }) == true
    }
    public func submit(from owner: GatorApplicationState) async {
        let generation = owner.generation, revision = owner.draftRevision
        await enqueue { [self] in
            guard owner.generation == generation, owner.draftRevision == revision else { throw BridgeFailure("The owner or draft changed before sending. Your current draft is kept; review it and send again.") }
            if let reason = correctionUnavailableReason { throw BridgeFailure(reason) }
            guard owner === current, canSubmit else { throw BridgeFailure(workingCount >= 3 ? "Three tasks are working. Conversation remains available when Go projects dialogue readiness; wait for that readiness or stop a task." : "Wait for the selected chat to be ready. Your draft is kept.") }
            await owner.submit()
            // Serialize capacity-changing admissions through their actual result,
            // not just the nonce. Navigation/dispatch cannot race a fourth task.
            await owner.awaitAdmissionResolution()
        }.value
    }
    public func retryAdmission(from owner: GatorApplicationState) async {
        let generation = owner.generation
        await enqueue { [self] in
            guard owner.generation == generation else { throw BridgeFailure("The original owner changed. Inspect the retained admission before retrying.") }
            guard owner === current, !switching, owner.snapshot?.value["dialogue_active"].bool == true || owner.snapshot?.running == true || workingCount < 3 else { throw BridgeFailure("Three tasks are working. Inspect the original admission before retrying.") }
            if let target = owner.pendingSubmission?.correctionTarget, !isCurrentCorrectionTarget(target) { throw BridgeFailure("The original correction target changed. Inspect its retained outcome; the uncertain request was not resent.") }
            await owner.retryOriginalAdmission(); await owner.awaitAdmissionResolution()
        }.value
    }
    public var inspection: NativeTaskInspection? { inspections[current.conversationID] }
    public var inspectedOwner: GatorApplicationState? { inspection.flatMap { owners[$0.ownerID] } }
    public var isInspectionCurrent: Bool { guard let inspection, let owner = inspectedOwner else { return false }; return owner.generation == inspection.generation && owner.conversationID == inspection.ownerID }
    public func inspectTask(_ id: String) {
        guard canNavigate, canOpenActivity(id, from: current), let owner = owners[id] else { issue = "Open the original chat to load its retained task state."; return }
        inspections[current.conversationID] = NativeTaskInspection(ownerID: id, generation: owner.generation)
    }
    public func closeTaskInspection() { inspections.removeValue(forKey: current.conversationID); current.requestComposerFocus() }
    public func focusComposer() { current.closeApprovalReview(); closeTaskInspection(); current.requestComposerFocus() }
    public func loadPlan(_ identity: NativePlanIdentity, owner: GatorApplicationState) async {
        await enqueue { [self] in
            guard owner === current, owners[identity.conversationID] === owner else { throw BridgeFailure("Open the original owner before loading its exact plan review.") }
            await owner.inspectPlan(identity)
        }.value
    }
    public func planAuthorizationUnavailable(owner: GatorApplicationState, review: NativePlanReview, action: NativePlanAction) -> String? {
        guard owner === current, owners[review.identity.conversationID] === owner, !switching, !stopping else { return "Select the original owner and inspect its current plan. Navigation never authorizes execution." }
        if let key = key(for: owner), uncertain.contains(key) { return "This owner has an unconfirmed task control. Inspect it before authorizing more work." }
        if workingCount >= 3 { return "Three tasks are working or have unconfirmed admissions. Finish or inspect one before starting reviewed work." }
        return owner.planAuthorizationUnavailable(review, action: action)
    }
    public func authorizePlan(owner: GatorApplicationState, review: NativePlanReview, action: NativePlanAction) async {
        let previousOperation = owner.planOperation?.id
        await enqueue { [self] in
            guard owner.planOperation?.id == previousOperation else { throw BridgeFailure("A plan decision changed while this input waited. Inspect its outcome before another explicit decision.") }
            if let reason = planAuthorizationUnavailable(owner: owner, review: review, action: action) { throw BridgeFailure(reason) }
            issue = ""
            await owner.authorizePlan(review, action: action) { [self] in
                owner === current && !switching && !stopping && workingCount <= 3 && owners[review.identity.conversationID] === owner
            }
            // Capacity and transitions are serialized through the operation-specific
            // result or truthful uncertainty, never through nonce acknowledgement.
            await owner.awaitPlanAuthorizationResult()
        }.value
    }
    public func recoveryUnavailable(owner: GatorApplicationState, recovery: NativeRequestRecovery, action: NativeRecoveryAction) -> String? {
        guard owner === current, owners[recovery.conversationID] === owner, !switching, !stopping else { return "Open and focus the original owner before reviewing recovery." }
        if let key = key(for: owner), uncertain.contains(key) { return "This owner has an unconfirmed host control. Inspect its retained state first." }
        if action != .reviewAccess, workingCount >= 3 { return "Three tasks are working or retain unconfirmed admissions. No work was stopped; wait for capacity." }
        return owner.recoveryUnavailable(recovery, action: action)
    }
    public func reviewRecoveryAccess(owner: GatorApplicationState, recovery: NativeRequestRecovery, proposed: Bool = false) async {
        let selections = proposed ? owner.recoveryAccess[recovery.id]?.selections : nil
        await enqueue { [self] in
            if let reason = recoveryUnavailable(owner: owner, recovery: recovery, action: .reviewAccess) { throw BridgeFailure(reason) }
            guard !proposed || selections == owner.recoveryAccess[recovery.id]?.selections else { throw BridgeFailure("Account selection changed while waiting. Review the current choices again.") }
            await owner.reviewRecoveryAccess(recovery, proposed: proposed)
        }.value
    }
    public func prepareRecovery(owner: GatorApplicationState, recovery: NativeRequestRecovery, action: NativeRecoveryAction) async {
        let previous = owner.recoveryPreparation?.id
        await enqueue { [self] in
            guard owner.recoveryPreparation?.id == previous else { throw BridgeFailure("A preparation decision changed while this input waited. Inspect the original task.") }
            if let reason = recoveryUnavailable(owner: owner, recovery: recovery, action: action) { throw BridgeFailure(reason) }
            await owner.prepareRecovery(recovery, action: action) { [self] in
                owner === current && !switching && !stopping && workingCount <= 3 && owners[recovery.conversationID] === owner
            }
            await owner.awaitRecoveryAdmission()
        }.value
    }
    public func checkReceipt(owner: GatorApplicationState, receipt: NativeRecoveryReceipt, generation: UUID, action: NativeReceiptAction) async {
        let previous = owner.receiptOperation?.id
        await enqueue { [self] in
            guard owner === current, owners[receipt.identity?.conversationID ?? ""] === owner, !switching, !stopping, owner.receiptOperation?.id == previous else { throw BridgeFailure("The selected owner or pending check changed. Inspect its retained receipt again.") }
            if let reason = owner.receiptUnavailable(receipt, generation: generation) { throw BridgeFailure(reason) }
            await owner.checkReceipt(receipt, generation: generation, action: action)
        }.value
    }
    public func respondToApproval(owner: GatorApplicationState, review: ApprovalReview, approved: Bool) async {
        guard !switching, !stopping, owner === current, owners[review.conversationID] === owner, owner.isCurrentReview(review) else { issue = "The selected owner or exact review changed. Inspect it again before deciding."; return }
        await owner.respondToApproval(review, approved: approved)
    }
    public func isCurrentCorrectionTarget(_ target: NativeCorrectionTarget) -> Bool {
        guard let owner = owners[target.ownerID], owner.isConnected, owner.generation == target.generation else { return false }
        if !target.taskTurnID.isEmpty, owner.snapshot?.value["dialogue_task"]["turn_id"].string != target.taskTurnID { return false }
        if target.kind == .action {
            guard let review = owner.currentApprovalReview, owner.isCurrentReview(review), review.token == target.reviewToken else { return false }
            return owner.snapshot?.value["dialogue_active"].bool == true
        }
        guard let plan = target.plan, let review = owner.planReviews[plan.taskID], review.identity == plan, owner.isCurrentPlanReview(review) else { return false }
        return owner.snapshot?.value["dialogue_active"].bool == true
    }
    public var correctionUnavailableReason: String? {
        guard let target = current.correctionTarget else { return nil }
        return isCurrentCorrectionTarget(target) ? nil : "This correction's owner or reviewed proposal changed. Your text is kept. Cancel this correction, inspect the current proposal and request a new correction."
    }
    public func requestActionCorrection(owner: GatorApplicationState, review: ApprovalReview) async {
        await enqueue { [self] in
            guard owner === current, owners[owner.conversationID] === owner, owner.isCurrentReview(review) else { throw BridgeFailure("The exact proposal changed or disconnected. Inspect it again; no correction was submitted.") }
            let target = NativeCorrectionTarget(kind: .action, ownerID: owner.conversationID, generation: owner.generation, reviewToken: review.token, plan: nil, taskTurnID: owner.snapshot?.value["dialogue_task"]["turn_id"].string ?? "")
            try await prepareCorrection(target, owner: owner)
        }.value
    }
    public func requestPlanRevision(owner: GatorApplicationState, review: NativePlanReview) async {
        await enqueue { [self] in
            guard owner === current, owners[owner.conversationID] === owner, owner.isCurrentPlanReview(review) else { throw BridgeFailure("This plan revision is stale. Inspect the current proposal before requesting a change.") }
            let target = NativeCorrectionTarget(kind: .plan, ownerID: owner.conversationID, generation: owner.generation, reviewToken: nil, plan: review.identity, taskTurnID: owner.snapshot?.value["dialogue_task"]["turn_id"].string ?? "")
            try await prepareCorrection(target, owner: owner)
        }.value
    }
    private func prepareCorrection(_ target: NativeCorrectionTarget, owner: GatorApplicationState) async throws {
        guard !switching, !stopping, isCurrentCorrectionTarget(target) else { throw BridgeFailure("Gator has not confirmed conversation readiness for this proposal. No correction was sent.") }
        let parentID = owner.snapshot?.value["dialogue_task"]["parent_id"].string ?? ""
        let requester: GatorApplicationState
        if parentID.isEmpty { requester = owner }
        else {
            guard let parent = owners[parentID], parent.isConnected else { throw BridgeFailure("The original requesting chat is unavailable. Open and reconnect it before requesting a correction.") }
            requester = parent
        }
        guard requester.correctionTarget == nil, requester.pendingSubmission == nil else { throw BridgeFailure("The requesting chat already has a correction or unconfirmed submission. Its draft was kept.") }
        if current !== requester { try await select(id: requester.conversationID) }
        guard isCurrentCorrectionTarget(target), requester.beginCorrection(target) else { throw BridgeFailure("The proposal changed while opening its requesting chat. Inspect it again.") }
        if requester === owner { requester.closeApprovalReview() }
        inspections.removeValue(forKey: requester.conversationID)
        // The composer itself explains this draft. Do not retain an informational
        // message as a warning after Go has admitted or completed the correction.
        issue = ""
    }
    public var correctionStatus: String? {
        guard let request = current.submission, let target = request.correctionTarget else { return nil }
        if request.phase == .uncertain { return "Correction admission uncertain · retained identity; no automatic retry" }
        let turnID = SHA256.hash(data: Data(request.id.utf8)).map { String(format: "%02x", $0) }.joined()
        guard let turn = current.snapshot?.value["dialogue"].array.first(where: { $0["id"].string == turnID }) else { return "Correction submitted · acceptance is not execution" }
        switch turn["state"].string {
        case "received", "routing", "ready": return "Correction admitted · Gator is considering it"
        case "dispatching": return "Correction routed · original owner's outcome pending"
        case "editing_plan": return "Revision requested · replacement is being prepared"
        case "failed", "interrupted": return "Correction not confirmed: " + turn["issue"].string
        case "guided":
            if target.kind == .action {
                guard let owner = owners[target.ownerID], owner.generation == target.generation else { return "Correction outcome retained · inspect the original task after reconnecting" }
                if !owner.isConnected { return "Original task disconnected · correction outcome requires inspection" }
                let token = owner.snapshot?.value["pending_approval"]["token"].string ?? ""
                let taskState = owner.snapshot?.value["dialogue_task"]["state"].string ?? ""
                let activityState = owner.messages.last(where: { $0.value["activity"] != .null })?.value["activity"]["state"].string ?? ""
                let terminal = taskState.isEmpty ? activityState : taskState
                if token.isEmpty, owner.snapshot?.running == false, ["completed", "failed", "stopped", "interrupted"].contains(terminal) {
                    return "Correction outcome retained · task " + terminal + "; inspect its result for execution or delivery evidence"
                }
                return !token.isEmpty && token != target.reviewToken ? "Replacement action needs a fresh exact review · nothing approved" : "Previous proposal withdrawn · correction accepted; replacement is not yet confirmed"
            }
            if let plan = target.plan, let summary = owners[target.ownerID]?.snapshot?.value["task_plans"].array.first(where: { $0["id"].string == plan.taskID }), summary["phase"].string == "review", summary["revision"].int > plan.revision { return "Replacement plan available · inspect the new revision before any execution" }
            return "Revision outcome retained · inspect the original task"
        default: return "Correction outcome: " + turn["state"].string + " · inspect the retained response"
        }
    }

    public func reconnect() async {
        let owner = current
        await enqueue { [self] in
            guard owner === current, owner.connectionStatus == .disconnected, let key = key(for: owner) else { return }
            switching = true; defer { switching = false }
            // The old transport is reaped by the same owner before replacement.
            seenSessions[key] = 0; seenDispatches[key] = 0; hostSignatures.removeValue(forKey: key)
            await owner.reconnect(); try await owner.awaitReady(); try validateIdentity(owner)
            try await focus(owner, true); hostSignatures.removeValue(forKey: key); scheduleHostProjection()
        }.value
    }

    private func changed(_ owner: GatorApplicationState, before: BridgeSnapshot?) {
        guard let key = key(for: owner), !stopping else { return }
        if before?.running == true, owner.snapshot?.running == false, owner !== current {
            let state = owner.snapshot?.messages.last(where: { $0.value["activity"] != .null })?.value["activity"]["state"].string ?? ""
            let status = state == "failed" ? "Needs attention" : state == "stopped" ? "Stopped" : state == "completed" ? "Completed" : "Work ended · inspect"
            attention[owner.conversationID] = NativeChatActivity(id: owner.conversationID, title: owner.title, status: status, summary: String((owner.snapshot?.value["dialogue_task"]["summary"].string ?? "").prefix(300)), needsAttention: true)
            if attention.count > 64, let oldest = attention.keys.sorted().first { attention.removeValue(forKey: oldest) }
        }
        if owner.isConnected, let raw = owner.snapshot?.value["dialogue_dispatch"], raw != .null, raw["sequence"].int > 0, raw["sequence"].int > seenDispatches[key, default: 0] {
            seenDispatches[key] = raw["sequence"].int
            let generation = owner.generation
            enqueue { [self] in try await dispatch(owner, generation: generation, raw: raw) }
        }
        if owner === current, owner.isConnected, let raw = owner.snapshot?.value["session_request"], raw != .null, raw["sequence"].int > 0, raw["sequence"].int > seenSessions[key, default: 0] {
            seenSessions[key] = raw["sequence"].int
            let generation = owner.generation
            enqueue { [self] in
                guard owner === current, owner.generation == generation, owner.snapshot?.value["session_request"] == raw else { return }
                let request = try SessionRequest(raw)
                guard request.view.isEmpty, !request.goal else {
                    try await focus(owner, true)
                    throw BridgeFailure("This native tranche supports chat navigation. This requested terminal page remains available in the terminal app.")
                }
                try await select(id: request.conversationID, new: request.new, source: request.source.isEmpty && request.new ? owner.snapshot?.value["source"].string ?? "" : request.source)
            }
        }
        if let revision = owner.snapshot?.value["preferences_revision"], revision != .null, before?.value["preferences_revision"] != nil, before?.value["preferences_revision"] != .null, before?.value["preferences_revision"] != revision {
            for other in records.values where other !== owner && other.isConnected {
                Task { [weak self] in do { try await other.host(.refreshPreferences) } catch { self?.issue = "Saved preferences changed, but a cached owner could not refresh: " + error.localizedDescription } }
            }
        }
        scheduleHostProjection()
    }
    private func scheduleHostProjection() {
        hostDirty = true
        guard hostTask == nil, !stopping else { return }
        hostTask = Task { [weak self] in
            guard let self else { return }
            while self.hostDirty && !self.stopping {
                self.hostDirty = false
                let live = self.records.filter { $0.value.isConnected }
                for (key, owner) in live {
                    let peers: [JSONValue] = live.values.compactMap { child in
                        guard child.snapshot?.value["dialogue_task"]["parent_id"].string == owner.conversationID, !owner.conversationID.isEmpty, case .object(var peer) = child.snapshot?.value["dialogue_task"] else { return nil }
                        peer.removeValue(forKey: "parent_id"); return .object(peer)
                    }.sorted { $0["conversation_id"].string < $1["conversation_id"].string }
                    let command = HostCommand.configure(canStart: self.workingCount < 3, otherBusy: live.values.contains { $0 !== owner && $0.snapshot?.running == true }, peers: peers)
                    let signature = JSONValue.object(command.fields)
                    guard self.hostSignatures[key] != signature, !self.stopping else { continue }
                    self.hostSignatures[key] = signature
                    do { try await owner.host(command) } catch { self.issue = "A chat's managed capabilities were not confirmed: " + error.localizedDescription }
                }
            }
            self.hostTask = nil
        }
    }
    private func dispatch(_ parent: GatorApplicationState, generation: UUID, raw: JSONValue) async throws {
        guard parent.isConnected, parent.generation == generation, parent.snapshot?.value["dialogue_dispatch"] == raw else { return }
        var errorText = "", cancelled = false, revising = false
        do {
            let request = try DialogueDispatch(raw)
            if request.kind == .task {
                guard workingCount < 3 else { throw BridgeFailure("Three tasks are working. Wait for one to finish or stop the selected task.") }
                let owner = try await create(id: request.childID)
                guard !stopping else { throw BridgeFailure("Gator is closing. The retained child was not bootstrapped.") }
                if let key = key(for: owner), uncertain.contains(key) { throw BridgeFailure("This task has an unconfirmed host action. Inspect it before another admission.") }
                var confirmed = false
                do {
                    try await owner.host(.bootstrap(parent: parent.conversationID, turn: request.turnID), timeout: operationTimeout) { $0.value["dialogue_bootstrap_result"]["turn_id"].string == request.turnID && $0.value["dialogue_bootstrap_result"]["child_id"].string == request.childID }
                    confirmed = true
                    let result = owner.snapshot?.value["dialogue_bootstrap_result"] ?? .null
                    if !result["error"].string.isEmpty { throw BridgeFailure(result["error"].string) }
                } catch { if !confirmed, let key = key(for: owner) { uncertain.insert(key) }; throw error }
            } else {
                guard let owner = owners[request.childID], owner.isConnected else { throw BridgeFailure("The original task owner is unavailable. Inspect it before another control.") }
                guard !stopping else { throw BridgeFailure("Gator is closing. No task control was sent.") }
                if request.kind == .revise_plan, owner.snapshot?.running != true, workingCount >= 3 { throw BridgeFailure("Three tasks are working. Wait before revising this plan.") }
                let local = (request.kind == .revise_plan || request.kind == .revise_action) && owner === parent && !request.ownerTask.isEmpty
                let origin = owner.snapshot?.value["dialogue_task"] ?? .null
                guard local || origin["parent_id"].string == parent.conversationID, !request.targetRun.isEmpty else { throw BridgeFailure("The original task provenance or control identity is unavailable.") }
                let control = DialogueControl(parentID: parent.conversationID, turnID: local ? request.ownerTask : origin["turn_id"].string, requestID: request.turnID, runID: request.targetRun, action: request.kind, instruction: request.instruction)
                var confirmed = false
                do {
                    try await owner.host(.control(control), timeout: operationTimeout) { $0.value["dialogue_control_result"]["request_id"].string == request.turnID }
                    confirmed = true
                    let result = owner.snapshot?.value["dialogue_control_result"] ?? .null
                    guard result["error"].string.isEmpty else { throw BridgeFailure(result["error"].string) }
                    cancelled = result["cancelled_plan"].bool && request.kind == .stop
                    revising = result["revising_plan"].bool && request.kind == .revise_plan
                } catch { if !confirmed, let key = key(for: owner) { uncertain.insert(key) }; throw error }
            }
        } catch { errorText = error.localizedDescription; issue = errorText }
        guard parent.isConnected, parent.generation == generation, parent.snapshot?.value["dialogue_dispatch"] == raw else { return }
        // Exactly one report, including truthful uncertainty. Never repeat a
        // bootstrap/control after timeout or interpret a nonce as success.
        try await parent.host(.result(turn: raw["turn_id"].string, child: raw["child_id"].string, error: errorText, cancelledPlan: cancelled, revisingPlan: revising), timeout: operationTimeout) { $0.value["dialogue_dispatch"] != raw }
    }
    public func shutdown(confirmed: Bool = false) async -> Bool {
        guard confirmed || quitGuard == nil else { return false }
        stopping = true
        await transition?.value
        for owner in records.values { if !(await owner.flushDrafts()) { stopping = false; return false } }
        var success = true
        for owner in records.values { if !(await owner.shutdown(confirmed: true)) { success = false } }
        return success
    }
}
