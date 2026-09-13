#include <QtTest/QTest>

#include "core/GoogleCalendarFreeBusyClient.h"
#include "core/GoogleDriveFilePickerClient.h"
#include "core/GoogleHttpClient.h"
#include "support/MockNetworkAccessManager.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QUrlQuery>

#include <chrono>
#include <future>
#include <variant>

class GoogleCalendarAvailabilityClientTest final : public QObject {
  Q_OBJECT

private slots:
  void queriesFreeBusyIntervals();
  void searchesDriveAttachmentsSafely();
  void rejectsInvalidAvailabilityInputBeforeNetwork();
};

void GoogleCalendarAvailabilityClientTest::queriesFreeBusyIntervals() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue(
      {.body = QByteArray(
           R"({"calendars":{"primary":{"busy":[{"start":"2026-08-01T09:00:00+08:00","end":"2026-08-01T10:00:00+08:00"}]},"team":{"busy":[]}}})")});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleCalendarFreeBusyClient client(http);
  std::future<hcb::GoogleCalendarFreeBusyResultOrError> future =
      client.query({.startAt = QStringLiteral("2026-08-01T00:00:00+08:00"),
                    .endAt = QStringLiteral("2026-08-02T00:00:00+08:00"),
                    .calendarIds = {QStringLiteral("primary"), QStringLiteral("team")}},
                   QStringLiteral("access-token"));
  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleCalendarFreeBusyResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarFreeBusyResult>(result));
  if (!std::holds_alternative<hcb::GoogleCalendarFreeBusyResult>(result)) {
    return;
  }
  const hcb::GoogleCalendarFreeBusyResult& availability =
      std::get<hcb::GoogleCalendarFreeBusyResult>(result);
  QCOMPARE(availability.intervalsByCalendar.value(QStringLiteral("primary")).size(), 1);
  QCOMPARE(availability.intervalsByCalendar.value(QStringLiteral("primary")).constFirst().startAt,
           QStringLiteral("2026-08-01T01:00:00.000Z"));
  QCOMPARE(manager.requests().size(), 1);
  const hcb::test::CapturedNetworkRequest& request = manager.requests().constFirst();
  QCOMPARE(request.operation, QNetworkAccessManager::PostOperation);
  QCOMPARE(request.request.url().path(), QStringLiteral("/calendar/v3/freeBusy"));
  QCOMPARE(request.request.rawHeader("Authorization"), QByteArray("Bearer access-token"));
  const QJsonObject body = QJsonDocument::fromJson(request.body).object();
  QCOMPARE(body.value(QStringLiteral("items")).toArray().size(), 2);
  QCOMPARE(body.value(QStringLiteral("timeMin")).toString(),
           QStringLiteral("2026-08-01T00:00:00+08:00"));
}

void GoogleCalendarAvailabilityClientTest::searchesDriveAttachmentsSafely() {
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue(
      {.body = QByteArray(
           R"({"files":[{"id":"file-1","name":"O'Brien plan","mimeType":"text/plain","webViewLink":"https://drive.google.com/open?id=file-1","iconLink":"https://drive.google.com/icon.png"}]})")});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleDriveFilePickerClient client(http);
  std::future<hcb::GoogleDriveAttachmentCandidatesOrError> future =
      client.search(QStringLiteral("O'Brien"), QStringLiteral("access-token"));
  QTRY_VERIFY_WITH_TIMEOUT(
      future.wait_for(std::chrono::milliseconds::zero()) == std::future_status::ready, 1'000);
  const hcb::GoogleDriveAttachmentCandidatesOrError result = future.get();
  QVERIFY(std::holds_alternative<QList<hcb::GoogleDriveAttachmentCandidate>>(result));
  if (!std::holds_alternative<QList<hcb::GoogleDriveAttachmentCandidate>>(result)) {
    return;
  }
  const QList<hcb::GoogleDriveAttachmentCandidate>& files =
      std::get<QList<hcb::GoogleDriveAttachmentCandidate>>(result);
  QCOMPARE(files.size(), 1);
  QCOMPARE(files.constFirst().webViewLink,
           QStringLiteral("https://drive.google.com/open?id=file-1"));
  QCOMPARE(manager.requests().size(), 1);
  const hcb::test::CapturedNetworkRequest& request = manager.requests().constFirst();
  QCOMPARE(request.operation, QNetworkAccessManager::GetOperation);
  QCOMPARE(request.request.url().path(), QStringLiteral("/drive/v3/files"));
  const QUrlQuery query(request.request.url());
  QCOMPARE(query.queryItemValue(QStringLiteral("pageSize")), QStringLiteral("50"));
  QCOMPARE(query.queryItemValue(QStringLiteral("includeItemsFromAllDrives")),
           QStringLiteral("true"));
  QCOMPARE(query.queryItemValue(QStringLiteral("supportsAllDrives")), QStringLiteral("true"));
  QCOMPARE(query.queryItemValue(QStringLiteral("q")),
           QStringLiteral("name contains 'O\\'Brien' and trashed = false"));
}

void GoogleCalendarAvailabilityClientTest::rejectsInvalidAvailabilityInputBeforeNetwork() {
  hcb::test::MockNetworkAccessManager manager;
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleCalendarFreeBusyClient client(http);
  std::future<hcb::GoogleCalendarFreeBusyResultOrError> future =
      client.query({.startAt = QStringLiteral("2026-08-01T00:00:00Z"),
                    .endAt = QStringLiteral("2026-08-02T00:00:00Z"),
                    .calendarIds = {QStringLiteral("primary"), QStringLiteral("primary")}},
                   QStringLiteral("access-token"));
  const hcb::GoogleCalendarFreeBusyResultOrError result = future.get();
  QVERIFY(std::holds_alternative<hcb::GoogleApiError>(result));
  QCOMPARE(manager.requests().size(), 0);
}

QTEST_GUILESS_MAIN(GoogleCalendarAvailabilityClientTest)

#include "GoogleCalendarAvailabilityClientTest.moc"
