import Foundation
import Observation
import CryptoKit

public enum NativeConnectionStatus: String, Sendable { case idle, starting, connected, disconnected, stopping, closed }

/// State for exactly one Go owner. The application-wide coordinator selects it for both scenes.
@MainActor @Observable
public final class GatorApplicationState {
    public private(set) var connectionStatus: NativeConnectionStatus = .idle
    public private(set) var snapshot: BridgeSnapshot?
    public private(set) var issue = ""
    public private(set) var persistenceIssue = ""
    public private(set) var submission: PendingSubmission?
    public private(set) var review: ApprovalReview?
    public private(set) var operation = ""
    public private(set) var approvalSending = false
    public private(set) var stopping = false
    public private(set) var generation = UUID()
    public private(set) var started = false
    public private(set) var draftText = ""
    public private(set) var messages: [BridgeMessage] = []
    public private(set) var transcript: [BridgeMessage] = []
    public private(set) var correctionTarget: NativeCorrectionTarget?
    public private(set) var planFirst = false
    public private(set) var focusRequest = UUID()
    public private(set) var planReviews: [String: NativePlanReview] = [:]
    public private(set) var planOperation: NativePlanOperation?
    public private(set) var receiptOperation: NativeReceiptOperation?
    @ObservationIgnored private var receiptDeadline: Task<Void, Never>?
    private var planReviewEpoch = UUID()
    @ObservationIgnored private var planControlDeadline: Task<Void, Never>?
    @ObservationIgnored private var retiredApprovalTokens: Set<String> = []
    @ObservationIgnored private var planReadRequest: (id: String, identity: NativePlanIdentity, generation: UUID)?

    @ObservationIgnored var changed: (@MainActor (GatorApplicationState, BridgeSnapshot?) -> Void)?
    var managed: Bool { configuration.managed }
    @ObservationIgnored private var configuration: BridgeLaunchConfiguration
    @ObservationIgnored private let store: NativeDraftStore
    @ObservationIgnored private let factory: @Sendable () -> any BridgeTransport
    @ObservationIgnored private var transport: (any BridgeTransport)?
    @ObservationIgnored private var eventTask: Task<Void, Never>?
    @ObservationIgnored private var archive = NativeArchive()
    @ObservationIgnored private var loaded = false
    @ObservationIgnored private var saveTask: Task<Void, Never>?
    @ObservationIgnored private var deadlineTask: Task<Void, Never>?
    @ObservationIgnored private var startupDeadline: Task<Void, Never>?
    @ObservationIgnored private var submissionNonce: String?
    @ObservationIgnored private var setting: SettingChange?
    @ObservationIgnored private let responseTimeout: Duration
    @ObservationIgnored private var restoringContext = ""

    private struct SettingChange {
        let request: String
        let generation: UUID
        let context: String
        let provider: String?
        let model: String?
        let effort: String?
    }

    public init(configuration: BridgeLaunchConfiguration, nativeDraftDirectory: URL, profile: String) {
        self.configuration = configuration
        self.store = NativeDraftStore(directory: nativeDraftDirectory, profile: profile)
        self.factory = { GoBridgeConnection() }
        self.responseTimeout = .seconds(20)
    }
    init(configuration: BridgeLaunchConfiguration, store: NativeDraftStore, responseTimeout: Duration = .seconds(20), factory: @escaping @Sendable () -> any BridgeTransport) {
        self.configuration = configuration; self.store = store; self.factory = factory; self.responseTimeout = responseTimeout
    }

    public var context: String { snapshot?.context ?? restoringContext }
    public var conversationID: String { snapshot?.conversationID ?? archive.selectedConversationID }
    public var title: String { snapshot?.title ?? "Gator" }
    public var pendingSubmission: PendingSubmission? { archive.pending }
    var draftRevision: UUID? { archive.drafts[context]?.revision }
    public var isConnected: Bool { connectionStatus == .connected }
    public var canEditDraft: Bool { loaded && !context.isEmpty }
    public var readiness: JSONValue { snapshot?.composer["readiness"] ?? .null }
    public var composerModels: [JSONValue] { snapshot?.composer["models"].array ?? [] }
    public var modelLabel: String {
        guard let snapshot else { return "Model unavailable" }
        return [snapshot.value["provider"].string, snapshot.value["model"].string].filter { !$0.isEmpty }.joined(separator: " / ")
    }
    public var canNavigate: Bool {
        isConnected && operation.isEmpty && !approvalSending && archive.pending == nil && !(snapshot?.unsafeToNavigate ?? true)
    }
    public var canConfigureModel: Bool {
        canNavigate && snapshot?.value["mode"].string == "chat" && snapshot?.composer["disabled"].string.isEmpty == true
    }
    public var sendUnavailableReason: String? {
        if !persistenceIssue.isEmpty { return "Draft storage needs attention. Sending is disabled to preserve your request." }
        if !isConnected { return "Reconnect to Gator before sending." }
        guard let snapshot, !context.isEmpty else { return "Opening the conversation…" }
        if archive.pending != nil { return "An earlier submission is unconfirmed. Inspect it before an explicit retry." }
        if !operation.isEmpty { return operation }
        if snapshot.value["dialogue_busy"].bool { return "Gator is considering your last message. Your next draft is kept here." }
        if (snapshot.busy || snapshot.hasApproval || snapshot.value["queued"].int > 0) && !(managed && snapshot.value["dialogue_active"].bool) { return "Work or review is in progress. You can keep drafting; conversation during work and queue controls arrive later." }
        if let notice = snapshot.unsupportedNotice, !(managed && (snapshot.value["dialogue_dispatch"] != .null || snapshot.value["session_request"] != .null)), !(managed && snapshot.value["dialogue_active"].bool && !snapshot.hasExternal) { return notice }
        if !snapshot.value["requests_issue"].string.isEmpty { return snapshot.value["requests_issue"].string }
        if snapshot.value["mode"].string != "chat" { return "Return to the conversation before sending." }
        if readiness["state"].string != "configured" { return readiness["issue"].string.isEmpty ? "Choose an already configured model. Provider setup is available in Gator’s terminal app." : readiness["issue"].string }
        if draftText.trimmingCharacters(in: .whitespacesAndNewlines).hasPrefix("/") { return "Native slash commands are not available yet. Use New chat, Recents and model controls above." }
        return nil
    }
    public var hasReviewableApproval: Bool { snapshot?.hasApproval == true && !retiredApprovalTokens.contains(snapshot?.value["pending_approval"]["token"].string ?? "") }
    public var canSend: Bool { sendUnavailableReason == nil && !draftText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
    public var canStop: Bool {
        isConnected && snapshot?.running == true && snapshot?.cancelling == false && snapshot?.value["mode"].string == "chat" && snapshot?.value["queued"].int == 0 && snapshot?.hasExternal == false && snapshot?.value["task_plans"].array.contains(where: { ["preparing", "revising"].contains($0["phase"].string) }) != true && !stopping && operation.isEmpty
    }
    public var canRetryAdmission: Bool {
        isConnected && archive.pending?.phase == .uncertain && archive.pending?.context == context && snapshot?.unsafeToNavigate == false && operation.isEmpty && persistenceIssue.isEmpty
    }
    public var reservesPlanCapacity: Bool {
        guard let pending = planOperation else { return false }
        if pending.phase == .accepted, pending.resultConfirmed,
           snapshot?.value["task_plans"].array.contains(where: { pending.identity.matches($0) && $0["state"].string == "queued" && $0["phase"].string == "approved" }) == true { return false }
        return pending.reservesCapacity
    }
    public var recovery: NativeRecoveryProjection? { snapshot.map(NativeRecoveryProjection.init) }
    public var quitGuard: String? {
        if receiptOperation?.unresolved == true { return "A read-only receipt check is unconfirmed. Its identity is retained; no check or external action will replay on restart." }
        if planOperation?.blocksAuthorization == true { return "A plan authorization outcome is unconfirmed. Its identity is retained and is never replayed. Quitting stops this owner." }
        if archive.pending != nil { return "A submission has an unconfirmed outcome. Its identity and draft will be kept; it will not be resent on restart." }
        if snapshot?.hasApproval == true { return "An exact action is waiting for review. Quitting cancels this Go owner; the approval will not be replayed." }
        if snapshot?.unsafeToNavigate == true || !operation.isEmpty { return "Gator is working or holding retained work. Quitting cancels this Go owner and keeps its durable history. Recovered work is not resumed automatically." }
        return nil
    }

    public func start() async {
        guard !started else { return }; started = true
        do {
            archive = try await store.load(); loaded = true
            if configuration.conversationID.isEmpty && configuration.bootMode != "new" { configuration.conversationID = archive.selectedConversationID }
            if managed {
                let requested = "conversation:" + configuration.conversationID
                archive.pending = archive.pendingByContext?[requested] ?? (archive.pending?.context == requested ? archive.pending : nil)
            }
            receiptOperation = archive.receiptOperationsByContext?["conversation:" + configuration.conversationID]
            submission = archive.pending
            planOperation = archive.planOperationsByContext?["conversation:" + configuration.conversationID]
            restoringContext = configuration.conversationID.isEmpty ? "" : "conversation:" + configuration.conversationID
            restoreDraft(restoringContext)
        } catch { persistenceIssue = "Native drafts could not be loaded: " + error.localizedDescription }
        await connect()
    }
    private func connect() async {
        connectionStatus = .starting; generation = UUID(); let ownerGeneration = generation
        let owner = factory(); transport = owner
        eventTask = Task { [weak self] in
            for await event in owner.events {
                guard let self, self.generation == ownerGeneration else { return }
                self.receive(event, generation: ownerGeneration)
            }
        }
        do {
            try await owner.start(configuration)
            startupDeadline?.cancel()
            startupDeadline = Task { [weak self, responseTimeout] in
                try? await Task.sleep(for: responseTimeout)
                guard !Task.isCancelled, let self, self.generation == ownerGeneration, self.connectionStatus == .starting else { return }
                self.issue = "Gator did not publish an initial state. Reconnect to try opening the conversation again."
                await owner.shutdown()
            }
        } catch { receive(.disconnected(error.localizedDescription), generation: ownerGeneration) }
    }
    func receive(_ event: BridgeEvent, generation ownerGeneration: UUID) {
        guard ownerGeneration == generation, connectionStatus != .closed else { return }
        let priorSnapshot = snapshot
        defer { changed?(self, priorSnapshot) }
        switch event {
        case .state(let next):
            startupDeadline?.cancel()
            connectionStatus = .connected
            let previous = snapshot
            let previousToken = previous?.value["pending_approval"]["token"].string ?? ""
            if !previousToken.isEmpty, previousToken != next.value["pending_approval"]["token"].string { retiredApprovalTokens.insert(previousToken) }
            snapshot = next
            if previous?.value["foreground"].bool == true && !next.value["foreground"].bool { planReviewEpoch = UUID() }
            if messages != next.messages { messages = next.messages }
            if previous?.value["messages"] != next.value["messages"] || previous?.value["dialogue"] != next.value["dialogue"] || previous?.value["dialogue_peers"] != next.value["dialogue_peers"] || previous?.context != next.context { transcript = next.transcript }
            if !next.conversationID.isEmpty { archive.selectedConversationID = next.conversationID; configuration.conversationID = next.conversationID; configuration.bootMode = "" }
            if previous?.context != next.context { restoreDraft(next.context) }
            if let pending = archive.pending, next.value["upload_result"]["request_id"].string == pending.id {
                deadlineTask?.cancel()
                let error = next.value["upload_result"]["error"].string
                if error.isEmpty {
                    var accepted = pending; accepted.phase = pending.planFirst == true ? .accepted : next.running ? .running : .accepted; submission = accepted
                    // Compare revision, not text: typing and then undoing is still a newer draft.
                    if archive.drafts[pending.context]?.revision == pending.draftRevision {
                        if var retained = archive.drafts[pending.context], retained.correction != nil {
                            retained.correction = nil; retained.revision = UUID(); retained.updated = Date(); archive.drafts[pending.context] = retained
                        } else { archive.drafts[pending.context] = NativeDraft() }
                        if context == pending.context { restoreDraft(context) }
                    }
                    archive.pending = nil
                } else {
                    var rejected = pending; rejected.phase = .failed; submission = rejected
                    archive.pending = nil; issue = error
                }
                submissionNonce = nil
            }
            if var current = submission, !current.phase.awaitingAcceptance, current.phase != .failed, current.phase != .stopped, current.phase != .completed {
                let turnID = SHA256.hash(data: Data(current.id.utf8)).map { String(format: "%02x", $0) }.joined()
                if current.planFirst == true {
                    // Preparing a proposal uses Go's running flag too. Admission
                    // of plan preparation is not authorization or execution.
                    current.phase = .accepted
                } else if managed, let turn = next.value["dialogue"].array.first(where: { $0["id"].string == turnID }) {
                    // A reply during Work belongs to its own retained turn. The
                    // unrelated Work's running flag cannot label it as running.
                    switch turn["state"].string {
                    case "answered", "clarification", "guided": current.phase = .completed
                    case "stopped": current.phase = .stopped
                    case "failed", "interrupted": current.phase = .failed
                    case "delegated":
                        let peer = next.value["dialogue_peers"].array.first { $0["conversation_id"] == turn["task_chat"] }
                        switch peer?["state"].string {
                        case "completed": current.phase = .completed
                        case "failed", "interrupted": current.phase = .failed
                        case "stopped": current.phase = .stopped
                        case "running": current.phase = .running
                        default: current.phase = .accepted
                        }
                    default: current.phase = .accepted
                    }
                }
                else if next.running { current.phase = .running }
                else if previous?.running == true {
                    let activityState = next.messages.last(where: { $0.value["activity"] != .null })?.value["activity"]["state"].string ?? ""
                    if activityState == "failed" { current.phase = .failed }
                    else if activityState == "stopped" { current.phase = .stopped }
                    else if activityState == "completed" { current.phase = .completed }
                }
                submission = current
            }
            if let setting, setting.generation == generation, setting.context == context, next.value["composer_result"]["request_id"].string == setting.request {
                let resultError = next.value["composer_result"]["error"].string
                if !resultError.isEmpty { issue = resultError }
                else if let provider = setting.provider, (next.value["provider"].string != provider || next.value["model"].string != setting.model) { issue = "The saved model selection was not confirmed. Review settings before sending." }
                else if let effort = setting.effort, next.composer["effort"].string != effort { issue = "The saved effort was not confirmed." }
                else { issue = "" }
                self.setting = nil; operation = ""; deadlineTask?.cancel()
            }
            if let reading = planReadRequest, reading.generation == generation, next.value["task_plan_result"]["request_id"].string == reading.id {
                let result = next.value["task_plan_result"], value = result["review"]
                if !result["error"].string.isEmpty { issue = result["error"].string }
                else if isCurrentPlan(reading.identity), reading.identity.matches(value) {
                    var inspected = NativePlanReview(identity: reading.identity, generation: generation, value: value)
                    inspected.summaryAtInspection = next.value["task_plans"].array.first(where: reading.identity.matches)
                    inspected.scopeAtInspection = NativePlanPresentation.scope(next)
                    inspected.focusEpoch = planReviewEpoch
                    planReviews[reading.identity.taskID] = inspected
                    // A replacement owner plus a fresh Go inspection can reconcile
                    // a retained uncertain request; it never resends that request.
                    if var pending = planOperation, pending.phase == .uncertain, pending.generation != generation, pending.identity.taskID == reading.identity.taskID {
                        pending.phase = .reconciled; pending.error = "Prior operation retained for inspection; reconnected owner inspected canonical state. No request was replayed."
                        setPlanOperation(pending)
                    }
                } else { issue = "The plan changed while loading. Inspect its current revision again." }
                planReadRequest = nil; operation = ""; deadlineTask?.cancel()
            }
            receiveReceiptResult(next)
            receivePlanOperation(next)
            if !next.running { stopping = false }
            if approvalSending && (previous?.value["pending_approval"] != next.value["pending_approval"]) { approvalSending = false; operation = ""; deadlineTask?.cancel(); review = nil }
            if !next.value["notice"].string.isEmpty { issue = next.value["notice"].string }
            // State ticks can be frequent: persistence is required only for durable client changes.
            if archive.selectedConversationID != previous?.conversationID || (submissionNonce == nil && previous?.value["upload_result"] != next.value["upload_result"]) { persist() }
        case .acknowledged(let nonce):
            if var pending = receiptOperation, pending.nonce == nonce, pending.phase == "submitted" { pending.phase = "acknowledged"; setReceiptOperation(pending) }
            if var pending = planOperation, pending.nonce == nonce, pending.phase == .submitted { pending.phase = .acknowledged; setPlanOperation(pending) }
            if nonce == submissionNonce, var pending = archive.pending, pending.phase == .submitted {
                pending.phase = .acknowledged; archive.pending = pending; submission = pending; persist()
            }
        case .disconnected(let reason):
            connectionStatus = .disconnected; issue = reason
            markReceiptUncertain()
            markUncertain(); markPlanUncertain("Original Go owner disconnected."); operation = ""; setting = nil; planReadRequest = nil; approvalSending = false; stopping = false
            deadlineTask?.cancel(); startupDeadline?.cancel(); planControlDeadline?.cancel()
        case .updateReady: break
        }
    }

    public func editDraft(_ text: String) {
        guard loaded, !context.isEmpty else { return }
        guard text.utf8.count <= (correctionTarget == nil ? 128 : 120) * 1024 else { issue = "The draft exceeds 128 KiB. Previous text is kept."; return }
        var draft = archive.drafts[context] ?? NativeDraft()
        if draft.correction != nil { draft.correction?.text = text } else { draft.text = text }
        draft.revision = UUID(); draft.updated = Date(); archive.drafts[context] = draft
        draftText = text; persist()
    }
    private func persist() {
        guard loaded else { return }
        let value = archive, previous = saveTask, store = store
        saveTask = Task { [weak self] in
            await previous?.value
            do {
                if self?.managed == true { try await store.saveOwner(value, context: "conversation:" + value.selectedConversationID) }
                else { try await store.save(value) }
                self?.persistenceIssue = ""
            }
            catch { self?.persistenceIssue = "Native drafts could not be saved: " + error.localizedDescription }
        }
    }
    public func flushDrafts() async -> Bool {
        await saveTask?.value
        // An unreadable store disables editing/sending. It must still be possible
        // to quit without rewriting that original file.
        return !loaded || persistenceIssue.isEmpty
    }
    private func markUncertain() {
        if var pending = archive.pending { pending.phase = .uncertain; archive.pending = pending; submission = pending; persist() }
    }
    public func submit() async {
        guard canSend, let draft = archive.drafts[context] else { issue = sendUnavailableReason ?? "Type a message first."; return }
        let text = draft.correction.map { $0.target.message($0.text) } ?? draft.text
        let pending = PendingSubmission(id: UUID().uuidString, context: context, text: text, draftRevision: draft.revision, phase: .submitted, planFirst: draft.correction == nil ? draft.planFirst : false, correctionTarget: draft.correction?.target)
        archive.pending = pending; submission = pending; issue = ""; persist()
        guard await flushDrafts() else { markUncertain(); return }
        await dispatchSubmission(pending)
    }
    public func retryOriginalAdmission() async {
        guard canRetryAdmission, var pending = archive.pending else { return }
        pending.phase = .submitted; archive.pending = pending; submission = pending; persist()
        guard await flushDrafts() else { markUncertain(); return }
        await dispatchSubmission(pending)
    }
    private func dispatchSubmission(_ pending: PendingSubmission) async {
        guard let owner = transport, isConnected, pending.context == context else { markUncertain(); return }
        let ownerGeneration = generation, nonce = UUID().uuidString
        submissionNonce = nonce; startDeadline { [weak self] in
            guard self?.archive.pending?.id == pending.id else { return }
            self?.markUncertain(); self?.issue = "Admission is unconfirmed. The original identity is kept. Reconnect and inspect before explicitly retrying."
        }
        do { try await owner.send(.submission(id: pending.id, context: pending.context, text: pending.text, planFirst: pending.planFirst == true), nonce: nonce) }
        catch {
            guard generation == ownerGeneration, archive.pending?.id == pending.id else { return }
            markUncertain(); issue = error.localizedDescription
        }
    }

    public func openConversation(_ id: String) async {
        guard canNavigate, let snapshot, snapshot.value["recent_conversations"].array.contains(where: { $0["id"].string == id }), let owner = transport else { return }
        await navigate(.conversation(id: id, listID: snapshot.value["recent_id"].string), owner: owner, expected: id)
    }
    public func newConversation() async {
        guard canNavigate, let owner = transport else { return }
        await navigate(.newConversation, owner: owner, expected: nil)
    }
    private func navigate(_ command: BridgeCommand, owner: any BridgeTransport, expected: String?) async {
        guard await flushDrafts() else { return }
        guard canNavigate else { return }
        operation = "Opening conversation…"; let ownerGeneration = generation, previous = context
        defer { if generation == ownerGeneration { operation = "" } }
        do {
            try await owner.send(command, nonce: UUID().uuidString)
            if let expected, conversationID != expected { issue = snapshot?.value["notice"].string ?? "The conversation did not open. Choose it from the refreshed list." }
            else if expected == nil && context == previous { issue = "Go did not confirm a new conversation. Your original conversation is kept." }
        } catch { if generation == ownerGeneration { issue = error.localizedDescription } }
    }

    public func selectModel(provider: String, model: String) async {
        guard canConfigureModel, let snapshot, composerModels.contains(where: { $0["provider"].string == provider && $0["model"].string == model && $0["unavailable"].string.isEmpty }) else { return }
        let request = UUID().uuidString
        await changeSetting(.model(id: snapshot.composer["id"].string, request: request, provider: provider, model: model), setting: SettingChange(request: request, generation: generation, context: context, provider: provider, model: model, effort: nil))
    }
    public func selectEffort(_ effort: String) async {
        guard canConfigureModel, let snapshot, ["low", "standard", "high"].contains(effort) else { return }
        let request = UUID().uuidString
        await changeSetting(.effort(id: snapshot.composer["id"].string, request: request, effort: effort), setting: SettingChange(request: request, generation: generation, context: context, provider: nil, model: nil, effort: effort))
    }
    public func selectWorkMode(_ mode: NativeWorkMode) async {
        guard canNavigate, let owner = transport else { issue = "Finish the current operation before changing this chat's work mode."; return }
        let original = generation, deadline = ContinuousClock.now + .seconds(5)
        operation = "Waiting for Gator's work mode…"
        defer { if generation == original { operation = "" } }
        do {
            try await owner.send(.workMode(mode), nonce: UUID().uuidString)
            while generation == original, isConnected, snapshot?.value["work_details"]["mode"].string != mode.rawValue {
                guard ContinuousClock.now < deadline else { throw BridgeFailure("Work mode was not confirmed. Inspect its current value before sending.") }
                try await Task.sleep(for: .milliseconds(10))
            }
        } catch { if generation == original { issue = error.localizedDescription } }
    }
    private func changeSetting(_ command: BridgeCommand, setting: SettingChange) async {
        guard let owner = transport else { return }; self.setting = setting; operation = "Waiting for saved settings…"; issue = ""
        startDeadline { [weak self] in self?.setting = nil; self?.operation = ""; self?.issue = "Settings outcome is unconfirmed. Inspect the current selection before trying again." }
        do { try await owner.send(command, nonce: UUID().uuidString) }
        catch { if generation == setting.generation, self.setting != nil { self.setting = nil; operation = ""; issue = error.localizedDescription } }
    }

    public func stopWork() async {
        guard canStop, let owner = transport else { return }
        let ownerGeneration = generation, originalContext = context
        stopping = true; issue = ""
        do {
            try await owner.send(.stop, nonce: UUID().uuidString)
            guard generation == ownerGeneration, context == originalContext else { return }
            // The acknowledgement is not a cancellation receipt. Wait for Go's state.
            if snapshot?.cancelling == true { stopping = true }
        } catch { if generation == ownerGeneration { issue = error.localizedDescription; stopping = false } }
    }
    public var currentApprovalReview: ApprovalReview? {
        guard let snapshot, hasReviewableApproval else { return nil }
        let pending = snapshot.value["pending_approval"], index = pending["message_index"].int
        let result = index >= 0 && index < messages.count ? messages[index].result : .null
        return ApprovalReview(conversationID: conversationID, context: context, generation: generation, interactionID: pending["id"].int, token: pending["token"].string, messageIndex: index, result: result)
    }
    public func openApprovalReview() { review = currentApprovalReview }
    public func closeApprovalReview() { if !approvalSending { review = nil; requestComposerFocus() } }
    public func isCurrentReview(_ candidate: ApprovalReview) -> Bool {
        guard isConnected, candidate.generation == generation, candidate.context == context, candidate.conversationID == conversationID, let snapshot else { return false }
        let pending = snapshot.value["pending_approval"]
        let exactResult = candidate.result != .null && candidate.messageIndex >= 0 && candidate.messageIndex < messages.count && messages[candidate.messageIndex].result == candidate.result
        return !retiredApprovalTokens.contains(candidate.token) && exactResult && candidate.interactionID > 0 && candidate.interactionID == pending["id"].int && candidate.token.count == 64 && candidate.token == pending["token"].string && candidate.messageIndex == pending["message_index"].int
    }
    public func respondToApproval(_ candidate: ApprovalReview, approved: Bool) async {
        guard !approvalSending, isCurrentReview(candidate), !approved || candidate.inspectable, let owner = transport else { return }
        if approved {
            guard candidate.messageIndex >= 0, candidate.messageIndex < messages.count, messages[candidate.messageIndex].result == candidate.result else { issue = "Approval details changed. Reopen the exact review."; return }
        }
        retiredApprovalTokens.insert(candidate.token)
        approvalSending = true; operation = "Waiting for the approval outcome…"
        startDeadline { [weak self] in self?.approvalSending = false; self?.operation = ""; self?.issue = "The approval outcome is unconfirmed. Inspect retained work; do not repeat an external action." }
        do { try await owner.send(.approval(id: candidate.interactionID, token: candidate.token, approved: approved), nonce: UUID().uuidString) }
        catch { if generation == candidate.generation { approvalSending = false; operation = ""; issue = error.localizedDescription } }
    }

    public func reconnect() async {
        guard connectionStatus == .disconnected || connectionStatus == .idle else { return }
        markUncertain(); markPlanUncertain("Owner reconnect requested."); guard await flushDrafts() else { return }
        operation = "Reconnecting…"
        await transport?.shutdown(); eventTask?.cancel(); transport = nil
        configuration.conversationID = archive.selectedConversationID
        review = nil; operation = ""; issue = ""
        await connect()
        // Deliberately no send here: restore is not a resubmission or approval replay.
    }
    public func shutdown(confirmed: Bool = false) async -> Bool {
        guard confirmed || quitGuard == nil else { return false }
        markReceiptUncertain()
        markUncertain(); markPlanUncertain("Application shutdown requested."); guard await flushDrafts() else { return false }
        connectionStatus = .stopping
        deadlineTask?.cancel(); startupDeadline?.cancel(); planControlDeadline?.cancel()
        await transport?.shutdown(); eventTask?.cancel(); transport = nil
        connectionStatus = .closed; review = nil; return true
    }

    private func restoreDraft(_ context: String) {
        let draft = archive.drafts[context]
        correctionTarget = draft?.correction?.target
        draftText = draft?.correction?.text ?? draft?.text ?? ""
        planFirst = draft?.correction == nil && draft?.planFirst == true
    }
    public func setPlanFirst(_ value: Bool) {
        guard canEditDraft, correctionTarget == nil else { return }
        var draft = archive.drafts[context] ?? NativeDraft()
        draft.planFirst = value; draft.revision = UUID(); draft.updated = Date(); archive.drafts[context] = draft
        planFirst = value; persist()
    }
    func beginCorrection(_ target: NativeCorrectionTarget) -> Bool {
        guard canEditDraft, archive.pending == nil, correctionTarget == nil else { issue = "An existing draft submission or correction is kept. Finish or cancel it before starting another correction."; return false }
        var draft = archive.drafts[context] ?? NativeDraft()
        draft.correction = NativeCorrectionDraft(target: target); draft.revision = UUID(); draft.updated = Date(); archive.drafts[context] = draft
        restoreDraft(context); persist(); requestComposerFocus(); return true
    }
    public func cancelCorrection() {
        guard archive.pending == nil, var draft = archive.drafts[context] else { return }
        draft.correction = nil; draft.revision = UUID(); draft.updated = Date(); archive.drafts[context] = draft
        restoreDraft(context); persist(); requestComposerFocus()
    }
    public func requestComposerFocus() { focusRequest = UUID() }
    public func isCurrentPlan(_ identity: NativePlanIdentity) -> Bool {
        isConnected && identity.conversationID == conversationID && snapshot?.value["task_plans"].array.contains(where: identity.matches) == true
    }
    public func isCurrentPlanReview(_ value: NativePlanReview) -> Bool {
        guard value.generation == generation, isCurrentPlan(value.identity), let snapshot else { return false }
        if let epoch = value.focusEpoch, epoch != planReviewEpoch { return false }
        if let captured = value.summaryAtInspection, snapshot.value["task_plans"].array.first(where: value.identity.matches) != captured { return false }
        if let captured = value.scopeAtInspection, captured != NativePlanPresentation.scope(snapshot) { return false }
        return true
    }
    func inspectPlan(_ identity: NativePlanIdentity) async {
        guard managed, isCurrentPlan(identity), snapshot?.value["foreground"].bool == true, archive.pending == nil, operation.isEmpty, let owner = transport else { issue = "Open the original chat and inspect its current plan revision."; return }
        let request = UUID().uuidString, original = generation
        planReadRequest = (request, identity, original); operation = "Loading exact proposed plan…"; issue = ""
        startDeadline { [weak self] in self?.planReadRequest = nil; self?.operation = ""; self?.issue = "Plan inspection was not confirmed. Your draft is kept; inspect again explicitly." }
        do { try await owner.send(.inspectPlan(identity, request: request), nonce: UUID().uuidString) }
        catch { if generation == original { planReadRequest = nil; operation = ""; issue = error.localizedDescription } }
    }

    public func planAuthorizationUnavailable(_ review: NativePlanReview, action: NativePlanAction) -> String? { planAuthorizationIssue(review, action: action) }
    private func planAuthorizationIssue(_ review: NativePlanReview, action: NativePlanAction, permittedPendingID: String? = nil) -> String? {
        let ownPrepared = permittedPendingID != nil && planOperation?.id == permittedPendingID && planOperation?.phase == .submitted
        guard managed, isConnected, let snapshot, snapshot.value["foreground"].bool else { return "Open and focus the original connected owner, then inspect again." }
        guard isCurrentPlanReview(review), planReviews[review.identity.taskID] == review else { return "This review changed. Inspect the exact current plan again." }
        guard review.summaryAtInspection != nil, review.scopeAtInspection != nil, NativePlanPresentation.complete(review) else { return "The full authoritative review is incomplete. Approval is unavailable." }
        if planOperation?.blocksAuthorization == true && !ownPrepared { return "A prior authorization is unconfirmed. Reconnect and inspect; do not repeat it." }
        if !ownPrepared, let prior = planOperation, prior.identity == review.identity, prior.action == action,
           ![NativePlanOperationPhase.rejected, .notSubmitted, .reconciled].contains(prior.phase) {
            return "This exact authorization was already submitted. Inspect its retained outcome; it will not be sent twice."
        }
        if (!operation.isEmpty && !ownPrepared) || archive.pending != nil || approvalSending || snapshot.running || snapshot.hasApproval || snapshot.hasExternal || snapshot.value["dialogue_busy"].bool || snapshot.value["dialogue_dispatch"] != .null || snapshot.value["pending_operation"].string.isEmpty == false || snapshot.value["receipt_busy"].bool || snapshot.value["catalog_busy"].bool { return "Finish the current operation or exact action review before authorizing this plan." }
        guard snapshot.value["mode"].string == "chat", snapshot.value["panel"] == .null, snapshot.value["session_request"] == .null else { return "Return to the original idle conversation and inspect again." }
        if !persistenceIssue.isEmpty { return "Native operation storage needs attention. Nothing will be submitted." }
        if !snapshot.value["requests_issue"].string.isEmpty { return snapshot.value["requests_issue"].string }
        guard review.value["state"].string == "queued", review.value["phase"].string == (action == .approve ? "review" : "approved") else { return action == .start ? "Start requires an approved, undispatched queued plan." : "Only a queued proposed plan can be approved." }
        if review.value["edit_state"].string == "preparing" { return "Wait for the replacement proposal and inspect it." }
        if review.value["attempts"].array.contains(where: { $0["uncertain"].bool }) { return "Prior effects are uncertain. Inspect retained evidence; authorization is unavailable." }
        return nil
    }
    func authorizePlan(_ review: NativePlanReview, action: NativePlanAction, finalAdmissionAllowed: @MainActor () -> Bool) async {
        guard planAuthorizationUnavailable(review, action: action) == nil, let transport else { return }
        let pending = NativePlanOperation(id: UUID().uuidString, nonce: UUID().uuidString, identity: review.identity, generation: generation, action: action)
        setPlanOperation(pending); operation = "Awaiting exact plan authorization confirmation…"; issue = ""
        guard await flushDrafts() else {
            var unsent = pending; unsent.phase = .notSubmitted; unsent.error = "Operation could not be durably recorded. No send was attempted."; setPlanOperation(unsent); operation = ""; return
        }
        // Recheck after persistence without using the pending-operation guard.
        guard generation == pending.generation, finalAdmissionAllowed(), planAuthorizationIssue(review, action: action, permittedPendingID: pending.id) == nil else {
            var unsent = pending; unsent.phase = .notSubmitted; unsent.error = "Owner, proposal, capacity or application state changed before send. Inspect again."; setPlanOperation(unsent); operation = ""; return
        }
        planControlDeadline?.cancel()
        planControlDeadline = Task { [weak self, responseTimeout] in
            try? await Task.sleep(for: responseTimeout)
            guard !Task.isCancelled, let self, self.planOperation?.id == pending.id, self.planOperation?.phase.awaitingResult == true else { return }
            self.markPlanUncertain("Go did not confirm the request ID."); self.operation = ""
        }
        do { try await transport.send(.authorizePlan(pending.identity, action: action, request: pending.id), nonce: pending.nonce) }
        catch { if generation == pending.generation { markPlanUncertain(error.localizedDescription); operation = "" } }
    }
    private func setPlanOperation(_ value: NativePlanOperation) {
        planOperation = value
        if archive.planOperationsByContext == nil { archive.planOperationsByContext = [:] }
        archive.planOperationsByContext?["conversation:" + value.identity.conversationID] = value
        persist()
    }
    private func markPlanUncertain(_ reason: String) {
        guard var value = planOperation, value.phase.awaitingResult || value.phase == .accepted else { return }
        value.phase = .uncertain; value.error = reason; setPlanOperation(value); planControlDeadline?.cancel()
    }
    private func receivePlanOperation(_ next: BridgeSnapshot) {
        guard var pending = planOperation else { return }
        let old = pending
        let result = next.value["task_plan_result"]
        if !pending.resultConfirmed, pending.generation == generation, result["request_id"].string == pending.id {
            pending.resultConfirmed = true; pending.error = result["error"].string
            pending.phase = pending.error.isEmpty ? .accepted : .rejected
            if !pending.error.isEmpty { planReviewEpoch = UUID() } // Retain the read, invalidate its controls until fresh inspection.
            operation = ""; planControlDeadline?.cancel()
        }
        if pending.resultConfirmed, pending.phase != .rejected,
           let summary = next.value["task_plans"].array.first(where: pending.identity.matches) {
            pending.workID = summary["work_id"].string
            switch summary["state"].string {
            case "running": if summary["phase"].string == "dispatching" && next.running { pending.phase = next.hasApproval ? .waitingReview : .working }
            case "completed": pending.phase = .completed
            case "failed", "interrupted": pending.phase = .failed
            case "stopped", "cancelled": pending.phase = .stopped
            default: break
            }
        }
        if pending != old { setPlanOperation(pending) }
    }
    func awaitPlanAuthorizationResult() async {
        while planOperation?.phase.awaitingResult == true && isConnected { try? await Task.sleep(for: .milliseconds(10)) }
    }

    public func receiptUnavailable(_ candidate: NativeRecoveryReceipt, generation expected: UUID) -> String? {
        guard isConnected, generation == expected, candidate.identity?.conversationID == conversationID else { return "Open the original connected owner and inspect its current evidence." }
        guard let current = recovery?.receipts.first(where: { $0.id == candidate.id }), current == candidate, current.identity != nil else { return "Exact retained bundle identity is missing or changed. A journal receipt alone cannot authorize a check." }
        if receiptOperation?.unresolved == true { return "A prior check is unconfirmed. Its request identity is retained; no automatic retry. Reconnect and inspect retained evidence." }
        guard operation.isEmpty, snapshot?.value["receipt_busy"].bool != true else { return "Wait for the current owner operation." }
        if !persistenceIssue.isEmpty { return "Native storage needs attention. No check was submitted." }
        return nil
    }
    func checkReceipt(_ candidate: NativeRecoveryReceipt, generation expected: UUID, action: NativeReceiptAction) async {
        guard receiptUnavailable(candidate, generation: expected) == nil, let identity = candidate.identity, let owner = transport else { return }
        var pending = NativeReceiptOperation(id: UUID().uuidString, nonce: UUID().uuidString, identity: identity, generation: generation, action: action)
        setReceiptOperation(pending); operation = "Awaiting read-only receipt check…"
        guard await flushDrafts() else { pending.phase = "uncertain"; setReceiptOperation(pending); operation = ""; return }
        guard generation == expected, isConnected, recovery?.receipts.contains(candidate) == true else {
            pending.phase = "failed"; pending.result = .object(["error": .string("Owner or evidence changed before send. No check submitted.")]); setReceiptOperation(pending); operation = ""; return
        }
        receiptDeadline?.cancel()
        receiptDeadline = Task { [weak self, responseTimeout] in
            try? await Task.sleep(for: responseTimeout)
            guard !Task.isCancelled, let self, self.receiptOperation?.id == pending.id, self.receiptOperation?.unresolved == true else { return }
            self.markReceiptUncertain(); self.operation = ""
        }
        do { try await owner.send(.receipt(identity, action: action, request: pending.id), nonce: pending.nonce) }
        catch { if generation == expected, receiptOperation?.id == pending.id, receiptOperation?.unresolved == true { markReceiptUncertain(); operation = "" } }
    }
    private func setReceiptOperation(_ value: NativeReceiptOperation) {
        receiptOperation = value
        if archive.receiptOperationsByContext == nil { archive.receiptOperationsByContext = [:] }
        archive.receiptOperationsByContext?["conversation:" + value.identity.conversationID] = value
        persist()
    }
    private func markReceiptUncertain() {
        guard var value = receiptOperation, value.unresolved else { return }
        value.phase = "uncertain"; setReceiptOperation(value); receiptDeadline?.cancel()
    }
    private func receiveReceiptResult(_ next: BridgeSnapshot) {
        guard var pending = receiptOperation, pending.unresolved, pending.generation == generation, pending.identity.conversationID == next.conversationID else { return }
        let result = next.value["receipt_result"]
        guard result["request_id"].string == pending.id else { return }
        pending.result = result
        pending.phase = result["error"].string.isEmpty && ["not_checked", "found", "ambiguous", "unconfirmed"].contains(result["check"]["status"].string) ? "confirmed" : "failed"
        setReceiptOperation(pending); receiptDeadline?.cancel(); operation = ""
    }

    // Host-only operations retain this owner's generation through every await.
    func host(_ command: HostCommand, timeout: Duration = .seconds(5), until accepted: @escaping @MainActor (BridgeSnapshot) -> Bool = { _ in true }) async throws {
        guard isConnected, let owner = transport else { throw BridgeFailure("Original Go owner is disconnected. Inspect retained work before retrying.") }
        let originalGeneration = generation
        try await owner.send(.host(command), nonce: UUID().uuidString)
        let deadline = ContinuousClock.now + timeout
        while true {
            guard generation == originalGeneration, isConnected else { throw BridgeFailure("Original Go owner changed or disconnected; the outcome is unconfirmed.") }
            if let snapshot, accepted(snapshot) { return }
            guard ContinuousClock.now < deadline else { throw BridgeFailure("The owner did not confirm this operation. Inspect retained work; it will not be retried automatically.") }
            try await Task.sleep(for: .milliseconds(10))
        }
    }
    func awaitReady(timeout: Duration = .seconds(15)) async throws {
        let deadline = ContinuousClock.now + timeout
        while !isConnected {
            if connectionStatus == .disconnected || connectionStatus == .closed { throw BridgeFailure(issue.isEmpty ? "Go owner could not open." : issue) }
            guard ContinuousClock.now < deadline else { throw BridgeFailure("Go did not confirm this chat. Original work was kept.") }
            try await Task.sleep(for: .milliseconds(10))
        }
    }
    func requestManagedConversation(_ id: String) async throws {
        guard managed, isConnected, let snapshot, snapshot.value["recent_conversations"].array.contains(where: { $0["id"].string == id }), let owner = transport else { throw BridgeFailure("This conversation is no longer in the current Go list.") }
        let sequence = snapshot.value["session_request"]["sequence"].int, notice = snapshot.value["notice"].string
        try await owner.send(.conversation(id: id, listID: snapshot.value["recent_id"].string), nonce: UUID().uuidString)
        try await awaitNavigation(after: sequence, notice: notice)
    }
    func requestManagedNew() async throws {
        guard managed, isConnected, let owner = transport else { throw BridgeFailure("Reconnect before opening a new chat.") }
        let sequence = snapshot?.value["session_request"]["sequence"].int ?? 0, notice = snapshot?.value["notice"].string ?? ""
        try await owner.send(.newConversation, nonce: UUID().uuidString)
        try await awaitNavigation(after: sequence, notice: notice)
    }
    private func awaitNavigation(after sequence: Int, notice: String) async throws {
        let originalGeneration = generation, deadline = ContinuousClock.now + .seconds(5)
        while isConnected, generation == originalGeneration {
            if let snapshot, snapshot.value["session_request"]["sequence"].int > sequence { return }
            if let next = snapshot?.value["notice"].string, !next.isEmpty, next != notice { throw BridgeFailure(next) }
            guard ContinuousClock.now < deadline else { throw BridgeFailure("Go did not project the requested navigation. Original selection and draft retained.") }
            try await Task.sleep(for: .milliseconds(10))
        }
        throw BridgeFailure("The requesting owner disconnected before navigation was confirmed.")
    }
    func awaitAdmissionResolution() async {
        while let pending = archive.pending, pending.phase == .submitted || pending.phase == .acknowledged {
            guard isConnected else { return }
            try? await Task.sleep(for: .milliseconds(10))
        }
    }

    private func startDeadline(_ action: @escaping @MainActor () -> Void) {
        deadlineTask?.cancel()
        deadlineTask = Task { [responseTimeout] in
            try? await Task.sleep(for: responseTimeout)
            if !Task.isCancelled { action() }
        }
    }
}
