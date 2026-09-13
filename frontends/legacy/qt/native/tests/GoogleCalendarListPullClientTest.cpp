#include <QtTest/QTest>

#include "core/GoogleCalendarListPullClient.h"
#include "core/GoogleHttpClient.h"
#include "support/MockNetworkAccessManager.h"

#include <QUrlQuery>
#include <QJsonObject>

#include <chrono>
#include <future>
#include <optional>
#include <variant>

class GoogleCalendarListPullClientTest final : public QObject {
  Q_OBJECT

private slots:
  void readsEveryPageAndNormalizesCalendars();
  void sendsIncrementalSyncTokenOnEveryPage();
  void rejectsMalformedCalendarPayloads();
  void propagatesGoogleTransportErrors();
};

void GoogleCalendarListPullClientTest::readsEveryPageAndNormalizesCalendars() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue(
      {.body = QByteArray(
           "{\"nextPageToken\":\"page-2\",\"items\":[{\"id\":\"calendar-1\",\"summary\":"
           "\"Work\",\"summaryOverride\":\"Team\",\"description\":\"Planning\",\"timeZone\":"
           "\"Asia/Singapore\",\"colorId\":\"9\",\"backgroundColor\":\"#123abc\",\"foregroundColor\":\"#FFFFFF\","
           "\"accessRole\":\"owner\",\"selected\":false,\"hidden\":true,\"primary\":true,"
           "\"defaultReminders\":[{\"method\":\"popup\",\"minutes\":10}],"
           "\"etag\":\"etag-1\"}]}"),
       .headers = {{QByteArray("Date"), QByteArray("Wed, 24 Jul 2024 10:00:00 GMT")}}});
  manager.enqueue(
      {.body = QByteArray(
           "{\"items\":[{\"id\":\"calendar-2\",\"deleted\":true,\"accessRole\":\"reader\"}]}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarListPullClient client(httpClient);

  std::future<hcb::GoogleCalendarListPullResultOrError> future =
      client.list({}, QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleCalendarListPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarListPullResult>(result));
  const hcb::GoogleCalendarListPullResult& pulled =
      std::get<hcb::GoogleCalendarListPullResult>(result);
  QCOMPARE(pulled.calendars.size(), 2);
  const hcb::GoogleCalendarMirror& first = pulled.calendars.at(0);
  QCOMPARE(first.id, QStringLiteral("calendar-1"));
  QCOMPARE(first.title, QStringLiteral("Team"));
  QCOMPARE(first.description, std::optional<QString>(QStringLiteral("Planning")));
  QCOMPARE(first.colorId, std::optional<QString>(QStringLiteral("9")));
  QCOMPARE(first.accessRole,
           std::optional<hcb::GoogleCalendarAccessRole>(hcb::GoogleCalendarAccessRole::Owner));
  QVERIFY(!first.selected);
  QVERIFY(first.hidden);
  QVERIFY(first.primary);
  QVERIFY(!first.deleted);
  const QJsonArray expectedReminders{
      QJsonObject{{QStringLiteral("method"), QStringLiteral("popup")},
                  {QStringLiteral("minutes"), 10}}};
  QCOMPARE(first.defaultReminders, expectedReminders);
  const hcb::GoogleCalendarMirror& second = pulled.calendars.at(1);
  QCOMPARE(second.title, QStringLiteral("Untitled calendar"));
  QCOMPARE(second.accessRole,
           std::optional<hcb::GoogleCalendarAccessRole>(hcb::GoogleCalendarAccessRole::Reader));
  QVERIFY(second.selected);
  QVERIFY(second.deleted);
  QCOMPARE(pulled.serverDate,
           std::optional<QString>(QStringLiteral("Wed, 24 Jul 2024 10:00:00 GMT")));

  QCOMPARE(manager.requests().size(), 2);
  const hcb::test::CapturedNetworkRequest& firstRequest = manager.requests().at(0);
  QCOMPARE(firstRequest.operation, QNetworkAccessManager::GetOperation);
  QCOMPARE(firstRequest.request.rawHeader("Authorization"), QByteArray("Bearer access-token"));
  QCOMPARE(firstRequest.request.url().path(), QStringLiteral("/calendar/v3/users/me/calendarList"));
  const QUrlQuery firstQuery(firstRequest.request.url());
  QCOMPARE(firstQuery.queryItemValue(QStringLiteral("maxResults")), QStringLiteral("250"));
  QCOMPARE(firstQuery.queryItemValue(QStringLiteral("showDeleted")), QStringLiteral("true"));
  QCOMPARE(firstQuery.queryItemValue(QStringLiteral("showHidden")), QStringLiteral("true"));
  QVERIFY(firstQuery.queryItemValue(QStringLiteral("fields")).contains(
      QStringLiteral("defaultReminders")));
  QVERIFY(!firstQuery.hasQueryItem(QStringLiteral("pageToken")));
  QCOMPARE(
      QUrlQuery(manager.requests().at(1).request.url()).queryItemValue(QStringLiteral("pageToken")),
      QStringLiteral("page-2"));
}

void GoogleCalendarListPullClientTest::sendsIncrementalSyncTokenOnEveryPage() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"nextPageToken\":\"page-2\",\"items\":[]}")});
  manager.enqueue({.body = QByteArray("{\"nextSyncToken\":\"sync-2\",\"items\":[]}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarListPullClient client(httpClient);

  std::future<hcb::GoogleCalendarListPullResultOrError> future =
      client.list({.syncToken = QStringLiteral("sync-1")}, QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleCalendarListPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarListPullResult>(result));
  QCOMPARE(std::get<hcb::GoogleCalendarListPullResult>(result).nextSyncToken,
           std::optional<QString>(QStringLiteral("sync-2")));
  QCOMPARE(manager.requests().size(), 2);
  for (const hcb::test::CapturedNetworkRequest& request : manager.requests()) {
    QCOMPARE(QUrlQuery(request.request.url()).queryItemValue(QStringLiteral("syncToken")),
             QStringLiteral("sync-1"));
  }
}

void GoogleCalendarListPullClientTest::rejectsMalformedCalendarPayloads() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue(
      {.body = QByteArray("{\"items\":[{\"id\":\"calendar-1\",\"backgroundColor\":\"red\"}]}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarListPullClient client(httpClient);

  std::future<hcb::GoogleCalendarListPullResultOrError> future =
      client.list({}, QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleCalendarListPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  QCOMPARE(std::get<hcb::GoogleApiError>(result).kind(), hcb::GoogleApiErrorKind::InvalidPayload);
}

void GoogleCalendarListPullClientTest::propagatesGoogleTransportErrors() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 429,
                   .body = QByteArray("{\"error\":{\"reason\":\"quotaExceeded\"}}"),
                   .error = QNetworkReply::UnknownServerError,
                   .headers = {{QByteArray("Retry-After"), QByteArray("3")}}});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarListPullClient client(httpClient);

  std::future<hcb::GoogleCalendarListPullResultOrError> future =
      client.list({}, QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleCalendarListPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  const hcb::GoogleApiError& error = std::get<hcb::GoogleApiError>(result);
  QCOMPARE(error.kind(), hcb::GoogleApiErrorKind::RateLimited);
  QCOMPARE(error.retryAfterMilliseconds(), std::optional<qint64>(3'000));
}

QTEST_GUILESS_MAIN(GoogleCalendarListPullClientTest)

#include "GoogleCalendarListPullClientTest.moc"
