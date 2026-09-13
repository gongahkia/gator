#include <QDir>
#include <QFileInfo>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include <algorithm>
#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

#include "core/CalendarReadService.h"
#include "data/SqliteConnection.h"
#include "sqlite3.h"

using namespace std::chrono_literals;

class CalendarReadServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void readsCalendarPagesAndOverlappingEventRanges();
  void includesRecurringMastersAndCancelledInstancesForProjection();
  void readsGoogleResolvedInstanceCachesAcrossFreshnessStates();
  void rejectsInvalidReadRequests();
};

namespace {

[[nodiscard]] std::optional<hcb::FilePath>
databasePathFor(const QTemporaryDir& temporaryDirectory) {
  return hcb::FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                         .filePath(QStringLiteral("hot-cross-buns.sqlite")));
}

template <typename Result> [[nodiscard]] Result awaitResult(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("calendar read service request timed out");
  }
  return future.get();
}

void verifyReady(hcb::CalendarReadService& service) {
  const std::shared_future<hcb::SqliteWriteResult> ready = service.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  const hcb::SqliteWriteResult& result = ready.get();
  QVERIFY2(!result.has_value(), qPrintable(result.has_value() ? result->message() : QString()));
}

void execute(sqlite3* handle, const char* sql) {
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  const QString message = errorMessage == nullptr ? QString() : QString::fromUtf8(errorMessage);
  sqlite3_free(errorMessage);
  QVERIFY2(result == SQLITE_OK, qPrintable(message));
}

void seed(hcb::SqliteConnection& connection) {
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('account-a', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z'), "
          "('account-b', 'google', 'connected', '[]', '[]', '2026-07-25T00:00:00Z')");
  execute(handle,
          "INSERT INTO local_calendars (id, account_id, remote_id, title, time_zone, is_selected, "
          "is_hidden, is_primary, updated_at, deleted_at) VALUES "
          "('calendar-work', 'account-a', 'work', 'Work', 'UTC', 1, 0, 1, "
          "'2026-07-25T00:00:00Z', NULL), "
          "('calendar-hidden', 'account-a', 'hidden', 'Hidden', 'UTC', 1, 1, 0, "
          "'2026-07-25T00:00:00Z', NULL), "
          "('calendar-other', 'account-b', 'other', 'Other', 'UTC', 1, 0, 1, "
          "'2026-07-25T00:00:00Z', NULL), "
          "('calendar-deleted', 'account-a', 'deleted', 'Deleted', 'UTC', 1, 0, 0, "
          "'2026-07-25T00:00:00Z', '2026-07-25T01:00:00Z')");
  execute(handle,
          "INSERT INTO local_calendar_events (id, calendar_id, remote_id, status, title, "
          "description, location, start_at, end_at, is_all_day, updated_at, deleted_at) VALUES "
          "('event-boundary', 'calendar-work', 'boundary', 'confirmed', 'Boundary', NULL, NULL, "
          "'2026-07-25T08:00:00Z', '2026-07-25T09:00:00Z', 0, '2026-07-25T00:00:00Z', NULL), "
          "('event-early', 'calendar-work', 'early', 'confirmed', 'Early', 'Focus time', "
          "'Room 1', '2026-07-25T08:30:00Z', '2026-07-25T09:30:00Z', 0, "
          "'2026-07-25T00:00:00Z', NULL), "
          "('event-mid', 'calendar-work', 'mid', 'tentative', 'Mid', NULL, NULL, "
          "'2026-07-25T10:00:00Z', '2026-07-25T11:00:00Z', 0, '2026-07-25T00:00:00Z', NULL), "
          "('event-next-day', 'calendar-work', 'next-day', 'confirmed', 'All day', NULL, NULL, "
          "'2026-07-26T00:00:00Z', '2026-07-27T00:00:00Z', 1, '2026-07-25T00:00:00Z', NULL), "
          "('event-cancelled', 'calendar-work', 'cancelled', 'cancelled', 'Cancelled', NULL, NULL, "
          "'2026-07-25T10:00:00Z', '2026-07-25T11:00:00Z', 0, '2026-07-25T00:00:00Z', NULL), "
          "('event-deleted', 'calendar-work', 'deleted', 'confirmed', 'Deleted', NULL, NULL, "
          "'2026-07-25T10:00:00Z', '2026-07-25T11:00:00Z', 0, '2026-07-25T00:00:00Z', "
          "'2026-07-25T01:00:00Z'), "
          "('event-hidden', 'calendar-hidden', 'hidden', 'confirmed', 'Hidden event', NULL, NULL, "
          "'2026-07-25T09:15:00Z', '2026-07-25T09:45:00Z', 0, '2026-07-25T00:00:00Z', NULL), "
          "('event-other', 'calendar-other', 'other', 'confirmed', 'Other event', NULL, NULL, "
          "'2026-07-25T10:15:00Z', '2026-07-25T10:45:00Z', 0, '2026-07-25T00:00:00Z', NULL)");
}

} // namespace

void CalendarReadServiceTest::readsCalendarPagesAndOverlappingEventRanges() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  hcb::CalendarReadService service(*databasePath);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);

  std::future<hcb::CalendarListPageResult> firstPage =
      service.listCalendars(hcb::CalendarListReadRequest{.limit = 1});
  const hcb::CalendarListPageResult firstResult = awaitResult(firstPage);
  QVERIFY(std::holds_alternative<hcb::CalendarListPage>(firstResult));
  const hcb::CalendarListPage& calendarPage = std::get<hcb::CalendarListPage>(firstResult);
  QCOMPARE(calendarPage.totalKnown, 2);
  QCOMPARE(calendarPage.items.size(), 1);
  QCOMPARE(calendarPage.items.at(0).id, QStringLiteral("calendar-other"));
  QCOMPARE(calendarPage.items.at(0).eventCount, std::int64_t{1});
  QCOMPARE(calendarPage.nextOffset, std::optional<std::int64_t>(1));

  std::future<hcb::CalendarListPageResult> selected = service.listCalendars(
      hcb::CalendarListReadRequest{.accountId = QStringLiteral("account-a"), .selectedOnly = true});
  const hcb::CalendarListPageResult selectedResult = awaitResult(selected);
  QVERIFY(std::holds_alternative<hcb::CalendarListPage>(selectedResult));
  const hcb::CalendarListPage& selectedPage = std::get<hcb::CalendarListPage>(selectedResult);
  QCOMPARE(selectedPage.totalKnown, 1);
  QCOMPARE(selectedPage.items.at(0).id, QStringLiteral("calendar-work"));

  std::future<hcb::CalendarLookupResult> found =
      service.findCalendar(QStringLiteral("calendar-work"));
  const hcb::CalendarLookupResult foundResult = awaitResult(found);
  QVERIFY(std::holds_alternative<std::optional<hcb::CalendarSummary>>(foundResult));
  const std::optional<hcb::CalendarSummary>& foundCalendar =
      std::get<std::optional<hcb::CalendarSummary>>(foundResult);
  QVERIFY(foundCalendar.has_value());
  if (!foundCalendar.has_value()) {
    return;
  }
  QCOMPARE(foundCalendar->accountId, QStringLiteral("account-a"));
  QCOMPARE(foundCalendar->eventCount, std::int64_t{4});
  QCOMPARE(foundCalendar->timeZone, std::optional<QString>(QStringLiteral("UTC")));

  std::future<hcb::CalendarLookupResult> deleted =
      service.findCalendar(QStringLiteral("calendar-deleted"));
  const hcb::CalendarLookupResult deletedResult = awaitResult(deleted);
  QVERIFY(std::holds_alternative<std::optional<hcb::CalendarSummary>>(deletedResult));
  QVERIFY(!std::get<std::optional<hcb::CalendarSummary>>(deletedResult).has_value());

  const hcb::CalendarEventRangeReadRequest range{.startAt = QStringLiteral("2026-07-25T09:00:00Z"),
                                                 .endAt = QStringLiteral("2026-07-25T12:00:00Z"),
                                                 .limit = 2};
  std::future<hcb::CalendarEventPageResult> firstEvents = service.listEvents(range);
  const hcb::CalendarEventPageResult firstEventsResult = awaitResult(firstEvents);
  QVERIFY(std::holds_alternative<hcb::CalendarEventPage>(firstEventsResult));
  const hcb::CalendarEventPage& eventPage = std::get<hcb::CalendarEventPage>(firstEventsResult);
  QCOMPARE(eventPage.totalKnown, 4);
  QCOMPARE(eventPage.items.size(), 2);
  QCOMPARE(eventPage.items.at(0).id, QStringLiteral("event-early"));
  QCOMPARE(eventPage.items.at(0).description, std::optional<QString>(QStringLiteral("Focus time")));
  QCOMPARE(eventPage.items.at(1).id, QStringLiteral("event-hidden"));
  QCOMPARE(eventPage.nextOffset, std::optional<std::int64_t>(2));

  std::future<hcb::CalendarEventPageResult> workEvents = service.listEvents(
      hcb::CalendarEventRangeReadRequest{.calendarIds = {QStringLiteral("calendar-work")},
                                         .startAt = QStringLiteral("2026-07-25T09:00:00Z"),
                                         .endAt = QStringLiteral("2026-07-25T12:00:00Z")});
  const hcb::CalendarEventPageResult workEventsResult = awaitResult(workEvents);
  QVERIFY(std::holds_alternative<hcb::CalendarEventPage>(workEventsResult));
  const hcb::CalendarEventPage& workEventPage = std::get<hcb::CalendarEventPage>(workEventsResult);
  QCOMPARE(workEventPage.totalKnown, 2);
  QCOMPARE(workEventPage.items.size(), 2);
  QCOMPARE(workEventPage.items.at(0).id, QStringLiteral("event-early"));
  QCOMPARE(workEventPage.items.at(1).id, QStringLiteral("event-mid"));
}

void CalendarReadServiceTest::includesRecurringMastersAndCancelledInstancesForProjection() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  hcb::CalendarReadService service(*databasePath);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);
  execute(connection.nativeHandle(),
          "INSERT INTO local_calendar_events (id, calendar_id, remote_id, status, title, start_at, "
          "end_at, is_all_day, recurrence_rule, recurring_remote_id, original_start_at, updated_at, "
          "deleted_at) VALUES "
          "('event-series', 'calendar-work', 'remote-series', 'confirmed', 'Series', "
          "'2024-01-01T09:00:00.000Z', '2024-01-01T10:00:00.000Z', 0, "
          "'RRULE:FREQ=DAILY', NULL, NULL, '2026-07-25T00:00:00Z', NULL), "
          "('event-series-cancelled', 'calendar-work', 'remote-instance', 'cancelled', "
          "'Cancelled instance', '2026-07-26T09:00:00.000Z', '2026-07-26T10:00:00.000Z', 0, "
          "NULL, 'remote-series', '2026-07-26T09:00:00.000Z', '2026-07-25T00:00:00Z', NULL)");
  std::future<hcb::CalendarEventPageResult> future = service.listEvents(
      {.calendarIds = {QStringLiteral("calendar-work")},
       .startAt = QStringLiteral("2026-07-25T00:00:00Z"),
       .endAt = QStringLiteral("2026-07-28T00:00:00Z")});
  const hcb::CalendarEventPageResult result = awaitResult(future);
  QVERIFY(std::holds_alternative<hcb::CalendarEventPage>(result));
  if (!std::holds_alternative<hcb::CalendarEventPage>(result)) {
    return;
  }
  const hcb::CalendarEventPage& page = std::get<hcb::CalendarEventPage>(result);
  QCOMPARE(page.totalKnown, 6);
  QCOMPARE(page.items.size(), 6);
  const auto cancelled = std::find_if(
      page.items.cbegin(), page.items.cend(), [](const hcb::CalendarEventSummary& event) {
        return event.id == QStringLiteral("event-series-cancelled");
      });
  QVERIFY(cancelled != page.items.cend());
  if (cancelled == page.items.cend()) {
    return;
  }
  QVERIFY(std::any_of(page.items.cbegin(), page.items.cend(), [](const hcb::CalendarEventSummary& event) {
    return event.id == QStringLiteral("event-series");
  }));
  QCOMPARE(cancelled->status, QStringLiteral("cancelled"));
  QCOMPARE(cancelled->originalStartAt,
           std::optional<QString>(QStringLiteral("2026-07-26T09:00:00.000Z")));
}

void CalendarReadServiceTest::readsGoogleResolvedInstanceCachesAcrossFreshnessStates() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  hcb::CalendarReadService service(*databasePath);
  verifyReady(service);
  hcb::SqliteConnectionResult connectionResult =
      hcb::SqliteConnectionFactory::open(*databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  seed(connection);
  execute(connection.nativeHandle(),
          "INSERT INTO local_calendar_events (id, calendar_id, remote_id, status, title, start_at, "
          "end_at, is_all_day, recurrence_rule, updated_at, deleted_at) VALUES "
          "('series', 'calendar-work', 'remote-series', 'confirmed', 'Series', "
          "'2024-01-01T09:00:00.000Z', '2024-01-01T10:00:00.000Z', 0, "
          "'RRULE:FREQ=YEARLY;BYMONTH=7;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-1', "
          "'2026-07-25T00:00:00Z', NULL), "
          "('series-instance', 'calendar-work', 'remote-instance', 'confirmed', 'Resolved instance', "
          "'2026-07-31T09:00:00.000Z', '2026-07-31T10:00:00.000Z', 0, NULL, "
          "'2026-07-25T00:00:00Z', NULL)");
  execute(connection.nativeHandle(),
          "UPDATE local_calendar_events SET recurring_remote_id = 'remote-series', "
          "original_start_at = '2026-07-31T09:00:00.000Z', is_instance_cache = 1 "
          "WHERE id = 'series-instance'");
  execute(connection.nativeHandle(),
          "INSERT INTO local_calendar_instance_coverage "
          "(calendar_id, recurring_remote_id, range_start_at, range_end_at, fetched_at, expires_at) "
          "VALUES ('calendar-work', 'remote-series', '2026-07-25T00:00:00.000Z', "
          "'2026-08-05T00:00:00.000Z', '2026-07-25T00:00:00.000Z', "
          "'2099-01-01T00:00:00.000Z')");
  const hcb::CalendarEventRangeReadRequest range{
      .calendarIds = {QStringLiteral("calendar-work")},
      .startAt = QStringLiteral("2026-07-25T00:00:00.000Z"),
      .endAt = QStringLiteral("2026-08-01T00:00:00.000Z")};
  std::future<hcb::CalendarEventPageResult> freshFuture = service.listEvents(range);
  const hcb::CalendarEventPageResult freshResult = awaitResult(freshFuture);
  QVERIFY(std::holds_alternative<hcb::CalendarEventPage>(freshResult));
  if (!std::holds_alternative<hcb::CalendarEventPage>(freshResult)) {
    return;
  }
  const hcb::CalendarEventPage& freshPage = std::get<hcb::CalendarEventPage>(freshResult);
  const auto master = std::find_if(
      freshPage.items.cbegin(), freshPage.items.cend(), [](const hcb::CalendarEventSummary& event) {
        return event.id == QStringLiteral("series");
      });
  QVERIFY(master != freshPage.items.cend());
  if (master == freshPage.items.cend()) {
    return;
  }
  QVERIFY(master->instanceRangeCached);
  QVERIFY(std::any_of(
      freshPage.items.cbegin(), freshPage.items.cend(), [](const hcb::CalendarEventSummary& event) {
        return event.id == QStringLiteral("series-instance");
      }));

  execute(connection.nativeHandle(),
          "UPDATE local_calendar_instance_coverage SET expires_at = '2026-07-25T00:00:00.001Z'");
  std::future<hcb::CalendarEventPageResult> staleFuture = service.listEvents(range);
  const hcb::CalendarEventPageResult staleResult = awaitResult(staleFuture);
  QVERIFY(std::holds_alternative<hcb::CalendarEventPage>(staleResult));
  if (!std::holds_alternative<hcb::CalendarEventPage>(staleResult)) {
    return;
  }
  const hcb::CalendarEventPage& stalePage = std::get<hcb::CalendarEventPage>(staleResult);
  const auto staleMaster = std::find_if(
      stalePage.items.cbegin(), stalePage.items.cend(), [](const hcb::CalendarEventSummary& event) {
        return event.id == QStringLiteral("series");
      });
  QVERIFY(staleMaster != stalePage.items.cend());
  if (staleMaster == stalePage.items.cend()) {
    return;
  }
  QVERIFY(!staleMaster->instanceRangeCached);
  QVERIFY(std::any_of(
      stalePage.items.cbegin(), stalePage.items.cend(), [](const hcb::CalendarEventSummary& event) {
        return event.id == QStringLiteral("series-instance");
      }));

  std::future<hcb::CalendarRecurringInstanceCacheTargetsResult> targetsFuture =
      service.listUncachedRecurringInstances({.calendarIds = {QStringLiteral("calendar-work")},
                                              .startAt = range.startAt,
                                              .endAt = range.endAt});
  const hcb::CalendarRecurringInstanceCacheTargetsResult targetsResult = awaitResult(targetsFuture);
  QVERIFY(std::holds_alternative<QList<hcb::CalendarRecurringInstanceCacheTarget>>(targetsResult));
  if (!std::holds_alternative<QList<hcb::CalendarRecurringInstanceCacheTarget>>(targetsResult)) {
    return;
  }
  const QList<hcb::CalendarRecurringInstanceCacheTarget>& targets =
      std::get<QList<hcb::CalendarRecurringInstanceCacheTarget>>(targetsResult);
  QCOMPARE(targets.size(), 1);
  QCOMPARE(targets.constFirst().calendarRemoteId, QStringLiteral("work"));
  QCOMPARE(targets.constFirst().recurringRemoteId, QStringLiteral("remote-series"));
}

void CalendarReadServiceTest::rejectsInvalidReadRequests() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(temporaryDirectory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  hcb::CalendarReadService service(*databasePath);
  verifyReady(service);

  std::future<hcb::CalendarListPageResult> invalidPage =
      service.listCalendars(hcb::CalendarListReadRequest{.limit = 101});
  const hcb::CalendarListPageResult invalidPageResult = awaitResult(invalidPage);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidPageResult));
  QCOMPARE(std::get<hcb::AppError>(invalidPageResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::CalendarLookupResult> invalidId =
      service.findCalendar(QStringLiteral(" calendar-work"));
  const hcb::CalendarLookupResult invalidIdResult = awaitResult(invalidId);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidIdResult));
  QCOMPARE(std::get<hcb::AppError>(invalidIdResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::CalendarEventPageResult> invalidRange = service.listEvents(
      hcb::CalendarEventRangeReadRequest{.startAt = QStringLiteral("2026-07-25T12:00:00Z"),
                                         .endAt = QStringLiteral("2026-07-25T12:00:00Z")});
  const hcb::CalendarEventPageResult invalidRangeResult = awaitResult(invalidRange);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidRangeResult));
  QCOMPARE(std::get<hcb::AppError>(invalidRangeResult).code(), hcb::AppErrorCode::Validation);

  std::future<hcb::CalendarEventPageResult> invalidCalendar = service.listEvents(
      hcb::CalendarEventRangeReadRequest{.calendarIds = {QStringLiteral(" calendar-work")},
                                         .startAt = QStringLiteral("2026-07-25T09:00:00Z"),
                                         .endAt = QStringLiteral("2026-07-25T12:00:00Z")});
  const hcb::CalendarEventPageResult invalidCalendarResult = awaitResult(invalidCalendar);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidCalendarResult));
  QCOMPARE(std::get<hcb::AppError>(invalidCalendarResult).code(), hcb::AppErrorCode::Validation);
}

QTEST_GUILESS_MAIN(CalendarReadServiceTest)

#include "CalendarReadServiceTest.moc"
