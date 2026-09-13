#pragma once

#include "core/OAuthTokenRevoker.h"

#include <QByteArray>
#include <QNetworkAccessManager>
#include <QUrl>

namespace hcb {

class OAuthTokenRevocationClient final : public QObject, public OAuthTokenRevoker {
  Q_OBJECT

public:
  explicit OAuthTokenRevocationClient(QObject* parent = nullptr,
                                      QNetworkAccessManager* manager = nullptr);

  [[nodiscard]] std::future<OAuthTokenRevocationResult> revoke(QString token) override;
  [[nodiscard]] static QUrl defaultRevocationEndpoint();
  [[nodiscard]] static OAuthTokenRevocationResult decodeResponse(int status,
                                                                 const QByteArray& responseBody);

private:
  QNetworkAccessManager* manager_;
};

} // namespace hcb
