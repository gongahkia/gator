#include "core/OAuthDisconnectService.h"

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

[[nodiscard]] std::optional<QString>
tokenForRevocation(const std::optional<OAuthStoredCredential>& credential) {
  if (!credential.has_value()) {
    return std::nullopt;
  }
  if (credential->refreshToken.has_value() && isValidToken(*credential->refreshToken)) {
    return credential->refreshToken;
  }
  return isValidToken(credential->accessToken) ? std::optional<QString>(credential->accessToken)
                                               : std::nullopt;
}

} // namespace

OAuthDisconnectService::OAuthDisconnectService(AccountStatusService& accountStatuses,
                                               OAuthCredentialStore& credentials,
                                               OAuthTokenRevoker& revoker)
    : accountStatuses_(accountStatuses), credentials_(credentials), revoker_(revoker) {}

std::future<OAuthDisconnectResultOrError> OAuthDisconnectService::disconnect(QString accountId) {
  if (!isValidAccountId(accountId)) {
    return readyFuture(OAuthDisconnectResultOrError(
        validationError(QStringLiteral("Account identifier is invalid"))));
  }
  auto completion = std::make_shared<std::promise<OAuthDisconnectResultOrError>>();
  std::future<OAuthDisconnectResultOrError> future = completion->get_future();
  try {
    std::thread([this, accountId = std::move(accountId), completion] {
      try {
        const OAuthCredentialReadResult credential = credentials_.read(accountId).get();
        if (std::holds_alternative<AppError>(credential)) {
          completion->set_value(std::get<AppError>(credential));
          return;
        }
        OAuthRemoteRevocationState revocationState = OAuthRemoteRevocationState::NotAttempted;
        std::optional<AppError> revocationError;
        if (const std::optional<QString> token =
                tokenForRevocation(std::get<std::optional<OAuthStoredCredential>>(credential));
            token.has_value()) {
          const OAuthTokenRevocationResult revocation = revoker_.revoke(*token).get();
          if (std::holds_alternative<AppError>(revocation)) {
            revocationState = OAuthRemoteRevocationState::Failed;
            revocationError = std::get<AppError>(revocation);
          } else {
            revocationState = OAuthRemoteRevocationState::Revoked;
          }
        }
        const OAuthCredentialDeleteResult erased = credentials_.erase(accountId).get();
        if (std::holds_alternative<AppError>(erased)) {
          completion->set_value(std::get<AppError>(erased));
          return;
        }
        const AccountStatusSaveResultOrError account = accountStatuses_.disconnect(accountId).get();
        if (std::holds_alternative<AppError>(account)) {
          completion->set_value(std::get<AppError>(account));
          return;
        }
        completion->set_value(
            OAuthDisconnectResult{.account = std::get<AccountStatusSaveResult>(account),
                                  .remoteRevocationState = revocationState,
                                  .remoteRevocationError = std::move(revocationError)});
      } catch (...) {
        completion->set_value(OAuthDisconnectResultOrError(
            cancelledError(QStringLiteral("OAuth disconnect operation failed"))));
      }
    }).detach();
  } catch (...) {
    completion->set_value(OAuthDisconnectResultOrError(
        cancelledError(QStringLiteral("OAuth disconnect operation failed"))));
  }
  return future;
}

} // namespace hcb
