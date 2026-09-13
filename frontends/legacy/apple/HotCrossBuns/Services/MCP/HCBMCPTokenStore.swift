import Foundation
import Security

enum HCBMCPTokenStore {
    private static let service = "com.gongahkia.hotcrossbuns.mcp"
    private static let account = "bearerToken"

    enum TokenError: Error {
        case randomBytes(OSStatus)
        case keychain(OSStatus)
    }

    static func loadOrCreateToken() throws -> String {
        if let token = loadToken() {
            return token
        }
        let token = try generateToken()
        try saveToken(token)
        return token
    }

    static func loadToken() -> String? {
        var query = baseQuery()
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        guard status == errSecSuccess,
              let data = result as? Data,
              let token = String(data: data, encoding: .utf8),
              token.isEmpty == false else {
            return nil
        }
        return token
    }

    static func resetToken() throws -> String {
        clearToken()
        let token = try generateToken()
        try saveToken(token)
        return token
    }

    static func clearToken() {
        SecItemDelete(baseQuery() as CFDictionary)
    }

    private static func saveToken(_ token: String) throws {
        let data = Data(token.utf8)
        var updateQuery = baseQuery()
        let attributes: [String: Any] = [kSecValueData as String: data]
        let updateStatus = SecItemUpdate(updateQuery as CFDictionary, attributes as CFDictionary)
        if updateStatus == errSecSuccess {
            return
        }
        if updateStatus != errSecItemNotFound {
            throw TokenError.keychain(updateStatus)
        }

        updateQuery[kSecValueData as String] = data
        updateQuery[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        let addStatus = SecItemAdd(updateQuery as CFDictionary, nil)
        guard addStatus == errSecSuccess else {
            throw TokenError.keychain(addStatus)
        }
    }

    private static func generateToken() throws -> String {
        var bytes = [UInt8](repeating: 0, count: 32)
        let status = SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes)
        guard status == errSecSuccess else {
            throw TokenError.randomBytes(status)
        }
        return Data(bytes).base64EncodedString()
    }

    private static func baseQuery() -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account
        ]
    }
}
