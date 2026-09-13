#include "app/WindowsCredentialAdapter.h"

#include <QCryptographicHash>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonParseError>

#if defined(Q_OS_WIN)
#include <windows.h>
#include <wincred.h>
#endif

#include <future>
#include <optional>
#include <string>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumAccountIdLength = 256;
constexpr qsizetype kMaximumTokenLength = 8'192;

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
  return configurationError(
      QStringLiteral("Windows Credential Manager is unavailable on this platform"));
}

[[nodiscard]] OAuthCredentialSaveResult unsupportedSave() {
  return configurationError(
      QStringLiteral("Windows Credential Manager is unavailable on this platform"));
}

[[nodiscard]] OAuthCredentialDeleteResult unsupportedDelete() {
  return configurationError(
      QStringLiteral("Windows Credential Manager is unavailable on this platform"));
}

#if defined(Q_OS_WIN)

constexpr char kCredentialTargetPrefix[] = "dev.hotcrossbuns.native.oauth.";

class CredentialGuard final {
public:
  explicit CredentialGuard(PCREDENTIALW credential) : credential_(credential) {}
  CredentialGuard(const CredentialGuard&) = delete;
  CredentialGuard& operator=(const CredentialGuard&) = delete;
  ~CredentialGuard() {
    if (credential_ != nullptr) {
      CredFree(credential_);
    }
  }

private:
  PCREDENTIALW credential_;
};

[[nodiscard]] std::wstring credentialTarget(const QString& accountId) {
  const QByteArray digest =
      QCryptographicHash::hash(accountId.toUtf8(), QCryptographicHash::Sha256).toHex();
  return QString::fromLatin1(kCredentialTargetPrefix)
      .append(QString::fromLatin1(digest))
      .toStdWString();
}

[[nodiscard]] QByteArray credentialPayload(const OAuthStoredCredential& credential) {
  QJsonObject object;
  object.insert(QStringLiteral("accessToken"), credential.accessToken);
  if (credential.refreshToken.has_value()) {
    object.insert(QStringLiteral("refreshToken"), *credential.refreshToken);
  }
  return QJsonDocument(object).toJson(QJsonDocument::Compact);
}

[[nodiscard]] OAuthCredentialReadResult decodeCredential(const QByteArray& payload) {
  QJsonParseError parseError;
  const QJsonDocument document = QJsonDocument::fromJson(payload, &parseError);
  if (parseError.error != QJsonParseError::NoError || !document.isObject()) {
    return configurationError(QStringLiteral("Windows Credential Manager credential is invalid"));
  }
  const QJsonObject object = document.object();
  const QJsonValue accessToken = object.value(QStringLiteral("accessToken"));
  const QJsonValue refreshToken = object.value(QStringLiteral("refreshToken"));
  if (!accessToken.isString() || (!refreshToken.isUndefined() && !refreshToken.isString())) {
    return configurationError(QStringLiteral("Windows Credential Manager credential is invalid"));
  }
  OAuthStoredCredential credential{.accessToken = accessToken.toString()};
  if (refreshToken.isString()) {
    credential.refreshToken = refreshToken.toString();
  }
  if (!isValidCredential(credential)) {
    return configurationError(QStringLiteral("Windows Credential Manager credential is invalid"));
  }
  return std::optional<OAuthStoredCredential>(std::move(credential));
}

[[nodiscard]] AppError credentialManagerError(DWORD error) {
  return error == ERROR_NO_SUCH_LOGON_SESSION
             ? configurationError(QStringLiteral(
                   "Windows Credential Manager is unavailable for this logon session"))
             : configurationError(QStringLiteral("Windows Credential Manager operation failed"));
}

[[nodiscard]] OAuthCredentialReadResult readCredential(const QString& accountId) {
  const std::wstring target = credentialTarget(accountId);
  PCREDENTIALW rawCredential = nullptr;
  if (!CredReadW(target.c_str(), CRED_TYPE_GENERIC, 0, &rawCredential)) {
    const DWORD error = GetLastError();
    return error == ERROR_NOT_FOUND
               ? OAuthCredentialReadResult(std::optional<OAuthStoredCredential>{})
               : OAuthCredentialReadResult(credentialManagerError(error));
  }
  const CredentialGuard credential(rawCredential);
  if (rawCredential->CredentialBlob == nullptr || rawCredential->CredentialBlobSize == 0 ||
      rawCredential->CredentialBlobSize > CRED_MAX_CREDENTIAL_BLOB_SIZE) {
    return configurationError(QStringLiteral("Windows Credential Manager credential is invalid"));
  }
  const QByteArray payload(reinterpret_cast<const char*>(rawCredential->CredentialBlob),
                           static_cast<qsizetype>(rawCredential->CredentialBlobSize));
  return decodeCredential(payload);
}

[[nodiscard]] OAuthCredentialSaveResult saveCredential(const QString& accountId,
                                                       const OAuthStoredCredential& credential) {
  std::wstring target = credentialTarget(accountId);
  QByteArray payload = credentialPayload(credential);
  if (payload.isEmpty() || payload.size() > static_cast<qsizetype>(CRED_MAX_CREDENTIAL_BLOB_SIZE)) {
    return configurationError(QStringLiteral("Windows Credential Manager credential is too large"));
  }
  CREDENTIALW value{};
  value.Type = CRED_TYPE_GENERIC;
  value.TargetName = target.data();
  value.CredentialBlobSize = static_cast<DWORD>(payload.size());
  value.CredentialBlob = reinterpret_cast<LPBYTE>(payload.data());
  value.Persist = CRED_PERSIST_LOCAL_MACHINE;
  return CredWriteW(&value, 0) != FALSE
             ? OAuthCredentialSaveResult(std::monostate{})
             : OAuthCredentialSaveResult(credentialManagerError(GetLastError()));
}

[[nodiscard]] OAuthCredentialDeleteResult eraseCredential(const QString& accountId) {
  const std::wstring target = credentialTarget(accountId);
  if (CredDeleteW(target.c_str(), CRED_TYPE_GENERIC, 0) != FALSE ||
      GetLastError() == ERROR_NOT_FOUND) {
    return std::monostate{};
  }
  return credentialManagerError(GetLastError());
}

#endif

} // namespace

std::future<OAuthCredentialReadResult> WindowsCredentialAdapter::read(QString accountId) {
  if (!isValidAccountId(accountId)) {
    return readyFuture(OAuthCredentialReadResult(
        configurationError(QStringLiteral("OAuth credential account identifier is invalid"))));
  }
#if defined(Q_OS_WIN)
  return runOnWorker<OAuthCredentialReadResult>(
      [accountId = std::move(accountId)] { return readCredential(accountId); }, unsupportedRead());
#else
  static_cast<void>(accountId);
  return readyFuture(unsupportedRead());
#endif
}

std::future<OAuthCredentialSaveResult>
WindowsCredentialAdapter::save(QString accountId, OAuthStoredCredential credential) {
  if (!isValidAccountId(accountId) || !isValidCredential(credential)) {
    return readyFuture(OAuthCredentialSaveResult(
        configurationError(QStringLiteral("OAuth credential input is invalid"))));
  }
#if defined(Q_OS_WIN)
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

std::future<OAuthCredentialDeleteResult> WindowsCredentialAdapter::erase(QString accountId) {
  if (!isValidAccountId(accountId)) {
    return readyFuture(OAuthCredentialDeleteResult(
        configurationError(QStringLiteral("OAuth credential account identifier is invalid"))));
  }
#if defined(Q_OS_WIN)
  return runOnWorker<OAuthCredentialDeleteResult>(
      [accountId = std::move(accountId)] { return eraseCredential(accountId); },
      unsupportedDelete());
#else
  static_cast<void>(accountId);
  return readyFuture(unsupportedDelete());
#endif
}

} // namespace hcb
