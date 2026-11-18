import SwiftUI
import GatorCore

struct NativeRecentConversation: Identifiable, Equatable {
    let id, title, status: String
    let current: Bool
    static func items(_ recent: [JSONValue], activity: [NativeChatActivity], current: String) -> [Self] {
        var seen: Set<String> = []
        return recent.compactMap { item in
            let id = item["id"].string
            guard !id.isEmpty, seen.insert(id).inserted else { return nil }
            let title = item["title"].string
            return Self(id: id, title: title.isEmpty ? id : title, status: activity.first { $0.id == id }?.status ?? "", current: id == current)
        }
    }
}
struct RecentConversationsMenu: View {
    @Environment(ManagedSessionCoordinator.self) private var sessions
    var body: some View {
        Menu {
            RecentConversationItems(sessions: sessions)
            Divider()
            AllConversationsButton(sessions: sessions)
        } label: { Label("Recent Conversations", systemImage: "clock") }
        .labelStyle(.titleAndIcon)
        .focusable()
        .help("Choose a recent conversation, or open All Conversations for the complete retained catalog.")
        .accessibilityIdentifier("gator.recent-conversations")
    }
}

struct RecentConversationItems: View {
    let sessions: ManagedSessionCoordinator
    var body: some View {
            let items = NativeRecentConversation.items(sessions.current.snapshot?.value["recent_conversations"].array ?? [], activity: sessions.activity, current: sessions.current.conversationID)
            ForEach(items) { item in
                Button {
                    guard !item.current else { return }
                    Task { await sessions.openConversation(item.id) }
                } label: {
                    Label(item.title + (item.status.isEmpty ? "" : " · " + item.status), systemImage: item.current ? "checkmark" : "bubble.left")
                }
                .help(item.title).disabled(!sessions.canNavigate)
                .accessibilityLabel((item.current ? "Current conversation: " : "Open conversation: ") + item.title + (item.status.isEmpty ? "" : ", " + item.status))
                .accessibilityIdentifier("gator.recent." + item.id)
            }
            if items.isEmpty { Text("No recent conversations") }
            Divider()
            Text("Recent conversations reported by Gator")
    }
}
