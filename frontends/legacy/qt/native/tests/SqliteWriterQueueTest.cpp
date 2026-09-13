#include <QDir>
#include <QFileInfo>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include <atomic>
#include <chrono>
#include <future>
#include <mutex>
#include <optional>
#include <thread>
#include <utility>
#include <variant>
#include <vector>

#include "data/SqliteWriterQueue.h"
#include "sqlite3.h"

using namespace std::chrono_literals;

class SqliteWriterQueueTest final : public QObject {
  Q_OBJECT

private slots:
  void serializesWritesOffTheCallingThread();
  void drainsAcceptedWritesDuringShutdown();
  void reportsConnectionStartupFailure();
};

namespace {

[[nodiscard]] std::optional<hcb::FilePath> databasePathFor(const QTemporaryDir& temporaryDirectory,
                                                           QString filename) {
  return hcb::FilePath::fromAbsolute(
      QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath()).filePath(std::move(filename)));
}

[[nodiscard]] hcb::SqliteWriteResult execute(sqlite3* handle, const char* sql) {
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  sqlite3_free(errorMessage);
  if (result != SQLITE_OK) {
    return hcb::AppError(hcb::AppErrorCode::Database,
                         QStringLiteral("SQLite test write failed (%1)").arg(result));
  }
  return std::nullopt;
}

void verifySuccess(std::future<hcb::SqliteWriteResult>& future) {
  QVERIFY(future.wait_for(2s) == std::future_status::ready);
  const hcb::SqliteWriteResult result = future.get();
  QVERIFY(!result.has_value());
}

} // namespace

void SqliteWriterQueueTest::serializesWritesOffTheCallingThread() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath =
      databasePathFor(temporaryDirectory, QStringLiteral("hot-cross-buns.sqlite"));
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }

  hcb::SqliteWriterQueue queue(*databasePath);
  const std::shared_future<hcb::SqliteWriteResult> ready = queue.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  QVERIFY(!ready.get().has_value());

  std::future<hcb::SqliteWriteResult> createTable =
      queue.enqueue([](hcb::SqliteConnection& connection) {
        return execute(connection.nativeHandle(),
                       "CREATE TABLE writes (sequence INTEGER PRIMARY KEY)");
      });
  verifySuccess(createTable);

  constexpr int writeCount = 32;
  std::atomic<int> activeWrites{0};
  std::atomic<int> maximumActiveWrites{0};
  std::thread::id writerThread;
  std::mutex observedMutex;
  std::vector<int> observedSequence;
  std::vector<std::future<hcb::SqliteWriteResult>> writes;
  writes.reserve(writeCount);
  for (int sequence = 0; sequence < writeCount; ++sequence) {
    writes.push_back(queue.enqueue([&, sequence](hcb::SqliteConnection& connection) {
      writerThread = std::this_thread::get_id();
      const int active = activeWrites.fetch_add(1) + 1;
      int maximum = maximumActiveWrites.load();
      while (active > maximum && !maximumActiveWrites.compare_exchange_weak(maximum, active)) {
      }
      std::this_thread::sleep_for(1ms);
      {
        std::lock_guard lock(observedMutex);
        observedSequence.push_back(sequence);
      }
      activeWrites.fetch_sub(1);
      const QByteArray sql =
          QStringLiteral("INSERT INTO writes (sequence) VALUES (%1)").arg(sequence).toUtf8();
      return execute(connection.nativeHandle(), sql.constData());
    }));
  }
  for (std::future<hcb::SqliteWriteResult>& write : writes) {
    verifySuccess(write);
  }

  QCOMPARE(maximumActiveWrites.load(), 1);
  QVERIFY(writerThread != std::this_thread::get_id());
  QCOMPARE(observedSequence.size(), static_cast<std::size_t>(writeCount));
  for (int sequence = 0; sequence < writeCount; ++sequence) {
    QCOMPARE(observedSequence.at(static_cast<std::size_t>(sequence)), sequence);
  }
}

void SqliteWriterQueueTest::drainsAcceptedWritesDuringShutdown() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath =
      databasePathFor(temporaryDirectory, QStringLiteral("hot-cross-buns.sqlite"));
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }

  hcb::SqliteWriterQueue queue(*databasePath);
  const std::shared_future<hcb::SqliteWriteResult> ready = queue.ready();
  QVERIFY(ready.wait_for(2s) == std::future_status::ready);
  QVERIFY(!ready.get().has_value());

  std::promise<void> firstWriteStarted;
  std::future<void> firstWriteStartedFuture = firstWriteStarted.get_future();
  std::promise<void> releasePromise;
  const std::shared_future<void> releaseFirstWrite = releasePromise.get_future().share();
  std::future<hcb::SqliteWriteResult> first =
      queue.enqueue([&firstWriteStarted,
                     releaseFirstWrite](hcb::SqliteConnection&) mutable -> hcb::SqliteWriteResult {
        firstWriteStarted.set_value();
        releaseFirstWrite.wait();
        return std::nullopt;
      });
  QVERIFY(firstWriteStartedFuture.wait_for(2s) == std::future_status::ready);
  std::future<hcb::SqliteWriteResult> second =
      queue.enqueue([](hcb::SqliteConnection&) -> hcb::SqliteWriteResult { return std::nullopt; });
  QVERIFY(second.wait_for(0ms) == std::future_status::timeout);

  std::thread shutdownThread([&queue] { queue.shutdown(); });
  releasePromise.set_value();
  shutdownThread.join();
  verifySuccess(first);
  verifySuccess(second);

  std::future<hcb::SqliteWriteResult> rejected =
      queue.enqueue([](hcb::SqliteConnection&) -> hcb::SqliteWriteResult { return std::nullopt; });
  QVERIFY(rejected.wait_for(0ms) == std::future_status::ready);
  QVERIFY(rejected.get().has_value());
}

void SqliteWriterQueueTest::reportsConnectionStartupFailure() {
  QTemporaryDir temporaryDirectory;
  QVERIFY(temporaryDirectory.isValid());
  const std::optional<hcb::FilePath> databasePath = databasePathFor(
      temporaryDirectory, QStringLiteral("missing-directory/hot-cross-buns.sqlite"));
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) {
    return;
  }

  hcb::SqliteWriterQueue queue(*databasePath);
  const std::shared_future<hcb::SqliteWriteResult> ready = queue.ready();
  QVERIFY(ready.wait_for(2s) == std::future_status::ready);
  const hcb::SqliteWriteResult startupResult = ready.get();
  QVERIFY(startupResult.has_value());
  QCOMPARE(startupResult->code(), hcb::AppErrorCode::Database);

  std::future<hcb::SqliteWriteResult> write =
      queue.enqueue([](hcb::SqliteConnection&) -> hcb::SqliteWriteResult { return std::nullopt; });
  QVERIFY(write.wait_for(0ms) == std::future_status::ready);
  const hcb::SqliteWriteResult writeResult = write.get();
  QVERIFY(writeResult.has_value());
  QCOMPARE(writeResult->code(), hcb::AppErrorCode::Database);
}

QTEST_GUILESS_MAIN(SqliteWriterQueueTest)

#include "SqliteWriterQueueTest.moc"
