#include <QtTest/QTest>

#include "core/OAuthTokenRevocationClient.h"

#include <future>
#include <variant>

class OAuthTokenRevocationClientTest final : public QObject {
  Q_OBJECT

private slots:
  void rejectsInvalidTokenWithoutNetwork();
  void usesGoogleRevocationEndpoint();
  void acceptsSuccessfulAndAlreadyInvalidatedResponses();
  void rejectsFailedResponses();
};

void OAuthTokenRevocationClientTest::rejectsInvalidTokenWithoutNetwork() {
  hcb::OAuthTokenRevocationClient client;
  std::future<hcb::OAuthTokenRevocationResult> future = client.revoke({});

  const hcb::OAuthTokenRevocationResult result = future.get();
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Validation);
}

void OAuthTokenRevocationClientTest::usesGoogleRevocationEndpoint() {
  const QUrl endpoint = hcb::OAuthTokenRevocationClient::defaultRevocationEndpoint();

  QCOMPARE(endpoint.scheme(), QStringLiteral("https"));
  QCOMPARE(endpoint.host(), QStringLiteral("oauth2.googleapis.com"));
  QCOMPARE(endpoint.path(), QStringLiteral("/revoke"));
  QVERIFY(endpoint.query().isEmpty());
  QVERIFY(endpoint.fragment().isEmpty());
}

void OAuthTokenRevocationClientTest::acceptsSuccessfulAndAlreadyInvalidatedResponses() {
  for (const auto& [status, response] :
       {std::pair{200, QByteArray{}},
        std::pair{400, QByteArray("{\"error\":\"invalid_token\"}")}}) {
    const hcb::OAuthTokenRevocationResult result =
        hcb::OAuthTokenRevocationClient::decodeResponse(status, response);
    QVERIFY(std::holds_alternative<std::monostate>(result));
  }
}

void OAuthTokenRevocationClientTest::rejectsFailedResponses() {
  for (const auto& [status, response] :
       {std::pair{400, QByteArray("{\"error\":\"invalid_request\"}")},
        std::pair{500, QByteArray{}},
        std::pair{400, QByteArray("not-json")}}) {
    const hcb::OAuthTokenRevocationResult result =
        hcb::OAuthTokenRevocationClient::decodeResponse(status, response);
    QVERIFY(std::holds_alternative<hcb::AppError>(result));
    QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Network);
  }
}

QTEST_GUILESS_MAIN(OAuthTokenRevocationClientTest)

#include "OAuthTokenRevocationClientTest.moc"
