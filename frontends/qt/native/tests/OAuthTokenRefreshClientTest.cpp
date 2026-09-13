#include <QtTest/QTest>

#include "core/OAuthTokenRefreshClient.h"

#include <future>
#include <variant>

class OAuthTokenRefreshClientTest final : public QObject {
  Q_OBJECT

private slots:
  void decodesRotatedAccessToken();
  void rejectsInvalidRefreshInputWithoutNetwork();
};

void OAuthTokenRefreshClientTest::decodesRotatedAccessToken() {
  const hcb::OAuthTokenRefreshResult result = hcb::OAuthTokenRefreshClient::decodeTokenResponse(
      R"({"access_token":"rotated-access-token","expires_in":1800,"scope":"openid email","token_type":"Bearer"})");

  QVERIFY(std::holds_alternative<hcb::OAuthRefreshedToken>(result));
  const hcb::OAuthRefreshedToken& token = std::get<hcb::OAuthRefreshedToken>(result);
  QCOMPARE(token.accessToken, QStringLiteral("rotated-access-token"));
  QCOMPARE(token.expiresInSeconds, std::optional<int>(1800));
  QCOMPARE(token.scope, std::optional<QString>(QStringLiteral("openid email")));
}

void OAuthTokenRefreshClientTest::rejectsInvalidRefreshInputWithoutNetwork() {
  hcb::OAuthTokenRefreshClient client;
  std::future<hcb::OAuthTokenRefreshResult> future = client.refresh(
      {.clientId = QStringLiteral("client-id"), .refreshToken = QStringLiteral("refresh-token")});

  const hcb::OAuthTokenRefreshResult result = future.get();
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Validation);
}

QTEST_GUILESS_MAIN(OAuthTokenRefreshClientTest)

#include "OAuthTokenRefreshClientTest.moc"
