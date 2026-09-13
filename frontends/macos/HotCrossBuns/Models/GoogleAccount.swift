import Foundation

struct GoogleAccount: Identifiable, Hashable, Codable, Sendable {
    let id: String
    var email: String
    var displayName: String
    var grantedScopes: Set<String>
    var authProvider: GoogleAuthProvider

    static let preview = GoogleAccount(
        id: "preview-account",
        email: "personal@example.com",
        displayName: "Personal Workspace",
        grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
        authProvider: .embeddedGoogleSignIn
    )

    init(
        id: String,
        email: String,
        displayName: String,
        grantedScopes: Set<String>,
        authProvider: GoogleAuthProvider = .embeddedGoogleSignIn
    ) {
        self.id = id
        self.email = email
        self.displayName = displayName
        self.grantedScopes = grantedScopes
        self.authProvider = authProvider
    }

    enum CodingKeys: String, CodingKey {
        case id
        case email
        case displayName
        case grantedScopes
        case authProvider
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        email = try container.decode(String.self, forKey: .email)
        displayName = try container.decode(String.self, forKey: .displayName)
        grantedScopes = try container.decode(Set<String>.self, forKey: .grantedScopes)
        authProvider = try container.decodeIfPresent(GoogleAuthProvider.self, forKey: .authProvider) ?? .embeddedGoogleSignIn
    }
}

enum GoogleScope {
    static let openID = "openid"
    static let email = "email"
    static let profile = "profile"
    static let tasks = "https://www.googleapis.com/auth/tasks"
    static let calendar = "https://www.googleapis.com/auth/calendar"
}

enum GoogleAuthProvider: String, Codable, Hashable, Sendable {
    case embeddedGoogleSignIn
    case customDesktopOAuth

    var title: String {
        switch self {
        case .embeddedGoogleSignIn:
            "Embedded Google Sign-In"
        case .customDesktopOAuth:
            "Custom desktop OAuth"
        }
    }
}

enum AuthState: Equatable, Sendable {
    case signedOut
    case authenticating
    case signedIn(GoogleAccount)
    case cancelled(String)
    case failed(String)

    var title: String {
        switch self {
        case .signedOut:
            "Not connected"
        case .authenticating:
            "Connecting"
        case .signedIn(let account):
            account.email
        case .cancelled:
            "Not connected"
        case .failed:
            "Connection failed"
        }
    }
}
