#include <QtTest>

#include "core/NativeCommandPaletteBenchmark.h"

#include <QJsonDocument>
#include <QJsonObject>

class NativeCommandPaletteBenchmarkTest final : public QObject {
  Q_OBJECT

private slots:
  void summarizesSamples();
  void rejectsEmptySamples();
  void parsesElapsedMilliseconds();
  void rejectsInvalidElapsedMilliseconds();
};

void NativeCommandPaletteBenchmarkTest::summarizesSamples() {
  const auto result = hcb::NativeCommandPaletteBenchmark::summarize({11, 2, 7});

  if (!result.has_value()) {
    QFAIL("expected benchmark summary");
    return;
  }
  const hcb::NativeCommandPaletteBenchmarkResult& summary = result.value();
  QCOMPARE(summary.minimumMilliseconds, 2);
  QCOMPARE(summary.medianMilliseconds, 7);
  QCOMPARE(summary.maximumMilliseconds, 11);
  QCOMPARE(summary.samplesMilliseconds, std::vector<qint64>({2, 7, 11}));

  const QJsonObject json =
      QJsonDocument::fromJson(hcb::NativeCommandPaletteBenchmark::toJson(summary)).object();
  QCOMPARE(json.value(QStringLiteral("schema_version")).toInt(), 1);
  QCOMPARE(json.value(QStringLiteral("iterations")).toInt(), 3);
  QCOMPARE(json.value(QStringLiteral("median_ms")).toInt(), 7);
}

void NativeCommandPaletteBenchmarkTest::rejectsEmptySamples() {
  QVERIFY(!hcb::NativeCommandPaletteBenchmark::summarize({}).has_value());
}

void NativeCommandPaletteBenchmarkTest::parsesElapsedMilliseconds() {
  const auto milliseconds = hcb::NativeCommandPaletteBenchmark::parseElapsedMilliseconds(
      QByteArrayLiteral("ignored\nHCB_COMMAND_PALETTE_OPEN_MS=17\n"));

  if (!milliseconds.has_value()) {
    QFAIL("expected elapsed duration");
    return;
  }
  QCOMPARE(milliseconds.value(), 17);
}

void NativeCommandPaletteBenchmarkTest::rejectsInvalidElapsedMilliseconds() {
  QVERIFY(!hcb::NativeCommandPaletteBenchmark::parseElapsedMilliseconds(
               QByteArrayLiteral("HCB_COMMAND_PALETTE_OPEN_MS=-1"))
               .has_value());
  QVERIFY(!hcb::NativeCommandPaletteBenchmark::parseElapsedMilliseconds(
               QByteArrayLiteral("HCB_COMMAND_PALETTE_OPEN_MS=x"))
               .has_value());
  QVERIFY(!hcb::NativeCommandPaletteBenchmark::parseElapsedMilliseconds(
               QByteArrayLiteral("HCB_COMMAND_PALETTE_OPEN_MS=999999999999999999999"))
               .has_value());
}

QTEST_MAIN(NativeCommandPaletteBenchmarkTest)
#include "NativeCommandPaletteBenchmarkTest.moc"
