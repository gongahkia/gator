#pragma once

#include "core/AppError.h"

#include <QString>

#include <future>
#include <optional>
#include <variant>

namespace hcb {

struct OAuthStoredCredential final {
  QString accessToken;
  std::optional<QString> refreshToken;
};

using OAuthCredentialReadResult = std::variant<std::optional<OAuthStoredCredential>, AppError>;
using OAuthCredentialSaveResult = std::variant<std::monostate, AppError>;
using OAuthCredentialDeleteResult = std::variant<std::monostate, AppError>;

class OAuthCredentialStore {
public:
  virtual ~OAuthCredentialStore() = default;

  [[nodiscard]] virtual std::future<OAuthCredentialReadResult> read(QString accountId) = 0;
  [[nodiscard]] virtual std::future<OAuthCredentialSaveResult>
  save(QString accountId, OAuthStoredCredential credential) = 0;
  [[nodiscard]] virtual std::future<OAuthCredentialDeleteResult> erase(QString accountId) = 0;
};

} // namespace hcb
