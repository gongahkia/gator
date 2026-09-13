#include <QtTest/QTest>

#include "core/OAuthTokenExchangeClient.h"

#include <variant>

class OAuthTokenExchangeClientTest final : public QObject {
  Q_OBJECT

private slots:
  void decodesTokenResponse();
  void rejectsInvalidTokenResponses();
};

void OAuthTokenExchangeClientTest::decodesTokenResponse() {
  const hcb::OAuthTokenExchangeResult result = hcb::OAuthTokenExchangeClient::decodeTokenResponse(
      R"({"access_token":"access-token","refresh_token":"refresh-token","expires_in":3600,"scope":"openid email","token_type":"Bearer"})");

  QVERIFY(std::holds_alternative<hcb::OAuthTokenSet>(result));
  const hcb::OAuthTokenSet& tokens = std::get<hcb::OAuthTokenSet>(result);
  QCOMPARE(tokens.accessToken, QStringLiteral("access-token"));
  QCOMPARE(tokens.refreshToken, std::optional<QString>(QStringLiteral("refresh-token")));
  QCOMPARE(tokens.expiresInSeconds, std::optional<int>(3600));
}

void OAuthTokenExchangeClientTest::rejectsInvalidTokenResponses() {
  for (const QByteArray& response :
       {QByteArray(),
        QByteArray("[]"),
        QByteArray("{}"),
        QByteArray("{\"access_token\":\"token\",\"expires_in\":0}"),
        QByteArray("{\"access_token\":\"token\",\"refresh_token\":1}")}) {
    const hcb::OAuthTokenExchangeResult result =
        hcb::OAuthTokenExchangeClient::decodeTokenResponse(response);
    QVERIFY(std::holds_alternative<hcb::AppError>(result));
    QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Network);
  }
}

QTEST_GUILESS_MAIN(OAuthTokenExchangeClientTest)

#include "OAuthTokenExchangeClientTest.moc"
