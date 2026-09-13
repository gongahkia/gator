#include <QtTest/QTest>

#include <QUrlQuery>

#include "core/GoogleHttpClient.h"

#include <future>
#include <optional>
#include <variant>

class GoogleHttpClientTest final : public QObject {
  Q_OBJECT

private slots:
  void buildsGoogleApiUrls();
  void rejectsUnsafeRequestPaths();
  void preservesSuccessfulResponseMetadata();
  void mapsFailedResponsesAndRetryAfter();
  void parsesHttpDateRetryAfter();
  void rejectsInvalidInputWithoutNetwork();
  void rejectsInvalidTimeoutWithoutNetwork();
};

void GoogleHttpClientTest::buildsGoogleApiUrls() {
  const std::optional<QUrl> url = hcb::GoogleHttpClient::buildUrl(
      {.path = QStringLiteral("/tasks/v1/users/@me/lists"),
       .query = {{.name = QStringLiteral("fields"), .value = QStringLiteral("items(id,title)")},
                 {.name = QStringLiteral("showCompleted"), .value = QStringLiteral("true")}}});

  QVERIFY(url.has_value());
  if (!url.has_value()) {
    return;
  }
  QCOMPARE(url->scheme(), QStringLiteral("https"));
  QCOMPARE(url->host(), QStringLiteral("www.googleapis.com"));
  QCOMPARE(url->path(), QStringLiteral("/tasks/v1/users/@me/lists"));
  const QUrlQuery query(*url);
  QCOMPARE(query.queryItemValue(QStringLiteral("fields")), QStringLiteral("items(id,title)"));
  QCOMPARE(query.queryItemValue(QStringLiteral("showCompleted")), QStringLiteral("true"));
}

void GoogleHttpClientTest::rejectsUnsafeRequestPaths() {
  for (const QString& path : {QString(),
                              QStringLiteral("tasks/v1/lists"),
                              QStringLiteral("//example.invalid/path"),
                              QStringLiteral("/tasks/v1/lists?alt=json"),
                              QStringLiteral("/tasks/v1/lists#fragment"),
                              QStringLiteral("/tasks\\v1\\lists")}) {
    QVERIFY(!hcb::GoogleHttpClient::buildUrl({.path = path}).has_value());
  }
}

void GoogleHttpClientTest::preservesSuccessfulResponseMetadata() {
  const hcb::GoogleHttpResult result = hcb::GoogleHttpClient::decodeResponse(
      200, QByteArray("{\"items\":[]}"), {}, QByteArray("Wed, 24 Jul 2024 12:00:00 GMT"));

  QVERIFY(std::holds_alternative<hcb::GoogleHttpResponse>(result));
  const hcb::GoogleHttpResponse& response = std::get<hcb::GoogleHttpResponse>(result);
  QCOMPARE(response.status, 200);
  QCOMPARE(response.body, QByteArray("{\"items\":[]}"));
  QCOMPARE(response.serverDate,
           std::optional<QString>(QStringLiteral("Wed, 24 Jul 2024 12:00:00 GMT")));
}

void GoogleHttpClientTest::mapsFailedResponsesAndRetryAfter() {
  const hcb::GoogleHttpResult result = hcb::GoogleHttpClient::decodeResponse(
      429,
      QByteArray("{\"error\":{\"reason\":\"quotaExceeded\"}}"),
      QByteArray("3"),
      {},
      QDateTime::fromString(QStringLiteral("2024-07-24T12:00:00Z"), Qt::ISODate));

  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  const hcb::GoogleApiError& error = std::get<hcb::GoogleApiError>(result);
  QCOMPARE(error.kind(), hcb::GoogleApiErrorKind::RateLimited);
  QCOMPARE(error.retryAfterMilliseconds(), std::optional<qint64>(3'000));
  QVERIFY(error.quotaExceeded());
}

void GoogleHttpClientTest::parsesHttpDateRetryAfter() {
  const QDateTime now = QDateTime::fromString(QStringLiteral("2024-07-24T12:00:00Z"), Qt::ISODate);
  const hcb::GoogleHttpResult result = hcb::GoogleHttpClient::decodeResponse(
      503, QByteArray("{}"), QByteArray("Wed, 24 Jul 2024 12:00:03 GMT"), {}, now);

  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  QCOMPARE(std::get<hcb::GoogleApiError>(result).retryAfterMilliseconds(),
           std::optional<qint64>(3'000));
}

void GoogleHttpClientTest::rejectsInvalidInputWithoutNetwork() {
  hcb::GoogleHttpClient client;
  std::future<hcb::GoogleHttpResult> future =
      client.send({.path = QStringLiteral("/tasks/v1/users/@me/lists")}, {});

  const hcb::GoogleHttpResult result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  QCOMPARE(std::get<hcb::GoogleApiError>(result).kind(), hcb::GoogleApiErrorKind::InvalidPayload);

  std::future<hcb::GoogleHttpResult> bodyFuture =
      client.send({.path = QStringLiteral("/tasks/v1/users/@me/lists"), .body = QByteArray("{}")},
                  QStringLiteral("access-token"));
  const hcb::GoogleHttpResult bodyResult = bodyFuture.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(bodyResult));
  QCOMPARE(std::get<hcb::GoogleApiError>(bodyResult).kind(),
           hcb::GoogleApiErrorKind::InvalidPayload);
}

void GoogleHttpClientTest::rejectsInvalidTimeoutWithoutNetwork() {
  hcb::GoogleHttpClient client;
  std::future<hcb::GoogleHttpResult> future = client.send(
      {.path = QStringLiteral("/tasks/v1/users/@me/lists"), .timeoutMilliseconds = 0},
      QStringLiteral("access-token"));
  const hcb::GoogleHttpResult result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  QCOMPARE(std::get<hcb::GoogleApiError>(result).kind(), hcb::GoogleApiErrorKind::InvalidPayload);
}

QTEST_GUILESS_MAIN(GoogleHttpClientTest)

#include "GoogleHttpClientTest.moc"
