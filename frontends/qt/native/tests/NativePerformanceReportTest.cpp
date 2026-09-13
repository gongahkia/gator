#include <QtTest>

#include "core/NativePerformanceReport.h"

#include <QJsonDocument>
#include <QJsonObject>

class NativePerformanceReportTest final : public QObject {
  Q_OBJECT

private slots:
  void serializesVersionedPlatformReport();
};

void NativePerformanceReportTest::serializesVersionedPlatformReport() {
  const hcb::NativePerformanceReport report{
      1,
      {QStringLiteral("macos"), QStringLiteral("15.0"), QStringLiteral("arm64")},
      {{12, 18, 25}, 12, 18, 25},
      1'000,
      73'760'768};

  QJsonParseError error;
  const QJsonDocument document =
      QJsonDocument::fromJson(hcb::NativePerformanceReportSerializer::toJson(report), &error);
  QCOMPARE(error.error, QJsonParseError::NoError);
  const QJsonObject root = document.object();
  QCOMPARE(root.value(QStringLiteral("schema_version")).toInt(), 1);
  const QJsonObject platform = root.value(QStringLiteral("platform")).toObject();
  QCOMPARE(platform.value(QStringLiteral("product_type")).toString(), QStringLiteral("macos"));
  QCOMPARE(platform.value(QStringLiteral("cpu_architecture")).toString(), QStringLiteral("arm64"));
  const QJsonObject launch = root.value(QStringLiteral("launch")).toObject();
  QCOMPARE(launch.value(QStringLiteral("iterations")).toInt(), 3);
  QCOMPARE(launch.value(QStringLiteral("median_ms")).toInt(), 18);
  const QJsonObject idleRss = root.value(QStringLiteral("idle_rss")).toObject();
  QCOMPARE(idleRss.value(QStringLiteral("duration_ms")).toInt(), 1'000);
  QCOMPARE(idleRss.value(QStringLiteral("bytes")).toInteger(), qint64{73'760'768});
}

QTEST_MAIN(NativePerformanceReportTest)
#include "NativePerformanceReportTest.moc"
