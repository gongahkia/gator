import AppKit
import CryptoKit
import Foundation
import Security

struct GoogleOAuthClientConfiguration: Codable, Hashable, Sendable {
    var clientID: String
    var clientSecret: String?

    var normalized: GoogleOAuthClientConfiguration {
        GoogleOAuthClientConfiguration(
            clientID: clientID.trimmingCharacters(in: .whitespacesAndNewlines),
            clientSecret: clientSecret?.trimmingCharacters(in: .whitespacesAndNewlines).nilIfEmpty
        )
    }

    var isValid: Bool {
        clientID.trimmingCharacters(in: .whitespacesAndNewlines).hasSuffix(".apps.googleusercontent.com")
    }

    var redactedClientID: String {
        let trimmed = clientID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.count > 18 else { return trimmed.isEmpty ? "Not configured" : "<configured>" }
        return "\(trimmed.prefix(8))...\(trimmed.suffix(18))"
    }
}

struct CustomGoogleOAuthTokenSet: Codable, Sendable, Equatable {
    var accessToken: String
    var refreshToken: String
    var expiresAt: Date
    var grantedScopes: Set<String>
    var account: GoogleAccount
    var idToken: String?

    var needsRefresh: Bool {
        expiresAt.timeIntervalSinceNow < 60
    }
}

protocol GoogleOAuthTokenStoring: Sendable {
    func loadClientConfiguration() -> GoogleOAuthClientConfiguration?
    func saveClientConfiguration(_ configuration: GoogleOAuthClientConfiguration) throws
    func clearClientConfiguration()
    func loadTokenSet(accountID: GoogleAccount.ID) -> CustomGoogleOAuthTokenSet?
    func loadFirstTokenSet() -> CustomGoogleOAuthTokenSet?
    func saveTokenSet(_ tokenSet: CustomGoogleOAuthTokenSet, accountID: GoogleAccount.ID) throws
    func deleteTokenSet(accountID: GoogleAccount.ID)
    func deleteAllTokenSets()
}

enum CustomGoogleOAuthError: LocalizedError, Equatable {
    case clientNotConfigured
    case invalidClientID
    case loopbackUnavailable
    case browserOpenFailed
    case timedOut
    case missingAuthorizationCode
    case stateMismatch
    case tokenExchangeFailed(String)
    case missingRefreshToken
    case missingAccountProfile
    case missingTokenSet

    var errorDescription: String? {
        switch self {
        case .clientNotConfigured:
            "Add your Google Cloud desktop OAuth client in Settings before connecting Google."
        case .invalidClientID:
            "That Google OAuth client ID does not look valid. Use the desktop client ID ending in .apps.googleusercontent.com."
        case .loopbackUnavailable:
            "Hot Cross Buns couldn't start the local OAuth callback listener. Check that localhost networking is available and try again."
        case .browserOpenFailed:
            "Hot Cross Buns couldn't open the Google consent page in your browser."
        case .timedOut:
            "Google sign-in timed out before the browser returned to Hot Cross Buns."
        case .missingAuthorizationCode:
            "Google returned to Hot Cross Buns without an authorization code."
        case .stateMismatch:
            "Google sign-in returned an unexpected state token. Try connecting again."
        case .tokenExchangeFailed(let message):
            "Google rejected the OAuth token exchange. \(message)"
        case .missingRefreshToken:
            "Google did not return a refresh token. Remove access for this OAuth client in your Google Account, then connect again."
        case .missingAccountProfile:
            "Google sign-in succeeded but did not return an account email. Try connecting again."
        case .missingTokenSet:
            "That Google account is no longer signed in on this Mac. Reconnect it from Settings."
        }
    }
}

final class CustomGoogleOAuthService: @unchecked Sendable {
    private let urlSession: URLSession
    private let tokenStore: GoogleOAuthTokenStoring
    private let activeAccountLock = NSLock()
    private var _activeAccountID: GoogleAccount.ID?
    private let callbackTimeoutNanoseconds: UInt64 = 120_000_000_000
    private let requiredScopes = [
        GoogleScope.openID,
        GoogleScope.email,
        GoogleScope.profile,
        GoogleScope.tasks,
        GoogleScope.calendar
    ]

    init(
        urlSession: URLSession = .shared,
        tokenStore: GoogleOAuthTokenStoring = GoogleOAuthKeychainStore()
    ) {
        self.urlSession = urlSession
        self.tokenStore = tokenStore
    }

    var activeAccountID: GoogleAccount.ID? {
        activeAccountLock.lock()
        defer { activeAccountLock.unlock() }
        return _activeAccountID
    }

    func setActiveAccountID(_ accountID: GoogleAccount.ID?) {
        activeAccountLock.lock()
        _activeAccountID = accountID
        activeAccountLock.unlock()
    }

    var clientConfiguration: GoogleOAuthClientConfiguration? {
        tokenStore.loadClientConfiguration()
    }

    var isConfigured: Bool {
        clientConfiguration?.normalized.isValid == true
    }

    func saveClientConfiguration(_ configuration: GoogleOAuthClientConfiguration) throws -> GoogleOAuthClientConfiguration {
        let normalized = configuration.normalized
        guard normalized.isValid else {
            throw CustomGoogleOAuthError.invalidClientID
        }
        try tokenStore.saveClientConfiguration(normalized)
        tokenStore.deleteAllTokenSets()
        setActiveAccountID(nil)
        return normalized
    }

    func clearClientConfiguration() {
        tokenStore.clearClientConfiguration()
        tokenStore.deleteAllTokenSets()
        setActiveAccountID(nil)
    }

    func clearTokenSet() {
        if let activeAccountID {
            clearTokenSet(accountID: activeAccountID)
        }
    }

    func clearTokenSet(accountID: GoogleAccount.ID) {
        tokenStore.deleteTokenSet(accountID: accountID)
        if activeAccountID == accountID {
            setActiveAccountID(nil)
        }
    }

    func clearAllTokenSets() {
        tokenStore.deleteAllTokenSets()
        setActiveAccountID(nil)
    }

    func restorePreviousSignIn() async throws -> GoogleAccount? {
        guard isConfigured else { return nil }
        guard var tokenSet = activeAccountID.flatMap({ tokenStore.loadTokenSet(accountID: $0) }) ?? tokenStore.loadFirstTokenSet() else {
            return nil
        }
        if tokenSet.needsRefresh {
            tokenSet = try await refresh(tokenSet: tokenSet)
        }
        setActiveAccountID(tokenSet.account.id)
        return tokenSet.account
    }

    @MainActor
    func signIn() async throws -> GoogleAccount {
        guard let config = clientConfiguration?.normalized else {
            throw CustomGoogleOAuthError.clientNotConfigured
        }
        guard config.isValid else {
            throw CustomGoogleOAuthError.invalidClientID
        }

        let server = OAuthLoopbackServer()
        let redirectURI = try server.start()
        defer { server.stop() }

        let state = Self.randomURLSafeString(byteCount: 32)
        let codeVerifier = Self.randomURLSafeString(byteCount: 64)
        let codeChallenge = Self.codeChallenge(for: codeVerifier)
        let authorizationURL = try authorizationURL(
            config: config,
            redirectURI: redirectURI,
            state: state,
            codeChallenge: codeChallenge
        )

        guard NSWorkspace.shared.open(authorizationURL) else {
            throw CustomGoogleOAuthError.browserOpenFailed
        }

        let callback = try await waitForCallback(from: server)
        guard callback.state == state else {
            throw CustomGoogleOAuthError.stateMismatch
        }
        guard let code = callback.code, code.isEmpty == false else {
            throw CustomGoogleOAuthError.missingAuthorizationCode
        }

        let response = try await exchangeAuthorizationCode(
            code,
            redirectURI: redirectURI.absoluteString,
            codeVerifier: codeVerifier,
            config: config
        )
        guard let refreshToken = response.refreshToken?.nilIfEmpty else {
            throw CustomGoogleOAuthError.missingRefreshToken
        }
        let account = try await account(from: response, accessToken: response.accessToken)
        let tokenSet = CustomGoogleOAuthTokenSet(
            accessToken: response.accessToken,
            refreshToken: refreshToken,
            expiresAt: Date().addingTimeInterval(TimeInterval(response.expiresIn)),
            grantedScopes: Set((response.scope ?? requiredScopes.joined(separator: " ")).split(separator: " ").map(String.init)),
            account: account,
            idToken: response.idToken
        )
        try tokenStore.saveTokenSet(tokenSet, accountID: account.id)
        setActiveAccountID(account.id)
        AppLogger.info("custom OAuth sign-in succeeded", category: .auth, metadata: ["email": GoogleAuthService.redact(account.email)])
        return account
    }

    func accessToken() async throws -> String? {
        guard isConfigured else { return nil }
        guard let activeAccountID else { return nil }
        guard var tokenSet = tokenStore.loadTokenSet(accountID: activeAccountID) else {
            throw CustomGoogleOAuthError.missingTokenSet
        }
        if tokenSet.needsRefresh {
            tokenSet = try await refresh(tokenSet: tokenSet)
        }
        return tokenSet.accessToken
    }

    private func waitForCallback(from server: OAuthLoopbackServer) async throws -> OAuthLoopbackCallback {
        try await withThrowingTaskGroup(of: OAuthLoopbackCallback.self) { group in
            group.addTask {
                try await server.waitForCallback()
            }
            group.addTask { [callbackTimeoutNanoseconds] in
                try await Task.sleep(nanoseconds: callbackTimeoutNanoseconds)
                throw CustomGoogleOAuthError.timedOut
            }
            guard let result = try await group.next() else {
                throw CustomGoogleOAuthError.timedOut
            }
            group.cancelAll()
            return result
        }
    }

    private func authorizationURL(
        config: GoogleOAuthClientConfiguration,
        redirectURI: URL,
        state: String,
        codeChallenge: String
    ) throws -> URL {
        var components = URLComponents(string: "https://accounts.google.com/o/oauth2/v2/auth")
        components?.queryItems = [
            URLQueryItem(name: "client_id", value: config.clientID),
            URLQueryItem(name: "redirect_uri", value: redirectURI.absoluteString),
            URLQueryItem(name: "response_type", value: "code"),
            URLQueryItem(name: "scope", value: requiredScopes.joined(separator: " ")),
            URLQueryItem(name: "access_type", value: "offline"),
            URLQueryItem(name: "prompt", value: "consent select_account"),
            URLQueryItem(name: "state", value: state),
            URLQueryItem(name: "code_challenge", value: codeChallenge),
            URLQueryItem(name: "code_challenge_method", value: "S256")
        ]
        guard let url = components?.url else {
            throw GoogleAPIError.invalidURL
        }
        return url
    }

    private func exchangeAuthorizationCode(
        _ code: String,
        redirectURI: String,
        codeVerifier: String,
        config: GoogleOAuthClientConfiguration
    ) async throws -> GoogleOAuthTokenResponse {
        var fields: [String: String] = [
            "code": code,
            "client_id": config.clientID,
            "redirect_uri": redirectURI,
            "grant_type": "authorization_code",
            "code_verifier": codeVerifier
        ]
        if let clientSecret = config.clientSecret?.nilIfEmpty {
            fields["client_secret"] = clientSecret
        }
        return try await tokenRequest(fields: fields)
    }

    private func refresh(tokenSet: CustomGoogleOAuthTokenSet) async throws -> CustomGoogleOAuthTokenSet {
        guard let config = clientConfiguration?.normalized else {
            throw CustomGoogleOAuthError.clientNotConfigured
        }
        var fields: [String: String] = [
            "client_id": config.clientID,
            "refresh_token": tokenSet.refreshToken,
            "grant_type": "refresh_token"
        ]
        if let clientSecret = config.clientSecret?.nilIfEmpty {
            fields["client_secret"] = clientSecret
        }
        let response = try await tokenRequest(fields: fields)
        var refreshed = tokenSet
        refreshed.accessToken = response.accessToken
        refreshed.expiresAt = Date().addingTimeInterval(TimeInterval(response.expiresIn))
        if let scope = response.scope {
            refreshed.grantedScopes = Set(scope.split(separator: " ").map(String.init))
        }
        if let idToken = response.idToken {
            refreshed.idToken = idToken
            if let account = try? Self.accountFromIDToken(idToken, scopes: refreshed.grantedScopes) {
                refreshed.account = account
            }
        }
        try tokenStore.saveTokenSet(refreshed, accountID: refreshed.account.id)
        return refreshed
    }

    private func tokenRequest(fields: [String: String]) async throws -> GoogleOAuthTokenResponse {
        var request = URLRequest(url: URL(string: "https://oauth2.googleapis.com/token")!)
        request.httpMethod = "POST"
        request.setValue("application/x-www-form-urlencoded", forHTTPHeaderField: "Content-Type")
        request.httpBody = fields.formURLEncoded().data(using: .utf8)
        let (data, response) = try await urlSession.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw GoogleAPIError.invalidResponse
        }
        guard (200..<300).contains(http.statusCode) else {
            let body = String(data: data, encoding: .utf8) ?? "HTTP \(http.statusCode)"
            AppLogger.error("custom OAuth token request failed", category: .auth, metadata: ["status": "\(http.statusCode)", "body": body])
            throw CustomGoogleOAuthError.tokenExchangeFailed(body.prefix(240).description)
        }
        return try JSONDecoder().decode(GoogleOAuthTokenResponse.self, from: data)
    }

    private func account(from response: GoogleOAuthTokenResponse, accessToken: String) async throws -> GoogleAccount {
        if let idToken = response.idToken,
           let account = try? Self.accountFromIDToken(
            idToken,
            scopes: Set((response.scope ?? requiredScopes.joined(separator: " ")).split(separator: " ").map(String.init))
           ) {
            return account
        }
        return try await fetchUserInfo(accessToken: accessToken, scopes: Set((response.scope ?? "").split(separator: " ").map(String.init)))
    }

    private func fetchUserInfo(accessToken: String, scopes: Set<String>) async throws -> GoogleAccount {
        var request = URLRequest(url: URL(string: "https://www.googleapis.com/oauth2/v3/userinfo")!)
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        let (data, response) = try await urlSession.data(for: request)
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw CustomGoogleOAuthError.missingAccountProfile
        }
        let profile = try JSONDecoder().decode(GoogleOAuthUserInfo.self, from: data)
        guard let email = profile.email?.nilIfEmpty else {
            throw CustomGoogleOAuthError.missingAccountProfile
        }
        return GoogleAccount(
            id: profile.sub ?? email,
            email: email,
            displayName: profile.name?.nilIfEmpty ?? email,
            grantedScopes: scopes.isEmpty ? Set(requiredScopes) : scopes,
            authProvider: .customDesktopOAuth
        )
    }

    private static func accountFromIDToken(_ idToken: String, scopes: Set<String>) throws -> GoogleAccount {
        let parts = idToken.split(separator: ".")
        guard parts.count >= 2,
              let payloadData = Data(base64URLEncoded: String(parts[1])) else {
            throw CustomGoogleOAuthError.missingAccountProfile
        }
        let profile = try JSONDecoder().decode(GoogleOAuthUserInfo.self, from: payloadData)
        guard let email = profile.email?.nilIfEmpty else {
            throw CustomGoogleOAuthError.missingAccountProfile
        }
        return GoogleAccount(
            id: profile.sub ?? email,
            email: email,
            displayName: profile.name?.nilIfEmpty ?? email,
            grantedScopes: scopes,
            authProvider: .customDesktopOAuth
        )
    }

    private static func randomURLSafeString(byteCount: Int) -> String {
        var bytes = [UInt8](repeating: 0, count: byteCount)
        _ = SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes)
        return Data(bytes).base64URLEncodedString()
    }

    private static func codeChallenge(for verifier: String) -> String {
        Data(SHA256.hash(data: Data(verifier.utf8))).base64URLEncodedString()
    }
}

private struct GoogleOAuthTokenResponse: Decodable {
    var accessToken: String
    var expiresIn: Int
    var refreshToken: String?
    var scope: String?
    var tokenType: String
    var idToken: String?

    enum CodingKeys: String, CodingKey {
        case accessToken = "access_token"
        case expiresIn = "expires_in"
        case refreshToken = "refresh_token"
        case scope
        case tokenType = "token_type"
        case idToken = "id_token"
    }
}

private struct GoogleOAuthUserInfo: Decodable {
    var sub: String?
    var email: String?
    var name: String?
}

final class GoogleOAuthKeychainStore: GoogleOAuthTokenStoring, @unchecked Sendable {
    private static let clientConfigurationAccount = "custom-google-oauth-client"
    private static let legacyTokenSetAccount = "custom-google-oauth-token-set"
    private static let tokenIndexAccount = "custom-google-oauth-token-index"
    private static let tokenSetAccountPrefix = "custom-google-oauth-token-set:"
    private static let service = "com.gongahkia.hotcrossbuns.custom-google-oauth"

    func loadClientConfiguration() -> GoogleOAuthClientConfiguration? {
        Self.load(GoogleOAuthClientConfiguration.self, account: Self.clientConfigurationAccount)
    }

    func saveClientConfiguration(_ configuration: GoogleOAuthClientConfiguration) throws {
        try Self.save(configuration, account: Self.clientConfigurationAccount)
    }

    func clearClientConfiguration() {
        Self.delete(account: Self.clientConfigurationAccount)
    }

    func loadTokenSet(accountID: GoogleAccount.ID) -> CustomGoogleOAuthTokenSet? {
        migrateLegacyTokenSetIfNeeded()
        return Self.load(CustomGoogleOAuthTokenSet.self, account: Self.tokenSetAccount(accountID: accountID))
    }

    func loadFirstTokenSet() -> CustomGoogleOAuthTokenSet? {
        migrateLegacyTokenSetIfNeeded()
        for accountID in tokenIndex() {
            if let tokenSet = loadTokenSet(accountID: accountID) {
                return tokenSet
            }
        }
        return nil
    }

    func saveTokenSet(_ tokenSet: CustomGoogleOAuthTokenSet, accountID: GoogleAccount.ID) throws {
        try Self.save(tokenSet, account: Self.tokenSetAccount(accountID: accountID))
        var index = tokenIndex()
        if index.contains(accountID) == false {
            index.append(accountID)
            try Self.save(index, account: Self.tokenIndexAccount)
        }
    }

    func deleteTokenSet(accountID: GoogleAccount.ID) {
        Self.delete(account: Self.tokenSetAccount(accountID: accountID))
        let index = tokenIndex().filter { $0 != accountID }
        try? Self.save(index, account: Self.tokenIndexAccount)
    }

    func deleteAllTokenSets() {
        for accountID in tokenIndex() {
            Self.delete(account: Self.tokenSetAccount(accountID: accountID))
        }
        Self.delete(account: Self.legacyTokenSetAccount)
        Self.delete(account: Self.tokenIndexAccount)
    }

    private func migrateLegacyTokenSetIfNeeded() {
        guard tokenIndex().isEmpty,
              let legacy = Self.load(CustomGoogleOAuthTokenSet.self, account: Self.legacyTokenSetAccount) else {
            return
        }
        do {
            try saveTokenSet(legacy, accountID: legacy.account.id)
            Self.delete(account: Self.legacyTokenSetAccount)
        } catch {
            AppLogger.warn("custom OAuth legacy token migration failed", category: .auth, metadata: [
                "error": String(describing: error)
            ])
        }
    }

    private func tokenIndex() -> [GoogleAccount.ID] {
        Self.load([GoogleAccount.ID].self, account: Self.tokenIndexAccount) ?? []
    }

    private static func tokenSetAccount(accountID: GoogleAccount.ID) -> String {
        "\(tokenSetAccountPrefix)\(accountID)"
    }

    private static func save<T: Encodable>(_ value: T, account: String) throws {
        let data = try JSONEncoder().encode(value)
        let query = baseQuery(account: account)
        let attributes: [String: Any] = [
            kSecValueData as String: data,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        ]
        let status = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
        if status == errSecSuccess {
            return
        }
        if status == errSecItemNotFound {
            var add = query
            add[kSecValueData as String] = data
            add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
            let addStatus = SecItemAdd(add as CFDictionary, nil)
            guard addStatus == errSecSuccess else {
                throw GoogleOAuthKeychainError.osStatus(addStatus)
            }
            return
        }
        throw GoogleOAuthKeychainError.osStatus(status)
    }

    private static func delete(account: String) {
        SecItemDelete(baseQuery(account: account) as CFDictionary)
    }

    private static func load<T: Decodable>(_ type: T.Type, account: String) -> T? {
        var query = baseQuery(account: account)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        guard status == errSecSuccess,
              let data = result as? Data else {
            return nil
        }
        return try? JSONDecoder().decode(type, from: data)
    }

    private static func baseQuery(account: String) -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account
        ]
    }
}

private enum GoogleOAuthKeychainError: LocalizedError {
    case osStatus(OSStatus)

    var errorDescription: String? {
        switch self {
        case .osStatus(let status):
            "Could not save Google OAuth credentials in Keychain (OSStatus \(status))."
        }
    }
}

private extension Dictionary where Key == String, Value == String {
    func formURLEncoded() -> String {
        sorted { $0.key < $1.key }
            .map { "\($0.key.formPercentEncoded())=\($0.value.formPercentEncoded())" }
            .joined(separator: "&")
    }
}

private extension String {
    var nilIfEmpty: String? {
        isEmpty ? nil : self
    }

    func formPercentEncoded() -> String {
        var allowed = CharacterSet.urlQueryAllowed
        allowed.remove(charactersIn: ":#[]@!$&'()*+,;=")
        return addingPercentEncoding(withAllowedCharacters: allowed) ?? self
    }
}

private extension Data {
    init?(base64URLEncoded string: String) {
        var value = string.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        let padding = value.count % 4
        if padding > 0 {
            value += String(repeating: "=", count: 4 - padding)
        }
        self.init(base64Encoded: value)
    }

    func base64URLEncodedString() -> String {
        base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }
}
