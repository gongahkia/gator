import SwiftUI
import AppKit
import GatorCore

@MainActor private var gatorAccent: Color { GatorTheme.accent }

struct ConversationView: View {
    @Environment(ManagedSessionCoordinator.self) private var sessions
    private var application: GatorApplicationState { sessions.current }
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.openWindow) private var openWindow
    let compact: Bool
    let quit: () -> Void
    @AppStorage("gator.notch.enabled") private var notchEnabled = true
    @State private var contentSize = CGSize(width: 540, height: 480)
    @State private var sessionsExpanded = false
    @State private var connectionDetails = false
    @State private var noticesHeight: CGFloat = 24

    var body: some View {
        VStack(spacing: 0) {
            if compact { header.padding(.horizontal, 14).padding(.vertical, 10); Divider() }
            if sessions.activity.count > 1 { sessionActivity.padding(.horizontal, 14).padding(.vertical, 6) }
            if compact, let inspection = sessions.inspection {
                TaskInspectorView(inspection: inspection, owner: sessions.inspectedOwner, compact: compact).padding(12)
            } else if let review = application.review {
                ApprovalReviewView(review: review, compact: compact).padding(12)
                Spacer(minLength: 0)
            } else {
                if application.transcript.isEmpty {
                    VStack(spacing: 12) {
                        Image(systemName: "bubble.left.and.bubble.right").font(.system(size: 30, weight: .light)).foregroundStyle(gatorAccent).accessibilityHidden(true)
                        Text("What can I help with?").font(compact ? .title3.weight(.medium) : .title2.weight(.medium))
                        Text("Ask a question, make a plan, or pick up where you left off.").font(.callout).foregroundStyle(.secondary).multilineTextAlignment(.center).frame(maxWidth: 330)
                    }.padding(24).frame(maxWidth: .infinity, maxHeight: .infinity)
                } else { NativeTranscriptView(compact: compact).id(application.context) }
            }
            ScrollView {
                notices.padding(.horizontal, 14).padding(.vertical, 4)
                    .onGeometryChange(for: CGFloat.self) { $0.size.height } action: { noticesHeight = max(24, $0) }
            }.frame(height: min(noticesHeight, compact ? 105 : 160)).accessibilityLabel("Conversation status and recovery")
            if application.hasReviewableApproval && application.review == nil && sessions.inspection == nil {
                Button { application.openApprovalReview() } label: { Label("An action needs your review", systemImage: "hand.raised").frame(maxWidth: .infinity) }
                    .buttonStyle(.bordered).padding(.horizontal, 14).padding(.top, 6)
            }
            if application.review == nil && (!compact || sessions.inspection == nil) { ComposerView(compact: compact).padding(12) }
        }
        .background(Color(nsColor: .windowBackgroundColor))
        .background(NativeWindowConfiguration(compact: compact).frame(width: 0, height: 0))
        .onGeometryChange(for: CGSize.self) { $0.size } action: { contentSize = $0 }
        .sheet(isPresented: Binding(get: { !compact && sessions.inspection != nil }, set: { if !$0 { sessions.closeTaskInspection() } })) {
            if let inspection = sessions.inspection {
                TaskInspectorView(inspection: inspection, owner: sessions.inspectedOwner, compact: false)
                    .padding(12)
                    .frame(width: max(400, min(660, contentSize.width - 48)), height: max(280, min(620, contentSize.height - 64)))
            }
        }
        .toolbar {
            if !compact {
                ToolbarItem(placement: .navigation) { RecentConversationsMenu() }
                ToolbarItemGroup(placement: .primaryAction) { headerActions }
            }
        }
        .navigationSubtitle(compact ? "" : application.title)
        .environment(application)
        .tint(gatorAccent)
        .transaction { if reduceMotion { $0.animation = nil; $0.disablesAnimations = true } }
        .environment(\.openURL, OpenURLAction { url in
            guard ["https", "http"].contains(url.scheme?.lowercased() ?? ""), url.user == nil, url.password == nil else { return .discarded }
            NSWorkspace.shared.open(url); return .handled
        })
        .accessibilityIdentifier(compact ? "gator.compact" : "gator.window")
    }

    private var header: some View {
        HStack(spacing: 10) {
            VStack(alignment: .leading, spacing: 2) {
                Text("Gator").font(.headline)
                Text(application.title).font(.caption).foregroundStyle(.secondary).lineLimit(1)
            }
            Spacer(minLength: 4)
            headerActions
        }
    }
    @ViewBuilder private var headerActions: some View {
        if !compact {
            Button { Task { await sessions.newConversation() } } label: { Image(systemName: "square.and.pencil") }
                .disabled(!sessions.canNavigate).help("New Conversation · ⌘N").accessibilityLabel("New Conversation")
        }
        Button { sessions.inspectTask(application.conversationID) } label: { Image(systemName: "doc.text.magnifyingglass") }
            .help("Inspect current task · ⌘I").accessibilityLabel("Inspect Current Task").disabled(application.snapshot == nil || sessions.switching).focusable()
        if let parent = application.snapshot?.value["dialogue_task"]["parent_id"].string, !parent.isEmpty {
            Button { Task { await sessions.openConversation(parent) } } label: { Image(systemName: "arrow.uturn.backward") }
                .disabled(sessions.switching).help("Return to requesting chat").accessibilityLabel("Return to Requesting Chat")
        }
        if application.canStop || application.stopping || application.snapshot?.cancelling == true {
            Button { let owner = application; Task { await owner.stopWork() } } label: { Label(application.stopping || application.snapshot?.cancelling == true ? "Stopping…" : "Stop", systemImage: "stop.fill") }
                .disabled(sessions.switching || !application.canStop).help("Stop this conversation's current work")
        }
        conversationMenu
        if compact {
            Button { openWindow(id: "conversation"); NSApp.activate(ignoringOtherApps: true) } label: { Image(systemName: "arrow.up.left.and.arrow.down.right") }
                .buttonStyle(.borderless).help("Expand this conversation").accessibilityLabel("Open full Gator window").focusable()
        }
    }
    private var modelMenu: some View {
        Menu {
            modelChoices
        } label: { Text(application.modelLabel).lineLimit(1) }
        .menuStyle(.borderlessButton).fixedSize().accessibilityLabel("Select Model").help(application.modelLabel)
    }
    @ViewBuilder private var modelChoices: some View {
            ForEach(application.composerModels, id: \.self) { model in
                Button((application.snapshot?.value["provider"] == model["provider"] && application.snapshot?.value["model"] == model["model"] ? "✓ " : "") + (model["label"].string.isEmpty ? model["model"].string : model["label"].string)) {
                    let owner = application; Task { await owner.selectModel(provider: model["provider"].string, model: model["model"].string) }
                }.disabled(sessions.switching || !application.canConfigureModel || !model["unavailable"].string.isEmpty)
            }
            if application.composerModels.isEmpty { Text("No configured models · use terminal setup") }
    }
    private var conversationMenu: some View {
        Menu {
            Button("New Conversation", systemImage: "square.and.pencil") { Task { await sessions.newConversation() } }.disabled(!sessions.canNavigate)
            Menu("Recent Conversations") {
                ForEach(application.snapshot?.value["recent_conversations"].array ?? [], id: \.self) { item in
                    Button(item["title"].string.isEmpty ? item["id"].string : item["title"].string) { Task { await sessions.openConversation(item["id"].string) } }.disabled(!sessions.canNavigate)
                }
                if application.snapshot?.value["recent_conversations"].array.isEmpty != false { Text("No recent conversations") }
            }
            Divider()
            Menu("Configured Models") { modelChoices }
            Menu("Effort") {
                ForEach(["low", "standard", "high"], id: \.self) { effort in
                    Button((application.snapshot?.composer["effort"].string == effort ? "✓ " : "") + effort.capitalized) { let owner = application; Task { await owner.selectEffort(effort) } }.disabled(sessions.switching || !application.canConfigureModel)
                }
            }
            Menu("Work Mode · This Chat") {
                ForEach(NativeWorkMode.allCases, id: \.rawValue) { mode in
                    Button((application.snapshot?.value["work_details"]["mode"].string == mode.rawValue ? "✓ " : "") + mode.label) { let owner = application; Task { await owner.selectWorkMode(mode) } }.disabled(!application.canNavigate || sessions.switching)
                }
                Text("Proposed actions still require exact review.")
            }
            Toggle("Notch Quick Access", isOn: $notchEnabled)
            Button("Connection & Model Details") { connectionDetails = true }
            Divider()
            Button("Reconnect and Inspect") { Task { await sessions.reconnect() } }.disabled(application.connectionStatus != .disconnected)
            Button("Quit Gator", role: .destructive, action: quit).keyboardShortcut("q")
        } label: { Image(systemName: "ellipsis.circle") }
        .menuStyle(.borderlessButton).fixedSize().help("Conversation controls").accessibilityLabel("More").focusable()
    }

    private var sessionActivity: some View {
        DisclosureGroup(isExpanded: $sessionsExpanded) {
            ScrollView {
                VStack(spacing: 4) {
                    ForEach(sessions.activity) { item in
                        HStack {
                        Button {
                            let id = item.id
                            Task { await sessions.openConversation(id) }
                        } label: {
                            HStack(alignment: .top, spacing: 8) {
                                Image(systemName: item.id == application.conversationID ? "checkmark.circle.fill" : item.needsAttention ? "circle.fill" : "bubble.left")
                                    .font(.caption).frame(width: 16)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(verbatim: item.title).fontWeight(item.id == application.conversationID ? .semibold : .regular).lineLimit(1)
                                    Text(verbatim: item.status).foregroundStyle(.secondary)
                                    if !item.summary.isEmpty { Text(verbatim: item.summary).foregroundStyle(.secondary).lineLimit(2) }
                                }
                                Spacer(minLength: 0)
                            }.padding(6).frame(maxWidth: .infinity, alignment: .leading)
                                .background(item.id == application.conversationID ? gatorAccent.opacity(0.10) : .clear, in: RoundedRectangle(cornerRadius: 6))
                        }.buttonStyle(.plain).disabled(sessions.switching)
                            .accessibilityLabel("Open chat: " + item.title + " · " + item.status)
                            .accessibilityIdentifier("gator.chat." + item.id)
                        Button { sessions.inspectTask(item.id) } label: { Image(systemName: "info.circle").frame(width: 24, height: 28) }
                            .accessibilityLabel("Inspect task: " + item.title).accessibilityIdentifier("gator.inspect." + item.id).disabled(sessions.switching || sessions.owners[item.id] == nil)
                        }
                    }
                }
            }.frame(maxHeight: compact ? 130 : 170)
        } label: {
            let others = sessions.activity.filter { $0.id != application.conversationID }
            Label(others.contains(where: { $0.status == "Working" || $0.status == "Stopping" }) ? "Work continues in another chat" : others.contains(where: \.needsAttention) ? "Another chat needs attention" : "Open chats and retained tasks", systemImage: "bubble.left.and.bubble.right")
        }.font(.caption).help("Inspect or return to an original chat without moving its work")
    }

    @ViewBuilder private var notices: some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack(spacing: 6) {
                Circle().fill(application.isConnected ? gatorAccent : .secondary).frame(width: 6, height: 6)
                Text(application.isConnected ? "Connected" : application.connectionStatus.rawValue.capitalized)
                    .help("Connection to Gator. This does not establish model-provider connectivity.")
                Spacer(minLength: 8)
                modelMenu
            }.font(.caption).foregroundStyle(.secondary)
            if let plan = application.planOperation {
                Label(plan.status, systemImage: plan.phase == .uncertain ? "exclamationmark.triangle" : "list.bullet.rectangle")
                    .font(.caption).textSelection(.enabled)
                if plan.phase == .uncertain {
                    DisclosureGroup("Retained plan authorization identity") {
                        Text(verbatim: "Request: " + plan.id + "\nTask: " + plan.identity.taskID + "\nRevision: \(plan.identity.revision)\n" + plan.error).textSelection(.enabled)
                        Text("Nothing is automatically retried. Quit/reopen or reconnect the original owner, then inspect the canonical plan before a new explicit decision.")
                    }.font(.caption)
                }
            }
            if let status = sessions.correctionStatus { Text(verbatim: status).font(.caption).textSelection(.enabled).accessibilityLabel("Correction status: " + status) }
            if let submission = application.submission, ![.completed, .accepted].contains(submission.phase) {
                Text((submission.planFirst == true ? "Plan preparation request: " : "Request: ") + submission.phase.rawValue).font(.caption).foregroundStyle(.secondary)
            }
            if let submitted = application.submission, ![.completed, .accepted].contains(submitted.phase), !(submitted.uploads ?? []).isEmpty {
                DisclosureGroup("Request attachments: \(submitted.uploads!.count) · \(submitted.phase.rawValue)") {
                    ForEach(submitted.uploads ?? []) { file in Text(verbatim: file.name).textSelection(.enabled) }
                    Text("Go admission does not establish that a model read every attachment.").font(.caption)
                }.font(.caption)
            }
            if let pending = application.pendingSubmission, pending.phase == .uncertain {
                DisclosureGroup("Original submission has an unconfirmed outcome") {
                    Text(verbatim: pending.text).font(.caption).textSelection(.enabled)
                    Text(verbatim: "Request ID: " + pending.id).font(.caption2).textSelection(.enabled)
                    Button("Retry Original Admission") { let owner = application; Task { await sessions.retryAdmission(from: owner) } }.disabled(!application.canRetryAdmission || sessions.switching)
                    Text("Explicit retry uses the same admission ID. It does not repeat an external action.").font(.caption)
                }.font(.caption)
            }
            if application.snapshot?.value["dialogue_busy"].bool == true { Label("Considering your message…", systemImage: "clock").font(.caption) }
            if !sessions.canSubmit, application.canSend, sessions.workingCount >= 3 { Text("Three tasks are working. Wait for conversation readiness or stop a task before starting more work.").font(.caption).foregroundStyle(.secondary) }
            if sessions.switching { Text("Opening chat… Original work continues.").font(.caption) }
            if !sessions.issue.isEmpty { NativeErrorDisclosure(text: sessions.issue) }
            if !application.issue.isEmpty { NativeErrorDisclosure(text: application.issue) }
            if !application.persistenceIssue.isEmpty { NativeErrorDisclosure(text: application.persistenceIssue) }
            if sessions.inspection == nil, application.review == nil, application.recovery?.unresolved == true || application.receiptOperation?.unresolved == true {
                Button("Inspect Recovery Evidence") { sessions.inspectTask(application.conversationID) }.help("Inspect retained evidence · ⌘I")
                    .accessibilityLabel("Inspect recovery evidence for the selected conversation")
            }
            if application.connectionStatus == .disconnected { Button("Reconnect and Inspect") { Task { await sessions.reconnect() } } }
            if application.snapshot?.running == true && application.review == nil {
                Text(application.snapshot?.value["dialogue_active"].bool == true ? "Work continues independently. You can keep talking here; Gator decides how to route your message." : "Preparing conversation during work… Your draft is kept.").font(.caption).foregroundStyle(.secondary)
            }
            if sessions.inspection == nil, application.review == nil, let notice = application.snapshot?.unsupportedNotice, application.snapshot?.value["dialogue_dispatch"] == .null, application.snapshot?.value["session_request"] == .null { NativeStatusMessage(text: notice, severity: .information).font(.caption) }
            if application.readiness["state"].string != "configured", application.isConnected {
                NativeStatusMessage(text: "\(application.readiness["issue"].string) Provider setup is available in Gator’s terminal app; your draft stays here.").font(.caption)
            }
        }.popover(isPresented: $connectionDetails, arrowEdge: .bottom) {
            VStack(alignment: .leading, spacing: 12) {
                Text("Connection & Model Details").font(.headline)
                Text(application.isConnected ? "Connected to Gator. Provider connectivity is verified only by an actual model request." : "Gator is disconnected. Saved drafts and original request identities are retained.")
                Text(verbatim: application.modelLabel)
                Text(verbatim: application.readiness["processing"]["description"].string)
                if let notice = application.snapshot?.unsupportedNotice { Text(verbatim: notice) }
            }.textSelection(.enabled).padding(16).frame(width: 340)

        }.frame(maxWidth: .infinity, alignment: .leading)
    }
}

private struct MessageRow: View, Equatable {
    @Environment(ManagedSessionCoordinator.self) private var sessions
    let message: BridgeMessage
    nonisolated static func == (lhs: Self, rhs: Self) -> Bool { lhs.message == rhs.message }
    var body: some View {
        HStack(alignment: .top) {
            if message.isUser { Spacer(minLength: 40) }
            VStack(alignment: .leading, spacing: 6) {
                if !message.text.isEmpty { Text(message.isUser ? "You" : "Gator").font(.caption.weight(.medium)).foregroundStyle(.secondary) }
                let inputs = message.value["inputs"].array.map(\.string) + message.value["input_ids"].array.map { ($0.string as NSString).lastPathComponent }
                if !inputs.isEmpty { ForEach(Array(Set(inputs)).sorted(), id: \.self) { name in Label { Text(verbatim: name) } icon: { Image(systemName: "paperclip") }.font(.caption) } }
                if message.isStructured {
                    DisclosureGroup(message.result["title"].string.isEmpty ? "Result details" : message.result["title"].string) {
                        Text(verbatim: message.result["summary"].string)
                        Text(verbatim: message.result.pretty).font(.system(.caption, design: .monospaced))
                    }.font(.callout).foregroundStyle(.secondary)
                    if validTaskLink {
                        Button("Open Original Task", systemImage: "arrow.up.forward.square") {
                            let id = message.result["preview"]["task_chat"].string
                            Task { await sessions.openConversation(id) }
                        }.disabled(sessions.switching)
                    }
                } else if message.isUser || message.text.hasPrefix("/") {
                    Text(verbatim: message.text)
                } else if message.text.isEmpty {
                    DisclosureGroup(message.value["bundle"] != .null ? "Retained work result" : "Retained work activity") {
                        Text(verbatim: message.value.pretty).font(.system(.caption, design: .monospaced))
                    }
                } else {
                    NativeMessageBody(text: message.text)
                }
            }
            .textSelection(.enabled).padding(message.isUser ? 12 : 4)
            .background(message.isUser ? gatorAccent.opacity(0.10) : .clear, in: RoundedRectangle(cornerRadius: 12))
            if !message.isUser { Spacer(minLength: 20) }
        }.frame(maxWidth: .infinity)
    }
    private var validTaskLink: Bool {
        let id = message.result["preview"]["task_chat"].string
        return !id.isEmpty && sessions.current.snapshot?.value["dialogue"].array.contains(where: { $0["task_chat"].string == id }) == true
    }
}

private struct NativeTranscriptView: View {
    @Environment(GatorApplicationState.self) private var application
    @Environment(ManagedSessionCoordinator.self) private var sessions
    @Environment(NativeReadingPositions.self) private var positions
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.controlActiveState) private var activeState
    let compact: Bool
    @State private var following = true
    @State private var nearBottom = true
    @State private var userScrolling = false
    @State private var visibleMessage: String?
    @State private var activityExpanded = false
    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 16) {
                    if application.transcript.isEmpty {
                        VStack(alignment: .leading, spacing: 8) {
                            Text("What can I help with?").font(.title2.weight(.medium))
                            Text("Ask a question, work through an idea, or get something done.").foregroundStyle(.secondary)
                        }.padding(.vertical, 24)
                    }
                    if application.snapshot?.value["dialogue_more"].bool == true {
                        Text("Earlier conversation history is available in the terminal app.").font(.caption).foregroundStyle(.secondary)
                    }
                    ForEach(application.transcript.filter(\.hasVisibleContent)) { message in MessageRow(message: message).equatable() }
                    if application.snapshot?.running == true {
                        VStack(alignment: .leading, spacing: 8) {
                            HStack { if reduceMotion { Image(systemName: "clock").accessibilityLabel(NativeInteractionSemantics.workingLabel(reduceMotion: true)) } else { ProgressView().controlSize(.small) }; Text(application.snapshot?.value["status"].string ?? "Working…").font(.callout).foregroundStyle(.secondary) }
                            ForEach(Array((application.snapshot?.value["activity"]["entries"].array ?? []).enumerated()), id: \.offset) { _, entry in
                                // Public assistant stream uses the same bounded native Markdown
                                // presentation as retained prose, including incomplete code fences.
                                // Structured tool activity and exact reviews remain literal.
                                if entry["kind"].string == "text" { NativeMessageBody(text: entry["text"].string) }
                            }
                            DisclosureGroup("Work activity", isExpanded: $activityExpanded) {
                                ForEach(Array((application.snapshot?.value["activity"]["entries"].array ?? []).filter { $0["kind"].string == "tool" }.suffix(8).enumerated()), id: \.offset) { _, entry in
                                    Text(verbatim: entry["tool"].string + " · " + entry["state"].string).textSelection(.enabled)
                                }
                                Button("Inspect Current Task") { sessions.inspectTask(application.conversationID) }
                                Text("Public task status and retained evidence are available in the inspector.")
                            }.font(.caption).foregroundStyle(.secondary)
                        }
                    }
                    Color.clear.frame(height: 1).id("latest")
                }.scrollTargetLayout().padding(compact ? 16 : 24).frame(maxWidth: 780, alignment: .leading).frame(maxWidth: .infinity)
            }
            .defaultScrollAnchor(.bottom)
            .scrollPosition(id: $visibleMessage, anchor: .top)
            .onScrollPhaseChange { _, new in
                if new == .interacting || new == .decelerating || new == .tracking { userScrolling = true }
                if new == .idle, userScrolling {
                    following = nearBottom
                    positions.remember(application.context, anchor: visibleMessage, following: following)
                    userScrolling = false
                }
            }
            .onScrollGeometryChange(for: Bool.self) { geometry in geometry.contentSize.height - geometry.visibleRect.maxY < 80 } action: { _, near in
                nearBottom = near
                if userScrolling { following = near }
            }
            .onChange(of: application.transcript) { _, _ in
                guard !userScrolling else { return }
                if following { proxy.scrollTo("latest", anchor: .bottom) }
                else if let visibleMessage, !application.transcript.contains(where: { $0.id == visibleMessage }) { restore(proxy) }
            }
            .onChange(of: application.snapshot?.value["activity"]) { _, _ in if following && !userScrolling { proxy.scrollTo("latest", anchor: .bottom) } }
            .task {
                await Task.yield()
                restore(proxy)
            }
            .onChange(of: activeState) { _, new in if new == .key && !userScrolling { restore(proxy) } }
            .overlay(alignment: .bottom) {
                if !following && !nearBottom {
                    Button("Jump to Latest", systemImage: "arrow.down") { following = true; positions.remember(application.context, anchor: nil, following: true); proxy.scrollTo("latest", anchor: .bottom) }
                        .buttonStyle(.bordered).background(.regularMaterial, in: Capsule()).padding(8)
                }
            }
        }
    }

    private func restore(_ proxy: ScrollViewProxy) {
        let saved = positions.restored(application.context, available: application.transcript.filter(\.hasVisibleContent).map(\.id))
        following = saved.following
        visibleMessage = saved.anchor
        if saved.following { proxy.scrollTo("latest", anchor: .bottom) }
        else if let anchor = saved.anchor { proxy.scrollTo(anchor, anchor: .top) }
    }
}
