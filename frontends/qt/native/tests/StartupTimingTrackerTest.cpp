#include <QtTest>

#include "core/StartupTimingTracker.h"

#include <chrono>
#include <cstdint>

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

class StartupTimingTrackerTest final : public QObject {
  Q_OBJECT

private slots:
  void recordsFirstOccurrenceOfEachSpan();
  void boundsRecordedSpans();
};

void StartupTimingTrackerTest::recordsFirstOccurrenceOfEachSpan() {
  TestClock clock(hcb::WallTimePoint{}, hcb::MonotonicTimePoint{});
  hcb::StructuredLogger logger(clock);
  hcb::StartupTimingTracker tracker(clock, logger);

  clock.setMonotonicTime(hcb::MonotonicTimePoint{std::chrono::milliseconds{125}});
  QVERIFY(tracker.mark(u"app.initialized"));
  clock.setMonotonicTime(hcb::MonotonicTimePoint{std::chrono::milliseconds{240}});
  QVERIFY(!tracker.mark(u"app.initialized"));
  QVERIFY(tracker.mark(u"qml.loaded"));

  const std::vector<hcb::StartupTimingSpan> spans = tracker.spans();
  QCOMPARE(spans.size(), std::size_t{2});
  QCOMPARE(spans.at(0).name, QStringLiteral("app.initialized"));
  QCOMPARE(spans.at(0).elapsed.count(), std::int64_t{125});
  QCOMPARE(spans.at(1).elapsed.count(), std::int64_t{240});

  const std::vector<hcb::LogEntry> entries = logger.entries();
  QCOMPARE(entries.size(), std::size_t{2});
  QCOMPARE(entries.at(1).metadata.value(QStringLiteral("span")), QStringLiteral("qml.loaded"));
  QCOMPARE(entries.at(1).metadata.value(QStringLiteral("elapsed_ms")), QStringLiteral("240"));
}

void StartupTimingTrackerTest::boundsRecordedSpans() {
  TestClock clock(hcb::WallTimePoint{}, hcb::MonotonicTimePoint{});
  hcb::StructuredLogger logger(clock);
  hcb::StartupTimingTracker tracker(clock, logger, 1);

  QVERIFY(tracker.mark(u"app.initialized"));
  QVERIFY(!tracker.mark(u"qml.loaded"));
  QCOMPARE(tracker.spans().size(), std::size_t{1});
}

QTEST_MAIN(StartupTimingTrackerTest)
#include "StartupTimingTrackerTest.moc"
