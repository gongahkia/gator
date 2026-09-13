#include <QtTest>

#include "core/NativeCapabilityReport.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>

class NativeCapabilityReportTest final : public QObject {
  Q_OBJECT

private slots:
  void serializesLegacyCompatibleContract();
  void redactsAndBoundsCopyableDiagnostics();
};

void NativeCapabilityReportTest::serializesLegacyCompatibleContract() {
  hcb::NativeCapabilityReportInput input{
      hcb::NativePlatform::MacOS,
      QStringLiteral("native-qt"),
      hcb::NativePackageFormat::Development,
      {.supportsAppPaths = true, .supportsDiagnosticsCollection = true},
      {{hcb::NativePathRole::Data,
        QStringLiteral("adapter"),
        QStringLiteral("/Users/ada/Library/Application Support/Hot Cross Buns")},
       {hcb::NativePathRole::Cache,
        QStringLiteral("adapter"),
        QStringLiteral("/Users/ada/Library/Caches/Hot Cross Buns")}},
      {{hcb::NativeCapabilityKey::CredentialStorage,
        hcb::NativeCapabilityDiagnosticSeverity::Blocker,
        QStringLiteral("Credential storage is not wired.")}},
      QStringLiteral("/Users/ada")};

  const hcb::NativeCapabilityReport report = hcb::NativeCapabilityReportBuilder::build(input);
  QCOMPARE(report.capabilities.size(), std::size_t{15});
  QCOMPARE(report.capabilities.front().key, hcb::NativeCapabilityKey::AppPaths);
  QCOMPARE(report.capabilities.back().key, hcb::NativeCapabilityKey::Packaging);

  const QByteArray json = hcb::NativeCapabilityReportBuilder::toJson(report);
  QVERIFY(!json.contains("/Users/ada"));
  const QJsonObject root = QJsonDocument::fromJson(json).object();
  QCOMPARE(root.value(QStringLiteral("platform")).toString(), QStringLiteral("darwin"));
  QCOMPARE(root.value(QStringLiteral("adapterId")).toString(), QStringLiteral("native-qt"));
  QCOMPARE(root.value(QStringLiteral("packageFormat")).toString(), QStringLiteral("development"));
  const QJsonObject flags = root.value(QStringLiteral("flags")).toObject();
  QCOMPARE(flags.size(), 19);
  QVERIFY(flags.value(QStringLiteral("supportsAppPaths")).toBool());
  QVERIFY(!flags.value(QStringLiteral("supportsTray")).toBool());

  const QJsonArray paths = root.value(QStringLiteral("paths")).toArray();
  QCOMPARE(paths.size(), 2);
  QCOMPARE(paths.at(0).toObject().value(QStringLiteral("role")).toString(), QStringLiteral("data"));
  QCOMPARE(paths.at(0).toObject().value(QStringLiteral("redactedPath")).toString(),
           QStringLiteral("<home>/Library/Application Support/Hot Cross Buns"));

  const QJsonArray capabilities = root.value(QStringLiteral("capabilities")).toArray();
  QCOMPARE(capabilities.size(), 15);
  QCOMPARE(capabilities.at(0).toObject().value(QStringLiteral("key")).toString(),
           QStringLiteral("appPaths"));
  QCOMPARE(capabilities.at(0).toObject().value(QStringLiteral("state")).toString(),
           QStringLiteral("ready"));
  QCOMPARE(capabilities.at(2).toObject().value(QStringLiteral("key")).toString(),
           QStringLiteral("tray"));
  QCOMPARE(capabilities.at(2).toObject().value(QStringLiteral("state")).toString(),
           QStringLiteral("unsupported"));

  const QJsonArray diagnostics = root.value(QStringLiteral("diagnostics")).toArray();
  QCOMPARE(diagnostics.size(), 1);
  QCOMPARE(diagnostics.at(0).toObject().value(QStringLiteral("severity")).toString(),
           QStringLiteral("blocker"));
}

void NativeCapabilityReportTest::redactsAndBoundsCopyableDiagnostics() {
  hcb::NativeCapabilityReportInput input{hcb::NativePlatform::Unknown,
                                         QStringLiteral("Bearer adapter-token"),
                                         hcb::NativePackageFormat::Unknown,
                                         {},
                                         {},
                                         {},
                                         QStringLiteral("/Users/ada")};
  for (int index = 0; index < 13; ++index) {
    input.paths.push_back({hcb::NativePathRole::Temp,
                           QStringLiteral("source.%1").arg(index),
                           QStringLiteral("/Users/ada/tmp/%1").arg(index)});
  }
  for (int index = 0; index < 41; ++index) {
    input.diagnostics.push_back(
        {hcb::NativeCapabilityKey::Diagnostics,
         hcb::NativeCapabilityDiagnosticSeverity::Warning,
         QStringLiteral("access_token=raw-token path=/Users/ada/%1").arg(index)});
  }

  const QByteArray json = hcb::NativeCapabilityReportBuilder::toJson(
      hcb::NativeCapabilityReportBuilder::build(std::move(input)));
  QVERIFY(!json.contains("adapter-token"));
  QVERIFY(!json.contains("raw-token"));
  QVERIFY(!json.contains("/Users/ada"));
  const QJsonObject root = QJsonDocument::fromJson(json).object();
  QCOMPARE(root.value(QStringLiteral("platform")).toString(), QStringLiteral("unknown"));
  QCOMPARE(root.value(QStringLiteral("packageFormat")).toString(), QStringLiteral("unknown"));
  QCOMPARE(root.value(QStringLiteral("paths")).toArray().size(), 12);
  QCOMPARE(root.value(QStringLiteral("diagnostics")).toArray().size(), 40);
}

QTEST_MAIN(NativeCapabilityReportTest)
#include "NativeCapabilityReportTest.moc"
