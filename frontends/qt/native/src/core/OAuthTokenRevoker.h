#pragma once

#include "core/AppError.h"

#include <QString>

#include <future>
#include <variant>

namespace hcb {

using OAuthTokenRevocationResult = std::variant<std::monostate, AppError>;

class OAuthTokenRevoker {
public:
  virtual ~OAuthTokenRevoker() = default;

  [[nodiscard]] virtual std::future<OAuthTokenRevocationResult> revoke(QString token) = 0;
};

} // namespace hcb
