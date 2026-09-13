#include <QtTest>

#include "core/NativeLaunchBenchmark.h"

#include <QJsonDocument>
#include <QJsonObject>

class NativeLaunchBenchmarkTest final : public QObject {
  Q_OBJECT

private slots:
  void summarizesSamples();
  void rejectsEmptySamples();
};

void NativeLaunchBenchmarkTest::summarizesSamples() {
  const auto result = hcb::NativeLaunchBenchmark::summarize({11, 2, 7});

  if (!result.has_value()) {
    QFAIL("expected benchmark summary");
    return;
  }
  const hcb::NativeLaunchBenchmarkResult& summary = result.value();
  QCOMPARE(summary.minimumMilliseconds, 2);
  QCOMPARE(summary.medianMilliseconds, 7);
  QCOMPARE(summary.maximumMilliseconds, 11);
  QCOMPARE(summary.samplesMilliseconds, std::vector<qint64>({2, 7, 11}));

  const QJsonObject json =
      QJsonDocument::fromJson(hcb::NativeLaunchBenchmark::toJson(summary)).object();
  QCOMPARE(json.value(QStringLiteral("schema_version")).toInt(), 1);
  QCOMPARE(json.value(QStringLiteral("iterations")).toInt(), 3);
  QCOMPARE(json.value(QStringLiteral("median_ms")).toInt(), 7);
}

void NativeLaunchBenchmarkTest::rejectsEmptySamples() {
  QVERIFY(!hcb::NativeLaunchBenchmark::summarize({}).has_value());
}

QTEST_MAIN(NativeLaunchBenchmarkTest)
#include "NativeLaunchBenchmarkTest.moc"
