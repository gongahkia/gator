#include <QtTest>

#include "core/DiagnosticsSnapshot.h"

#include <chrono>
#include <cstddef>

namespace {

class TestClock final : public hcb::Clock {
public:
  TestClock(hcb::WallTimePoint wallTime, hcb::MonotonicTimePoint monotonicTime)
      : wallTime_(wallTime), monotonicTime_(monotonicTime) {}

  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override { return wallTime_; }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return monotonicTime_;
  }

  void setMonotonicTime(hcb::MonotonicTimePoint monotonicTime) noexcept {
    monotonicTime_ = monotonicTime;
  }

private:
  hcb::WallTimePoint wallTime_;
  hcb::MonotonicTimePoint monotonicTime_;
};

} // namespace

class DiagnosticsSnapshotTest final : public QObject {
  Q_OBJECT

private slots:
  void buildsVersionedRedactedSnapshot();
};

void DiagnosticsSnapshotTest::buildsVersionedRedactedSnapshot() {
  const hcb::WallTimePoint wallTime{std::chrono::seconds{1'725'000'000}};
  TestClock clock(wallTime, hcb::MonotonicTimePoint{});
  hcb::StructuredLogger logger(clock);
  hcb::StartupTimingTracker startupTimings(clock, logger);
  hcb::UiTransitionTimingTracker uiTransitionTimings(clock, logger);

  clock.setMonotonicTime(hcb::MonotonicTimePoint{std::chrono::milliseconds{12}});
  QVERIFY(startupTimings.mark(u"application.initialized"));
  QVERIFY(uiTransitionTimings.begin(QStringLiteral("navigation.tasks")));
  clock.setMonotonicTime(hcb::MonotonicTimePoint{std::chrono::milliseconds{31}});
  QVERIFY(uiTransitionTimings.complete(QStringLiteral("navigation.tasks")));
  logger.log(hcb::LogLevel::Warning, {u"sync", u"access_token=raw-access-token"});

  const hcb::DiagnosticsSnapshot snapshot =
      hcb::DiagnosticsSnapshotBuilder::build(clock,
                                             {QStringLiteral("Hot Cross Buns"),
                                              QStringLiteral("Bearer raw-build-token"),
                                              QStringLiteral("macos")},
                                             startupTimings,
                                             uiTransitionTimings,
                                             logger);

  QCOMPARE(snapshot.schemaVersion, hcb::diagnosticsSchemaVersion);
  QCOMPARE(snapshot.generatedAt, wallTime);
  QCOMPARE(snapshot.build.applicationName, QStringLiteral("Hot Cross Buns"));
  QVERIFY(!snapshot.build.applicationVersion.contains(QStringLiteral("raw-build-token")));
  QCOMPARE(snapshot.build.platform, QStringLiteral("macos"));
  QCOMPARE(snapshot.startupTimings.size(), std::size_t{1});
  QCOMPARE(snapshot.uiTransitionTimings.size(), std::size_t{1});
  QCOMPARE(snapshot.logs.size(), std::size_t{3});
  for (const hcb::LogEntry& entry : snapshot.logs) {
    QVERIFY(!entry.message.contains(QStringLiteral("raw-access-token")));
  }
}

QTEST_MAIN(DiagnosticsSnapshotTest)
#include "DiagnosticsSnapshotTest.moc"
