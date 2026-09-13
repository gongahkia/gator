#include <QtTest/QTest>

#include "core/Clock.h"
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

class SyncCheckpointStoreTest final : public QObject {
  Q_OBJECT

private slots:
  void persistsAndReplacesCalendarCheckpoints();
  void rejectsInvalidCheckpointInput();
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
    qFatal("sync checkpoint request timed out");
  }
  return future.get();
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
  constexpr char sql[] = R"(
INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, missing_scopes_json, updated_at)
VALUES ('account-1', 'google', 'connected', '[]', '[]', '2024-08-01T00:00:00.000Z')
)";
  char* error = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &error);
  sqlite3_free(error);
  QCOMPARE(result, SQLITE_OK);
}

void verifyReady(hcb::SyncCheckpointStore& store) {
  const std::shared_future<hcb::SqliteWriteResult> ready = store.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  QVERIFY(!ready.get().has_value());
}

} // namespace

void SyncCheckpointStoreTest::persistsAndReplacesCalendarCheckpoints() {
  hcb::test::TemporarySqliteDatabaseResult databaseResult =
      hcb::test::TemporarySqliteDatabase::create();
  QVERIFY(
      std::holds_alternative<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(databaseResult));
  if (!std::holds_alternative<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(
          databaseResult)) {
    return;
  }
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database =
      std::move(std::get<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(databaseResult));
  prepareAccount(*database);
  TestClock clock;
  hcb::SyncCheckpointStore store(database->databasePath(), clock);
  verifyReady(store);
  const hcb::SyncCheckpointKey key{.accountId = QStringLiteral("account-1"),
                                   .resourceType = hcb::SyncCheckpointResourceType::CalendarEvent,
                                   .resourceId = QStringLiteral("calendar-1")};

  const QJsonObject metadata{{QStringLiteral("singleEvents"), false}};
  std::future<hcb::SyncCheckpointSaveResult> firstSave =
      store.save(key, QStringLiteral("sync-1"), metadata);
  const hcb::SyncCheckpointSaveResult firstResult = awaitResult(firstSave);
  QVERIFY(std::holds_alternative<hcb::SyncCheckpoint>(firstResult));
  QCOMPARE(std::get<hcb::SyncCheckpoint>(firstResult).syncToken, QStringLiteral("sync-1"));

  std::future<hcb::SyncCheckpointLookupResult> loaded = store.find(key);
  const hcb::SyncCheckpointLookupResult loadedResult = awaitResult(loaded);
  QVERIFY(std::holds_alternative<std::optional<hcb::SyncCheckpoint>>(loadedResult));
  QVERIFY(std::get<std::optional<hcb::SyncCheckpoint>>(loadedResult).has_value());
  QCOMPARE(std::get<std::optional<hcb::SyncCheckpoint>>(loadedResult)->syncToken,
           QStringLiteral("sync-1"));
  QCOMPARE(std::get<std::optional<hcb::SyncCheckpoint>>(loadedResult)->metadata, metadata);

  std::future<hcb::SyncCheckpointSaveResult> replacement =
      store.save(key, QStringLiteral("sync-2"), metadata);
  QVERIFY(std::holds_alternative<hcb::SyncCheckpoint>(awaitResult(replacement)));
  std::future<hcb::SyncCheckpointLookupResult> replaced = store.find(key);
  const hcb::SyncCheckpointLookupResult replacedResult = awaitResult(replaced);
  QVERIFY(std::holds_alternative<std::optional<hcb::SyncCheckpoint>>(replacedResult));
  QCOMPARE(std::get<std::optional<hcb::SyncCheckpoint>>(replacedResult)->syncToken,
           QStringLiteral("sync-2"));
}

void SyncCheckpointStoreTest::rejectsInvalidCheckpointInput() {
  hcb::test::TemporarySqliteDatabaseResult databaseResult =
      hcb::test::TemporarySqliteDatabase::create();
  QVERIFY(
      std::holds_alternative<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(databaseResult));
  if (!std::holds_alternative<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(
          databaseResult)) {
    return;
  }
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database =
      std::move(std::get<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(databaseResult));
  TestClock clock;
  hcb::SyncCheckpointStore store(database->databasePath(), clock);
  verifyReady(store);
  std::future<hcb::SyncCheckpointSaveResult> invalid =
      store.save({.accountId = QStringLiteral("account-1"),
                  .resourceType = hcb::SyncCheckpointResourceType::CalendarList,
                  .resourceId = QStringLiteral("calendar-list")},
                 QString());
  const hcb::SyncCheckpointSaveResult result = awaitResult(invalid);
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Validation);
}

QTEST_GUILESS_MAIN(SyncCheckpointStoreTest)

#include "SyncCheckpointStoreTest.moc"
