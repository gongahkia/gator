#include <QtTest/QTest>

#include "core/GoogleHttpClient.h"
#include "core/GoogleTaskPullClient.h"
#include "support/MockNetworkAccessManager.h"

#include <QUrlQuery>

#include <chrono>
#include <future>
#include <optional>
#include <variant>

class GoogleTaskPullClientTest final : public QObject {
  Q_OBJECT

private slots:
  void readsEveryPageAndNormalizesTasks();
  void appliesIncrementalFilterRules();
  void rejectsMalformedTaskPayloads();
  void propagatesGoogleTransportErrors();
};

void GoogleTaskPullClientTest::readsEveryPageAndNormalizesTasks() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue(
      {.body = QByteArray(
           "{\"nextPageToken\":\"page-2\",\"items\":[{\"id\":\"task-1\",\"title\":\"Write "
           "report\",\"notes\":\"draft\",\"status\":\"completed\",\"due\":\"2024-07-26T23:59:59Z\","
           "\"completed\":\"2024-07-25T12:00:00+02:00\",\"deleted\":false,\"hidden\":true,"
           "\"parent\":\"parent-1\",\"position\":\"0001\",\"assignmentInfo\":{},\"etag\":\"etag-1\",\"updated\":\"2024-"
           "07-25T13:00:00Z\"}]}"),
       .headers = {{QByteArray("Date"), QByteArray("Thu, 25 Jul 2024 13:00:00 GMT")}}});
  manager.enqueue({.body = QByteArray(
                       "{\"items\":[{\"id\":\"task-2\",\"updated\":\"2024-07-26T00:00:00Z\"}]}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskPullClient client(httpClient);

  std::future<hcb::GoogleTaskPullResultOrError> future =
      client.list({.taskListId = QStringLiteral("list-1"),
                   .completedMin = QStringLiteral("2024-07-20T00:00:00Z")},
                  QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleTaskPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleTaskPullResult>(result));
  const hcb::GoogleTaskPullResult& pulled = std::get<hcb::GoogleTaskPullResult>(result);
  QCOMPARE(pulled.tasks.size(), 2);
  const hcb::GoogleTaskMirror& task = pulled.tasks.at(0);
  QCOMPARE(task.taskListId, QStringLiteral("list-1"));
  QCOMPARE(task.parentId, std::optional<QString>(QStringLiteral("parent-1")));
  QCOMPARE(task.status, hcb::GoogleTaskStatus::Completed);
  QCOMPARE(task.dueAt, std::optional<QString>(QStringLiteral("2024-07-26T00:00:00.000Z")));
  QCOMPARE(task.completedAt, std::optional<QString>(QStringLiteral("2024-07-25T10:00:00.000Z")));
  QVERIFY(task.hidden);
  QVERIFY(task.isAssigned);
  QVERIFY(!task.deleted);
  QCOMPARE(pulled.tasks.at(1).title, QStringLiteral("Untitled task"));
  QCOMPARE(pulled.serverDate,
           std::optional<QString>(QStringLiteral("Thu, 25 Jul 2024 13:00:00 GMT")));

  QCOMPARE(manager.requests().size(), 2);
  const hcb::test::CapturedNetworkRequest& first = manager.requests().at(0);
  QCOMPARE(first.operation, QNetworkAccessManager::GetOperation);
  QCOMPARE(first.request.rawHeader("Authorization"), QByteArray("Bearer access-token"));
  QCOMPARE(first.request.url().path(), QStringLiteral("/tasks/v1/lists/list-1/tasks"));
  const QUrlQuery firstQuery(first.request.url());
  QCOMPARE(firstQuery.queryItemValue(QStringLiteral("showAssigned")), QStringLiteral("true"));
  QCOMPARE(firstQuery.queryItemValue(QStringLiteral("showCompleted")), QStringLiteral("true"));
  QCOMPARE(firstQuery.queryItemValue(QStringLiteral("showDeleted")), QStringLiteral("true"));
  QCOMPARE(firstQuery.queryItemValue(QStringLiteral("showHidden")), QStringLiteral("true"));
  QCOMPARE(firstQuery.queryItemValue(QStringLiteral("maxResults")), QStringLiteral("100"));
  QVERIFY(firstQuery.queryItemValue(QStringLiteral("fields")).contains(
      QStringLiteral("assignmentInfo")));
  QCOMPARE(firstQuery.queryItemValue(QStringLiteral("completedMin")),
           QStringLiteral("2024-07-20T00:00:00Z"));
  QCOMPARE(
      QUrlQuery(manager.requests().at(1).request.url()).queryItemValue(QStringLiteral("pageToken")),
      QStringLiteral("page-2"));
}

void GoogleTaskPullClientTest::appliesIncrementalFilterRules() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"items\":[]}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskPullClient client(httpClient);

  std::future<hcb::GoogleTaskPullResultOrError> future =
      client.list({.taskListId = QStringLiteral("list-1"),
                   .updatedMin = QStringLiteral("2024-07-21T00:00:00Z"),
                   .completedMin = QStringLiteral("2024-07-20T00:00:00Z"),
                   .showCompleted = false},
                  QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  QVERIFY(std::holds_alternative<hcb::GoogleTaskPullResult>(future.get()));
  const QUrlQuery query(manager.requests().front().request.url());
  QCOMPARE(query.queryItemValue(QStringLiteral("updatedMin")),
           QStringLiteral("2024-07-21T00:00:00Z"));
  QVERIFY(!query.hasQueryItem(QStringLiteral("completedMin")));
  QCOMPARE(query.queryItemValue(QStringLiteral("showCompleted")), QStringLiteral("false"));
}

void GoogleTaskPullClientTest::rejectsMalformedTaskPayloads() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"items\":[{\"id\":\"task-1\",\"status\":\"unknown\"}]}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskPullClient client(httpClient);

  std::future<hcb::GoogleTaskPullResultOrError> future =
      client.list({.taskListId = QStringLiteral("list-1")}, QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleTaskPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  QCOMPARE(std::get<hcb::GoogleApiError>(result).kind(), hcb::GoogleApiErrorKind::InvalidPayload);
}

void GoogleTaskPullClientTest::propagatesGoogleTransportErrors() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 429,
                   .body = QByteArray("{\"error\":{\"reason\":\"quotaExceeded\"}}"),
                   .error = QNetworkReply::UnknownServerError,
                   .headers = {{QByteArray("Retry-After"), QByteArray("3")}}});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleTaskPullClient client(httpClient);

  std::future<hcb::GoogleTaskPullResultOrError> future =
      client.list({.taskListId = QStringLiteral("list-1")}, QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleTaskPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  const hcb::GoogleApiError& error = std::get<hcb::GoogleApiError>(result);
  QCOMPARE(error.kind(), hcb::GoogleApiErrorKind::RateLimited);
  QCOMPARE(error.retryAfterMilliseconds(), std::optional<qint64>(3'000));
}

QTEST_GUILESS_MAIN(GoogleTaskPullClientTest)

#include "GoogleTaskPullClientTest.moc"
