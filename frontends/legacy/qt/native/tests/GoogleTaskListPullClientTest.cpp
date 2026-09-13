#include <QtTest/QTest>

#include "core/GoogleHttpClient.h"
#include "core/GoogleTaskListPullClient.h"
#include "support/MockNetworkAccessManager.h"

#include <QUrlQuery>

#include <chrono>
#include <future>
#include <optional>
#include <variant>

class GoogleTaskListPullClientTest final : public QObject {
  Q_OBJECT

private slots:
  void readsEveryPageAndNormalizesTaskLists();
  void rejectsMalformedTaskListPayloads();
  void propagatesGoogleTransportErrors();
};

void GoogleTaskListPullClientTest::readsEveryPageAndNormalizesTaskLists() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray(
                       "{\"nextPageToken\":\"page-2\",\"items\":[{\"id\":\"list-1\",\"title\":"
                       "\"Work\",\"updated\":\"2024-07-24T12:00:00+02:00\",\"etag\":\"etag-1\"}]}"),
                   .headers = {{QByteArray("Date"), QByteArray("Wed, 24 Jul 2024 10:00:00 GMT")}}});
  manager.enqueue(
      {.body = QByteArray("{\"items\":[{\"id\":\"list-2\",\"updated\":\"2024-07-25T12:00:00Z\"}]}"),
       .headers = {{QByteArray("Date"), QByteArray("Thu, 25 Jul 2024 12:00:00 GMT")}}});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskListPullClient client(httpClient);

  std::future<hcb::GoogleTaskListPullResultOrError> future =
      client.list(QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleTaskListPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleTaskListPullResult>(result));
  const hcb::GoogleTaskListPullResult& pulled = std::get<hcb::GoogleTaskListPullResult>(result);
  QCOMPARE(pulled.taskLists.size(), 2);
  QCOMPARE(pulled.taskLists.at(0).id, QStringLiteral("list-1"));
  QCOMPARE(pulled.taskLists.at(0).title, QStringLiteral("Work"));
  QCOMPARE(pulled.taskLists.at(0).updatedAt,
           std::optional<QString>(QStringLiteral("2024-07-24T10:00:00.000Z")));
  QCOMPARE(pulled.taskLists.at(0).etag, std::optional<QString>(QStringLiteral("etag-1")));
  QCOMPARE(pulled.taskLists.at(1).title, QStringLiteral("Untitled list"));
  QCOMPARE(pulled.serverDate,
           std::optional<QString>(QStringLiteral("Wed, 24 Jul 2024 10:00:00 GMT")));

  QCOMPARE(manager.requests().size(), 2);
  const hcb::test::CapturedNetworkRequest& first = manager.requests().at(0);
  QCOMPARE(first.operation, QNetworkAccessManager::GetOperation);
  QCOMPARE(first.request.rawHeader("Authorization"), QByteArray("Bearer access-token"));
  QCOMPARE(first.request.url().path(), QStringLiteral("/tasks/v1/users/@me/lists"));
  const QUrlQuery firstQuery(first.request.url());
  QCOMPARE(firstQuery.queryItemValue(QStringLiteral("maxResults")), QStringLiteral("1000"));
  QCOMPARE(firstQuery.queryItemValue(QStringLiteral("fields")),
           QStringLiteral("nextPageToken,items(id,title,updated,etag)"));
  QVERIFY(!firstQuery.hasQueryItem(QStringLiteral("pageToken")));
  const QUrlQuery secondQuery(manager.requests().at(1).request.url());
  QCOMPARE(secondQuery.queryItemValue(QStringLiteral("pageToken")), QStringLiteral("page-2"));
}

void GoogleTaskListPullClientTest::rejectsMalformedTaskListPayloads() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"items\":[{\"id\":\"\",\"title\":\"Invalid\"}]}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskListPullClient client(httpClient);

  std::future<hcb::GoogleTaskListPullResultOrError> future =
      client.list(QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleTaskListPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  QCOMPARE(std::get<hcb::GoogleApiError>(result).kind(), hcb::GoogleApiErrorKind::InvalidPayload);
}

void GoogleTaskListPullClientTest::propagatesGoogleTransportErrors() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 429,
                   .body = QByteArray("{\"error\":{\"reason\":\"quotaExceeded\"}}"),
                   .error = QNetworkReply::UnknownServerError,
                   .headers = {{QByteArray("Retry-After"), QByteArray("3")}}});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskListPullClient client(httpClient);

  std::future<hcb::GoogleTaskListPullResultOrError> future =
      client.list(QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleTaskListPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  const hcb::GoogleApiError& error = std::get<hcb::GoogleApiError>(result);
  QCOMPARE(error.kind(), hcb::GoogleApiErrorKind::RateLimited);
  QCOMPARE(error.retryAfterMilliseconds(), std::optional<qint64>(3'000));
}

QTEST_GUILESS_MAIN(GoogleTaskListPullClientTest)

#include "GoogleTaskListPullClientTest.moc"
