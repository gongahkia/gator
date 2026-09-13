#include <QtTest/QTest>

#include <QJsonDocument>
#include <QJsonObject>

#include "core/NativeLocalSearchBenchmark.h"

class NativeLocalSearchBenchmarkTest final : public QObject {
  Q_OBJECT

private slots:
  void measuresIndexedSearches();
  void summarizesSortedSamples();
  void rejectsUnsupportedSamples();
};

void NativeLocalSearchBenchmarkTest::measuresIndexedSearches() {
  const std::optional<hcb::NativeLocalSearchBenchmarkResult> result =
      hcb::NativeLocalSearchBenchmark::run(3);
  QVERIFY(result.has_value());
  if (!result.has_value()) {
    return;
  }
  QCOMPARE(result->corpusTaskCount, std::size_t{10'000});
  QCOMPARE(result->matchedResultCount, std::size_t{100});
  QCOMPARE(result->samplesNanoseconds.size(), std::size_t{3});
  QVERIFY(result->minimumNanoseconds >= 0);
  QVERIFY(result->medianNanoseconds >= result->minimumNanoseconds);
  QVERIFY(result->maximumNanoseconds >= result->medianNanoseconds);
  const QJsonObject json =
      QJsonDocument::fromJson(hcb::NativeLocalSearchBenchmark::toJson(*result)).object();
  QCOMPARE(json.value(QStringLiteral("corpus_task_count")).toInteger(), qint64{10'000});
  QCOMPARE(json.value(QStringLiteral("matched_result_count")).toInteger(), qint64{100});
  QCOMPARE(json.value(QStringLiteral("iterations")).toInteger(), qint64{3});
}

void NativeLocalSearchBenchmarkTest::summarizesSortedSamples() {
  const std::optional<hcb::NativeLocalSearchBenchmarkResult> result =
      hcb::NativeLocalSearchBenchmark::summarize(250, 100, {11, 2, 7});
  QVERIFY(result.has_value());
  if (!result.has_value()) {
    return;
  }
  QCOMPARE(result->samplesNanoseconds, std::vector<qint64>({2, 7, 11}));
  QCOMPARE(result->minimumNanoseconds, qint64{2});
  QCOMPARE(result->medianNanoseconds, qint64{7});
  QCOMPARE(result->maximumNanoseconds, qint64{11});
}

void NativeLocalSearchBenchmarkTest::rejectsUnsupportedSamples() {
  QVERIFY(!hcb::NativeLocalSearchBenchmark::run(0).has_value());
  QVERIFY(!hcb::NativeLocalSearchBenchmark::run(21).has_value());
  QVERIFY(!hcb::NativeLocalSearchBenchmark::summarize(0, 100, {1}).has_value());
  QVERIFY(!hcb::NativeLocalSearchBenchmark::summarize(250, 0, {1}).has_value());
  QVERIFY(!hcb::NativeLocalSearchBenchmark::summarize(250, 100, {}).has_value());
  QVERIFY(!hcb::NativeLocalSearchBenchmark::summarize(250, 100, {-1}).has_value());
}

QTEST_GUILESS_MAIN(NativeLocalSearchBenchmarkTest)

#include "NativeLocalSearchBenchmarkTest.moc"
