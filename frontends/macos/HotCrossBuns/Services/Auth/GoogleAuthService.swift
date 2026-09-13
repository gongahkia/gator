import AppKit
import Foundation
import GoogleSignIn
import Observation

@MainActor
@Observable
final class GoogleAuthService {
    private let bundle: Bundle
    private let customOAuthService: CustomGoogleOAuthService
    private let requiredScopes = [GoogleScope.tasks, GoogleScope.calendar]

    init(bundle: Bundle = .main, customOAuthService: CustomGoogleOAuthService = CustomGoogleOAuthService()) {
        self.bundle = bundle
        self.customOAuthService = customOAuthService
    }

    var isConfigured: Bool {
        Self.isConfigured(clientID: clientID) || customOAuthService.isConfigured
    }

    var customOAuthClientConfiguration: GoogleOAuthClientConfiguration? {
        customOAuthService.clientConfiguration
    }

    func setActiveAccountID(_ accountID: GoogleAccount.ID?) {
        customOAuthService.setActiveAccountID(accountID)
    }

    func saveCustomOAuthClientConfiguration(clientID: String, clientSecret: String?) throws -> GoogleOAuthClientConfiguration {
        try customOAuthService.saveClientConfiguration(
            GoogleOAuthClientConfiguration(clientID: clientID, clientSecret: clientSecret)
        )
    }

    func clearCustomOAuthClientConfiguration() {
        customOAuthService.clearClientConfiguration()
    }

    func restorePreviousSignIn() async throws -> GoogleAccount? {
        guard isConfigured else {
            AppLogger.warn("restore skipped: OAuth not configured", category: .auth)
            return nil
        }

        if customOAuthService.isConfigured,
           let account = try await customOAuthService.restorePreviousSignIn() {
            AppLogger.info("restore custom OAuth: success", category: .auth, metadata: ["email": Self.redact(account.email)])
            return account
        }

        guard Self.isConfigured(clientID: clientID) else {
            AppLogger.info("restore skipped: embedded OAuth not configured", category: .auth)
            return nil
        }

        guard GIDSignIn.sharedInstance.hasPreviousSignIn() else {
            AppLogger.info("restore: no previous session", category: .auth)
            return nil
        }

        do {
            let user = try await GIDSignIn.sharedInstance.restorePreviousSignIn()
            let missingScopes = Self.missingScopes(requiredScopes: requiredScopes, grantedScopes: user.grantedScopes)
            guard missingScopes.isEmpty else {
                AppLogger.warn("restore: scopes revoked, forcing sign-out", category: .auth, metadata: ["missing": missingScopes.count.description])
                GIDSignIn.sharedInstance.signOut()
                return nil
            }
            let acc = try account(from: user)
            AppLogger.info("restore: success", category: .auth, metadata: ["email": Self.redact(acc.email)])
            return acc
        } catch {
            AppLogger.error("restore failed", category: .auth, metadata: ["error": String(describing: error)])
            throw error
        }
    }

    static func isConfigured(clientID: String?) -> Bool {
        let placeholderClientID = "your-macos-oauth-client-id.apps.googleusercontent.com"
        guard let clientID else {
            return false
        }
        let trimmed = clientID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return false }
        guard trimmed.hasPrefix("$(") == false else { return false }
        guard trimmed != placeholderClientID else { return false }
        return true
    }

    static func missingScopes(requiredScopes: [String], grantedScopes: [String]?) -> [String] {
        requiredScopes.filter { scope in
            grantedScopes?.contains(scope) != true
        }
    }

    static func redact(_ email: String) -> String {
        guard let at = email.firstIndex(of: "@") else { return "<redacted>" }
        let local = email[..<at]
        let domain = email[email.index(after: at)...]
        let prefix = local.prefix(2)
        return "\(prefix)***@\(domain)"
    }

    func signIn() async throws -> GoogleAccount {
        guard isConfigured else {
            AppLogger.error("signIn: OAuth not configured", category: .auth)
            throw GoogleAuthError.notConfigured
        }

        if customOAuthService.isConfigured {
            return try await customOAuthService.signIn()
        }

        if let currentUser = GIDSignIn.sharedInstance.currentUser {
            AppLogger.info("signIn: reusing current user", category: .auth)
            return try await accountWithRequiredScopes(for: currentUser)
        }

        AppLogger.info("signIn: presenting Google sheet", category: .auth)
        // Smoke-test the Keychain just before the Google flow so if the
        // SDK later fails with -2 "keychain error" we have a definitive
        // reading of whether the process can Keychain at all. Logs the
        // OSStatus of add/read/delete for every sign-in attempt.
        KeychainProbe.runWriteProbe()
        let anchor = try currentPresentationAnchor()
        do {
            let result = try await GIDSignIn.sharedInstance.signIn(
                withPresenting: anchor,
                hint: nil,
                additionalScopes: requiredScopes
            )
            let acc = try account(from: result.user)
            AppLogger.info("signIn: success", category: .auth, metadata: ["email": Self.redact(acc.email)])
            return acc
        } catch {
            AppLogger.error("signIn failed", category: .auth, metadata: Self.errorMetadata(error))
            throw error
        }
    }

    // GIDSignIn collapses the real Keychain failure (OSStatus + GTMAppAuth
    // domain) behind a single Code=-2 "keychain error" NSError. Walk the
    // NSUnderlyingError chain so we can see what actually failed.
    static func errorMetadata(_ error: Error) -> [String: String] {
        var metadata: [String: String] = ["error": String(describing: error)]
        let ns = error as NSError
        metadata["domain"] = ns.domain
        metadata["code"] = String(ns.code)
        metadata["userInfoKeys"] = ns.userInfo.keys.sorted().joined(separator: ",")
        var depth = 0
        var current: NSError? = ns.userInfo[NSUnderlyingErrorKey] as? NSError
        while let underlying = current, depth < 4 {
            metadata["underlying\(depth).domain"] = underlying.domain
            metadata["underlying\(depth).code"] = String(underlying.code)
            metadata["underlying\(depth).description"] = underlying.localizedDescription
            metadata["underlying\(depth).userInfoKeys"] = underlying.userInfo.keys.sorted().joined(separator: ",")
            current = underlying.userInfo[NSUnderlyingErrorKey] as? NSError
            depth += 1
        }
        return metadata
    }

    static func isUserCancellation(_ error: Error) -> Bool {
        let ns = error as NSError
        return ns.domain == kGIDSignInErrorDomain && ns.code == GIDSignInError.canceled.rawValue
    }

    func signOut(accountID: GoogleAccount.ID? = nil) {
        if let accountID {
            customOAuthService.clearTokenSet(accountID: accountID)
        } else {
            customOAuthService.clearTokenSet()
        }
        GIDSignIn.sharedInstance.signOut()
    }

    func disconnect(accountID: GoogleAccount.ID? = nil) async throws {
        if customOAuthService.isConfigured {
            if let accountID {
                customOAuthService.clearTokenSet(accountID: accountID)
            } else {
                customOAuthService.clearTokenSet()
            }
            return
        }

        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
            GIDSignIn.sharedInstance.disconnect { error in
                if let error {
                    continuation.resume(throwing: error)
                } else {
                    continuation.resume(returning: ())
                }
            }
        }
    }

    func handleRedirectURL(_ url: URL) -> Bool {
        GIDSignIn.sharedInstance.handle(url)
    }

    private var clientID: String? {
        bundle.object(forInfoDictionaryKey: "GIDClientID") as? String
    }

    private func accountWithRequiredScopes(for user: GIDGoogleUser) async throws -> GoogleAccount {
        let missingScopes = Self.missingScopes(requiredScopes: requiredScopes, grantedScopes: user.grantedScopes)

        guard missingScopes.isEmpty == false else {
            return try account(from: user)
        }

        let result = try await addScopes(missingScopes, to: user)
        return try account(from: result.user)
    }

    private func addScopes(_ scopes: [String], to user: GIDGoogleUser) async throws -> GIDSignInResult {
        let anchor = try currentPresentationAnchor()
        return try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<GIDSignInResult, Error>) in
            user.addScopes(scopes, presenting: anchor) { result, error in
                if let result {
                    continuation.resume(returning: result)
                } else {
                    continuation.resume(throwing: error ?? GoogleAuthError.emptySignInResult)
                }
            }
        }
    }

    private func account(from user: GIDGoogleUser) throws -> GoogleAccount {
        try Self.buildAccount(
            userID: user.userID,
            email: user.profile?.email,
            displayName: user.profile?.name,
            grantedScopes: user.grantedScopes
        )
    }

    private func currentPresentationAnchor() throws -> NSWindow {
        try Self.resolvePresentationAnchor(
            keyWindow: NSApplication.shared.keyWindow,
            mainWindow: NSApplication.shared.mainWindow,
            windows: NSApplication.shared.windows
        )
    }

    static func buildAccount(
        userID: String?,
        email: String?,
        displayName: String?,
        grantedScopes: [String]?
    ) throws -> GoogleAccount {
        guard let email, email.isEmpty == false else {
            throw GoogleAuthError.missingProfile
        }

        return GoogleAccount(
            id: userID ?? email,
            email: email,
            displayName: displayName ?? email,
            grantedScopes: Set(grantedScopes ?? []),
            authProvider: .embeddedGoogleSignIn
        )
    }

    static func resolvePresentationAnchor(
        keyWindow: NSWindow?,
        mainWindow: NSWindow?,
        windows: [NSWindow]
    ) throws -> NSWindow {
        if let keyWindow {
            return keyWindow
        }

        if let mainWindow {
            return mainWindow
        }

        if let visibleWindow = windows.first(where: { $0.isVisible }) {
            return visibleWindow
        }

        throw GoogleAuthError.noPresentationAnchor
    }
}

enum GoogleAuthError: LocalizedError, Equatable {
    case notConfigured
    case noPresentationAnchor
    case noCurrentUser
    case missingProfile
    case emptySignInResult

    var errorDescription: String? {
        switch self {
        case .notConfigured:
            "This build of Hot Cross Buns isn't configured for Google sign-in. Build it with your own Google Cloud OAuth client, then try connecting again."
        case .noPresentationAnchor:
            "Couldn't find a window to open the Google sign-in sheet. Focus Hot Cross Buns and try again."
        case .noCurrentUser:
            "You're not signed in to Google yet."
        case .missingProfile:
            "Google sign-in didn't return a profile for that account. Try signing in again."
        case .emptySignInResult:
            "The Google sign-in sheet closed without finishing. Try again."
        }
    }
}

enum GoogleTokenRefreshError: LocalizedError {
    case noCurrentUser
    case refreshFailed(Error)

    var errorDescription: String? {
        switch self {
        case .noCurrentUser:
            "You're not signed in to Google yet. Open Settings and tap Connect Google."
        case .refreshFailed(let underlying):
            "Your Google session expired (\(underlying.localizedDescription)). Open Settings and tap Reconnect Google."
        }
    }

    // Token refresh failures mean the user needs to re-authenticate — surface
    // to the reconnect CTA rather than the generic sync banner.
    var requiresReconnect: Bool {
        true
    }
}

struct GoogleSignInAccessTokenProvider: AccessTokenProviding {
    private let customOAuthService: CustomGoogleOAuthService
    private let tokenCache = GoogleAccessTokenMemo(ttl: 30)

    init(customOAuthService: CustomGoogleOAuthService = CustomGoogleOAuthService()) {
        self.customOAuthService = customOAuthService
    }

    @MainActor
    func accessToken() async throws -> String {
        let cacheKey = customOAuthService.activeAccountID.map { "custom:\($0)" } ?? "embedded"
        if let cached = await tokenCache.validToken(for: cacheKey) {
            return cached
        }

        let token = try await freshAccessToken()
        await tokenCache.store(token, for: cacheKey)
        return token
    }

    @MainActor
    private func freshAccessToken() async throws -> String {
        do {
            if let customToken = try await customOAuthService.accessToken() {
                return customToken
            }
        } catch {
            throw GoogleTokenRefreshError.refreshFailed(error)
        }

        guard let user = GIDSignIn.sharedInstance.currentUser else {
            AppLogger.warn("accessToken: no current user", category: .auth)
            throw GoogleTokenRefreshError.noCurrentUser
        }

        do {
            let refreshedUser = try await user.refreshTokensIfNeeded()
            return refreshedUser.accessToken.tokenString
        } catch {
            AppLogger.error("token refresh failed", category: .auth, metadata: ["error": String(describing: error)])
            throw GoogleTokenRefreshError.refreshFailed(error)
        }
    }
}

private actor GoogleAccessTokenMemo {
    private let ttl: TimeInterval
    private var tokensByKey: [String: (token: String, expiresAt: Date)] = [:]

    init(ttl: TimeInterval) {
        self.ttl = ttl
    }

    func validToken(for key: String, now: Date = Date()) -> String? {
        guard let cached = tokensByKey[key], cached.expiresAt > now else {
            return nil
        }
        return cached.token
    }

    func store(_ token: String, for key: String, now: Date = Date()) {
        tokensByKey[key] = (token, now.addingTimeInterval(ttl))
    }
}
