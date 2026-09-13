#include <QtTest/QTest>

#include <QJsonDocument>
#include <QJsonObject>

#include "core/NativeQuickCaptureBenchmark.h"

class NativeQuickCaptureBenchmarkTest final : public QObject {
  Q_OBJECT

private slots:
  void measuresDurableCaptures();
  void summarizesSortedSamples();
  void rejectsUnsupportedSamples();
};

void NativeQuickCaptureBenchmarkTest::measuresDurableCaptures() {
  const std::optional<hcb::NativeQuickCaptureBenchmarkResult> result =
      hcb::NativeQuickCaptureBenchmark::run(3);
  QVERIFY(result.has_value());
  if (!result.has_value()) {
    return;
  }
  QCOMPARE(result->samplesNanoseconds.size(), std::size_t{3});
  QVERIFY(result->minimumNanoseconds >= 0);
  QVERIFY(result->medianNanoseconds >= result->minimumNanoseconds);
  QVERIFY(result->maximumNanoseconds >= result->medianNanoseconds);
  const QJsonObject json =
      QJsonDocument::fromJson(hcb::NativeQuickCaptureBenchmark::toJson(*result)).object();
  QCOMPARE(json.value(QStringLiteral("iterations")).toInteger(), qint64{3});
}

void NativeQuickCaptureBenchmarkTest::summarizesSortedSamples() {
  const std::optional<hcb::NativeQuickCaptureBenchmarkResult> result =
      hcb::NativeQuickCaptureBenchmark::summarize({11, 2, 7});
  QVERIFY(result.has_value());
  if (!result.has_value()) {
    return;
  }
  QCOMPARE(result->samplesNanoseconds, std::vector<qint64>({2, 7, 11}));
  QCOMPARE(result->minimumNanoseconds, qint64{2});
  QCOMPARE(result->medianNanoseconds, qint64{7});
  QCOMPARE(result->maximumNanoseconds, qint64{11});
}

void NativeQuickCaptureBenchmarkTest::rejectsUnsupportedSamples() {
  QVERIFY(!hcb::NativeQuickCaptureBenchmark::run(0).has_value());
  QVERIFY(!hcb::NativeQuickCaptureBenchmark::run(21).has_value());
  QVERIFY(!hcb::NativeQuickCaptureBenchmark::summarize({}).has_value());
  QVERIFY(!hcb::NativeQuickCaptureBenchmark::summarize({-1}).has_value());
}

QTEST_GUILESS_MAIN(NativeQuickCaptureBenchmarkTest)

#include "NativeQuickCaptureBenchmarkTest.moc"
