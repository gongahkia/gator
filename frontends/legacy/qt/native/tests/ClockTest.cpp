#include <QtTest>

#include "core/Clock.h"

namespace {

class ControlledClock final : public hcb::Clock {
public:
  ControlledClock(hcb::WallTimePoint wallTime, hcb::MonotonicTimePoint monotonicTime)
      : wallTime_(wallTime), monotonicTime_(monotonicTime) {}

  [[nodiscard]] hcb::WallTimePoint wallNow() const noexcept override { return wallTime_; }
  [[nodiscard]] hcb::MonotonicTimePoint monotonicNow() const noexcept override {
    return monotonicTime_;
  }

private:
  hcb::WallTimePoint wallTime_;
  hcb::MonotonicTimePoint monotonicTime_;
};

} // namespace

class ClockTest final : public QObject {
  Q_OBJECT

private slots:
  void readsSystemWallAndMonotonicTime();
  void supportsControlledTimeInjection();
};

void ClockTest::readsSystemWallAndMonotonicTime() {
  const hcb::SystemClock clock;

  const hcb::WallTimePoint wallBefore = std::chrono::system_clock::now();
  const hcb::WallTimePoint wallNow = clock.wallNow();
  const hcb::WallTimePoint wallAfter = std::chrono::system_clock::now();
  QVERIFY(wallBefore <= wallNow);
  QVERIFY(wallNow <= wallAfter);

  const hcb::MonotonicTimePoint monotonicBefore = std::chrono::steady_clock::now();
  const hcb::MonotonicTimePoint monotonicNow = clock.monotonicNow();
  const hcb::MonotonicTimePoint monotonicAfter = std::chrono::steady_clock::now();
  QVERIFY(monotonicBefore <= monotonicNow);
  QVERIFY(monotonicNow <= monotonicAfter);
}

void ClockTest::supportsControlledTimeInjection() {
  const hcb::WallTimePoint wallTime{std::chrono::seconds{1'725'000'000}};
  const hcb::MonotonicTimePoint monotonicTime{std::chrono::milliseconds{123'456}};
  const ControlledClock clock(wallTime, monotonicTime);

  QCOMPARE(clock.wallNow(), wallTime);
  QCOMPARE(clock.monotonicNow(), monotonicTime);
}

QTEST_MAIN(ClockTest)
#include "ClockTest.moc"
