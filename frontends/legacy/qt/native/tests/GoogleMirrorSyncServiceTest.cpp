#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QtTest/QTest>

#include "core/CalendarReadService.h"
#include "core/Clock.h"
#include "core/GoogleCalendarEventPullClient.h"
#include "core/GoogleCalendarListPullClient.h"
#include "core/GoogleCalendarMirrorSyncService.h"
#include "core/GoogleHttpClient.h"
#include "core/GoogleMirrorStore.h"
#include "core/GoogleSyncRecoveryService.h"
#include "core/GoogleTaskListPullClient.h"
#include "core/GoogleTaskMirrorSyncService.h"
#include "core/GoogleTaskPullClient.h"
#include "core/SyncCheckpointStore.h"
#include "core/TaskMutationService.h"
#include "core/TaskRecurrenceMarker.h"
#include "data/LocalSchema.h"
#include "data/SqliteConnection.h"
#include "sqlite3.h"
#include "support/MockNetworkAccessManager.h"
#include "support/TemporarySqliteDatabase.h"

#include <QUrlQuery>

#include <chrono>
#include <future>
#include <memory>
#include <optional>
#include <utility>
#include <variant>

using namespace std::chrono_literals;

class GoogleMirrorSyncServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void tasksUsePersistedOverlapWatermark();
  void tasksReconcileExactRecurringDuplicatesAfterPull();
  void calendarReusesTokensAndRecoversInvalidEventToken();
};

namespace {

class TestClock final : public hcb::Clock {
public:
  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override {
    return hcb::WallTimePoint{std::chrono::seconds{1'700'000'000}};
  }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return hcb::MonotonicTimePoint{};
  }
};

void execute(sqlite3* handle, const char* sql) {
  char* error = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &error);
  const QString message = error == nullptr ? QString() : QString::fromUtf8(error);
  sqlite3_free(error);
  QVERIFY2(result == SQLITE_OK, qPrintable(message));
}

[[nodiscard]] int count(sqlite3* handle, const char* sql) {
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
      SQLITE_OK) {
    qFatal("SQLite count preparation failed");
  }
  if (sqlite3_step(statement) != SQLITE_ROW) {
    sqlite3_finalize(statement);
    qFatal("SQLite count query failed");
  }
  const int value = sqlite3_column_int(statement, 0);
  if (sqlite3_finalize(statement) != SQLITE_OK) {
    qFatal("SQLite count finalization failed");
  }
  return value;
}

void prepareAccount(const hcb::test::TemporarySqliteDatabase& database) {
  hcb::SqliteConnectionResult connectionResult =
      database.open(hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  const hcb::SqliteMigrationRunResultOrError schemaResult =
      hcb::LocalSchema::initialize(connection);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult)) {
    return;
  }
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  if (handle == nullptr) {
    return;
  }
  execute(handle,
          "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
          "missing_scopes_json, updated_at) VALUES "
          "('google', 'google', 'connected', '[]', '[]', '2024-07-24T10:00:00.000Z')");
}

void verifyReady(const std::shared_future<hcb::SqliteWriteResult>& ready) {
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  QVERIFY(!ready.get().has_value());
}

template <typename Result> [[nodiscard]] Result await(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("mirror sync future timed out");
  }
  return future.get();
}

[[nodiscard]] std::unique_ptr<hcb::test::TemporarySqliteDatabase> makeDatabase() {
  hcb::test::TemporarySqliteDatabaseResult result =
      hcb::test::TemporarySqliteDatabase::create();
  if (!std::holds_alternative<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(result)) {
    return {};
  }
  return std::move(std::get<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(result));
}

} // namespace

void GoogleMirrorSyncServiceTest::tasksUsePersistedOverlapWatermark() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = makeDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  prepareAccount(*database);
  TestClock clock;
  hcb::GoogleMirrorStore mirror(database->databasePath(), clock);
  hcb::SyncCheckpointStore checkpoints(database->databasePath(), clock);
  verifyReady(mirror.ready());
  verifyReady(checkpoints.ready());

  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.status = 429, .body = QByteArray("{\"error\":{\"message\":\"slow\"}}")});
  manager.enqueue({.body = QByteArray("{\"items\":[{\"id\":\"list-1\",\"title\":\"Inbox\"}]}")});
  manager.enqueue(
      {.body = QByteArray("{\"items\":[{\"id\":\"task-1\",\"title\":\"One\","
                           "\"status\":\"needsAction\"}]}"),
       .headers = {{QByteArray("Date"), QByteArray("Wed, 24 Jul 2024 10:00:00 GMT")}}});
  manager.enqueue({.body = QByteArray("{\"items\":[{\"id\":\"list-1\",\"title\":\"Inbox\"}]}")});
  manager.enqueue({.body = QByteArray("{\"items\":[]}")});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleTaskListPullClient listClient(http);
  hcb::GoogleTaskPullClient taskClient(http);
  hcb::GoogleTaskMirrorSyncService service(
      listClient,
      taskClient,
      mirror,
      checkpoints,
      clock,
      hcb::SyncBackoffPolicy({.baseDelayMilliseconds = 0,
                              .maximumDelayMilliseconds = 0,
                              .jitterMilliseconds = 0,
                              .maximumAttempts = 2}));

  std::future<hcb::GoogleTaskMirrorSyncResultOrError> first =
      service.sync(QStringLiteral("google"), QStringLiteral("access-token"));
  QTRY_VERIFY_WITH_TIMEOUT(first.wait_for(0ms) == std::future_status::ready, 2'000);
  const hcb::GoogleTaskMirrorSyncResultOrError firstResult = first.get();
  if (std::holds_alternative<hcb::GoogleApiError>(firstResult)) {
    QFAIL(qPrintable(std::get<hcb::GoogleApiError>(firstResult).message()));
  }
  if (std::holds_alternative<hcb::AppError>(firstResult)) {
    QFAIL(qPrintable(std::get<hcb::AppError>(firstResult).message()));
  }
  QVERIFY(std::holds_alternative<hcb::GoogleTaskMirrorSyncResult>(firstResult));
  QCOMPARE(std::get<hcb::GoogleTaskMirrorSyncResult>(firstResult).fullReconciledListCount, 1);

  std::future<hcb::GoogleTaskMirrorSyncResultOrError> second =
      service.sync(QStringLiteral("google"), QStringLiteral("access-token"));
  QTRY_VERIFY_WITH_TIMEOUT(second.wait_for(0ms) == std::future_status::ready, 2'000);
  const hcb::GoogleTaskMirrorSyncResultOrError secondResult = second.get();
  QVERIFY(std::holds_alternative<hcb::GoogleTaskMirrorSyncResult>(secondResult));
  QCOMPARE(std::get<hcb::GoogleTaskMirrorSyncResult>(secondResult).fullReconciledListCount, 0);
  QCOMPARE(manager.requests().size(), 5);
  const QUrlQuery deltaQuery(manager.requests().at(4).request.url());
  QCOMPARE(deltaQuery.queryItemValue(QStringLiteral("updatedMin")),
           QStringLiteral("2024-07-24T09:58:00.000Z"));
  QCOMPARE(deltaQuery.queryItemValue(QStringLiteral("showAssigned")), QStringLiteral("true"));
  QCOMPARE(deltaQuery.queryItemValue(QStringLiteral("showCompleted")), QStringLiteral("true"));
  QCOMPARE(deltaQuery.queryItemValue(QStringLiteral("showDeleted")), QStringLiteral("true"));
  QCOMPARE(deltaQuery.queryItemValue(QStringLiteral("showHidden")), QStringLiteral("true"));

  std::future<hcb::SyncCheckpointLookupResult> checkpoint = checkpoints.find(
      {.accountId = QStringLiteral("google"),
       .resourceType = hcb::SyncCheckpointResourceType::TaskListWatermark,
       .resourceId = QStringLiteral("list-1")});
  const hcb::SyncCheckpointLookupResult checkpointResult = await(checkpoint);
  QVERIFY(std::holds_alternative<std::optional<hcb::SyncCheckpoint>>(checkpointResult));
  QCOMPARE(std::get<std::optional<hcb::SyncCheckpoint>>(checkpointResult)->syncToken,
           QStringLiteral("2024-07-24T09:58:00.000Z"));
  hcb::CancellationSource cancellation;
  QVERIFY(cancellation.requestStop());
  std::future<hcb::GoogleTaskMirrorSyncResultOrError> cancelled =
      service.sync(QStringLiteral("google"), QStringLiteral("access-token"), cancellation.token());
  QTRY_VERIFY_WITH_TIMEOUT(cancelled.wait_for(0ms) == std::future_status::ready, 2'000);
  const hcb::GoogleTaskMirrorSyncResultOrError cancelledResult = cancelled.get();
  QVERIFY(std::holds_alternative<hcb::AppError>(cancelledResult));
  QCOMPARE(std::get<hcb::AppError>(cancelledResult).code(), hcb::AppErrorCode::Cancelled);
  QCOMPARE(manager.requests().size(), 5);
}

void GoogleMirrorSyncServiceTest::tasksReconcileExactRecurringDuplicatesAfterPull() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = makeDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  prepareAccount(*database);
  TestClock clock;
  hcb::GoogleMirrorStore mirror(database->databasePath(), clock);
  hcb::SyncCheckpointStore checkpoints(database->databasePath(), clock);
  hcb::TaskMutationService mutations(database->databasePath(), clock);
  verifyReady(mirror.ready());
  verifyReady(checkpoints.ready());
  verifyReady(mutations.ready());
  hcb::TaskRecurrenceMarker marker{
      .seriesId = QStringLiteral("18e14b9a-30df-459c-9e94-a10f6f6babb2"),
      .occurrenceId = QStringLiteral("18e14b9a-30df-459c-9e94-a10f6f6babb2:0"),
      .frequency = hcb::TaskRecurrenceFrequency::Weekly,
      .anchorDate = QStringLiteral("2026-07-26"),
      .timeZone = QStringLiteral("Asia/Singapore"),
      .templateTitle = QStringLiteral("Duplicate"),
      .templateDueDate = QStringLiteral("2026-07-26"),
      .templatePriority = QStringLiteral("medium")};
  const hcb::TaskRecurrenceSerializationResult serialized =
      hcb::serializeTaskRecurrenceNotes(QStringLiteral("body"), marker);
  QVERIFY(!serialized.error.has_value());
  if (serialized.error.has_value()) {
    return;
  }
  const QJsonArray tasks{QJsonObject{{QStringLiteral("id"), QStringLiteral("duplicate-a")},
                                     {QStringLiteral("title"), QStringLiteral("Duplicate")},
                                     {QStringLiteral("notes"), serialized.notes},
                                     {QStringLiteral("status"), QStringLiteral("needsAction")},
                                     {QStringLiteral("etag"), QStringLiteral("etag-a")}},
                         QJsonObject{{QStringLiteral("id"), QStringLiteral("duplicate-b")},
                                     {QStringLiteral("title"), QStringLiteral("Duplicate")},
                                     {QStringLiteral("notes"), serialized.notes},
                                     {QStringLiteral("status"), QStringLiteral("needsAction")},
                                     {QStringLiteral("etag"), QStringLiteral("etag-b")}}};
  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray("{\"items\":[{\"id\":\"list-1\",\"title\":\"Inbox\"}]}")});
  manager.enqueue({.body = QJsonDocument(QJsonObject{{QStringLiteral("items"), tasks}})
                               .toJson(QJsonDocument::Compact)});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleTaskListPullClient listClient(http);
  hcb::GoogleTaskPullClient taskClient(http);
  hcb::GoogleTaskMirrorSyncService service(listClient,
                                           taskClient,
                                           mirror,
                                           checkpoints,
                                           clock,
                                           hcb::SyncBackoffPolicy({.baseDelayMilliseconds = 0,
                                                                   .maximumDelayMilliseconds = 0,
                                                                   .jitterMilliseconds = 0,
                                                                   .maximumAttempts = 2}),
                                           &mutations);
  std::future<hcb::GoogleTaskMirrorSyncResultOrError> sync =
      service.sync(QStringLiteral("google"), QStringLiteral("access-token"));
  QTRY_VERIFY_WITH_TIMEOUT(sync.wait_for(0ms) == std::future_status::ready, 2'000);
  const hcb::GoogleTaskMirrorSyncResultOrError syncResult = sync.get();
  QVERIFY(std::holds_alternative<hcb::GoogleTaskMirrorSyncResult>(syncResult));
  if (!std::holds_alternative<hcb::GoogleTaskMirrorSyncResult>(syncResult)) {
    return;
  }
  QCOMPARE(std::get<hcb::GoogleTaskMirrorSyncResult>(syncResult).removedRecurringTaskDuplicateCount,
           1);
  hcb::SqliteConnectionResult connectionResult =
      database->open(hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  if (handle == nullptr) {
    return;
  }
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_tasks WHERE deleted_at IS NOT NULL"), 1);
}

void GoogleMirrorSyncServiceTest::calendarReusesTokensAndRecoversInvalidEventToken() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = makeDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  prepareAccount(*database);
  TestClock clock;
  hcb::GoogleMirrorStore mirror(database->databasePath(), clock);
  hcb::SyncCheckpointStore checkpoints(database->databasePath(), clock);
  hcb::CalendarReadService calendarRead(database->databasePath());
  verifyReady(mirror.ready());
  verifyReady(checkpoints.ready());
  verifyReady(calendarRead.ready());

  hcb::test::MockNetworkAccessManager manager;
  manager.enqueue({.body = QByteArray(
      "{\"nextSyncToken\":\"calendar-list-token-1\",\"items\":[{\"id\":\"primary\","
      "\"summary\":\"Primary\",\"accessRole\":\"owner\",\"selected\":true,"
      "\"primary\":true}]}")});
  manager.enqueue({.body = QByteArray(
      "{\"nextSyncToken\":\"event-token-1\",\"items\":[{\"id\":\"event-1\","
      "\"status\":\"confirmed\",\"summary\":\"Planning\",\"start\":{\"dateTime\":"
      "\"2026-07-26T09:00:00Z\"},\"end\":{\"dateTime\":\"2026-07-26T10:00:00Z\"}}]}")});
  manager.enqueue({.body = QByteArray("{\"nextSyncToken\":\"calendar-list-token-2\",\"items\":[]}")});
  manager.enqueue({.body = QByteArray("{\"nextSyncToken\":\"event-token-2\",\"items\":[]}")});
  manager.enqueue({.body = QByteArray("{\"nextSyncToken\":\"calendar-list-token-3\",\"items\":[]}")});
  manager.enqueue({.status = 410, .body = QByteArray("{\"error\":{\"message\":\"gone\"}}")});
  manager.enqueue({.body = QByteArray("{\"nextSyncToken\":\"event-token-resync\",\"items\":[]}")});
  hcb::GoogleHttpClient http(nullptr, &manager);
  hcb::GoogleCalendarListPullClient listClient(http);
  hcb::GoogleCalendarEventPullClient eventClient(http);
  hcb::GoogleSyncRecoveryService recovery(checkpoints);
  hcb::GoogleCalendarMirrorSyncService service(
      listClient, eventClient, calendarRead, mirror, checkpoints, recovery);

  std::future<hcb::GoogleCalendarMirrorSyncResultOrError> first =
      service.sync(QStringLiteral("google"), QStringLiteral("access-token"));
  QTRY_VERIFY_WITH_TIMEOUT(first.wait_for(0ms) == std::future_status::ready, 2'000);
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarMirrorSyncResult>(first.get()));

  hcb::SqliteConnectionResult connectionResult =
      database->open(hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  if (handle == nullptr) {
    return;
  }
  execute(handle,
          "INSERT INTO local_pending_mutations "
          "(id, account_id, resource_type, resource_id, operation, payload_json, status, "
          "attempt_count, updated_at) "
          "SELECT 'event-mutation', 'google', 'event', id, 'update', '{}', 'pending', 0, "
          "'2024-07-24T10:00:00.000Z' FROM local_calendar_events WHERE remote_id = 'event-1'");

  std::future<hcb::GoogleCalendarMirrorSyncResultOrError> second =
      service.sync(QStringLiteral("google"), QStringLiteral("access-token"));
  QTRY_VERIFY_WITH_TIMEOUT(second.wait_for(0ms) == std::future_status::ready, 2'000);
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarMirrorSyncResult>(second.get()));

  std::future<hcb::GoogleCalendarMirrorSyncResultOrError> third =
      service.sync(QStringLiteral("google"), QStringLiteral("access-token"));
  QTRY_VERIFY_WITH_TIMEOUT(third.wait_for(0ms) == std::future_status::ready, 2'000);
  const hcb::GoogleCalendarMirrorSyncResultOrError thirdResult = third.get();
  QVERIFY(std::holds_alternative<hcb::GoogleCalendarMirrorSyncResult>(thirdResult));
  QCOMPARE(std::get<hcb::GoogleCalendarMirrorSyncResult>(thirdResult).fullReconciledCalendarCount,
           1);
  QCOMPARE(manager.requests().size(), 7);
  QCOMPARE(QUrlQuery(manager.requests().at(2).request.url()).queryItemValue(
               QStringLiteral("syncToken")),
           QStringLiteral("calendar-list-token-1"));
  QCOMPARE(QUrlQuery(manager.requests().at(3).request.url()).queryItemValue(
               QStringLiteral("syncToken")),
           QStringLiteral("event-token-1"));
  QCOMPARE(QUrlQuery(manager.requests().at(4).request.url()).queryItemValue(
               QStringLiteral("syncToken")),
           QStringLiteral("calendar-list-token-2"));
  QCOMPARE(QUrlQuery(manager.requests().at(5).request.url()).queryItemValue(
               QStringLiteral("syncToken")),
           QStringLiteral("event-token-2"));
  QVERIFY(!QUrlQuery(manager.requests().at(6).request.url()).hasQueryItem(
      QStringLiteral("syncToken")));
  QCOMPARE(count(handle, "SELECT COUNT(*) FROM local_calendar_events WHERE deleted_at IS NULL"),
           1);

  std::future<hcb::SyncCheckpointLookupResult> listCheckpoint = checkpoints.find(
      {.accountId = QStringLiteral("google"),
       .resourceType = hcb::SyncCheckpointResourceType::CalendarList,
       .resourceId = QStringLiteral("calendar-list")});
  const hcb::SyncCheckpointLookupResult listCheckpointResult = await(listCheckpoint);
  QVERIFY(std::holds_alternative<std::optional<hcb::SyncCheckpoint>>(listCheckpointResult));
  QCOMPARE(std::get<std::optional<hcb::SyncCheckpoint>>(listCheckpointResult)->syncToken,
           QStringLiteral("calendar-list-token-3"));
  std::future<hcb::SyncCheckpointLookupResult> eventCheckpoint = checkpoints.find(
      {.accountId = QStringLiteral("google"),
       .resourceType = hcb::SyncCheckpointResourceType::CalendarEvent,
       .resourceId = QStringLiteral("primary")});
  const hcb::SyncCheckpointLookupResult eventCheckpointResult = await(eventCheckpoint);
  QVERIFY(std::holds_alternative<std::optional<hcb::SyncCheckpoint>>(eventCheckpointResult));
  QCOMPARE(std::get<std::optional<hcb::SyncCheckpoint>>(eventCheckpointResult)->syncToken,
           QStringLiteral("event-token-resync"));
  QCOMPARE(std::get<std::optional<hcb::SyncCheckpoint>>(eventCheckpointResult)
               ->metadata.value(QStringLiteral("singleEvents")),
           QJsonValue(false));
}

QTEST_GUILESS_MAIN(GoogleMirrorSyncServiceTest)

#include "GoogleMirrorSyncServiceTest.moc"
