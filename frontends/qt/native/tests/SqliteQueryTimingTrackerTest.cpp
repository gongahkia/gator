#include <QtTest/QTest>

#include <algorithm>
#include <chrono>
#include <memory>
#include <optional>
#include <utility>
#include <variant>
#include <vector>

#include "core/Clock.h"
#include "data/SqliteConnection.h"
#include "data/SqliteQueryTimingTracker.h"
#include "sqlite3.h"
#include "support/TemporarySqliteDatabase.h"

class SqliteQueryTimingTrackerTest final : public QObject {
  Q_OBJECT

private slots:
  void recordsReadAndWriteStatements();
  void filtersSlowStatementsAtCallerThreshold();
  void boundsSamplesAndStopsAfterClear();
  void replacesPreviousTracker();
  void remainsAttachedWhenConnectionMoves();
};

namespace {

class TestClock final : public hcb::Clock {
public:
  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override { return wallTime_; }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return hcb::MonotonicTimePoint{};
  }

private:
  const hcb::WallTimePoint wallTime_{std::chrono::seconds{1'725'000'000}};
};

[[nodiscard]] std::optional<hcb::SqliteConnection>
openConnection(const hcb::test::TemporarySqliteDatabase& database) {
  hcb::SqliteConnectionResult result = database.open(hcb::SqliteOpenMode::ReadWriteCreate);
  if (std::holds_alternative<hcb::AppError>(result)) {
    return std::nullopt;
  }
  return std::move(std::get<hcb::SqliteConnection>(result));
}

[[nodiscard]] std::unique_ptr<hcb::test::TemporarySqliteDatabase> createDatabase() {
  hcb::test::TemporarySqliteDatabaseResult result = hcb::test::TemporarySqliteDatabase::create();
  if (std::holds_alternative<hcb::AppError>(result)) {
    return nullptr;
  }
  return std::move(std::get<std::unique_ptr<hcb::test::TemporarySqliteDatabase>>(result));
}

[[nodiscard]] int execute(sqlite3* handle, const char* sql) {
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  sqlite3_free(errorMessage);
  return result;
}

} // namespace

void SqliteQueryTimingTrackerTest::recordsReadAndWriteStatements() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  std::optional<hcb::SqliteConnection> connection = openConnection(*database);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  const TestClock clock;
  const std::shared_ptr<hcb::SqliteQueryTimingTracker> tracker =
      std::make_shared<hcb::SqliteQueryTimingTracker>(clock);
  QVERIFY(!connection->installQueryTimingTracker(tracker).has_value());

  QCOMPARE(execute(connection->nativeHandle(), "CREATE TABLE entries (id INTEGER PRIMARY KEY)"),
           SQLITE_OK);
  QCOMPARE(execute(connection->nativeHandle(), "SELECT id FROM entries"), SQLITE_OK);

  const std::vector<hcb::SqliteQueryTimingSample> samples = tracker->samples();
  QVERIFY(samples.size() >= std::size_t{2});
  QVERIFY(std::any_of(
      samples.cbegin(), samples.cend(), [](const auto& sample) { return !sample.readOnly; }));
  QVERIFY(std::any_of(
      samples.cbegin(), samples.cend(), [](const auto& sample) { return sample.readOnly; }));
  QCOMPARE(samples.back().timestamp, clock.wallNow());
  QVERIFY(samples.back().elapsed >= std::chrono::nanoseconds::zero());
}

void SqliteQueryTimingTrackerTest::filtersSlowStatementsAtCallerThreshold() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  std::optional<hcb::SqliteConnection> connection = openConnection(*database);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  const TestClock clock;
  const std::shared_ptr<hcb::SqliteQueryTimingTracker> tracker =
      std::make_shared<hcb::SqliteQueryTimingTracker>(clock);
  QVERIFY(!connection->installQueryTimingTracker(tracker).has_value());
  QCOMPARE(execute(connection->nativeHandle(), "SELECT 1"), SQLITE_OK);
  QCOMPARE(execute(connection->nativeHandle(), "SELECT 2"), SQLITE_OK);

  const std::vector<hcb::SqliteQueryTimingSample> all = tracker->samples();
  const std::vector<hcb::SqliteQueryTimingSample> immediate =
      tracker->slowSamples(std::chrono::nanoseconds::zero());
  QCOMPARE(immediate.size(), all.size());
  for (std::size_t index = 0; index < all.size(); ++index) {
    QCOMPARE(immediate.at(index).sequence, all.at(index).sequence);
  }
  QVERIFY(tracker->slowSamples(std::chrono::hours{24}).empty());
}

void SqliteQueryTimingTrackerTest::boundsSamplesAndStopsAfterClear() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  std::optional<hcb::SqliteConnection> connection = openConnection(*database);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  const TestClock clock;
  const std::shared_ptr<hcb::SqliteQueryTimingTracker> tracker =
      std::make_shared<hcb::SqliteQueryTimingTracker>(clock, 2);
  QVERIFY(!connection->installQueryTimingTracker(tracker).has_value());

  QCOMPARE(execute(connection->nativeHandle(), "SELECT 1"), SQLITE_OK);
  QCOMPARE(execute(connection->nativeHandle(), "SELECT 2"), SQLITE_OK);
  QCOMPARE(execute(connection->nativeHandle(), "SELECT 3"), SQLITE_OK);
  QCOMPARE(tracker->size(), std::size_t{2});
  const std::vector<hcb::SqliteQueryTimingSample> bounded = tracker->samples();
  QVERIFY(bounded.at(0).sequence < bounded.at(1).sequence);

  connection->clearQueryTimingTracker();
  const std::size_t sampleCount = tracker->size();
  QCOMPARE(execute(connection->nativeHandle(), "SELECT 4"), SQLITE_OK);
  QCOMPARE(tracker->size(), sampleCount);
}

void SqliteQueryTimingTrackerTest::replacesPreviousTracker() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  std::optional<hcb::SqliteConnection> connection = openConnection(*database);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  const TestClock clock;
  const std::shared_ptr<hcb::SqliteQueryTimingTracker> firstTracker =
      std::make_shared<hcb::SqliteQueryTimingTracker>(clock);
  const std::shared_ptr<hcb::SqliteQueryTimingTracker> secondTracker =
      std::make_shared<hcb::SqliteQueryTimingTracker>(clock);
  QVERIFY(!connection->installQueryTimingTracker(firstTracker).has_value());
  QCOMPARE(execute(connection->nativeHandle(), "SELECT 1"), SQLITE_OK);
  QCOMPARE(firstTracker->size(), std::size_t{1});

  QVERIFY(!connection->installQueryTimingTracker(secondTracker).has_value());
  QCOMPARE(execute(connection->nativeHandle(), "SELECT 2"), SQLITE_OK);
  QCOMPARE(firstTracker->size(), std::size_t{1});
  QCOMPARE(secondTracker->size(), std::size_t{1});
}

void SqliteQueryTimingTrackerTest::remainsAttachedWhenConnectionMoves() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  std::optional<hcb::SqliteConnection> connection = openConnection(*database);
  QVERIFY(connection.has_value());
  if (!connection.has_value()) {
    return;
  }
  const TestClock clock;
  const std::shared_ptr<hcb::SqliteQueryTimingTracker> tracker =
      std::make_shared<hcb::SqliteQueryTimingTracker>(clock);
  QVERIFY(!connection->installQueryTimingTracker(tracker).has_value());
  hcb::SqliteConnection movedConnection = std::move(*connection);

  QCOMPARE(execute(movedConnection.nativeHandle(), "SELECT 1"), SQLITE_OK);
  QCOMPARE(tracker->size(), std::size_t{1});
}

QTEST_GUILESS_MAIN(SqliteQueryTimingTrackerTest)

#include "SqliteQueryTimingTrackerTest.moc"
