#include "core/CredentialHealthDiagnostics.h"

#include <future>
#include <memory>
#include <optional>
#include <thread>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumAccountIdLength = 256;
constexpr qsizetype kMaximumTokenLength = 8'192;

[[nodiscard]] AppError validationError(QString message) {
  return AppError(AppErrorCode::Validation, std::move(message));
}

[[nodiscard]] AppError cancelledError(QString message) {
  return AppError(AppErrorCode::Cancelled, std::move(message));
}

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

[[nodiscard]] bool isValidAccountId(const QString& accountId) {
  return !accountId.isEmpty() && accountId == accountId.trimmed() &&
         accountId.size() <= kMaximumAccountIdLength && !accountId.contains(QChar::Null);
}

[[nodiscard]] bool isValidToken(const QString& token) {
  return !token.isEmpty() && token.size() <= kMaximumTokenLength && !token.contains(QChar::Null);
}

[[nodiscard]] CredentialStorageHealth
credentialStorageHealth(const std::optional<OAuthStoredCredential>& credential) {
  if (!credential.has_value()) {
    return CredentialStorageHealth::Missing;
  }
  if (!isValidToken(credential->accessToken) ||
      (credential->refreshToken.has_value() && !isValidToken(*credential->refreshToken))) {
    return CredentialStorageHealth::Invalid;
  }
  return credential->refreshToken.has_value() ? CredentialStorageHealth::Refreshable
                                              : CredentialStorageHealth::AccessOnly;
}

} // namespace

bool CredentialHealthDiagnostic::isReady() const noexcept {
  return connectionState.has_value() && *connectionState == AccountConnectionState::Connected &&
         missingScopes.isEmpty() && credentialStorage == CredentialStorageHealth::Refreshable;
}

CredentialHealthDiagnostics::CredentialHealthDiagnostics(AccountStatusService& accountStatuses,
                                                         OAuthCredentialStore& credentials)
    : accountStatuses_(accountStatuses), credentials_(credentials) {}

std::future<CredentialHealthDiagnosticResult>
CredentialHealthDiagnostics::inspect(QString accountId) {
  if (!isValidAccountId(accountId)) {
    return readyFuture(CredentialHealthDiagnosticResult(
        validationError(QStringLiteral("Account identifier is invalid"))));
  }
  auto completion = std::make_shared<std::promise<CredentialHealthDiagnosticResult>>();
  std::future<CredentialHealthDiagnosticResult> future = completion->get_future();
  try {
    std::thread([this, accountId = std::move(accountId), completion] {
      try {
        const AccountStatusLookupResult accountResult = accountStatuses_.find(accountId).get();
        if (std::holds_alternative<AppError>(accountResult)) {
          completion->set_value(std::get<AppError>(accountResult));
          return;
        }
        const std::optional<AccountStatus> account =
            std::get<std::optional<AccountStatus>>(accountResult);
        if (!account.has_value()) {
          completion->set_value(CredentialHealthDiagnostic{.accountId = accountId});
          return;
        }
        const OAuthCredentialReadResult credentialResult = credentials_.read(accountId).get();
        if (std::holds_alternative<AppError>(credentialResult)) {
          completion->set_value(std::get<AppError>(credentialResult));
          return;
        }
        const std::optional<OAuthStoredCredential> credential =
            std::get<std::optional<OAuthStoredCredential>>(credentialResult);
        completion->set_value(
            CredentialHealthDiagnostic{.accountId = accountId,
                                       .connectionState = account->connectionState,
                                       .missingScopes = account->missingScopes,
                                       .credentialStorage = credentialStorageHealth(credential)});
      } catch (...) {
        completion->set_value(CredentialHealthDiagnosticResult(
            cancelledError(QStringLiteral("Credential health inspection failed"))));
      }
    }).detach();
  } catch (...) {
    completion->set_value(CredentialHealthDiagnosticResult(
        cancelledError(QStringLiteral("Credential health inspection failed"))));
  }
  return future;
}

} // namespace hcb
