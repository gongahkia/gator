import AppKit
import GoogleSignIn
import XCTest
@testable import HotCrossBunsMac

@MainActor
final class GoogleAuthServiceHelpersTests: XCTestCase {
    func testMissingScopesReturnsOnlyRevokedScopes() {
        let missing = GoogleAuthService.missingScopes(
            requiredScopes: [GoogleScope.tasks, GoogleScope.calendar],
            grantedScopes: [GoogleScope.tasks]
        )

        XCTAssertEqual(missing, [GoogleScope.calendar])
    }

    func testBuildAccountRequiresEmailAndFallsBackDisplayName() throws {
        let account = try GoogleAuthService.buildAccount(
            userID: nil,
            email: "person@example.com",
            displayName: nil,
            grantedScopes: [GoogleScope.tasks]
        )

        XCTAssertEqual(account.id, "person@example.com")
        XCTAssertEqual(account.displayName, "person@example.com")
        XCTAssertEqual(account.grantedScopes, [GoogleScope.tasks])
        XCTAssertEqual(account.authProvider, .embeddedGoogleSignIn)

        XCTAssertThrowsError(
            try GoogleAuthService.buildAccount(
                userID: "abc",
                email: nil,
                displayName: "No Email",
                grantedScopes: nil
            )
        ) { error in
            XCTAssertEqual(error as? GoogleAuthError, .missingProfile)
        }
    }

    func testResolvePresentationAnchorPrefersKeyThenMain() throws {
        let keyWindow = NSWindow()
        let mainWindow = NSWindow()

        XCTAssertTrue(
            try GoogleAuthService.resolvePresentationAnchor(
                keyWindow: keyWindow,
                mainWindow: mainWindow,
                windows: []
            ) === keyWindow
        )

        XCTAssertTrue(
            try GoogleAuthService.resolvePresentationAnchor(
                keyWindow: nil,
                mainWindow: mainWindow,
                windows: []
            ) === mainWindow
        )
    }

    func testResolvePresentationAnchorThrowsWhenNoWindowAvailable() {
        XCTAssertThrowsError(
            try GoogleAuthService.resolvePresentationAnchor(
                keyWindow: nil,
                mainWindow: nil,
                windows: []
            )
        ) { error in
            XCTAssertEqual(error as? GoogleAuthError, .noPresentationAnchor)
        }
    }

    func testRedactKeepsOnlyFirstTwoLocalPartCharacters() {
        XCTAssertEqual(GoogleAuthService.redact("person@example.com"), "pe***@example.com")
        XCTAssertEqual(GoogleAuthService.redact("x@y.z"), "x***@y.z")
        XCTAssertEqual(GoogleAuthService.redact("not-an-email"), "<redacted>")
    }

    func testErrorMetadataIncludesUnderlyingNSErrorChain() {
        let deepest = NSError(
            domain: "deep.domain",
            code: 42,
            userInfo: [NSLocalizedDescriptionKey: "very broken"]
        )
        let middle = NSError(
            domain: "middle.domain",
            code: 21,
            userInfo: [NSUnderlyingErrorKey: deepest]
        )
        let top = NSError(
            domain: "top.domain",
            code: 9,
            userInfo: [NSUnderlyingErrorKey: middle]
        )

        let metadata = GoogleAuthService.errorMetadata(top)
        XCTAssertEqual(metadata["domain"], "top.domain")
        XCTAssertEqual(metadata["code"], "9")
        XCTAssertEqual(metadata["underlying0.domain"], "middle.domain")
        XCTAssertEqual(metadata["underlying0.code"], "21")
        XCTAssertEqual(metadata["underlying1.domain"], "deep.domain")
        XCTAssertEqual(metadata["underlying1.code"], "42")
        XCTAssertEqual(metadata["underlying1.description"], "very broken")
    }

    func testIsUserCancellationDetectsGoogleCancelError() {
        let error = NSError(
            domain: kGIDSignInErrorDomain,
            code: GIDSignInError.canceled.rawValue
        )

        XCTAssertTrue(GoogleAuthService.isUserCancellation(error))
        XCTAssertFalse(GoogleAuthService.isUserCancellation(NSError(domain: "other", code: 1)))
    }

    func testCustomOAuthClientConfigurationNormalizesAndValidates() {
        let valid = GoogleOAuthClientConfiguration(
            clientID: "  abc.apps.googleusercontent.com  ",
            clientSecret: "  secret  "
        ).normalized

        XCTAssertTrue(valid.isValid)
        XCTAssertEqual(valid.clientID, "abc.apps.googleusercontent.com")
        XCTAssertEqual(valid.clientSecret, "secret")

        let invalid = GoogleOAuthClientConfiguration(clientID: "not-a-google-client", clientSecret: "")
        XCTAssertFalse(invalid.normalized.isValid)
        XCTAssertNil(invalid.normalized.clientSecret)
    }

    func testCustomOAuthAccessTokenUsesActiveAccountTokenSet() async throws {
        let store = InMemoryGoogleOAuthTokenStore(
            clientConfiguration: GoogleOAuthClientConfiguration(clientID: "abc.apps.googleusercontent.com", clientSecret: nil)
        )
        let personal = Self.tokenSet(accountID: "personal", email: "person@example.com", accessToken: "personal-token")
        let work = Self.tokenSet(accountID: "work", email: "work@example.com", accessToken: "work-token")
        try store.saveTokenSet(personal, accountID: personal.account.id)
        try store.saveTokenSet(work, accountID: work.account.id)
        let service = CustomGoogleOAuthService(tokenStore: store)

        service.setActiveAccountID(personal.account.id)
        let personalToken = try await service.accessToken()
        XCTAssertEqual(personalToken, "personal-token")

        service.setActiveAccountID(work.account.id)
        let workToken = try await service.accessToken()
        XCTAssertEqual(workToken, "work-token")
    }

    func testCustomOAuthClientChangeClearsAllAccountTokenSets() throws {
        let store = InMemoryGoogleOAuthTokenStore(
            clientConfiguration: GoogleOAuthClientConfiguration(clientID: "old.apps.googleusercontent.com", clientSecret: nil)
        )
        try store.saveTokenSet(Self.tokenSet(accountID: "a", email: "a@example.com", accessToken: "a-token"), accountID: "a")
        try store.saveTokenSet(Self.tokenSet(accountID: "b", email: "b@example.com", accessToken: "b-token"), accountID: "b")
        let service = CustomGoogleOAuthService(tokenStore: store)

        _ = try service.saveClientConfiguration(
            GoogleOAuthClientConfiguration(clientID: "new.apps.googleusercontent.com", clientSecret: nil)
        )

        XCTAssertNil(store.loadTokenSet(accountID: "a"))
        XCTAssertNil(store.loadTokenSet(accountID: "b"))
        XCTAssertEqual(store.loadClientConfiguration()?.clientID, "new.apps.googleusercontent.com")
    }

    func testCustomOAuthClientClearRemovesConfigurationTokensAndActiveAccount() throws {
        let store = InMemoryGoogleOAuthTokenStore(
            clientConfiguration: GoogleOAuthClientConfiguration(clientID: "old.apps.googleusercontent.com", clientSecret: nil)
        )
        try store.saveTokenSet(Self.tokenSet(accountID: "a", email: "a@example.com", accessToken: "a-token"), accountID: "a")
        try store.saveTokenSet(Self.tokenSet(accountID: "b", email: "b@example.com", accessToken: "b-token"), accountID: "b")
        let service = CustomGoogleOAuthService(tokenStore: store)
        service.setActiveAccountID("b")

        service.clearClientConfiguration()

        XCTAssertNil(store.loadClientConfiguration())
        XCTAssertNil(store.loadTokenSet(accountID: "a"))
        XCTAssertNil(store.loadTokenSet(accountID: "b"))
        XCTAssertNil(service.activeAccountID)
    }

    func testCustomOAuthAccessTokenThrowsWhenActiveTokenSetIsMissing() async throws {
        let store = InMemoryGoogleOAuthTokenStore(
            clientConfiguration: GoogleOAuthClientConfiguration(clientID: "abc.apps.googleusercontent.com", clientSecret: nil)
        )
        let service = CustomGoogleOAuthService(tokenStore: store)
        service.setActiveAccountID("missing")

        do {
            _ = try await service.accessToken()
            XCTFail("Expected missingTokenSet")
        } catch let error as CustomGoogleOAuthError {
            XCTAssertEqual(error, .missingTokenSet)
        }
    }

    func testCustomOAuthRefreshUsesOnlyActiveAccountRefreshToken() async throws {
        MockURLProtocol.reset()
        defer { MockURLProtocol.reset() }

        let store = InMemoryGoogleOAuthTokenStore(
            clientConfiguration: GoogleOAuthClientConfiguration(clientID: "abc.apps.googleusercontent.com", clientSecret: nil)
        )
        try store.saveTokenSet(
            Self.tokenSet(
                accountID: "a",
                email: "a@example.com",
                accessToken: "a-token",
                expiresAt: Date().addingTimeInterval(-60)
            ),
            accountID: "a"
        )
        try store.saveTokenSet(
            Self.tokenSet(
                accountID: "b",
                email: "b@example.com",
                accessToken: "b-token",
                expiresAt: Date().addingTimeInterval(-60)
            ),
            accountID: "b"
        )
        MockURLProtocol.requestHandler = { request in
            let body = Self.requestBodyString(request)
            XCTAssertTrue(body.contains("refresh_token=b-token-refresh"), body)
            XCTAssertFalse(body.contains("refresh_token=a-token-refresh"), body)
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            return (
                response,
                Data(#"{"access_token":"b-refreshed","expires_in":3600,"scope":"https://www.googleapis.com/auth/tasks https://www.googleapis.com/auth/calendar","token_type":"Bearer"}"#.utf8)
            )
        }
        let service = CustomGoogleOAuthService(urlSession: MockURLProtocol.testSession(), tokenStore: store)
        service.setActiveAccountID("b")

        let token = try await service.accessToken()

        XCTAssertEqual(token, "b-refreshed")
        XCTAssertEqual(store.loadTokenSet(accountID: "a")?.accessToken, "a-token")
        XCTAssertEqual(store.loadTokenSet(accountID: "b")?.accessToken, "b-refreshed")
        XCTAssertEqual(MockURLProtocol.capturedRequests.count, 1)
    }

    func testCustomOAuthRestoreUsesActiveAccountWhenAvailable() async throws {
        let store = InMemoryGoogleOAuthTokenStore(
            clientConfiguration: GoogleOAuthClientConfiguration(clientID: "abc.apps.googleusercontent.com", clientSecret: nil)
        )
        try store.saveTokenSet(Self.tokenSet(accountID: "a", email: "a@example.com", accessToken: "a-token"), accountID: "a")
        try store.saveTokenSet(Self.tokenSet(accountID: "b", email: "b@example.com", accessToken: "b-token"), accountID: "b")
        let service = CustomGoogleOAuthService(tokenStore: store)

        service.setActiveAccountID("b")
        let restored = try await service.restorePreviousSignIn()

        XCTAssertEqual(restored?.id, "b")
    }

    private static func tokenSet(
        accountID: String,
        email: String,
        accessToken: String,
        expiresAt: Date = Date().addingTimeInterval(3_600)
    ) -> CustomGoogleOAuthTokenSet {
        CustomGoogleOAuthTokenSet(
            accessToken: accessToken,
            refreshToken: "\(accessToken)-refresh",
            expiresAt: expiresAt,
            grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
            account: GoogleAccount(
                id: accountID,
                email: email,
                displayName: email,
                grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
                authProvider: .customDesktopOAuth
            ),
            idToken: nil
        )
    }

    private static func requestBodyString(_ request: URLRequest) -> String {
        if let body = request.httpBody {
            return String(data: body, encoding: .utf8) ?? ""
        }
        guard let stream = request.httpBodyStream else { return "" }
        stream.open()
        defer { stream.close() }

        var data = Data()
        var buffer = [UInt8](repeating: 0, count: 1_024)
        while stream.hasBytesAvailable {
            let read = stream.read(&buffer, maxLength: buffer.count)
            if read > 0 {
                data.append(buffer, count: read)
            } else {
                break
            }
        }
        return String(data: data, encoding: .utf8) ?? ""
    }
}

final class InMemoryGoogleOAuthTokenStore: GoogleOAuthTokenStoring, @unchecked Sendable {
    private let lock = NSLock()
    private var clientConfiguration: GoogleOAuthClientConfiguration?
    private var tokenSets: [GoogleAccount.ID: CustomGoogleOAuthTokenSet] = [:]

    init(clientConfiguration: GoogleOAuthClientConfiguration?) {
        self.clientConfiguration = clientConfiguration
    }

    func loadClientConfiguration() -> GoogleOAuthClientConfiguration? {
        lock.lock()
        defer { lock.unlock() }
        return clientConfiguration
    }

    func saveClientConfiguration(_ configuration: GoogleOAuthClientConfiguration) throws {
        lock.lock()
        clientConfiguration = configuration
        lock.unlock()
    }

    func clearClientConfiguration() {
        lock.lock()
        clientConfiguration = nil
        lock.unlock()
    }

    func loadTokenSet(accountID: GoogleAccount.ID) -> CustomGoogleOAuthTokenSet? {
        lock.lock()
        defer { lock.unlock() }
        return tokenSets[accountID]
    }

    func loadFirstTokenSet() -> CustomGoogleOAuthTokenSet? {
        lock.lock()
        defer { lock.unlock() }
        return tokenSets.keys.sorted().compactMap { tokenSets[$0] }.first
    }

    func saveTokenSet(_ tokenSet: CustomGoogleOAuthTokenSet, accountID: GoogleAccount.ID) throws {
        lock.lock()
        tokenSets[accountID] = tokenSet
        lock.unlock()
    }

    func deleteTokenSet(accountID: GoogleAccount.ID) {
        lock.lock()
        tokenSets.removeValue(forKey: accountID)
        lock.unlock()
    }

    func deleteAllTokenSets() {
        lock.lock()
        tokenSets = [:]
        lock.unlock()
    }
}
