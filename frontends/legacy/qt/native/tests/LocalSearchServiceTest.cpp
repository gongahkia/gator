#include <QtTest/QTest>

#include "core/LocalSearchService.h"
#include "core/LocalSearchQuery.h"
#include "core/Cancellation.h"
#include "data/LocalSchema.h"
#include "sqlite3.h"
#include "support/TemporarySqliteDatabase.h"

#include <chrono>
#include <future>
#include <memory>
#include <optional>
#include <utility>
#include <variant>

using namespace std::chrono_literals;

class LocalSearchServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void returnsRankedAndPagedResultsAcrossResources();
  void returnsCancelledForStoppedSearch();
  void rejectsInvalidRequests();
  void appliesStructuredFiltersLocally();
  void returnsAllStructuredMatchesBeyondLegacyCandidateCap();
  void rejectsInvalidStructuredSyntax();
};

namespace {

[[nodiscard]] std::unique_ptr<hcb::test::TemporarySqliteDatabase> createDatabase() {
  hcb::test::TemporarySqliteDatabaseResult result = hcb::test::TemporarySqliteDatabase::create();
  if (!std::holds_alternative<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(result)) {
    return nullptr;
  }
  return std::move(std::get<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(result));
}

[[nodiscard]] int execute(sqlite3* handle, const char* sql) {
  char* error = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &error);
  sqlite3_free(error);
  return result;
}

void initializeDatabase(const hcb::test::TemporarySqliteDatabase& database) {
  hcb::SqliteConnectionResult connectionResult =
      database.open(hcb::SqliteOpenMode::ReadWriteCreate);
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    qFatal("database connection failed");
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(
          hcb::LocalSchema::initialize(connection))) {
    qFatal("database migration failed");
  }
  sqlite3* const handle = connection.nativeHandle();
  QCOMPARE(
      execute(
          handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES ('account', 'google', 'connected', '[]', "
          "'[]', '2026-07-25T00:00:00Z'); "
          "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) "
          "VALUES ('list', 'account', 'list', 'Release planning', '2026-07-25T00:00:00Z'); "
          "INSERT INTO local_tasks (id, task_list_id, remote_id, title, notes, updated_at) "
          "VALUES ('task', 'list', 'task', 'Release review', 'Prepare the release', "
          "'2026-07-25T00:00:00Z'); "
          "INSERT INTO local_calendars (id, account_id, remote_id, title, updated_at) "
          "VALUES ('calendar', 'account', 'calendar', 'Release calendar', '2026-07-25T00:00:00Z'); "
          "INSERT INTO local_calendar_events (id, calendar_id, remote_id, title, description, "
          "start_at, end_at, updated_at) VALUES ('event', 'calendar', 'event', 'Review', "
          "'Release retrospective', '2026-07-26T10:00:00Z', '2026-07-26T11:00:00Z', "
          "'2026-07-25T00:00:00Z')"),
      SQLITE_OK);
}

[[nodiscard]] hcb::LocalSearchPage awaitPage(std::future<hcb::LocalSearchPageResult>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("local search timed out");
  }
  const hcb::LocalSearchPageResult result = future.get();
  if (!std::holds_alternative<hcb::LocalSearchPage>(result)) {
    qFatal("local search failed: %s", qPrintable(std::get<hcb::AppError>(result).message()));
  }
  return std::get<hcb::LocalSearchPage>(result);
}

} // namespace

void LocalSearchServiceTest::returnsRankedAndPagedResultsAcrossResources() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  initializeDatabase(*database);
  hcb::LocalSearchService service(database->databasePath());
  QCOMPARE(service.ready().wait_for(2s), std::future_status::ready);
  QVERIFY(!service.ready().get().has_value());
  std::future<hcb::LocalSearchPageResult> firstFuture =
      service.search({.query = QStringLiteral("release"), .limit = 2});
  const hcb::LocalSearchPage first = awaitPage(firstFuture);
  QCOMPARE(first.items.size(), 2);
  QVERIFY(first.totalKnown >= 4);
  QVERIFY(first.hasMore);
  QCOMPARE(first.items.front().resource, hcb::LocalSearchResource::Task);
  std::future<hcb::LocalSearchPageResult> secondFuture =
      service.search({.query = QStringLiteral("release"), .offset = 2, .limit = 2});
  const hcb::LocalSearchPage second = awaitPage(secondFuture);
  QCOMPARE(second.items.size(), 2);
  QVERIFY(second.hasMore);
  std::future<hcb::LocalSearchPageResult> thirdFuture =
      service.search({.query = QStringLiteral("release"), .offset = 4, .limit = 2});
  const hcb::LocalSearchPage third = awaitPage(thirdFuture);
  QCOMPARE(third.items.size(), 1);
  QVERIFY(!third.hasMore);
}

void LocalSearchServiceTest::rejectsInvalidRequests() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  initializeDatabase(*database);
  hcb::LocalSearchService service(database->databasePath());
  std::future<hcb::LocalSearchPageResult> invalid = service.search({.query = QStringLiteral("  ")});
  if (invalid.wait_for(2s) != std::future_status::ready) {
    qFatal("invalid local search timed out");
  }
  const hcb::LocalSearchPageResult result = invalid.get();
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Validation);
}

void LocalSearchServiceTest::returnsCancelledForStoppedSearch() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  initializeDatabase(*database);
  hcb::LocalSearchService service(database->databasePath());
  hcb::CancellationSource cancellation;
  QVERIFY(cancellation.requestStop());
  std::future<hcb::LocalSearchPageResult> future =
      service.search({.query = QStringLiteral("release")}, cancellation.token());
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("cancelled local search timed out");
  }
  const hcb::LocalSearchPageResult result = future.get();
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Cancelled);
}

void LocalSearchServiceTest::appliesStructuredFiltersLocally() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  initializeDatabase(*database);
  hcb::LocalSearchService service(database->databasePath());
  std::future<hcb::LocalSearchPageResult> taskFuture =
      service.search({.query = QStringLiteral("source:tasks status:open due:none priority:none")});
  const hcb::LocalSearchPage tasks = awaitPage(taskFuture);
  QCOMPARE(tasks.items.size(), 1);
  QCOMPARE(tasks.items.front().resource, hcb::LocalSearchResource::Task);
  QCOMPARE(tasks.items.front().id, QStringLiteral("task"));

  std::future<hcb::LocalSearchPageResult> eventFuture =
      service.search({.query = QStringLiteral("source:calendar start:2026-07-26")});
  const hcb::LocalSearchPage events = awaitPage(eventFuture);
  QCOMPARE(events.items.size(), 1);
  QCOMPARE(events.items.front().resource, hcb::LocalSearchResource::Event);
  QCOMPARE(events.items.front().id, QStringLiteral("event"));

  std::future<hcb::LocalSearchPageResult> noteFuture =
      service.search({.query = QStringLiteral("source:notes body:yes")});
  const hcb::LocalSearchPage notes = awaitPage(noteFuture);
  QCOMPARE(notes.items.size(), 1);
  QCOMPARE(notes.items.front().resource, hcb::LocalSearchResource::Note);
  QCOMPARE(notes.items.front().id, QStringLiteral("task"));
}

void LocalSearchServiceTest::returnsAllStructuredMatchesBeyondLegacyCandidateCap() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  initializeDatabase(*database);
  hcb::SqliteConnectionResult connectionResult =
      database->open(hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  QCOMPARE(execute(connection.nativeHandle(), R"(
    WITH RECURSIVE numbers(value) AS (
      VALUES(1) UNION ALL SELECT value + 1 FROM numbers WHERE value < 300
    )
    INSERT INTO local_tasks (id, task_list_id, remote_id, title, notes, updated_at)
    SELECT 'bulk-' || value, 'list', 'bulk-' || value, 'bulk review ' || value, '',
           '2026-07-25T00:00:00Z' FROM numbers
  )"), SQLITE_OK);
  hcb::LocalSearchService service(database->databasePath());
  QCOMPARE(service.ready().wait_for(2s), std::future_status::ready);
  QVERIFY(!service.ready().get().has_value());
  std::future<hcb::LocalSearchPageResult> firstFuture =
      service.search({.query = QStringLiteral("bulk source:tasks"), .limit = 100});
  const hcb::LocalSearchPage first = awaitPage(firstFuture);
  QCOMPARE(first.totalKnown, 300);
  QCOMPARE(first.items.size(), 100);
  QVERIFY(first.hasMore);
  std::future<hcb::LocalSearchPageResult> finalFuture =
      service.search({.query = QStringLiteral("bulk source:tasks"), .offset = 200, .limit = 100});
  const hcb::LocalSearchPage final = awaitPage(finalFuture);
  QCOMPARE(final.items.size(), 100);
  QVERIFY(!final.hasMore);
}

void LocalSearchServiceTest::rejectsInvalidStructuredSyntax() {
  const hcb::LocalSearchQueryResult parsed =
      hcb::LocalSearchQuery::parse(QStringLiteral("status:unrecognised"));
  QVERIFY(std::holds_alternative<hcb::AppError>(parsed));
  QCOMPARE(std::get<hcb::AppError>(parsed).code(), hcb::AppErrorCode::Validation);
}

QTEST_GUILESS_MAIN(LocalSearchServiceTest)

#include "LocalSearchServiceTest.moc"
