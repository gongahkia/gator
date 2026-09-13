#include <QtTest/QTest>

#include "core/GoogleCalendarEventPullClient.h"
#include "core/GoogleHttpClient.h"
#include "support/MockNetworkAccessManager.h"

#include <QUrlQuery>

#include <chrono>
#include <future>
#include <optional>
#include <variant>

class GoogleCalendarEventPullClientTest final : public QObject {
  Q_OBJECT

private slots:
  void readsEveryPageAndNormalizesEvents();
  void sendsIncrementalSyncTokenOnEveryPage();
  void readsResolvedInstancesForBoundedRange();
  void acceptsCancelledEventTombstones();
  void truncatesOversizedEventTitle();
  void rejectsMalformedEventPayloads();
  void rejectsInvalidRequest();
  void propagatesGoogleTransportErrors();
};

void GoogleCalendarEventPullClientTest::readsEveryPageAndNormalizesEvents() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue(
      {.body = QByteArray(
           "{\"nextPageToken\":\"page-2\",\"items\":[{\"id\":\"event-1\",\"status\":\"confirmed\","
           "\"summary\":\"Planning\",\"description\":\"Roadmap\",\"location\":\"Room 1\","
           "\"start\":{\"dateTime\":\"2024-07-24T12:00:00+02:00\",\"timeZone\":\"Europe/Berlin\"},"
           "\"end\":{\"dateTime\":\"2024-07-24T13:00:00+02:00\",\"timeZone\":\"Europe/Berlin\"},"
           "\"recurringEventId\":\"series-1\",\"originalStartTime\":{\"dateTime\":\"2024-07-24T12:"
           "00:00+02:00\"},"
           "\"recurrence\":[\"RRULE:FREQ=WEEKLY\"],\"colorId\":\"5\",\"transparency\":"
           "\"transparent\",\"attendees\":[{\"email\":\"guest@example.com\",\"displayName\":"
           "\"Guest\",\"responseStatus\":\"accepted\",\"self\":true}],\"reminders\":{\"useDefault\":false,"
           "\"overrides\":[{\"method\":\"popup\",\"minutes\":10}]},"
           "\"conferenceData\":{\"entryPoints\":[{\"entryPointType\":\"video\",\"uri\":\"https://meet.google.com/abc\"}]},"
           "\"attachments\":[{\"fileUrl\":\"https://drive.google.com/open?id=file-1\",\"title\":\"Spec\",\"mimeType\":\"text/plain\"}],"
           "\"guestsCanInviteOthers\":false,\"guestsCanModify\":true,\"guestsCanSeeOtherGuests\":false,"
           "\"visibility\":\"private\",\"timeZone\":\"Europe/Berlin\",\"eventType\":\"focusTime\","
           "\"focusTimeProperties\":{\"autoDeclineMode\":\"declineNone\",\"chatStatus\":\"available\"},"
           "\"etag\":\"etag-1\",\"sequence\":3,\"updated\":\"2024-07-24T10:00:00Z\"}]}"),
       .headers = {{QByteArray("Date"), QByteArray("Wed, 24 Jul 2024 10:00:00 GMT")}}});
  manager.enqueue(
      {.body = QByteArray(
           "{\"nextSyncToken\":\"sync-2\",\"items\":[{\"id\":\"event-2\",\"status\":\"tentative\","
           "\"start\":{\"date\":\"2024-07-25\"},\"end\":{\"date\":\"2024-07-26\"}}]}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarEventPullClient client(httpClient);

  std::future<hcb::GoogleCalendarEventPullResultOrError> future =
      client.list({.calendarId = QStringLiteral("calendar-1")}, QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleCalendarEventPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarEventPullResult>(result));
  const hcb::GoogleCalendarEventPullResult& pulled =
      std::get<hcb::GoogleCalendarEventPullResult>(result);
  QCOMPARE(pulled.events.size(), 2);
  const hcb::GoogleCalendarEventMirror& first = pulled.events.at(0);
  QCOMPARE(first.calendarId, QStringLiteral("calendar-1"));
  QCOMPARE(first.status, hcb::GoogleCalendarEventStatus::Confirmed);
  QCOMPARE(first.startAt, std::optional<QString>(QStringLiteral("2024-07-24T10:00:00.000Z")));
  QCOMPARE(first.endAt, std::optional<QString>(QStringLiteral("2024-07-24T11:00:00.000Z")));
  QVERIFY(!first.allDay);
  QCOMPARE(first.recurrence, QList<QString>{QStringLiteral("RRULE:FREQ=WEEKLY")});
  QCOMPARE(first.sequence, std::optional<qint64>(3));
  QCOMPARE(first.updatedAt, std::optional<QString>(QStringLiteral("2024-07-24T10:00:00.000Z")));
  QCOMPARE(first.attendees.size(), 1);
  QCOMPARE(first.attendees.at(0).toObject().value(QStringLiteral("email")).toString(),
           QStringLiteral("guest@example.com"));
  QVERIFY(first.attendees.at(0).toObject().value(QStringLiteral("self")).toBool());
  QCOMPARE(first.reminders.value(QStringLiteral("useDefault")).toBool(), false);
  QCOMPARE(first.reminders.value(QStringLiteral("overrides")).toArray().at(0)
               .toObject()
               .value(QStringLiteral("minutes"))
               .toInteger(),
           10);
  QCOMPARE(first.eventType, std::optional<QString>(QStringLiteral("focusTime")));
  QCOMPARE(first.conferenceData.value(QStringLiteral("entryPoints")).toArray().at(0)
               .toObject()
               .value(QStringLiteral("uri"))
               .toString(),
           QStringLiteral("https://meet.google.com/abc"));
  QCOMPARE(first.attachments.size(), 1);
  QCOMPARE(first.guestPermissions.value(QStringLiteral("guestsCanModify")).toBool(), true);
  QCOMPARE(first.statusProperties.value(QStringLiteral("focusTimeProperties")).toObject()
               .value(QStringLiteral("chatStatus"))
               .toString(),
           QStringLiteral("available"));
  const hcb::GoogleCalendarEventMirror& second = pulled.events.at(1);
  QCOMPARE(second.title, QStringLiteral("Untitled event"));
  QCOMPARE(second.startAt, std::optional<QString>(QStringLiteral("2024-07-25T00:00:00.000Z")));
  QVERIFY(second.allDay);
  QCOMPARE(pulled.nextSyncToken, std::optional<QString>(QStringLiteral("sync-2")));
  QCOMPARE(pulled.serverDate,
           std::optional<QString>(QStringLiteral("Wed, 24 Jul 2024 10:00:00 GMT")));

  QCOMPARE(manager.requests().size(), 2);
  const hcb::test::CapturedNetworkRequest& firstRequest = manager.requests().at(0);
  QCOMPARE(firstRequest.operation, QNetworkAccessManager::GetOperation);
  QCOMPARE(firstRequest.request.rawHeader("Authorization"), QByteArray("Bearer access-token"));
  QCOMPARE(firstRequest.request.url().path(),
           QStringLiteral("/calendar/v3/calendars/calendar-1/events"));
  const QUrlQuery query(firstRequest.request.url());
  QCOMPARE(query.queryItemValue(QStringLiteral("maxResults")), QStringLiteral("250"));
  QCOMPARE(query.queryItemValue(QStringLiteral("showDeleted")), QStringLiteral("true"));
  QCOMPARE(query.queryItemValue(QStringLiteral("showHiddenInvitations")), QStringLiteral("true"));
  QCOMPARE(query.queryItemValue(QStringLiteral("singleEvents")), QStringLiteral("false"));
  QCOMPARE(query.queryItemValue(QStringLiteral("conferenceDataVersion")), QStringLiteral("1"));
  const QString fields = query.queryItemValue(QStringLiteral("fields"));
  QVERIFY(fields.contains(QStringLiteral("attendees")));
  QVERIFY(fields.contains(QStringLiteral("reminders")));
  QVERIFY(fields.contains(QStringLiteral("attachments")));
  QVERIFY(!fields.contains(QStringLiteral("timeZone,")));
  QCOMPARE(
      QUrlQuery(manager.requests().at(1).request.url()).queryItemValue(QStringLiteral("pageToken")),
      QStringLiteral("page-2"));
}

void GoogleCalendarEventPullClientTest::sendsIncrementalSyncTokenOnEveryPage() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"nextPageToken\":\"page-2\",\"items\":[]}")});
  manager.enqueue({.body = QByteArray("{\"nextSyncToken\":\"sync-2\",\"items\":[]}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarEventPullClient client(httpClient);

  std::future<hcb::GoogleCalendarEventPullResultOrError> future = client.list(
      {.calendarId = QStringLiteral("calendar-1"), .syncToken = QStringLiteral("sync-1")},
      QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleCalendarEventPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarEventPullResult>(result));
  QCOMPARE(std::get<hcb::GoogleCalendarEventPullResult>(result).nextSyncToken,
           std::optional<QString>(QStringLiteral("sync-2")));
  QCOMPARE(manager.requests().size(), 2);
  for (const hcb::test::CapturedNetworkRequest& request : manager.requests()) {
    QCOMPARE(QUrlQuery(request.request.url()).queryItemValue(QStringLiteral("syncToken")),
             QStringLiteral("sync-1"));
  }
}

void GoogleCalendarEventPullClientTest::readsResolvedInstancesForBoundedRange() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray(
      "{\"items\":[{\"id\":\"instance-1\",\"status\":\"confirmed\",\"summary\":\"Planning\","
      "\"start\":{\"dateTime\":\"2026-08-01T09:00:00Z\"},\"end\":{\"dateTime\":\"2026-08-01T10:00:00Z\"},"
      "\"recurringEventId\":\"series-1\",\"originalStartTime\":{\"dateTime\":\"2026-08-01T09:00:00Z\"}}]}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarEventPullClient client(httpClient);

  std::future<hcb::GoogleCalendarEventInstancesPullResultOrError> future = client.instances(
      {.calendarId = QStringLiteral("calendar-1"),
       .recurringEventId = QStringLiteral("series-1"),
       .timeMin = QStringLiteral("2026-08-01T00:00:00.000Z"),
       .timeMax = QStringLiteral("2026-08-02T00:00:00.000Z")},
      QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleCalendarEventInstancesPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarEventInstancesPullResult>(result));
  const hcb::GoogleCalendarEventInstancesPullResult& pulled =
      std::get<hcb::GoogleCalendarEventInstancesPullResult>(result);
  QCOMPARE(pulled.events.size(), 1);
  QCOMPARE(pulled.events.front().id, QStringLiteral("instance-1"));
  QCOMPARE(pulled.events.front().originalStartAt,
           std::optional<QString>(QStringLiteral("2026-08-01T09:00:00.000Z")));

  QCOMPARE(manager.requests().size(), 1);
  const QUrl requestUrl = manager.requests().front().request.url();
  QCOMPARE(requestUrl.path(),
           QStringLiteral("/calendar/v3/calendars/calendar-1/events/series-1/instances"));
  const QUrlQuery query(requestUrl);
  QCOMPARE(query.queryItemValue(QStringLiteral("maxResults")), QStringLiteral("2500"));
  QCOMPARE(query.queryItemValue(QStringLiteral("timeMin")),
           QStringLiteral("2026-08-01T00:00:00.000Z"));
  QCOMPARE(query.queryItemValue(QStringLiteral("timeMax")),
           QStringLiteral("2026-08-02T00:00:00.000Z"));
  QVERIFY(query.queryItemValue(QStringLiteral("fields")).contains(
      QStringLiteral("originalStartTime")));
}

void GoogleCalendarEventPullClientTest::acceptsCancelledEventTombstones() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"items\":[{\"id\":\"event-1\",\"status\":\"cancelled\","
                                      "\"recurringEventId\":\"series-1\"}]}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarEventPullClient client(httpClient);

  std::future<hcb::GoogleCalendarEventPullResultOrError> future =
      client.list({.calendarId = QStringLiteral("calendar-1")}, QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleCalendarEventPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarEventPullResult>(result));
  const hcb::GoogleCalendarEventMirror& event =
      std::get<hcb::GoogleCalendarEventPullResult>(result).events.front();
  QCOMPARE(event.status, hcb::GoogleCalendarEventStatus::Cancelled);
  QVERIFY(!event.startAt.has_value());
  QVERIFY(!event.endAt.has_value());
}

void GoogleCalendarEventPullClientTest::truncatesOversizedEventTitle() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue(
      {.body = QByteArray("{\"items\":[{\"id\":\"event-1\",\"status\":\"confirmed\",\"summary\":\"") +
                   QByteArray(501, 'x') +
                   QByteArray("\",\"start\":{\"date\":\"2026-08-05\"},\"end\":{\"date\":\"2026-08-06\"}}]}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarEventPullClient client(httpClient);

  std::future<hcb::GoogleCalendarEventPullResultOrError> future =
      client.list({.calendarId = QStringLiteral("calendar-1")}, QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleCalendarEventPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarEventPullResult>(result));
  QCOMPARE(std::get<hcb::GoogleCalendarEventPullResult>(result).events.front().title.size(), 500);
}

void GoogleCalendarEventPullClientTest::rejectsMalformedEventPayloads() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"items\":[{\"id\":\"event-1\",\"status\":\"confirmed\","
                                      "\"start\":{\"date\":\"2024-07-24\"}}]}")});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarEventPullClient client(httpClient);

  std::future<hcb::GoogleCalendarEventPullResultOrError> future =
      client.list({.calendarId = QStringLiteral("calendar-1")}, QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleCalendarEventPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  QCOMPARE(std::get<hcb::GoogleApiError>(result).kind(), hcb::GoogleApiErrorKind::InvalidPayload);
}

void GoogleCalendarEventPullClientTest::rejectsInvalidRequest() {
  hcb::test::MockNetworkAccessManager manager;
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarEventPullClient client(httpClient);

  std::future<hcb::GoogleCalendarEventPullResultOrError> future =
      client.list({.calendarId = QStringLiteral("bad/calendar")}, QStringLiteral("access-token"));

  QVERIFY(future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready);
  const hcb::GoogleCalendarEventPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  QCOMPARE(std::get<hcb::GoogleApiError>(result).kind(), hcb::GoogleApiErrorKind::InvalidPayload);
  QCOMPARE(manager.requests().size(), 0);
}

void GoogleCalendarEventPullClientTest::propagatesGoogleTransportErrors() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 429,
                   .body = QByteArray("{\"error\":{\"reason\":\"quotaExceeded\"}}"),
                   .error = QNetworkReply::UnknownServerError,
                   .headers = {{QByteArray("Retry-After"), QByteArray("3")}}});
  hcb::GoogleHttpClient httpClient(nullptr, &manager);
  hcb::GoogleCalendarEventPullClient client(httpClient);

  std::future<hcb::GoogleCalendarEventPullResultOrError> future =
      client.list({.calendarId = QStringLiteral("calendar-1")}, QStringLiteral("access-token"));

  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleCalendarEventPullResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  const hcb::GoogleApiError& error = std::get<hcb::GoogleApiError>(result);
  QCOMPARE(error.kind(), hcb::GoogleApiErrorKind::RateLimited);
  QCOMPARE(error.retryAfterMilliseconds(), std::optional<qint64>(3'000));
}

QTEST_GUILESS_MAIN(GoogleCalendarEventPullClientTest)

#include "GoogleCalendarEventPullClientTest.moc"
