#include "core/NativePerformanceReport.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSysInfo>

namespace hcb {

NativePerformancePlatform NativePerformanceReportSerializer::currentPlatform() {
  return NativePerformancePlatform{
      QSysInfo::productType(), QSysInfo::productVersion(), QSysInfo::currentCpuArchitecture()};
}

QByteArray NativePerformanceReportSerializer::toJson(const NativePerformanceReport& report) {
  QJsonArray launchSamples;
  for (const qint64 milliseconds : report.launch.samplesMilliseconds) {
    launchSamples.append(milliseconds);
  }
  const QJsonObject document{
      {QStringLiteral("schema_version"), static_cast<qint64>(report.schemaVersion)},
      {QStringLiteral("platform"),
       QJsonObject{{QStringLiteral("product_type"), report.platform.productType},
                   {QStringLiteral("product_version"), report.platform.productVersion},
                   {QStringLiteral("cpu_architecture"), report.platform.cpuArchitecture}}},
      {QStringLiteral("launch"),
       QJsonObject{{QStringLiteral("iterations"),
                    static_cast<qint64>(report.launch.samplesMilliseconds.size())},
                   {QStringLiteral("minimum_ms"), report.launch.minimumMilliseconds},
                   {QStringLiteral("median_ms"), report.launch.medianMilliseconds},
                   {QStringLiteral("maximum_ms"), report.launch.maximumMilliseconds},
                   {QStringLiteral("samples_ms"), launchSamples}}},
      {QStringLiteral("idle_rss"),
       QJsonObject{{QStringLiteral("duration_ms"), report.idleDurationMilliseconds},
                   {QStringLiteral("bytes"), report.idleRssBytes}}}};
  return QJsonDocument(document).toJson(QJsonDocument::Compact);
}

} // namespace hcb
