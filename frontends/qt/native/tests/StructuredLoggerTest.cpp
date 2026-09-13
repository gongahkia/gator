#include <QtTest>

#include "core/StructuredLogger.h"

namespace {

class TestClock final : public hcb::Clock {
public:
  TestClock(hcb::WallTimePoint wallTime, hcb::MonotonicTimePoint monotonicTime)
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

class StructuredLoggerTest final : public QObject {
  Q_OBJECT

private slots:
  void boundsEntriesAndFiltersByLevel();
  void redactsMessageAndMetadata();
};

void StructuredLoggerTest::boundsEntriesAndFiltersByLevel() {
  const hcb::WallTimePoint wallTime{std::chrono::seconds{1'725'000'000}};
  const hcb::MonotonicTimePoint monotonicTime{std::chrono::milliseconds{123'456}};
  const TestClock clock(wallTime, monotonicTime);
  hcb::StructuredLogger logger(clock, 2);

  logger.log(hcb::LogLevel::Debug, {u"sync", u"debug event"});
  logger.log(hcb::LogLevel::Info, {u"sync", u"info event"});
  logger.log(hcb::LogLevel::Error, {u"sync", u"error event"});

  QCOMPARE(logger.size(), std::size_t{2});
  const std::vector<hcb::LogEntry> entries = logger.entries(hcb::LogLevel::Info);
  QCOMPARE(entries.size(), std::size_t{2});
  QCOMPARE(entries.at(0).sequence, std::uint64_t{2});
  QCOMPARE(entries.at(0).timestamp, wallTime);
  QCOMPARE(entries.at(1).level, hcb::LogLevel::Error);
}

void StructuredLoggerTest::redactsMessageAndMetadata() {
  const TestClock clock(hcb::WallTimePoint{}, hcb::MonotonicTimePoint{});
  hcb::StructuredLogger logger(clock);

  logger.log(hcb::LogLevel::Warning,
             {u"oauth", u"Authorization: Bearer fake-bearer-token"},
             {{QStringLiteral("refreshToken"), QStringLiteral("fake-refresh-token")},
              {QStringLiteral("reason"), QStringLiteral("access_token=fake-access-token")}});

  const std::vector<hcb::LogEntry> entries = logger.entries();
  QCOMPARE(entries.size(), std::size_t{1});
  const hcb::LogEntry& entry = entries.front();
  QVERIFY(!entry.message.contains(QStringLiteral("fake-bearer-token")));
  QVERIFY(!entry.metadata.contains(QStringLiteral("refreshToken")));
  QCOMPARE(entry.metadata.value(QStringLiteral("[redacted]")), QStringLiteral("[redacted]"));
  QVERIFY(!entry.metadata.value(QStringLiteral("reason"))
               .contains(QStringLiteral("fake-access-token")));
}

QTEST_MAIN(StructuredLoggerTest)
#include "StructuredLoggerTest.moc"
