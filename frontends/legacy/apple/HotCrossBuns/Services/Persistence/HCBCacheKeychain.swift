import Foundation
import CryptoKit
import Security

// Caches the derived cache-encryption SymmetricKey in the macOS Keychain so
// users don't re-enter their passphrase on every launch. Separate from the
// Google-auth Keychain entries (different service name) so disconnecting
// Google doesn't clear the cache key, and vice versa.
//
// Storing derived key (not passphrase) — if an attacker pulls the Keychain
// they already have enough to decrypt the cache, but they also would have
// needed the passphrase anyway. The derivation step exists to stretch a
// weak passphrase against offline brute force on the *ciphertext*, not to
// gate Keychain access.
enum HCBCacheKeychain {
    private static let service = "com.gongahkia.hotcrossbuns.cacheKey"
    private static let account = "primary"

    enum KeychainError: Error {
        case unexpected(OSStatus)
    }

    static func save(_ key: SymmetricKey) throws {
        let raw = key.withUnsafeBytes { Data($0) }
        // Remove any existing entry first — SecItemUpdate fails if attrs mismatch
        // and SecItemAdd fails on duplicates.
        let delete: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account
        ]
        SecItemDelete(delete as CFDictionary)

        let add: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecValueData as String: raw,
            kSecAttrAccessible as String: kSecAttrAccessibleWhenUnlockedThisDeviceOnly
        ]
        let status = SecItemAdd(add as CFDictionary, nil)
        guard status == errSecSuccess else {
            throw KeychainError.unexpected(status)
        }
    }

    static func load() -> SymmetricKey? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne
        ]
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        guard status == errSecSuccess, let data = result as? Data else { return nil }
        return SymmetricKey(data: data)
    }

    static func clear() {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account
        ]
        SecItemDelete(query as CFDictionary)
    }
}
