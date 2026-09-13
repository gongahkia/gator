#include "core/OAuthBrowserAuthorizationLauncher.h"

#include <QDesktopServices>

#include <utility>

namespace hcb {
namespace {

[[nodiscard]] AppError validationError(QString message) {
  return AppError(AppErrorCode::Validation, std::move(message));
}

[[nodiscard]] AppError networkError(QString message) {
  return AppError(AppErrorCode::Network, std::move(message));
}

[[nodiscard]] bool defaultOpenUrl(const QUrl& url) { return QDesktopServices::openUrl(url); }

} // namespace

OAuthBrowserAuthorizationLauncher::OAuthBrowserAuthorizationLauncher(OAuthBrowserUrlOpener opener)
    : opener_(opener ? std::move(opener) : OAuthBrowserUrlOpener(defaultOpenUrl)) {}

OAuthBrowserAuthorizationLaunchResult
OAuthBrowserAuthorizationLauncher::launch(const QUrl& authorizationUrl) const {
  if (!isAllowedAuthorizationUrl(authorizationUrl)) {
    return validationError(QStringLiteral("OAuth authorization URL is not allowed"));
  }
  if (!opener_(authorizationUrl)) {
    return networkError(QStringLiteral("OAuth authorization URL could not be opened"));
  }
  return std::monostate{};
}

bool OAuthBrowserAuthorizationLauncher::isAllowedAuthorizationUrl(const QUrl& authorizationUrl) {
  return authorizationUrl.isValid() && authorizationUrl.scheme() == QStringLiteral("https") &&
         authorizationUrl.host().compare(QStringLiteral("accounts.google.com"),
                                         Qt::CaseInsensitive) == 0 &&
         (authorizationUrl.port() == -1 || authorizationUrl.port() == 443) &&
         authorizationUrl.userInfo().isEmpty() && authorizationUrl.fragment().isEmpty();
}

} // namespace hcb
