#pragma once

#include "core/AppError.h"

#include <QNetworkAccessManager>

#include <future>
#include <optional>
#include <variant>

namespace hcb {

struct OAuthTokenRefreshRequest final {
  QString clientId;
  QString refreshToken;
  std::optional<QString> clientSecret;
};

struct OAuthRefreshedToken final {
  QString accessToken;
  std::optional<int> expiresInSeconds;
  std::optional<QString> scope;
  std::optional<QString> tokenType;
};

using OAuthTokenRefreshResult = std::variant<OAuthRefreshedToken, AppError>;

class OAuthTokenRefreshClient final : public QObject {
  Q_OBJECT

public:
  explicit OAuthTokenRefreshClient(QObject* parent = nullptr,
                                   QNetworkAccessManager* manager = nullptr);

  [[nodiscard]] std::future<OAuthTokenRefreshResult> refresh(OAuthTokenRefreshRequest request);
  [[nodiscard]] static OAuthTokenRefreshResult decodeTokenResponse(const QByteArray& responseBody);

private:
  QNetworkAccessManager* manager_;
};

} // namespace hcb
