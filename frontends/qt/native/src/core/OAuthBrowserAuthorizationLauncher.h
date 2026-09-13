#pragma once

#include "core/AppError.h"

#include <QUrl>

#include <functional>
#include <variant>

namespace hcb {

using OAuthBrowserAuthorizationLaunchResult = std::variant<std::monostate, AppError>;
using OAuthBrowserUrlOpener = std::function<bool(const QUrl&)>;

class OAuthBrowserAuthorizationLauncher final {
public:
  explicit OAuthBrowserAuthorizationLauncher(OAuthBrowserUrlOpener opener = {});

  [[nodiscard]] OAuthBrowserAuthorizationLaunchResult launch(const QUrl& authorizationUrl) const;
  [[nodiscard]] static bool isAllowedAuthorizationUrl(const QUrl& authorizationUrl);

private:
  OAuthBrowserUrlOpener opener_;
};

} // namespace hcb
