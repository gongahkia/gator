#pragma once

#include "core/AppError.h"

#include <QNetworkAccessManager>
#include <QUrl>

#include <future>
#include <optional>
#include <variant>

namespace hcb {

struct OAuthTokenExchangeRequest final {
  QString code;
  QString codeVerifier;
  QUrl redirectUri;
  QString clientId;
  std::optional<QString> clientSecret;
};

struct OAuthTokenSet final {
  QString accessToken;
  std::optional<QString> refreshToken;
  std::optional<int> expiresInSeconds;
  std::optional<QString> scope;
  std::optional<QString> tokenType;
};

using OAuthTokenExchangeResult = std::variant<OAuthTokenSet, AppError>;

class OAuthTokenExchangeClient final : public QObject {
  Q_OBJECT

public:
  explicit OAuthTokenExchangeClient(QObject* parent = nullptr,
                                    QUrl tokenEndpoint = defaultTokenEndpoint(),
                                    QNetworkAccessManager* manager = nullptr);

  [[nodiscard]] std::future<OAuthTokenExchangeResult> exchange(OAuthTokenExchangeRequest request);
  [[nodiscard]] static QUrl defaultTokenEndpoint();
  [[nodiscard]] static OAuthTokenExchangeResult decodeTokenResponse(const QByteArray& responseBody);

private:
  QNetworkAccessManager* manager_;
  QUrl tokenEndpoint_;
};

} // namespace hcb
