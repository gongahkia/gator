#include <QtTest>

#include "core/DiagnosticsJsonExporter.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>

#include <chrono>

class DiagnosticsJsonExporterTest final : public QObject {
  Q_OBJECT

private slots:
  void exportsBoundedRedactedJson();
  void boundsManualSnapshotData();
};

void DiagnosticsJsonExporterTest::exportsBoundedRedactedJson() {
  hcb::DiagnosticsSnapshot snapshot;
  snapshot.generatedAt = hcb::WallTimePoint{std::chrono::seconds{1'725'000'000}};
  snapshot.build = {QStringLiteral("Hot Cross Buns"),
                    QStringLiteral("Bearer raw-build-token"),
                    QStringLiteral("macos")};
  snapshot.startupTimings = {
      {QStringLiteral("startup.access_token=raw-startup-token"), std::chrono::milliseconds{12}}};
  snapshot.uiTransitionTimings = {
      {QStringLiteral("navigation.tasks"), std::chrono::milliseconds{19}}};
  snapshot.logs = {{7,
                    hcb::WallTimePoint{std::chrono::seconds{1'725'000'001}},
                    hcb::LogLevel::Warning,
                    QStringLiteral("sync"),
                    QStringLiteral("access_token=raw-log-token"),
                    {{QStringLiteral("authorization"), QStringLiteral("Bearer raw-metadata-token")},
                     {QStringLiteral("reason"), QStringLiteral("network retry")}}}};

  const QByteArray bundle = hcb::DiagnosticsJsonExporter::exportSnapshot(snapshot);
  QVERIFY(!bundle.contains("raw-build-token"));
  QVERIFY(!bundle.contains("raw-startup-token"));
  QVERIFY(!bundle.contains("raw-log-token"));
  QVERIFY(!bundle.contains("raw-metadata-token"));

  QJsonParseError parseError;
  const QJsonDocument document = QJsonDocument::fromJson(bundle, &parseError);
  QCOMPARE(parseError.error, QJsonParseError::NoError);
  QVERIFY(document.isObject());

  const QJsonObject root = document.object();
  QCOMPARE(root.value(QStringLiteral("schema_version")).toInt(),
           static_cast<int>(hcb::diagnosticsSchemaVersion));
  QCOMPARE(root.value(QStringLiteral("generated_at")).toString(),
           QStringLiteral("2024-08-30T06:40:00.000Z"));
  const QJsonObject build = root.value(QStringLiteral("build")).toObject();
  QCOMPARE(build.value(QStringLiteral("application_name")).toString(),
           QStringLiteral("Hot Cross Buns"));
  QCOMPARE(build.value(QStringLiteral("application_version")).toString(),
           QStringLiteral("Bearer [redacted]"));

  const QJsonArray startupTimings = root.value(QStringLiteral("startup_timings")).toArray();
  QCOMPARE(startupTimings.size(), 1);
  QCOMPARE(startupTimings.at(0).toObject().value(QStringLiteral("elapsed_ms")).toInt(), 12);

  const QJsonArray logs = root.value(QStringLiteral("logs")).toArray();
  QCOMPARE(logs.size(), 1);
  const QJsonObject log = logs.at(0).toObject();
  QCOMPARE(log.value(QStringLiteral("level")).toString(), QStringLiteral("warning"));
  QCOMPARE(log.value(QStringLiteral("message")).toString(),
           QStringLiteral("access_token=[redacted]"));
  QCOMPARE(log.value(QStringLiteral("metadata"))
               .toObject()
               .value(QStringLiteral("[redacted]"))
               .toString(),
           QStringLiteral("[redacted]"));
  QCOMPARE(root.value(QStringLiteral("mutation_telemetry")).toArray().size(), 0);
}

void DiagnosticsJsonExporterTest::boundsManualSnapshotData() {
  hcb::DiagnosticsSnapshot snapshot;
  for (int index = 0; index < 33; ++index) {
    snapshot.startupTimings.push_back(
        {QStringLiteral("startup.%1").arg(index), std::chrono::milliseconds{index}});
  }
  for (int index = 0; index < 65; ++index) {
    snapshot.uiTransitionTimings.push_back(
        {QStringLiteral("transition.%1").arg(index), std::chrono::milliseconds{index}});
  }
  hcb::LogMetadata metadata;
  for (int index = 0; index < 21; ++index) {
    metadata.insert(QStringLiteral("field.%1").arg(index), QStringLiteral("value"));
  }
  for (int index = 0; index < 501; ++index) {
    snapshot.logs.push_back({static_cast<std::uint64_t>(index),
                             {},
                             hcb::LogLevel::Info,
                             QStringLiteral("diagnostics"),
                             QStringLiteral("event"),
                             metadata});
  }

  const QJsonObject root =
      QJsonDocument::fromJson(hcb::DiagnosticsJsonExporter::exportSnapshot(snapshot)).object();
  QCOMPARE(root.value(QStringLiteral("startup_timings")).toArray().size(), 32);
  QCOMPARE(root.value(QStringLiteral("ui_transition_timings")).toArray().size(), 64);
  const QJsonArray logs = root.value(QStringLiteral("logs")).toArray();
  QCOMPARE(logs.size(), 500);
  QCOMPARE(logs.at(0).toObject().value(QStringLiteral("metadata")).toObject().size(), 20);
}

QTEST_MAIN(DiagnosticsJsonExporterTest)
#include "DiagnosticsJsonExporterTest.moc"
