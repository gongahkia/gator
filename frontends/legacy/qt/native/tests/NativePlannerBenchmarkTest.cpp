#include <QtTest/QTest>

#include <QJsonDocument>
#include <QJsonObject>

#include "core/NativePerformanceFixture.h"
#include "core/NativePlannerBenchmark.h"

class NativePlannerBenchmarkTest final : public QObject {
  Q_OBJECT

private slots:
  void laysOutTheComplete15kEventFixture();
  void rejectsEmptyFixture();
};

void NativePlannerBenchmarkTest::laysOutTheComplete15kEventFixture() {
  const std::optional<hcb::NativePlannerBenchmarkResult> result =
      hcb::NativePlannerBenchmark::run(hcb::NativePerformanceFixtureGenerator::event15k());
  QVERIFY(result.has_value());
  if (!result.has_value()) {
    return;
  }
  QCOMPARE(result->eventCount, std::size_t{15'000});
  QCOMPARE(result->timedLayoutCount + result->allDaySegmentCount + result->allDayOverflowCount,
           result->eventCount);
  QVERIFY(result->timedDayCount > 0);
  QVERIFY(result->elapsedNanoseconds > 0);
  const QJsonObject json =
      QJsonDocument::fromJson(hcb::NativePlannerBenchmark::toJson(*result)).object();
  QCOMPARE(json.value(QStringLiteral("eventCount")).toInteger(), qint64{15'000});
}

void NativePlannerBenchmarkTest::rejectsEmptyFixture() {
  QVERIFY(!hcb::NativePlannerBenchmark::run({}).has_value());
}

QTEST_GUILESS_MAIN(NativePlannerBenchmarkTest)

#include "NativePlannerBenchmarkTest.moc"
