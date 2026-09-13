#include <QtTest/QTest>

#include "core/GoogleHttpClient.h"
#include "core/OAuthTokenExchangeClient.h"
#include "core/OAuthTokenRefreshClient.h"
#include "core/OAuthTokenRevocationClient.h"
#include "support/MockNetworkAccessManager.h"

#include <QNetworkRequest>
#include <QUrlQuery>

#include <chrono>
#include <optional>
#include <variant>

namespace {

using hcb::test::CapturedNetworkRequest;
using hcb::test::MockNetworkAccessManager;
using hcb::test::MockNetworkResponse;

[[nodiscard]] QUrl loopbackRedirectUri() {
  return QUrl(QStringLiteral("http://127.0.0.1:38421/oauth/google/callback"));
}

[[nodiscard]] QString validVerifier() { return QString(43, u'a'); }

} // namespace

class MockedOAuthTransportTest final : public QObject {
  Q_OBJECT

private slots:
  void exchangesAuthorizationCodeThroughMock();
  void reportsTokenExchangeHttpErrorCode();
  void refreshesAccessTokenThroughMock();
  void reportsTokenRefreshHttpErrorCode();
  void revokesTokenThroughMock();
  void sendsGoogleTransportRequestThroughMock();
};

void MockedOAuthTransportTest::exchangesAuthorizationCodeThroughMock() {
  MockNetworkAccessManager manager;
  manager.enqueue(
      {.body = QByteArray(
           "{\"access_token\":\"access\",\"refresh_token\":\"refresh\",\"expires_in\":3600}")});
  hcb::OAuthTokenExchangeClient client(
      nullptr, hcb::OAuthTokenExchangeClient::defaultTokenEndpoint(), &manager);

  std::future<hcb::OAuthTokenExchangeResult> future =
      client.exchange({.code = QStringLiteral("authorization-code"),
                       .codeVerifier = validVerifier(),
                       .redirectUri = loopbackRedirectUri(),
                       .clientId = QStringLiteral("client-id-12345"),
                       .clientSecret = QStringLiteral("client-secret")});

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::OAuthTokenExchangeResult result = future.get();
  QVERIFY(std::holds_alternative<hcb::OAuthTokenSet>(result));
  QCOMPARE(std::get<hcb::OAuthTokenSet>(result).accessToken, QStringLiteral("access"));
  QCOMPARE(manager.requests().size(), 1);
  const CapturedNetworkRequest& request = manager.requests().front();
  QCOMPARE(request.operation, QNetworkAccessManager::PostOperation);
  QCOMPARE(request.request.url(), hcb::OAuthTokenExchangeClient::defaultTokenEndpoint());
  QCOMPARE(request.request.header(QNetworkRequest::ContentTypeHeader).toString(),
           QStringLiteral("application/x-www-form-urlencoded"));
  const QUrlQuery form(QString::fromUtf8(request.body));
  QCOMPARE(form.queryItemValue(QStringLiteral("grant_type")), QStringLiteral("authorization_code"));
  QCOMPARE(form.queryItemValue(QStringLiteral("code")), QStringLiteral("authorization-code"));
  QCOMPARE(form.queryItemValue(QStringLiteral("code_verifier")), validVerifier());
  QCOMPARE(form.queryItemValue(QStringLiteral("client_secret")), QStringLiteral("client-secret"));
}

void MockedOAuthTransportTest::reportsTokenExchangeHttpErrorCode() {
  MockNetworkAccessManager manager;
  manager.enqueue({.status = 400,
                   .body = QByteArray("{\"error\":\"invalid_grant\",\"error_description\":\"Code verifier is required\"}"),
                   .error = QNetworkReply::ContentAccessDenied});
  hcb::OAuthTokenExchangeClient client(
      nullptr, hcb::OAuthTokenExchangeClient::defaultTokenEndpoint(), &manager);

  std::future<hcb::OAuthTokenExchangeResult> future =
      client.exchange({.code = QStringLiteral("authorization-code"),
                       .codeVerifier = validVerifier(),
                       .redirectUri = loopbackRedirectUri(),
                       .clientId = QStringLiteral("client-id-12345")});

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::OAuthTokenExchangeResult result = future.get();
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).message(),
           QStringLiteral(
               "OAuth token exchange failed (HTTP 400: invalid_grant — Code verifier is required)"));
}

void MockedOAuthTransportTest::refreshesAccessTokenThroughMock() {
  MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"access_token\":\"refreshed\",\"scope\":\"tasks\"}")});
  hcb::OAuthTokenRefreshClient client(nullptr, &manager);

  std::future<hcb::OAuthTokenRefreshResult> future = client.refresh(
      {.clientId = QStringLiteral("client-id-12345"), .refreshToken = QStringLiteral("refresh")});

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::OAuthTokenRefreshResult result = future.get();
  QVERIFY(std::holds_alternative<hcb::OAuthRefreshedToken>(result));
  QCOMPARE(std::get<hcb::OAuthRefreshedToken>(result).accessToken, QStringLiteral("refreshed"));
  QCOMPARE(manager.requests().size(), 1);
  const CapturedNetworkRequest& request = manager.requests().front();
  QCOMPARE(request.operation, QNetworkAccessManager::PostOperation);
  QCOMPARE(request.request.url(), hcb::OAuthTokenExchangeClient::defaultTokenEndpoint());
  const QUrlQuery form(QString::fromUtf8(request.body));
  QCOMPARE(form.queryItemValue(QStringLiteral("grant_type")), QStringLiteral("refresh_token"));
  QCOMPARE(form.queryItemValue(QStringLiteral("refresh_token")), QStringLiteral("refresh"));
}

void MockedOAuthTransportTest::reportsTokenRefreshHttpErrorCode() {
  MockNetworkAccessManager manager;
  manager.enqueue({.status = 400,
                   .body = QByteArray("{\"error\":\"invalid_client\",\"error_description\":\"Client secret is invalid\"}"),
                   .error = QNetworkReply::ContentAccessDenied});
  hcb::OAuthTokenRefreshClient client(nullptr, &manager);

  std::future<hcb::OAuthTokenRefreshResult> future = client.refresh(
      {.clientId = QStringLiteral("client-id-12345"), .refreshToken = QStringLiteral("refresh")});

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::OAuthTokenRefreshResult result = future.get();
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).message(),
           QStringLiteral(
               "OAuth token refresh failed (HTTP 400: invalid_client — Client secret is invalid)"));
}

void MockedOAuthTransportTest::revokesTokenThroughMock() {
  MockNetworkAccessManager manager;
  manager.enqueue({.status = 400,
                   .body = QByteArray("{\"error\":\"invalid_token\"}"),
                   .error = QNetworkReply::ContentAccessDenied});
  hcb::OAuthTokenRevocationClient client(nullptr, &manager);

  std::future<hcb::OAuthTokenRevocationResult> future = client.revoke(QStringLiteral("token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  QVERIFY(std::holds_alternative<std::monostate>(future.get()));
  QCOMPARE(manager.requests().size(), 1);
  const CapturedNetworkRequest& request = manager.requests().front();
  QCOMPARE(request.operation, QNetworkAccessManager::PostOperation);
  QCOMPARE(request.request.url(), hcb::OAuthTokenRevocationClient::defaultRevocationEndpoint());
  const QUrlQuery form(QString::fromUtf8(request.body));
  QCOMPARE(form.queryItemValue(QStringLiteral("token")), QStringLiteral("token"));
}

void MockedOAuthTransportTest::sendsGoogleTransportRequestThroughMock() {
  MockNetworkAccessManager manager;
  manager.enqueue({.status = 429,
                   .body = QByteArray("{\"error\":{\"reason\":\"quotaExceeded\"}}"),
                   .error = QNetworkReply::UnknownServerError,
                   .headers = {{QByteArray("Retry-After"), QByteArray("3")}}});
  hcb::GoogleHttpClient client(nullptr, &manager);

  std::future<hcb::GoogleHttpResult> future =
      client.send({.method = hcb::GoogleHttpMethod::Patch,
                   .path = QStringLiteral("/tasks/v1/lists/list-id/tasks/task-id"),
                   .query = {{.name = QStringLiteral("alt"), .value = QStringLiteral("json")}},
                   .body = QByteArray("{\"title\":\"updated\"}"),
                   .ifMatch = QStringLiteral("etag")},
                  QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleHttpResult result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  const hcb::GoogleApiError& error = std::get<hcb::GoogleApiError>(result);
  QCOMPARE(error.kind(), hcb::GoogleApiErrorKind::RateLimited);
  QCOMPARE(error.retryAfterMilliseconds(), std::optional<qint64>(3'000));
  QCOMPARE(manager.requests().size(), 1);
  const CapturedNetworkRequest& request = manager.requests().front();
  QCOMPARE(request.operation, QNetworkAccessManager::CustomOperation);
  QCOMPARE(request.request.url(),
           QUrl(QStringLiteral(
               "https://www.googleapis.com/tasks/v1/lists/list-id/tasks/task-id?alt=json")));
  QCOMPARE(request.request.rawHeader("Authorization"), QByteArray("Bearer access-token"));
  QCOMPARE(request.request.rawHeader("If-Match"), QByteArray("etag"));
  QCOMPARE(request.request.header(QNetworkRequest::ContentTypeHeader).toString(),
           QStringLiteral("application/json"));
  QCOMPARE(request.body, QByteArray("{\"title\":\"updated\"}"));
  QCOMPARE(request.request.attribute(QNetworkRequest::RedirectPolicyAttribute).toInt(),
           static_cast<int>(QNetworkRequest::ManualRedirectPolicy));
}

QTEST_GUILESS_MAIN(MockedOAuthTransportTest)

#include "MockedOAuthTransportTest.moc"
