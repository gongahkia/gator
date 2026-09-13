import Foundation
import CryptoKit
import CommonCrypto

// Passphrase-based AES-256-GCM encryption for the local cache file and its
// snapshots (§6.12). Google itself never sees any of this — we're protecting
// the on-disk JSON mirror + offline mutation queue from a stolen laptop, not
// adding a second canonical store.
//
// Threat model:
//  - Protect: local cache snapshot + pending mutations at rest.
//  - Against: another macOS user on the same machine, or a disk-image thief.
//  - NOT against: live process memory, a jailbroken system, or a weak passphrase.
//
// Forgot passphrase = local cache is unrecoverable. Google data is untouched,
// so a fresh sign-in restores everything *except* any pending mutations that
// had not yet round-tripped. The Settings UI warns about this.
enum HCBCacheCrypto {
    // Random salt written next to the cache. Separate from the cache file so
    // we don't need to decrypt the cache to derive the key.
    static let saltBytes = 16
    // 200k iterations — standard 2024+ bar for interactive PBKDF2. Tune up
    // in a future pass if the hardware floor rises; the persisted wrapper
    // records the iteration count so old blobs still decrypt.
    static let pbkdf2Iterations: UInt32 = 200_000

    // On-disk wrapper when the cache file is encrypted. `version` lets us
    // rotate the KDF or cipher in the future without breaking existing caches.
    struct EncryptedBlob: Codable, Equatable {
        let version: Int // 1 = PBKDF2-HMAC-SHA256 + AES-GCM-256
        let iterations: UInt32
        let salt: Data // base64 on wire
        let nonce: Data
        let ciphertext: Data // includes GCM tag
    }

    struct CryptoError: Error, Equatable {
        let message: String
    }

    // Derives a 256-bit SymmetricKey from the user's passphrase + salt.
    // Uses CommonCrypto PBKDF2-HMAC-SHA256 — CryptoKit doesn't expose
    // password stretching, only HKDF (which is inappropriate here).
    static func deriveKey(passphrase: String, salt: Data, iterations: UInt32 = pbkdf2Iterations) throws -> SymmetricKey {
        let passphraseData = Data(passphrase.utf8)
        var derivedKey = Data(count: 32)

        let status = passphraseData.withUnsafeBytes { passBytes -> Int32 in
            salt.withUnsafeBytes { saltBytes -> Int32 in
                derivedKey.withUnsafeMutableBytes { outBytes -> Int32 in
                    CCKeyDerivationPBKDF(
                        CCPBKDFAlgorithm(kCCPBKDF2),
                        passBytes.bindMemory(to: Int8.self).baseAddress,
                        passphraseData.count,
                        saltBytes.bindMemory(to: UInt8.self).baseAddress,
                        salt.count,
                        CCPseudoRandomAlgorithm(kCCPRFHmacAlgSHA256),
                        iterations,
                        outBytes.bindMemory(to: UInt8.self).baseAddress,
                        32
                    )
                }
            }
        }

        guard status == kCCSuccess else {
            throw CryptoError(message: "Key derivation failed (status \(status)).")
        }
        return SymmetricKey(data: derivedKey)
    }

    static func randomSalt() -> Data {
        var bytes = Data(count: saltBytes)
        let status = bytes.withUnsafeMutableBytes { buf -> Int32 in
            SecRandomCopyBytes(kSecRandomDefault, saltBytes, buf.baseAddress!)
        }
        precondition(status == errSecSuccess, "SecRandomCopyBytes failed — system RNG unavailable")
        return bytes
    }

    // Encrypts `plaintext` and returns a Codable wrapper with all metadata
    // the reader needs to decrypt. Fresh nonce per call (AES-GCM nonce must
    // never repeat under the same key).
    static func encrypt(_ plaintext: Data, key: SymmetricKey, salt: Data, iterations: UInt32 = pbkdf2Iterations) throws -> EncryptedBlob {
        let sealed = try AES.GCM.seal(plaintext, using: key)
        guard let combined = sealed.combined else {
            throw CryptoError(message: "AES-GCM sealing returned nil combined payload.")
        }
        // `combined` = nonce (12) || ciphertext || tag (16). We split the
        // nonce out for clarity in the on-disk format; tag stays inside
        // ciphertext where AES.GCM.SealedBox expects it.
        let nonce = Data(sealed.nonce)
        let ctWithTag = combined.suffix(combined.count - nonce.count)
        return EncryptedBlob(
            version: 1,
            iterations: iterations,
            salt: salt,
            nonce: nonce,
            ciphertext: Data(ctWithTag)
        )
    }

    static func decrypt(_ blob: EncryptedBlob, key: SymmetricKey) throws -> Data {
        guard blob.version == 1 else {
            throw CryptoError(message: "Unsupported cache crypto version \(blob.version).")
        }
        let nonce = try AES.GCM.Nonce(data: blob.nonce)
        let sealed = try AES.GCM.SealedBox(combined: Data(nonce) + blob.ciphertext)
        return try AES.GCM.open(sealed, using: key)
    }
}
