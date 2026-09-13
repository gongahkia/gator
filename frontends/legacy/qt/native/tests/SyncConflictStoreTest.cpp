#include <QtTest/QTest>

#include "core/Clock.h"
#include "core/SyncConflictStore.h"
#include "support/TemporarySqliteDatabase.h"

#include <chrono>
#include <future>
#include <memory>
#include <optional>
#include <utility>
#include <variant>

using namespace std::chrono_literals;

class SyncConflictStoreTest final : public QObject {
  Q_OBJECT

private slots:
  void recordsListsResolvesAndReopensConflicts();
  void rejectsInvalidConflictInput();
};

namespace {

class FixedClock final : public hcb::Clock {
public:
  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override {
    return hcb::WallTimePoint{std::chrono::milliseconds{1'753'408'000'123}};
  }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return hcb::MonotonicTimePoint{};
  }
};

template <typename Result> [[nodiscard]] Result awaitResult(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) {
    qFatal("sync conflict store request timed out");
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

void verifyReady(hcb::SyncConflictStore& store) {
  const std::shared_future<hcb::SqliteWriteResult> ready = store.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  QVERIFY(!ready.get().has_value());
}

[[nodiscard]] hcb::SyncConflict record(hcb::SyncConflictStore& store,
                                       hcb::SyncConflictInput input) {
  std::future<hcb::SyncConflictResult> future = store.record(std::move(input));
  const hcb::SyncConflictResult result = awaitResult(future);
  if (!std::holds_alternative<hcb::SyncConflict>(result)) {
    qFatal("sync conflict record failed");
  }
  return std::get<hcb::SyncConflict>(result);
}

} // namespace

void SyncConflictStoreTest::recordsListsResolvesAndReopensConflicts() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::SyncConflictStore store(database->databasePath(), clock);
  verifyReady(store);
  const hcb::SyncConflict first =
      record(store,
             {.resource = hcb::SyncConflictResource::Event,
              .resourceId = QStringLiteral("event-1"),
              .mutationId = QStringLiteral("mutation:event:1"),
              .errorCode = QStringLiteral("precondition_failed"),
              .errorMessage = QStringLiteral("Remote event changed"),
              .baseSnapshot = QJsonObject{{QStringLiteral("summary"), QStringLiteral("Base")}},
              .localPayload = QJsonObject{{QStringLiteral("summary"), QStringLiteral("Local")}},
              .remoteSnapshot = QJsonObject{{QStringLiteral("summary"), QStringLiteral("Remote")}},
              .remoteEtag = QStringLiteral("etag-remote"),
              .policy = hcb::SyncConflictPolicy::AskEachTime});
  QVERIFY(first.id.startsWith(QStringLiteral("conflict:")));
  QCOMPARE(first.resolution, std::optional<hcb::SyncConflictResolution>{});
  QCOMPARE(first.baseSnapshot, QJsonObject({{QStringLiteral("summary"), QStringLiteral("Base")}}));
  QCOMPARE(first.localPayload, QJsonObject({{QStringLiteral("summary"), QStringLiteral("Local")}}));
  QCOMPARE(first.remoteSnapshot,
           QJsonObject({{QStringLiteral("summary"), QStringLiteral("Remote")}}));
  QCOMPARE(first.remoteEtag, std::optional<QString>(QStringLiteral("etag-remote")));
  QCOMPARE(first.policy, hcb::SyncConflictPolicy::AskEachTime);
  std::future<hcb::SyncConflictListResult> listedFuture = store.listUnresolved();
  const hcb::SyncConflictListResult listedResult = awaitResult(listedFuture);
  QVERIFY(std::holds_alternative<QList<hcb::SyncConflict>>(listedResult));
  if (!std::holds_alternative<QList<hcb::SyncConflict>>(listedResult)) {
    return;
  }
  QCOMPARE(std::get<QList<hcb::SyncConflict>>(listedResult).size(), 1);
  std::future<hcb::SyncConflictResult> resolvedFuture =
      store.resolve(first.id, hcb::SyncConflictResolution::KeepRemote);
  const hcb::SyncConflictResult resolvedResult = awaitResult(resolvedFuture);
  QVERIFY(std::holds_alternative<hcb::SyncConflict>(resolvedResult));
  if (!std::holds_alternative<hcb::SyncConflict>(resolvedResult)) {
    return;
  }
  const hcb::SyncConflict resolved = std::get<hcb::SyncConflict>(resolvedResult);
  QCOMPARE(resolved.resolution,
           std::optional<hcb::SyncConflictResolution>(hcb::SyncConflictResolution::KeepRemote));
  QVERIFY(resolved.resolvedAt.has_value());
  std::future<hcb::SyncConflictListResult> emptyFuture = store.listUnresolved();
  const hcb::SyncConflictListResult emptyResult = awaitResult(emptyFuture);
  QVERIFY(std::holds_alternative<QList<hcb::SyncConflict>>(emptyResult));
  QVERIFY(std::get<QList<hcb::SyncConflict>>(emptyResult).isEmpty());
  std::future<hcb::SyncConflictListResult> historyFuture = store.listResolved();
  const hcb::SyncConflictListResult historyResult = awaitResult(historyFuture);
  QVERIFY(std::holds_alternative<QList<hcb::SyncConflict>>(historyResult));
  QCOMPARE(std::get<QList<hcb::SyncConflict>>(historyResult).size(), 1);
  QCOMPARE(std::get<QList<hcb::SyncConflict>>(historyResult).constFirst().id, first.id);
  QCOMPARE(std::get<QList<hcb::SyncConflict>>(historyResult).constFirst().resolution,
           std::optional<hcb::SyncConflictResolution>(hcb::SyncConflictResolution::KeepRemote));
  const hcb::SyncConflict reopened =
      record(store,
             {.resource = hcb::SyncConflictResource::Event,
              .resourceId = QStringLiteral("event-1"),
              .mutationId = QStringLiteral("mutation:event:1"),
              .errorCode = QStringLiteral("conflict"),
              .errorMessage = QStringLiteral("Remote event changed again"),
              .localPayload = QJsonObject{{QStringLiteral("summary"), QStringLiteral("Updated")}}});
  QCOMPARE(reopened.id, first.id);
  QCOMPARE(reopened.errorCode, QStringLiteral("conflict"));
  QVERIFY(!reopened.resolution.has_value());
  QVERIFY(!reopened.resolvedAt.has_value());
}

void SyncConflictStoreTest::rejectsInvalidConflictInput() {
  std::unique_ptr<hcb::test::TemporarySqliteDatabase> database = createDatabase();
  QVERIFY(database != nullptr);
  if (database == nullptr) {
    return;
  }
  FixedClock clock;
  hcb::SyncConflictStore store(database->databasePath(), clock);
  verifyReady(store);
  std::future<hcb::SyncConflictResult> invalid =
      store.record({.resourceId = QStringLiteral("event-1"),
                    .mutationId = QStringLiteral("mutation:event:1"),
                    .errorCode = QString(),
                    .errorMessage = QStringLiteral("Remote event changed")});
  const hcb::SyncConflictResult invalidResult = awaitResult(invalid);
  QVERIFY(std::holds_alternative<hcb::AppError>(invalidResult));
  QCOMPARE(std::get<hcb::AppError>(invalidResult).code(), hcb::AppErrorCode::Validation);
  std::future<hcb::SyncConflictResult> missing =
      store.resolve(QStringLiteral("conflict:missing"), hcb::SyncConflictResolution::KeepLocal);
  const hcb::SyncConflictResult missingResult = awaitResult(missing);
  QVERIFY(std::holds_alternative<hcb::AppError>(missingResult));
  QCOMPARE(std::get<hcb::AppError>(missingResult).code(), hcb::AppErrorCode::Validation);
}

QTEST_GUILESS_MAIN(SyncConflictStoreTest)

#include "SyncConflictStoreTest.moc"
