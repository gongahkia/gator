#include <QGuiApplication>
#include <QtTest/QTest>

#include <QJsonDocument>
#include <QJsonObject>

#include "core/NativeCalendarNavigationBenchmark.h"

class NativeCalendarNavigationBenchmarkTest final : public QObject {
  Q_OBJECT

private slots:
  void measuresMonthNavigationFrames();
  void summarizesSortedSamples();
  void rejectsUnsupportedSamples();
};

void NativeCalendarNavigationBenchmarkTest::measuresMonthNavigationFrames() {
  const std::optional<hcb::NativeCalendarNavigationBenchmarkResult> result =
      hcb::NativeCalendarNavigationBenchmark::run(3);
  QVERIFY(result.has_value());
  if (!result.has_value()) {
    return;
  }
  QCOMPARE(result->eventCount, std::size_t{25'000});
  QCOMPARE(result->samplesNanoseconds.size(), std::size_t{3});
  QVERIFY(result->minimumNanoseconds >= 0);
  QVERIFY(result->medianNanoseconds >= result->minimumNanoseconds);
  QVERIFY(result->maximumNanoseconds >= result->medianNanoseconds);
  const QJsonObject json =
      QJsonDocument::fromJson(hcb::NativeCalendarNavigationBenchmark::toJson(*result)).object();
  QCOMPARE(json.value(QStringLiteral("event_count")).toInteger(), qint64{25'000});
  QCOMPARE(json.value(QStringLiteral("frames")).toInteger(), qint64{3});
}

void NativeCalendarNavigationBenchmarkTest::summarizesSortedSamples() {
  const std::optional<hcb::NativeCalendarNavigationBenchmarkResult> result =
      hcb::NativeCalendarNavigationBenchmark::summarize(1'000, {11, 2, 7});
  QVERIFY(result.has_value());
  if (!result.has_value()) {
    return;
  }
  QCOMPARE(result->samplesNanoseconds, std::vector<qint64>({2, 7, 11}));
  QCOMPARE(result->minimumNanoseconds, qint64{2});
  QCOMPARE(result->medianNanoseconds, qint64{7});
  QCOMPARE(result->maximumNanoseconds, qint64{11});
}

void NativeCalendarNavigationBenchmarkTest::rejectsUnsupportedSamples() {
  QVERIFY(!hcb::NativeCalendarNavigationBenchmark::run(0).has_value());
  QVERIFY(!hcb::NativeCalendarNavigationBenchmark::run(61).has_value());
  QVERIFY(!hcb::NativeCalendarNavigationBenchmark::summarize(0, {1}).has_value());
  QVERIFY(!hcb::NativeCalendarNavigationBenchmark::summarize(1'000, {}).has_value());
  QVERIFY(!hcb::NativeCalendarNavigationBenchmark::summarize(1'000, {-1}).has_value());
}

int main(int argc, char* argv[]) {
  QGuiApplication application(argc, argv);
  NativeCalendarNavigationBenchmarkTest test;
  return QTest::qExec(&test, argc, argv);
}

#include "NativeCalendarNavigationBenchmarkTest.moc"
