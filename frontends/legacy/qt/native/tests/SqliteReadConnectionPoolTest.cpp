#include <QDir>
#include <QFileInfo>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include <atomic>
#include <chrono>
#include <future>
#include <optional>
#include <set>
#include <stdexcept>
#include <thread>
#include <utility>
#include <variant>
#include <vector>

#include "data/SqliteReadConnectionPool.h"
#include "sqlite3.h"

using namespace std::chrono_literals;

class SqliteReadConnectionPoolTest final : public QObject {
  Q_OBJECT

private slots:
  void boundsConcurrentReadsAndUsesWorkerThreads();
  void returnsTypedReadResults();
  void reportsConnectionStartupFailure();
};

namespace {

[[nodiscard]] std::optional<hcb::FilePath> databasePathFor(const QTemporaryDir& temporaryDirectory,
                                                           QString filename) {
  return hcb::FilePath::fromAbsolute(
      QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath()).filePath(std::move(filename)));
}

[[nodiscard]] std::optional<hcb::AppError> execute(sqlite3* handle, const char* sql) {
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  sqlite3_free(errorMessage);
  if (result != SQLITE_OK) {
    return hcb::AppError(hcb::AppErrorCode::Database,
                         QStringLiteral("SQLite test setup failed (%1)").arg(result));
  }
  return std::nullopt;
}

[[nodiscard]] bool prepareDatabase(const hcb::FilePath& databasePath) {
  hcb::SqliteConnectionResult result =
      hcb::SqliteConnectionFactory::open(databasePath, hcb::SqliteOpenMode::ReadWriteCreate);
  if (!std::holds_alternative<hcb::SqliteConnection>(result)) {
    return false;
  }
  hcb::SqliteConnection connection = std::move(std::get<hcb::SqliteConnection>(result));
  return !execute(connection.nativeHandle(),
                  "CREATE TABLE reads (id INTEGER PRIMARY KEY); INSERT INTO reads VALUES (1), (2), "
                  "(3);")
              .has_value();
}

[[nodiscard]] int readCount(hcb::SqliteConnection& connection) {
  sqlite3_stmt* statement = nullptr;
  if (sqlite3_prepare_v2(
          connection.nativeHandle(), "SELECT COUNT(*) FROM reads", -1, &statement, nullptr) !=
      SQLITE_OK) {
    throw std::runtime_error("SQLite query preparation failed");
  }
  const int stepResult = sqlite3_step(statement);
  const int count = sqlite3_column_int(statement, 0);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_ROW || finalizeResult != SQLITE_OK) {
    throw std::runtime_error("SQLite query execution failed");
  }
  return count;
}

} // namespace

void SqliteReadConnectionPoolTest::boundsConcurrentReadsAndUsesWorkerThreads() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath =
      databasePathFor(temporaryDirectory, QStringLiteral("hot-cross-buns.sqlite"));
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  QVERIFY(prepareDatabase(*databasePath));

  hcb::SqliteReadConnectionPool pool(*databasePath, 2);
  const std::shared_future<std::optional<hcb::AppError>> ready = pool.ready();
  QVERIFY(ready.wait_for(2s) == std::future_status::ready);
  const std::optional<hcb::AppError> startupError = ready.get();
  QVERIFY2(!startupError.has_value(),
           qPrintable(startupError.has_value() ? startupError->message() : QString()));

  std::atomic<int> startedReads{0};
  std::promise<void> firstTwoStarted;
  std::atomic<bool> startedPromiseSet{false};
  std::promise<void> releasePromise;
  const std::shared_future<void> releaseReads = releasePromise.get_future().share();
  std::vector<std::future<hcb::SqliteReadResult<std::thread::id>>> reads;
  for (int index = 0; index < 3; ++index) {
    reads.push_back(pool.enqueue([&](hcb::SqliteConnection& connection) {
      const int started = startedReads.fetch_add(1) + 1;
      if (started == 2 && !startedPromiseSet.exchange(true)) {
        firstTwoStarted.set_value();
      }
      releaseReads.wait();
      if (readCount(connection) != 3) {
        throw std::runtime_error("SQLite read count is invalid");
      }
      return std::this_thread::get_id();
    }));
  }

  std::future<void> firstTwoStartedFuture = firstTwoStarted.get_future();
  QVERIFY(firstTwoStartedFuture.wait_for(2s) == std::future_status::ready);
  QCOMPARE(startedReads.load(), 2);
  QVERIFY(reads.at(2).wait_for(0ms) == std::future_status::timeout);
  releasePromise.set_value();

  std::set<std::thread::id> workerThreads;
  for (std::future<hcb::SqliteReadResult<std::thread::id>>& read : reads) {
    QVERIFY(read.wait_for(2s) == std::future_status::ready);
    const hcb::SqliteReadResult<std::thread::id> result = read.get();
    QVERIFY(std::holds_alternative<std::thread::id>(result));
    if (!std::holds_alternative<std::thread::id>(result)) {
      return;
    }
    const std::thread::id workerThread = std::get<std::thread::id>(result);
    QVERIFY(workerThread != std::this_thread::get_id());
    workerThreads.insert(workerThread);
  }
  QCOMPARE(workerThreads.size(), static_cast<std::size_t>(2));
}

void SqliteReadConnectionPoolTest::returnsTypedReadResults() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath =
      databasePathFor(temporaryDirectory, QStringLiteral("hot-cross-buns.sqlite"));
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }
  QVERIFY(prepareDatabase(*databasePath));

  hcb::SqliteReadConnectionPool pool(*databasePath, 1);
  const std::shared_future<std::optional<hcb::AppError>> ready = pool.ready();
  QVERIFY(ready.wait_for(2s) == std::future_status::ready);
  QVERIFY(!ready.get().has_value());

  std::future<hcb::SqliteReadResult<int>> read =
      pool.enqueue([](hcb::SqliteConnection& connection) { return readCount(connection); });
  QVERIFY(read.wait_for(2s) == std::future_status::ready);
  const hcb::SqliteReadResult<int> result = read.get();
  QVERIFY(std::holds_alternative<int>(result));
  if (!std::holds_alternative<int>(result)) {
    return;
  }
  QCOMPARE(std::get<int>(result), 3);
}

void SqliteReadConnectionPoolTest::reportsConnectionStartupFailure() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(
      temporaryDirectory, QStringLiteral("missing-directory/hot-cross-buns.sqlite"));
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }

  hcb::SqliteReadConnectionPool pool(*databasePath, 2);
  const std::shared_future<std::optional<hcb::AppError>> ready = pool.ready();
  QVERIFY(ready.wait_for(2s) == std::future_status::ready);
  const std::optional<hcb::AppError> startupError = ready.get();
  QVERIFY(startupError.has_value());
  QCOMPARE(startupError->code(), hcb::AppErrorCode::Database);

  std::future<hcb::SqliteReadResult<int>> read =
      pool.enqueue([](hcb::SqliteConnection&) { return 0; });
  QVERIFY(read.wait_for(0ms) == std::future_status::ready);
  const hcb::SqliteReadResult<int> readResult = read.get();
  QVERIFY(std::holds_alternative<hcb::AppError>(readResult));
  if (!std::holds_alternative<hcb::AppError>(readResult)) {
    return;
  }
  QCOMPARE(std::get<hcb::AppError>(readResult).code(), hcb::AppErrorCode::Database);
}

QTEST_GUILESS_MAIN(SqliteReadConnectionPoolTest)

#include "SqliteReadConnectionPoolTest.moc"
