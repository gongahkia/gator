#include <QGuiApplication>
#include <QtTest/QTest>

#include <QJsonDocument>
#include <QJsonObject>

#include "core/NativeTaskScrollBenchmark.h"

class NativeTaskScrollBenchmarkTest final : public QObject {
  Q_OBJECT

private slots:
  void measuresVirtualizedTaskScrollFrames();
  void summarizesSortedSamples();
  void rejectsUnsupportedSamples();
};

void NativeTaskScrollBenchmarkTest::measuresVirtualizedTaskScrollFrames() {
  const std::optional<hcb::NativeTaskScrollBenchmarkResult> result =
      hcb::NativeTaskScrollBenchmark::run(3);
  QVERIFY(result.has_value());
  if (!result.has_value()) {
    return;
  }
  QCOMPARE(result->taskCount, std::size_t{10'000});
  QCOMPARE(result->bulkSelectionSamplesNanoseconds.size(), std::size_t{3});
  QVERIFY(result->firstCachedRenderNanoseconds >= 0);
  QCOMPARE(result->samplesNanoseconds.size(), std::size_t{3});
  QVERIFY(result->minimumNanoseconds >= 0);
  QVERIFY(result->medianNanoseconds >= result->minimumNanoseconds);
  QVERIFY(result->maximumNanoseconds >= result->medianNanoseconds);
  const QJsonObject json =
      QJsonDocument::fromJson(hcb::NativeTaskScrollBenchmark::toJson(*result)).object();
  QCOMPARE(json.value(QStringLiteral("task_count")).toInteger(), qint64{10'000});
  QVERIFY(json.value(QStringLiteral("first_cached_render_ns")).toInteger() >= 0);
  QCOMPARE(json.value(QStringLiteral("bulk_selection"))
               .toObject()
               .value(QStringLiteral("iterations"))
               .toInteger(),
           qint64{3});
  QCOMPARE(json.value(QStringLiteral("frames")).toInteger(), qint64{3});
}

void NativeTaskScrollBenchmarkTest::summarizesSortedSamples() {
  const std::optional<hcb::NativeTaskScrollBenchmarkResult> result =
      hcb::NativeTaskScrollBenchmark::summarize(10'000, {11, 2, 7}, 8, {11, 2, 7});
  QVERIFY(result.has_value());
  if (!result.has_value()) {
    return;
  }
  QCOMPARE(result->samplesNanoseconds, std::vector<qint64>({2, 7, 11}));
  QCOMPARE(result->minimumNanoseconds, qint64{2});
  QCOMPARE(result->medianNanoseconds, qint64{7});
  QCOMPARE(result->bulkSelectionMedianNanoseconds, qint64{7});
  QCOMPARE(result->maximumNanoseconds, qint64{11});
}

void NativeTaskScrollBenchmarkTest::rejectsUnsupportedSamples() {
  QVERIFY(!hcb::NativeTaskScrollBenchmark::run(0).has_value());
  QVERIFY(!hcb::NativeTaskScrollBenchmark::run(61).has_value());
  QVERIFY(!hcb::NativeTaskScrollBenchmark::summarize(0, {1}, 1, {1}).has_value());
  QVERIFY(!hcb::NativeTaskScrollBenchmark::summarize(10'000, {}, 1, {1}).has_value());
  QVERIFY(!hcb::NativeTaskScrollBenchmark::summarize(10'000, {1}, -1, {1}).has_value());
  QVERIFY(!hcb::NativeTaskScrollBenchmark::summarize(10'000, {1}, 1, {}).has_value());
  QVERIFY(!hcb::NativeTaskScrollBenchmark::summarize(10'000, {1}, 1, {-1}).has_value());
}

int main(int argc, char* argv[]) {
  QGuiApplication application(argc, argv);
  NativeTaskScrollBenchmarkTest test;
  return QTest::qExec(&test, argc, argv);
}

#include "NativeTaskScrollBenchmarkTest.moc"
