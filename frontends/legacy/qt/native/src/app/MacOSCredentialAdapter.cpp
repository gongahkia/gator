#include "app/MacOSCredentialAdapter.h"

#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonParseError>

#if defined(Q_OS_MACOS)
#include <Security/Security.h>
#endif

#include <future>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumAccountIdLength = 256;
constexpr qsizetype kMaximumTokenLength = 8'192;
constexpr qsizetype kMaximumCredentialPayloadLength = 70'000;
constexpr char kKeychainServiceName[] = "dev.hotcrossbuns.native.oauth";

[[nodiscard]] AppError configurationError(QString message) {
  return AppError(AppErrorCode::Configuration, std::move(message));
}

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

template <typename Result, typename Operation>
[[nodiscard]] std::future<Result> runOnWorker(Operation operation, Result fallback) {
  try {
    return std::async(std::launch::async,
                      [operation = std::move(operation), fallback = std::move(fallback)]() mutable {
                        try {
                          return operation();
                        } catch (...) {
                          return fallback;
                        }
                      });
  } catch (...) {
    return readyFuture(std::move(fallback));
  }
}

[[nodiscard]] bool isValidAccountId(const QString& accountId) {
  return !accountId.isEmpty() && accountId == accountId.trimmed() &&
         accountId.size() <= kMaximumAccountIdLength && !accountId.contains(QChar::Null);
}

[[nodiscard]] bool isValidToken(const QString& token) {
  return !token.isEmpty() && token.size() <= kMaximumTokenLength && !token.contains(QChar::Null);
}

[[nodiscard]] bool isValidCredential(const OAuthStoredCredential& credential) {
  return isValidToken(credential.accessToken) &&
         (!credential.refreshToken.has_value() || isValidToken(*credential.refreshToken));
}

[[nodiscard]] OAuthCredentialReadResult unsupportedRead() {
  return configurationError(QStringLiteral("macOS Keychain is unavailable on this platform"));
}

[[nodiscard]] OAuthCredentialSaveResult unsupportedSave() {
  return configurationError(QStringLiteral("macOS Keychain is unavailable on this platform"));
}

[[nodiscard]] OAuthCredentialDeleteResult unsupportedDelete() {
  return configurationError(QStringLiteral("macOS Keychain is unavailable on this platform"));
}

#if defined(Q_OS_MACOS)

template <typename T> class ScopedCF final {
public:
  explicit ScopedCF(T value = nullptr) : value_(value) {}
  ScopedCF(const ScopedCF&) = delete;
  ScopedCF& operator=(const ScopedCF&) = delete;
  ScopedCF(ScopedCF&& other) noexcept : value_(std::exchange(other.value_, nullptr)) {}
  ScopedCF& operator=(ScopedCF&& other) noexcept {
    if (this != &other) {
      reset();
      value_ = std::exchange(other.value_, nullptr);
    }
    return *this;
  }
  ~ScopedCF() { reset(); }

  [[nodiscard]] T get() const noexcept { return value_; }

private:
  void reset() noexcept {
    if (value_ != nullptr) {
      CFRelease(value_);
      value_ = nullptr;
    }
  }

  T value_;
};

[[nodiscard]] ScopedCF<CFStringRef> cfString(const QString& value) {
  const QByteArray utf8 = value.toUtf8();
  return ScopedCF<CFStringRef>(
      CFStringCreateWithBytes(kCFAllocatorDefault,
                              reinterpret_cast<const UInt8*>(utf8.constData()),
                              static_cast<CFIndex>(utf8.size()),
                              kCFStringEncodingUTF8,
                              false));
}

[[nodiscard]] ScopedCF<CFMutableDictionaryRef> keychainQuery(const QString& accountId) {
  ScopedCF<CFMutableDictionaryRef> query(CFDictionaryCreateMutable(
      kCFAllocatorDefault, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks));
  const ScopedCF<CFStringRef> service = cfString(QString::fromLatin1(kKeychainServiceName));
  const ScopedCF<CFStringRef> account = cfString(accountId);
  CFDictionarySetValue(query.get(), kSecClass, kSecClassGenericPassword);
  CFDictionarySetValue(query.get(), kSecAttrService, service.get());
  CFDictionarySetValue(query.get(), kSecAttrAccount, account.get());
  CFDictionarySetValue(query.get(), kSecAttrSynchronizable, kCFBooleanFalse);
  return query;
}

[[nodiscard]] ScopedCF<CFDataRef> credentialPayload(const OAuthStoredCredential& credential) {
  QJsonObject object;
  object.insert(QStringLiteral("accessToken"), credential.accessToken);
  if (credential.refreshToken.has_value()) {
    object.insert(QStringLiteral("refreshToken"), *credential.refreshToken);
  }
  const QByteArray encoded = QJsonDocument(object).toJson(QJsonDocument::Compact);
  return ScopedCF<CFDataRef>(CFDataCreate(kCFAllocatorDefault,
                                          reinterpret_cast<const UInt8*>(encoded.constData()),
                                          static_cast<CFIndex>(encoded.size())));
}

[[nodiscard]] OAuthCredentialReadResult readCredential(const QString& accountId) {
  const ScopedCF<CFMutableDictionaryRef> query = keychainQuery(accountId);
  CFDictionarySetValue(query.get(), kSecReturnData, kCFBooleanTrue);
  CFDictionarySetValue(query.get(), kSecMatchLimit, kSecMatchLimitOne);
  CFTypeRef rawResult = nullptr;
  const OSStatus status = SecItemCopyMatching(query.get(), &rawResult);
  const ScopedCF<CFTypeRef> result(rawResult);
  if (status == errSecItemNotFound) {
    return std::optional<OAuthStoredCredential>{};
  }
  if (status != errSecSuccess || rawResult == nullptr ||
      CFGetTypeID(rawResult) != CFDataGetTypeID()) {
    return configurationError(QStringLiteral("macOS Keychain credential read failed"));
  }
  const CFDataRef data = static_cast<CFDataRef>(rawResult);
  const CFIndex payloadLength = CFDataGetLength(data);
  if (payloadLength <= 0 || payloadLength > kMaximumCredentialPayloadLength) {
    return configurationError(QStringLiteral("macOS Keychain credential is invalid"));
  }
  const QByteArray payload(reinterpret_cast<const char*>(CFDataGetBytePtr(data)),
                           static_cast<qsizetype>(payloadLength));
  QJsonParseError parseError;
  const QJsonDocument document = QJsonDocument::fromJson(payload, &parseError);
  if (parseError.error != QJsonParseError::NoError || !document.isObject()) {
    return configurationError(QStringLiteral("macOS Keychain credential is invalid"));
  }
  const QJsonObject object = document.object();
  const QJsonValue accessToken = object.value(QStringLiteral("accessToken"));
  const QJsonValue refreshToken = object.value(QStringLiteral("refreshToken"));
  if (!accessToken.isString() || (!refreshToken.isUndefined() && !refreshToken.isString())) {
    return configurationError(QStringLiteral("macOS Keychain credential is invalid"));
  }
  OAuthStoredCredential credential{.accessToken = accessToken.toString()};
  if (refreshToken.isString()) {
    credential.refreshToken = refreshToken.toString();
  }
  if (!isValidCredential(credential)) {
    return configurationError(QStringLiteral("macOS Keychain credential is invalid"));
  }
  return std::optional<OAuthStoredCredential>(std::move(credential));
}

[[nodiscard]] OAuthCredentialSaveResult saveCredential(const QString& accountId,
                                                       const OAuthStoredCredential& credential) {
  const ScopedCF<CFMutableDictionaryRef> query = keychainQuery(accountId);
  const ScopedCF<CFDataRef> payload = credentialPayload(credential);
  ScopedCF<CFMutableDictionaryRef> attributes(CFDictionaryCreateMutable(
      kCFAllocatorDefault, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks));
  CFDictionarySetValue(attributes.get(), kSecValueData, payload.get());
  OSStatus status = SecItemUpdate(query.get(), attributes.get());
  if (status == errSecItemNotFound) {
    CFDictionarySetValue(query.get(), kSecValueData, payload.get());
    status = SecItemAdd(query.get(), nullptr);
  }
  return status == errSecSuccess ? OAuthCredentialSaveResult(std::monostate{})
                                 : OAuthCredentialSaveResult(configurationError(
                                       QStringLiteral("macOS Keychain credential save failed")));
}

[[nodiscard]] OAuthCredentialDeleteResult eraseCredential(const QString& accountId) {
  const ScopedCF<CFMutableDictionaryRef> query = keychainQuery(accountId);
  const OSStatus status = SecItemDelete(query.get());
  return status == errSecSuccess || status == errSecItemNotFound
             ? OAuthCredentialDeleteResult(std::monostate{})
             : OAuthCredentialDeleteResult(
                   configurationError(QStringLiteral("macOS Keychain credential erase failed")));
}

#endif

} // namespace

std::future<OAuthCredentialReadResult> MacOSCredentialAdapter::read(QString accountId) {
  if (!isValidAccountId(accountId)) {
    return readyFuture(OAuthCredentialReadResult(
        configurationError(QStringLiteral("OAuth credential account identifier is invalid"))));
  }
#if defined(Q_OS_MACOS)
  return runOnWorker<OAuthCredentialReadResult>(
      [accountId = std::move(accountId)] { return readCredential(accountId); }, unsupportedRead());
#else
  static_cast<void>(accountId);
  return readyFuture(unsupportedRead());
#endif
}

std::future<OAuthCredentialSaveResult>
MacOSCredentialAdapter::save(QString accountId, OAuthStoredCredential credential) {
  if (!isValidAccountId(accountId) || !isValidCredential(credential)) {
    return readyFuture(OAuthCredentialSaveResult(
        configurationError(QStringLiteral("OAuth credential input is invalid"))));
  }
#if defined(Q_OS_MACOS)
  return runOnWorker<OAuthCredentialSaveResult>(
      [accountId = std::move(accountId), credential = std::move(credential)] {
        return saveCredential(accountId, credential);
      },
      unsupportedSave());
#else
  static_cast<void>(accountId);
  static_cast<void>(credential);
  return readyFuture(unsupportedSave());
#endif
}

std::future<OAuthCredentialDeleteResult> MacOSCredentialAdapter::erase(QString accountId) {
  if (!isValidAccountId(accountId)) {
    return readyFuture(OAuthCredentialDeleteResult(
        configurationError(QStringLiteral("OAuth credential account identifier is invalid"))));
  }
#if defined(Q_OS_MACOS)
  return runOnWorker<OAuthCredentialDeleteResult>(
      [accountId = std::move(accountId)] { return eraseCredential(accountId); },
      unsupportedDelete());
#else
  static_cast<void>(accountId);
  return readyFuture(unsupportedDelete());
#endif
}

} // namespace hcb
