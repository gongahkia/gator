#include <QtTest>

#include "app/ApplicationLifecycle.h"

#include <atomic>
#include <barrier>
#include <thread>

class ApplicationLifecycleTest final : public QObject {
  Q_OBJECT

private slots:
  void startsAndStopsInOrder();
  void supportsStopDuringStartup();
  void recoversFromFailureThroughShutdown();
  void preservesStoppingStateWhenFailureArrivesLate();
  void rejectsInvalidTransitions();
  void convergesWhenStartupFailureRacesWithShutdown();
};

void ApplicationLifecycleTest::startsAndStopsInOrder() {
  hcb::ApplicationLifecycle lifecycle;

  QCOMPARE(lifecycle.state(), hcb::ApplicationState::Starting);
  QVERIFY(lifecycle.markReady());
  QCOMPARE(lifecycle.state(), hcb::ApplicationState::Ready);
  QVERIFY(lifecycle.requestStop());
  QCOMPARE(lifecycle.state(), hcb::ApplicationState::Stopping);
  QVERIFY(lifecycle.markStopped());
  QCOMPARE(lifecycle.state(), hcb::ApplicationState::Stopped);
}

void ApplicationLifecycleTest::supportsStopDuringStartup() {
  hcb::ApplicationLifecycle lifecycle;

  QVERIFY(lifecycle.requestStop());
  QVERIFY(lifecycle.markStopped());
  QCOMPARE(lifecycle.state(), hcb::ApplicationState::Stopped);
}

void ApplicationLifecycleTest::recoversFromFailureThroughShutdown() {
  hcb::ApplicationLifecycle lifecycle;

  QVERIFY(lifecycle.fail());
  QCOMPARE(lifecycle.state(), hcb::ApplicationState::Failed);
  QVERIFY(lifecycle.requestStop());
  QVERIFY(lifecycle.markStopped());
}

void ApplicationLifecycleTest::preservesStoppingStateWhenFailureArrivesLate() {
  hcb::ApplicationLifecycle lifecycle;

  QVERIFY(lifecycle.markReady());
  QVERIFY(lifecycle.requestStop());
  QCOMPARE(lifecycle.state(), hcb::ApplicationState::Stopping);
  QVERIFY(!lifecycle.fail());
  QCOMPARE(lifecycle.state(), hcb::ApplicationState::Stopping);
  QVERIFY(lifecycle.markStopped());
  QCOMPARE(lifecycle.state(), hcb::ApplicationState::Stopped);
}

void ApplicationLifecycleTest::rejectsInvalidTransitions() {
  hcb::ApplicationLifecycle lifecycle;

  QVERIFY(!lifecycle.markStopped());
  QVERIFY(lifecycle.markReady());
  QVERIFY(!lifecycle.markReady());
  QVERIFY(lifecycle.requestStop());
  QVERIFY(!lifecycle.requestStop());
  QVERIFY(lifecycle.markStopped());
  QVERIFY(!lifecycle.fail());
}

void ApplicationLifecycleTest::convergesWhenStartupFailureRacesWithShutdown() {
  for (int iteration = 0; iteration < 128; ++iteration) {
    hcb::ApplicationLifecycle lifecycle;
    std::atomic_bool failureSucceeded{false};
    std::atomic_bool stopSucceeded{false};
    std::barrier startLine(3);
    std::thread failure([&] {
      startLine.arrive_and_wait();
      failureSucceeded.store(lifecycle.fail(), std::memory_order_release);
    });
    std::thread stop([&] {
      startLine.arrive_and_wait();
      stopSucceeded.store(lifecycle.requestStop(), std::memory_order_release);
    });

    startLine.arrive_and_wait();
    failure.join();
    stop.join();

    QVERIFY(failureSucceeded.load(std::memory_order_acquire) ||
            stopSucceeded.load(std::memory_order_acquire));
    const hcb::ApplicationState state = lifecycle.state();
    QVERIFY(state == hcb::ApplicationState::Failed || state == hcb::ApplicationState::Stopping);
    if (state == hcb::ApplicationState::Failed) {
      QVERIFY(lifecycle.requestStop());
    }
    QCOMPARE(lifecycle.state(), hcb::ApplicationState::Stopping);
    QVERIFY(lifecycle.markStopped());
    QCOMPARE(lifecycle.state(), hcb::ApplicationState::Stopped);
  }
}

QTEST_MAIN(ApplicationLifecycleTest)
#include "ApplicationLifecycleTest.moc"
