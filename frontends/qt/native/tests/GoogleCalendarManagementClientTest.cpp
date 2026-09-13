#include <QtTest/QTest>

#include "core/GoogleCalendarManagementClient.h"
#include "core/GoogleHttpClient.h"
#include "support/MockNetworkAccessManager.h"

#include <QJsonDocument>
#include <QJsonObject>

#include <chrono>
#include <future>
#include <variant>

class GoogleCalendarManagementClientTest final : public QObject {
  Q_OBJECT

private slots:
  void createsCalendar();
  void subscribesCalendar();
  void updatesAndDeletesCalendar();
  void updatesAndRemovesCalendarListEntry();
  void surfacesApiFailure();
  void rejectsInvalidInputBeforeNetwork();
};

void GoogleCalendarManagementClientTest::createsCalendar() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"id\":\"calendar-created\"}")});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleCalendarManagementClient client(http);

  std::future<hcb::GoogleCalendarManagementResultOrError> future = client.create(
      {.title = QStringLiteral("Team"),
       .description = QStringLiteral("Planning"),
       .timeZone = QStringLiteral("Asia/Singapore")},
      QStringLiteral("access-token"));
  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleCalendarManagementResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarManagementResult>(result));
  QCOMPARE(std::get<hcb::GoogleCalendarManagementResult>(result).calendarId,
           QStringLiteral("calendar-created"));
  QCOMPARE(manager.requests().size(), 1);
  const hcb::test::CapturedNetworkRequest& request = manager.requests().constFirst();
  QCOMPARE(request.operation, QNetworkAccessManager::PostOperation);
  QCOMPARE(request.request.url().path(), QStringLiteral("/calendar/v3/calendars"));
  QCOMPARE(request.request.rawHeader("Authorization"), QByteArray("Bearer access-token"));
  const QJsonObject body = QJsonDocument::fromJson(request.body).object();
  QCOMPARE(body.value(QStringLiteral("summary")).toString(), QStringLiteral("Team"));
  QCOMPARE(body.value(QStringLiteral("description")).toString(), QStringLiteral("Planning"));
  QCOMPARE(body.value(QStringLiteral("timeZone")).toString(), QStringLiteral("Asia/Singapore"));
}

void GoogleCalendarManagementClientTest::subscribesCalendar() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"id\":\"calendar-shared\"}")});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleCalendarManagementClient client(http);

  std::future<hcb::GoogleCalendarManagementResultOrError> future = client.subscribe(
      {.calendarId = QStringLiteral("calendar-shared"), .selected = false, .hidden = true,
       .colorId = QStringLiteral("4")},
      QStringLiteral("access-token"));
  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleCalendarManagementResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarManagementResult>(result));
  QCOMPARE(manager.requests().size(), 1);
  const hcb::test::CapturedNetworkRequest& request = manager.requests().constFirst();
  QCOMPARE(request.request.url().path(), QStringLiteral("/calendar/v3/users/me/calendarList"));
  const QJsonObject body = QJsonDocument::fromJson(request.body).object();
  QCOMPARE(body.value(QStringLiteral("id")).toString(), QStringLiteral("calendar-shared"));
  QVERIFY(!body.value(QStringLiteral("selected")).toBool());
  QVERIFY(body.value(QStringLiteral("hidden")).toBool());
  QCOMPARE(body.value(QStringLiteral("colorId")).toString(), QStringLiteral("4"));
}

void GoogleCalendarManagementClientTest::updatesAndDeletesCalendar() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"id\":\"calendar-owned\"}")});
  manager.enqueue({.status = 204});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleCalendarManagementClient client(http);

  std::future<hcb::GoogleCalendarManagementResultOrError> updated = client.update(
      {.calendarId = QStringLiteral("calendar-owned"),
       .title = QStringLiteral("Renamed"),
       .description = QStringLiteral("Description"),
       .timeZone = QStringLiteral("UTC")},
      QStringLiteral("access-token"));
  QTRY_VERIFY_WITH_TIMEOUT(
      updated.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarManagementResult>(updated.get()));

  std::future<hcb::GoogleCalendarManagementResultOrError> removed =
      client.remove(QStringLiteral("calendar-owned"), QStringLiteral("access-token"));
  QTRY_VERIFY_WITH_TIMEOUT(
      removed.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarManagementResult>(removed.get()));

  QCOMPARE(manager.requests().size(), 2);
  const hcb::test::CapturedNetworkRequest& patch = manager.requests().at(0);
  QCOMPARE(patch.operation, QNetworkAccessManager::CustomOperation);
  QCOMPARE(patch.request.attribute(QNetworkRequest::CustomVerbAttribute).toByteArray(),
           QByteArray("PATCH"));
  QCOMPARE(patch.request.url().path(), QStringLiteral("/calendar/v3/calendars/calendar-owned"));
  const QJsonObject body = QJsonDocument::fromJson(patch.body).object();
  QCOMPARE(body.value(QStringLiteral("summary")).toString(), QStringLiteral("Renamed"));
  QCOMPARE(body.value(QStringLiteral("timeZone")).toString(), QStringLiteral("UTC"));
  const hcb::test::CapturedNetworkRequest& remove = manager.requests().at(1);
  QCOMPARE(remove.operation, QNetworkAccessManager::DeleteOperation);
  QCOMPARE(remove.request.url().path(), QStringLiteral("/calendar/v3/calendars/calendar-owned"));
}

void GoogleCalendarManagementClientTest::updatesAndRemovesCalendarListEntry() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"id\":\"calendar-shared\"}")});
  manager.enqueue({.status = 204});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleCalendarManagementClient client(http);

  std::future<hcb::GoogleCalendarManagementResultOrError> updated = client.updateListEntry(
      {.calendarId = QStringLiteral("calendar-shared"),
       .selected = false,
       .hidden = true,
       .colorId = QStringLiteral("7")},
      QStringLiteral("access-token"));
  QTRY_VERIFY_WITH_TIMEOUT(
      updated.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarManagementResult>(updated.get()));

  std::future<hcb::GoogleCalendarManagementResultOrError> removed =
      client.removeListEntry(QStringLiteral("calendar-shared"), QStringLiteral("access-token"));
  QTRY_VERIFY_WITH_TIMEOUT(
      removed.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarManagementResult>(removed.get()));

  QCOMPARE(manager.requests().size(), 2);
  const hcb::test::CapturedNetworkRequest& patch = manager.requests().at(0);
  QCOMPARE(patch.request.url().path(),
           QStringLiteral("/calendar/v3/users/me/calendarList/calendar-shared"));
  const QJsonObject body = QJsonDocument::fromJson(patch.body).object();
  QVERIFY(!body.value(QStringLiteral("selected")).toBool());
  QVERIFY(body.value(QStringLiteral("hidden")).toBool());
  QCOMPARE(body.value(QStringLiteral("colorId")).toString(), QStringLiteral("7"));
  const hcb::test::CapturedNetworkRequest& remove = manager.requests().at(1);
  QCOMPARE(remove.operation, QNetworkAccessManager::DeleteOperation);
  QCOMPARE(remove.request.url().path(),
           QStringLiteral("/calendar/v3/users/me/calendarList/calendar-shared"));
}

void GoogleCalendarManagementClientTest::surfacesApiFailure() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 403,
                   .body = QByteArray(
                       "{\"error\":{\"code\":403,\"message\":\"Insufficient permissions\"}}")});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleCalendarManagementClient client(http);

  std::future<hcb::GoogleCalendarManagementResultOrError> future =
      client.remove(QStringLiteral("calendar-reader"), QStringLiteral("access-token"));
  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleCalendarManagementResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  QCOMPARE(std::get<hcb::GoogleApiError>(result).kind(), hcb::GoogleApiErrorKind::Forbidden);
  QCOMPARE(std::get<hcb::GoogleApiError>(result).status(), std::optional<int>(403));
}

void GoogleCalendarManagementClientTest::rejectsInvalidInputBeforeNetwork() {
  hcb::test::MockNetworkAccessManager manager;
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleCalendarManagementClient client(http);

  std::future<hcb::GoogleCalendarManagementResultOrError> future =
      client.create({.title = QStringLiteral(" ")}, QStringLiteral("access-token"));
  const hcb::GoogleCalendarManagementResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  QCOMPARE(std::get<hcb::GoogleApiError>(result).kind(), hcb::GoogleApiErrorKind::InvalidPayload);
  QCOMPARE(manager.requests().size(), 0);

  std::future<hcb::GoogleCalendarManagementResultOrError> update = client.update(
      {.calendarId = QStringLiteral("calendar-owned"),
       .title = QStringLiteral("Calendar"),
       .timeZone = QStringLiteral("Invalid/TimeZone")},
      QStringLiteral("access-token"));
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(update.get()));
  std::future<hcb::GoogleCalendarManagementResultOrError> listUpdate = client.updateListEntry(
      {.calendarId = QStringLiteral(" ")}, QStringLiteral("access-token"));
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(listUpdate.get()));
  QCOMPARE(manager.requests().size(), 0);
}

QTEST_GUILESS_MAIN(GoogleCalendarManagementClientTest)

#include "GoogleCalendarManagementClientTest.moc"
