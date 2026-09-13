#include <QtTest/QTest>

#include "core/Clock.h"
#include "core/GoogleApiError.h"
#include "core/GoogleSyncRecoveryService.h"
#include "core/SyncCheckpointStore.h"
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

class GoogleSyncRecoveryServiceTest final : public QObject {
  Q_OBJECT

private slots:
  void clearsExpiredCheckpointBeforeFullResync();
  void preservesCheckpointForOtherGoogleErrors();
};

namespace {

class TestClock final : public hcb::Clock {
public:
  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override {
    return hcb::WallTimePoint{std::chrono::seconds{1'725'000'000}};
  }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return hcb::MonotonicTimePoint{};
  }
};

template <typename Result> [[nodiscard]] Result awaitResult(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("sync recovery request timed out");
  }
  return future.get();
}

[[nodiscard]] std::unique_ptr<hcb::test::TemporarySqliteDatabase> createDatabase() {
  hcb::test::TemporarySqliteDatabaseResult result = hcb::test::TemporarySqliteDatabase::create();
  if (!std::holds_alternative<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(result)) {
    return nullptr;
  }
  return std::move(std::get<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(result));
}

void prepareAccount(const hcb::test::TemporarySqliteDatabase& database) {
  hcb::SqliteConnectionResult connectionResult =
      database.open(hcb::SqliteOpenMode::ReadWriteCreate);
  QVERIFY(std::holds_alternative<hcb::SqliteConnection>(connectionResult));
  if (!std::holds_alternative<hcb::SqliteConnection>(connectionResult)) {
    return;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(connectionResult));
  hcb::SqliteMigrationRunResultOrError schemaResult = hcb::LocalSchema::initialize(connection);
  QVERIFY(std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult));
  if (!std::holds_alternative<hcb::SqliteMigrationRunResult>(schemaResult)) {
    return;
  }
  sqlite3* const handle = connection.nativeHandle();
  QVERIFY(handle != nullptr);
  if (handle == nullptr) {
    return;
  }
  char* error = nullptr;
  const int result = sqlite3_exec(
      handle,
      "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
      "missing_scopes_json, updated_at) VALUES ('account-1', 'google', 'connected', '[]', '[]', "
      "'2024-08-01T00:00:00.000Z')",
      nullptr,
      nullptr,
      &error);
  sqlite3_free(error);
  QCOMPARE(result, SQLITE_OK);
}

void verifyReady(hcb::SyncCheckpointStore& store) {
  const std::shared_future<hcb::SqliteWriteResult> ready = store.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  QVERIFY(!ready.get().has_value());
}

[[nodiscard]] hcb::SyncCheckpointKey checkpointKey() {
  return {.accountId = QStringLiteral("account-1"),
          .resourceType = hcb::SyncCheckpointResourceType::CalendarEvent,
          .resourceId = QStringLiteral("calendar-1")};
}

} // namespace

void GoogleSyncRecoveryServiceTest::clearsExpiredCheckpointBeforeFullResync() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  prepareAccount(*database);
  TestClock clock;
  hcb::SyncCheckpointStore store(database->databasePath(), clock);
  verifyReady(store);
  std::future<hcb::SyncCheckpointSaveResult> saved =
      store.save(checkpointKey(), QStringLiteral("sync-1"));
  QVERIFY(std::holds_alternative<hcb::SyncCheckpoint>(awaitResult(saved)));
  hcb::GoogleSyncRecoveryService recovery(store);
  const hcb::GoogleApiError invalidToken = hcb::GoogleApiError::fromHttpStatus(410, u"expired");

  std::future<hcb::GoogleSyncRecoveryResultOrError> recovered =
      recovery.recover(checkpointKey(), invalidToken);
  const hcb::GoogleSyncRecoveryResultOrError result = awaitResult(recovered);
  QVERIFY(std::holds_alternative<hcb::GoogleSyncRecoveryResult>(result));
  QCOMPARE(std::get<hcb::GoogleSyncRecoveryResult>(result),
           hcb::GoogleSyncRecoveryResult::ReadyForFullResync);
  std::future<hcb::SyncCheckpointLookupResult> lookup = store.find(checkpointKey());
  const hcb::SyncCheckpointLookupResult lookupResult = awaitResult(lookup);
  QVERIFY(std::holds_alternative<std::optional<hcb::SyncCheckpoint>>(lookupResult));
  QVERIFY(!std::get<std::optional<hcb::SyncCheckpoint>>(lookupResult).has_value());
}

void GoogleSyncRecoveryServiceTest::preservesCheckpointForOtherGoogleErrors() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  prepareAccount(*database);
  TestClock clock;
  hcb::SyncCheckpointStore store(database->databasePath(), clock);
  verifyReady(store);
  std::future<hcb::SyncCheckpointSaveResult> saved =
      store.save(checkpointKey(), QStringLiteral("sync-1"));
  QVERIFY(std::holds_alternative<hcb::SyncCheckpoint>(awaitResult(saved)));
  hcb::GoogleSyncRecoveryService recovery(store);
  const hcb::GoogleApiError rateLimited =
      hcb::GoogleApiError::fromHttpStatus(429, u"quotaExceeded");

  std::future<hcb::GoogleSyncRecoveryResultOrError> recovered =
      recovery.recover(checkpointKey(), rateLimited);
  const hcb::GoogleSyncRecoveryResultOrError result = awaitResult(recovered);
  QVERIFY(std::holds_alternative<hcb::GoogleSyncRecoveryResult>(result));
  QCOMPARE(std::get<hcb::GoogleSyncRecoveryResult>(result),
           hcb::GoogleSyncRecoveryResult::NotRequired);
  std::future<hcb::SyncCheckpointLookupResult> lookup = store.find(checkpointKey());
  const hcb::SyncCheckpointLookupResult lookupResult = awaitResult(lookup);
  QVERIFY(std::holds_alternative<std::optional<hcb::SyncCheckpoint>>(lookupResult));
  QCOMPARE(std::get<std::optional<hcb::SyncCheckpoint>>(lookupResult)->syncToken,
           QStringLiteral("sync-1"));
}

QTEST_GUILESS_MAIN(GoogleSyncRecoveryServiceTest)

#include "GoogleSyncRecoveryServiceTest.moc"
