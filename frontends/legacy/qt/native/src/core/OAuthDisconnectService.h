#pragma once

#include "core/AccountStatusService.h"
#include "core/OAuthCredentialStore.h"
#include "core/OAuthTokenRevoker.h"

#include <future>
#include <cstdint>
#include <optional>
#include <variant>

namespace hcb {

enum class OAuthRemoteRevocationState : std::uint8_t {
  NotAttempted,
  Revoked,
  Failed
};

struct OAuthDisconnectResult final {
  AccountStatusSaveResult account;
  OAuthRemoteRevocationState remoteRevocationState;
  std::optional<AppError> remoteRevocationError;
};

using OAuthDisconnectResultOrError = std::variant<OAuthDisconnectResult, AppError>;

class OAuthDisconnectService final {
public:
  OAuthDisconnectService(AccountStatusService& accountStatuses,
                         OAuthCredentialStore& credentials,
                         OAuthTokenRevoker& revoker);

  [[nodiscard]] std::future<OAuthDisconnectResultOrError> disconnect(QString accountId);

private:
  AccountStatusService& accountStatuses_;
  OAuthCredentialStore& credentials_;
  OAuthTokenRevoker& revoker_;
};

} // namespace hcb
