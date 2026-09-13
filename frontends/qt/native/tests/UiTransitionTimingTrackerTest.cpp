#include <QtTest>

#include "core/UiTransitionTimingTracker.h"

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

class UiTransitionTimingTrackerTest final : public QObject {
  Q_OBJECT

private slots:
  void recordsCompletedTransition();
  void rejectsDuplicateAndUnknownTransitions();
  void retainsMostRecentBoundedSpans();
};

void UiTransitionTimingTrackerTest::recordsCompletedTransition() {
  TestClock clock(hcb::WallTimePoint{}, hcb::MonotonicTimePoint{});
  hcb::StructuredLogger logger(clock);
  hcb::UiTransitionTimingTracker tracker(clock, logger);

  QVERIFY(tracker.begin(QStringLiteral("navigation.calendar")));
  clock.setMonotonicTime(hcb::MonotonicTimePoint{std::chrono::milliseconds{48}});
  QVERIFY(tracker.complete(QStringLiteral("navigation.calendar")));

  const std::vector<hcb::UiTransitionTimingSpan> spans = tracker.spans();
  QCOMPARE(spans.size(), std::size_t{1});
  QCOMPARE(spans.front().name, QStringLiteral("navigation.calendar"));
  QCOMPARE(spans.front().elapsed.count(), std::int64_t{48});
  const std::vector<hcb::LogEntry> entries = logger.entries();
  QCOMPARE(entries.size(), std::size_t{1});
  QCOMPARE(entries.front().category, QStringLiteral("ui.transition"));
}

void UiTransitionTimingTrackerTest::rejectsDuplicateAndUnknownTransitions() {
  TestClock clock(hcb::WallTimePoint{}, hcb::MonotonicTimePoint{});
  hcb::StructuredLogger logger(clock);
  hcb::UiTransitionTimingTracker tracker(clock, logger);

  QVERIFY(tracker.begin(QStringLiteral("navigation.tasks")));
  QVERIFY(!tracker.begin(QStringLiteral("navigation.tasks")));
  QVERIFY(!tracker.complete(QStringLiteral("navigation.notes")));
  QVERIFY(tracker.complete(QStringLiteral("navigation.tasks")));
  QVERIFY(!tracker.complete(QStringLiteral("navigation.tasks")));
}

void UiTransitionTimingTrackerTest::retainsMostRecentBoundedSpans() {
  TestClock clock(hcb::WallTimePoint{}, hcb::MonotonicTimePoint{});
  hcb::StructuredLogger logger(clock);
  hcb::UiTransitionTimingTracker tracker(clock, logger, 1, 1);

  QVERIFY(tracker.begin(QStringLiteral("navigation.tasks")));
  QVERIFY(tracker.complete(QStringLiteral("navigation.tasks")));
  QVERIFY(tracker.begin(QStringLiteral("navigation.notes")));
  QVERIFY(tracker.complete(QStringLiteral("navigation.notes")));

  const std::vector<hcb::UiTransitionTimingSpan> spans = tracker.spans();
  QCOMPARE(spans.size(), std::size_t{1});
  QCOMPARE(spans.front().name, QStringLiteral("navigation.notes"));
}

QTEST_MAIN(UiTransitionTimingTrackerTest)
#include "UiTransitionTimingTrackerTest.moc"
