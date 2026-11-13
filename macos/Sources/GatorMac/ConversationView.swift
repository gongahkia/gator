import SwiftUI
import AppKit
import GatorCore

private let gatorAccent = Color(red: 0.18, green: 0.48, blue: 0.37)

struct ConversationView: View {
    @Environment(ManagedSessionCoordinator.self) private var sessions
    private var application: GatorApplicationState { sessions.current }
    @Environment(\.openWindow) private var openWindow
    let compact: Bool
    let quit: () -> Void
    @State private var following = true
    @State private var nearBottom = true
    @State private var phase = ScrollPhase.idle
    @State private var activityExpanded = false
    @State private var sessionsExpanded = false

    var body: some View {
        VStack(spacing: 0) {
            header.padding(.horizontal, 16).padding(.vertical, 12)
            Divider()
            if sessions.activity.count > 1 { sessionActivity.padding(.horizontal, 14).padding(.vertical, 6) }
            if let review = application.review {
                ApprovalReviewView(review: review, compact: compact).padding(12)
                Spacer(minLength: 0)
            } else {
                transcript
            }
            notices.padding(.horizontal, 14)
            if application.snapshot?.hasApproval == true && application.review == nil {
                Button { application.openApprovalReview() } label: { Label("An action needs your review", systemImage: "hand.raised").frame(maxWidth: .infinity) }
                    .buttonStyle(.bordered).padding(.horizontal, 14).padding(.top, 6)
            }
            if application.review == nil { ComposerView(compact: compact).padding(12) }
        }
        .background(Color(nsColor: .windowBackgroundColor))
        .environment(application)
        .tint(gatorAccent)
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
            if let parent = application.snapshot?.value["dialogue_task"]["parent_id"].string, !parent.isEmpty {
                Button { Task { await sessions.openConversation(parent) } } label: { Image(systemName: "arrow.uturn.backward").frame(width: 24, height: 26) }
                    .disabled(sessions.switching).help("Return to the requesting chat").accessibilityLabel("Return to Requesting Chat")
            }
            if application.canStop || application.stopping || application.snapshot?.cancelling == true {
                Button { let owner = application; Task { await owner.stopWork() } } label: { Label(application.stopping || application.snapshot?.cancelling == true ? "Stopping…" : "Stop", systemImage: "stop.fill") }
                    .disabled(sessions.switching || !application.canStop).help("Stop current Go-owned work and retain its evidence")
            }
            Menu {
                Button("New Conversation", systemImage: "square.and.pencil") { Task { await sessions.newConversation() } }.disabled(!sessions.canNavigate)
                Section("Recent Conversations") {
                    ForEach(application.snapshot?.value["recent_conversations"].array ?? [], id: \.self) { item in
                        Button(item["title"].string.isEmpty ? item["id"].string : item["title"].string) { Task { await sessions.openConversation(item["id"].string) } }
                            .disabled(!sessions.canNavigate)
                    }
                    if application.snapshot?.value["recent_conversations"].array.isEmpty != false { Text("No recent conversations") }
                }
                Section("Configured Models") {
                    ForEach(application.composerModels, id: \.self) { model in
                        Button(model["label"].string.isEmpty ? model["model"].string : model["label"].string) { let owner = application; Task { await owner.selectModel(provider: model["provider"].string, model: model["model"].string) } }
                            .disabled(sessions.switching || !application.canConfigureModel || !model["unavailable"].string.isEmpty)
                    }
                }
                Section("Effort") {
                    ForEach(["low", "standard", "high"], id: \.self) { effort in
                        Button(effort.capitalized) { let owner = application; Task { await owner.selectEffort(effort) } }.disabled(sessions.switching || !application.canConfigureModel)
                    }
                }
                Divider()
                Button("Reconnect and Inspect") { Task { await sessions.reconnect() } }.disabled(application.connectionStatus != .disconnected)
                Button("Quit Gator", role: .destructive, action: quit).keyboardShortcut("q")
            } label: { Image(systemName: "ellipsis").frame(width: 28, height: 28) }
            .menuStyle(.borderlessButton).fixedSize().help("Conversations, models and Gator controls")
            if compact {
                Button { openWindow(id: "conversation"); NSApp.activate(ignoringOtherApps: true) } label: { Image(systemName: "arrow.up.left.and.arrow.down.right").frame(width: 28, height: 28) }
                    .buttonStyle(.plain).help("Expand this conversation").accessibilityLabel("Open full Gator window")
            }
        }
    }

    private var sessionActivity: some View {
        DisclosureGroup(isExpanded: $sessionsExpanded) {
            ScrollView {
                VStack(spacing: 4) {
                    ForEach(sessions.activity) { item in
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
                    }
                }
            }.frame(maxHeight: compact ? 130 : 170)
        } label: {
            let others = sessions.activity.filter { $0.id != application.conversationID }
            Label(others.contains(where: { $0.status == "Working" || $0.status == "Stopping" }) ? "Work continues in another chat" : others.contains(where: \.needsAttention) ? "Another chat needs attention" : "Open chats and retained tasks", systemImage: "bubble.left.and.bubble.right")
        }.font(.caption).help("Inspect or return to an original chat without moving its work")
    }

    private var transcript: some View {
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
                            HStack { ProgressView().controlSize(.small); Text(application.snapshot?.value["status"].string ?? "Working…").font(.callout).foregroundStyle(.secondary) }
                            ForEach(Array((application.snapshot?.value["activity"]["entries"].array ?? []).enumerated()), id: \.offset) { _, entry in
                                if entry["kind"].string == "text" { Text(verbatim: entry["text"].string).textSelection(.enabled) }
                            }
                            DisclosureGroup("Work activity", isExpanded: $activityExpanded) {
                                Text(verbatim: application.snapshot?.value["activity"].pretty ?? "").font(.system(.caption, design: .monospaced)).textSelection(.enabled)
                            }.font(.caption).foregroundStyle(.secondary)
                        }
                    }
                    Color.clear.frame(height: 1).id("latest")
                }.padding(compact ? 16 : 24).frame(maxWidth: 780, alignment: .leading).frame(maxWidth: .infinity)
            }
            .defaultScrollAnchor(.bottom)
            .onScrollPhaseChange { _, new in phase = new }
            .onScrollGeometryChange(for: Bool.self) { geometry in geometry.contentSize.height - geometry.visibleRect.maxY < 80 } action: { _, near in
                nearBottom = near
                if phase == .interacting || phase == .decelerating { following = near }
            }
            .onChange(of: application.transcript) { _, _ in if following { proxy.scrollTo("latest", anchor: .bottom) } }
            .onChange(of: application.snapshot?.value["activity"]) { _, _ in if following { proxy.scrollTo("latest", anchor: .bottom) } }
            .onChange(of: application.context) { _, _ in following = true; proxy.scrollTo("latest", anchor: .bottom) }
            .overlay(alignment: .bottom) {
                if !following && !nearBottom {
                    Button("Jump to Latest", systemImage: "arrow.down") { following = true; proxy.scrollTo("latest", anchor: .bottom) }
                        .buttonStyle(.bordered).background(.regularMaterial, in: Capsule()).padding(8)
                }
            }
        }
    }
    @ViewBuilder private var notices: some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack(spacing: 6) {
                Circle().fill(application.isConnected ? gatorAccent : .secondary).frame(width: 6, height: 6)
                Text(application.isConnected ? "Engine connected" : application.connectionStatus.rawValue.capitalized)
                Spacer()
                Text(application.modelLabel).lineLimit(1)
            }.font(.caption).foregroundStyle(.secondary)
            if let submission = application.submission {
                Text("Request: \(submission.phase.rawValue)").font(.caption).foregroundStyle(.secondary)
            }
            if let pending = application.pendingSubmission, pending.phase == .uncertain {
                DisclosureGroup("Original submission has an unconfirmed outcome") {
                    Text(verbatim: pending.text).font(.caption).textSelection(.enabled)
                    Text(verbatim: "Request ID: " + pending.id).font(.caption2).textSelection(.enabled)
                    Button("Retry Original Admission") { let owner = application; Task { await sessions.retryAdmission(from: owner) } }.disabled(!application.canRetryAdmission || sessions.switching)
                    Text("Explicit retry uses the same admission ID. It does not repeat an external action.").font(.caption)
                }.font(.caption)
            }
            if application.snapshot?.value["dialogue_busy"].bool == true { ProgressView("Considering your message…").controlSize(.small).font(.caption) }
            if !sessions.canSubmit, application.canSend, sessions.workingCount >= 3 { Text("Three tasks are working. Wait for conversation readiness or stop a task before starting more work.").font(.caption).foregroundStyle(.secondary) }
            if sessions.switching { Text("Opening chat… Original work continues.").font(.caption) }
            if !sessions.issue.isEmpty { Text(verbatim: sessions.issue).font(.caption).foregroundStyle(.orange).textSelection(.enabled) }
            if !application.issue.isEmpty { Text(verbatim: application.issue).font(.caption).foregroundStyle(.orange).textSelection(.enabled).lineLimit(3) }
            if !application.persistenceIssue.isEmpty { Text(verbatim: application.persistenceIssue).font(.caption).foregroundStyle(.red).textSelection(.enabled) }
            if application.connectionStatus == .disconnected { Button("Reconnect and Inspect") { Task { await sessions.reconnect() } } }
            if application.snapshot?.running == true && application.review == nil {
                Text(application.snapshot?.value["dialogue_active"].bool == true ? "Work continues independently. You can keep talking here; Gator decides how to route your message." : "Preparing conversation during work… Your draft is kept.").font(.caption).foregroundStyle(.secondary)
            }
            if let notice = application.snapshot?.unsupportedNotice, application.snapshot?.value["dialogue_dispatch"] == .null, application.snapshot?.value["session_request"] == .null { Text(verbatim: notice).font(.caption).foregroundStyle(.secondary).lineLimit(3) }
            if application.readiness["state"].string != "configured", application.isConnected {
                Text("\(application.readiness["issue"].string) Provider setup is available in Gator’s terminal app; your draft stays here.").font(.caption).foregroundStyle(.secondary)
            } else if !application.readiness["processing"]["description"].string.isEmpty {
                DisclosureGroup("Model processing · configuration, not a connectivity test") {
                    Text(verbatim: application.readiness["processing"]["description"].string).textSelection(.enabled)
                }.font(.caption2).foregroundStyle(.secondary)
            }
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
                Text(message.isUser ? "You" : "Gator").font(.caption.weight(.medium)).foregroundStyle(.secondary)
                if message.isStructured {
                    Text(verbatim: message.result["title"].string).font(.headline)
                    Text(verbatim: message.result["summary"].string)
                    DisclosureGroup("Exact details") { Text(verbatim: message.result.pretty).font(.system(.caption, design: .monospaced)) }
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
                    Text((try? AttributedString(markdown: message.text, options: .init(interpretedSyntax: .inlineOnlyPreservingWhitespace))) ?? AttributedString(message.text))
                }
            }
            .textSelection(.enabled).padding(12)
            .background(message.isUser ? gatorAccent.opacity(0.10) : Color.primary.opacity(0.035), in: RoundedRectangle(cornerRadius: 14))
            if !message.isUser { Spacer(minLength: 20) }
        }.frame(maxWidth: .infinity)
    }
    private var validTaskLink: Bool {
        let id = message.result["preview"]["task_chat"].string
        return !id.isEmpty && sessions.current.snapshot?.value["dialogue"].array.contains(where: { $0["task_chat"].string == id }) == true
    }
}
