#include <QDir>
#include <QFileInfo>
#include <QTemporaryDir>
#include <QtTest/QTest>

#include "core/Clock.h"
#include "core/MutationTelemetryStore.h"

#include <chrono>
#include <future>
#include <optional>
#include <variant>

using namespace std::chrono_literals;

namespace {

class TestClock final : public hcb::Clock {
public:
  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override {
    return wallTime_ + std::chrono::milliseconds{ticks_++};
  }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return hcb::MonotonicTimePoint{};
  }

private:
  hcb::WallTimePoint wallTime_{std::chrono::seconds{1'725'000'000}};
  mutable int ticks_{0};
};

[[nodiscard]] std::optional<hcb::FilePath> databasePathFor(const QTemporaryDir& directory) {
  return hcb::FilePath::fromAbsolute(QDir(QFileInfo(directory.path()).canonicalFilePath())
                                         .filePath(QStringLiteral("hot-cross-buns.sqlite")));
}

template <typename Result> [[nodiscard]] Result await(std::future<Result>& future) {
  if (future.wait_for(2s) != std::future_status::ready) qFatal("mutation telemetry timed out");
  return future.get();
}

} // namespace

class MutationTelemetryStoreTest final : public QObject {
  Q_OBJECT

private slots:
  void persistsBoundedRedactedMutationPhases();
  void rejectsContentAndInvalidIdentifiers();
};

void MutationTelemetryStoreTest::persistsBoundedRedactedMutationPhases() {
  QTemporaryDir directory;
  QVERIFY(directory.isValid());
  const auto databasePath = databasePathFor(directory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) return;
  TestClock clock;
  hcb::MutationTelemetryStore store(*databasePath, clock);
  const auto ready = store.ready();
  QCOMPARE(ready.wait_for(2s), std::future_status::ready);
  QVERIFY(!ready.get().has_value());
  for (int index = 0; index < hcb::MutationTelemetryStore::maximumRecords + 3; ++index) {
    std::future<hcb::MutationTelemetryWriteResult> write = store.record(
        {.mutationId = QStringLiteral("mutation:event:%1").arg(index),
         .resource = QStringLiteral("event"),
         .operation = QStringLiteral("event.move"),
         .scope = QStringLiteral("this_and_following"),
         .allDay = false,
         .targetStartAt = QStringLiteral("2026-08-05T10:00:00.000Z"),
         .targetEndAt = QStringLiteral("2026-08-05T11:00:00.000Z"),
         .phase = index == hcb::MutationTelemetryStore::maximumRecords + 2
                      ? hcb::MutationTelemetryPhase::RemoteFailed
                      : hcb::MutationTelemetryPhase::Intent,
         .remoteOutcome = index == hcb::MutationTelemetryStore::maximumRecords + 2
                              ? std::optional<QString>(QStringLiteral("failed"))
                              : std::nullopt,
         .errorCode = index == hcb::MutationTelemetryStore::maximumRecords + 2
                          ? std::optional<QString>(QStringLiteral("rate_limited"))
                          : std::nullopt,
         .rollbackReason = index == hcb::MutationTelemetryStore::maximumRecords + 2
                               ? std::optional<QString>(QStringLiteral("keep_retry"))
                               : std::nullopt});
    QVERIFY(std::holds_alternative<hcb::MutationTelemetryRecord>(await(write)));
  }
  std::future<hcb::MutationTelemetryReadResult> read = store.recent();
  const hcb::MutationTelemetryReadResult result = await(read);
  QVERIFY(std::holds_alternative<QList<hcb::MutationTelemetryRecord>>(result));
  const QList<hcb::MutationTelemetryRecord> records =
      std::get<QList<hcb::MutationTelemetryRecord>>(result);
  QCOMPARE(records.size(), hcb::MutationTelemetryStore::maximumRecords);
  QCOMPARE(records.first().operation, QStringLiteral("event.move"));
  QCOMPARE(records.first().scope, QStringLiteral("this_and_following"));
  QCOMPARE(records.first().targetStartAt,
           std::optional<QString>(QStringLiteral("2026-08-05T10:00:00.000Z")));
  QCOMPARE(records.first().rollbackReason,
           std::optional<QString>(QStringLiteral("keep_retry")));
}

void MutationTelemetryStoreTest::rejectsContentAndInvalidIdentifiers() {
  QTemporaryDir directory;
  QVERIFY(directory.isValid());
  const auto databasePath = databasePathFor(directory);
  QVERIFY(databasePath.has_value());
  if (!databasePath.has_value()) return;
  TestClock clock;
  hcb::MutationTelemetryStore store(*databasePath, clock);
  std::future<hcb::MutationTelemetryWriteResult> invalid = store.record(
      {.resource = QStringLiteral("event"), .operation = QStringLiteral("event.move"),
       .scope = QStringLiteral("none"),
       .targetStartAt = QStringLiteral("title=private event"),
       .rollbackReason = QStringLiteral("event title")});
  const hcb::MutationTelemetryWriteResult result = await(invalid);
  QVERIFY(std::holds_alternative<hcb::AppError>(result));
  QCOMPARE(std::get<hcb::AppError>(result).code(), hcb::AppErrorCode::Validation);
}

QTEST_GUILESS_MAIN(MutationTelemetryStoreTest)

#include "MutationTelemetryStoreTest.moc"
