#include "core/DiagnosticsJsonExporter.h"

#include "core/SecretRedactor.h"

#include <QDateTime>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QTimeZone>

#include <algorithm>
#include <chrono>

namespace hcb {
namespace {

constexpr std::size_t kMaximumStartupTimings = 32;
constexpr std::size_t kMaximumUiTransitionTimings = 64;
constexpr std::size_t kMaximumLogs = 500;
constexpr int kMaximumMutationTelemetry = 500;
constexpr std::size_t kMaximumMetadataEntries = 20;

QString timestampString(WallTimePoint timestamp) {
  const auto milliseconds =
      std::chrono::duration_cast<std::chrono::milliseconds>(timestamp.time_since_epoch()).count();
  return QDateTime::fromMSecsSinceEpoch(milliseconds, QTimeZone::UTC).toString(Qt::ISODateWithMs);
}

QString safeText(QStringView value, qsizetype maximumLength, QStringView fallback) {
  const QString redacted = SecretRedactor::redactText(value, maximumLength);
  return redacted.isEmpty() ? fallback.toString() : redacted;
}

QString logLevelName(LogLevel level) {
  switch (level) {
  case LogLevel::Debug:
    return QStringLiteral("debug");
  case LogLevel::Info:
    return QStringLiteral("info");
  case LogLevel::Warning:
    return QStringLiteral("warning");
  case LogLevel::Error:
    return QStringLiteral("error");
  }
  return QStringLiteral("unknown");
}

QJsonObject metadataObject(const LogMetadata& metadata) {
  QJsonObject object;
  std::size_t count = 0;
  for (auto iterator = metadata.cbegin(); iterator != metadata.cend(); ++iterator) {
    if (count >= kMaximumMetadataEntries) {
      break;
    }
    const bool sensitiveKey = SecretRedactor::isSensitiveKey(iterator.key());
    object.insert(SecretRedactor::redactKey(iterator.key()),
                  sensitiveKey ? QStringLiteral("[redacted]")
                               : safeText(iterator.value(), 500, QStringLiteral("value")));
    ++count;
  }
  return object;
}

QJsonArray startupTimingsArray(const std::vector<StartupTimingSpan>& spans) {
  QJsonArray timings;
  const std::size_t count = std::min(spans.size(), kMaximumStartupTimings);
  for (std::size_t index = 0; index < count; ++index) {
    const StartupTimingSpan& span = spans[index];
    timings.append(QJsonObject{
        {QStringLiteral("name"), safeText(span.name, 120, QStringLiteral("startup.unknown"))},
        {QStringLiteral("elapsed_ms"), static_cast<qint64>(span.elapsed.count())}});
  }
  return timings;
}

QJsonArray uiTransitionTimingsArray(const std::vector<UiTransitionTimingSpan>& spans) {
  QJsonArray timings;
  const std::size_t count = std::min(spans.size(), kMaximumUiTransitionTimings);
  for (std::size_t index = 0; index < count; ++index) {
    const UiTransitionTimingSpan& span = spans[index];
    timings.append(QJsonObject{
        {QStringLiteral("name"), safeText(span.name, 120, QStringLiteral("transition.unknown"))},
        {QStringLiteral("elapsed_ms"), static_cast<qint64>(span.elapsed.count())}});
  }
  return timings;
}

QJsonArray logsArray(const std::vector<LogEntry>& entries) {
  QJsonArray logs;
  const std::size_t count = std::min(entries.size(), kMaximumLogs);
  for (std::size_t index = 0; index < count; ++index) {
    const LogEntry& entry = entries[index];
    logs.append(QJsonObject{
        {QStringLiteral("sequence"), static_cast<qint64>(entry.sequence)},
        {QStringLiteral("timestamp"), timestampString(entry.timestamp)},
        {QStringLiteral("level"), logLevelName(entry.level)},
        {QStringLiteral("category"), safeText(entry.category, 80, QStringLiteral("misc"))},
        {QStringLiteral("message"), safeText(entry.message, 1'000, QStringLiteral("event"))},
        {QStringLiteral("metadata"), metadataObject(entry.metadata)}});
  }
  return logs;
}

QString telemetryPhaseName(MutationTelemetryPhase phase) {
  switch (phase) {
  case MutationTelemetryPhase::Intent: return QStringLiteral("intent");
  case MutationTelemetryPhase::RemoteApplied: return QStringLiteral("remote_applied");
  case MutationTelemetryPhase::RemoteFailed: return QStringLiteral("remote_failed");
  case MutationTelemetryPhase::Rollback: return QStringLiteral("rollback");
  }
  return QStringLiteral("intent");
}

QJsonArray mutationTelemetryArray(const QList<MutationTelemetryRecord>& records) {
  QJsonArray telemetry;
  const qsizetype count = std::min(records.size(), static_cast<qsizetype>(kMaximumMutationTelemetry));
  for (int index = 0; index < count; ++index) {
    const MutationTelemetryRecord& record = records.at(index);
    telemetry.append(QJsonObject{{QStringLiteral("mutation_id"), safeText(record.mutationId, 128, QStringLiteral("local"))},
                                 {QStringLiteral("resource"), safeText(record.resource, 32, QStringLiteral("unknown"))},
                                 {QStringLiteral("operation"), safeText(record.operation, 128, QStringLiteral("unknown"))},
                                 {QStringLiteral("scope"), safeText(record.scope, 32, QStringLiteral("none"))},
                                 {QStringLiteral("all_day"), record.allDay},
                                 {QStringLiteral("target_start_at"), record.targetStartAt.value_or(QString())},
                                 {QStringLiteral("target_end_at"), record.targetEndAt.value_or(QString())},
                                 {QStringLiteral("phase"), telemetryPhaseName(record.phase)},
                                 {QStringLiteral("remote_outcome"), record.remoteOutcome.value_or(QString())},
                                 {QStringLiteral("error_code"), record.errorCode.value_or(QString())},
                                 {QStringLiteral("rollback_reason"), record.rollbackReason.value_or(QString())},
                                 {QStringLiteral("created_at"), record.createdAt}});
  }
  return telemetry;
}

} // namespace

QByteArray DiagnosticsJsonExporter::exportSnapshot(const DiagnosticsSnapshot& snapshot) {
  const QJsonObject build{
      {QStringLiteral("application_name"),
       safeText(snapshot.build.applicationName, 120, QStringLiteral("unknown"))},
      {QStringLiteral("application_version"),
       safeText(snapshot.build.applicationVersion, 80, QStringLiteral("unknown"))},
      {QStringLiteral("platform"),
       safeText(snapshot.build.platform, 80, QStringLiteral("unknown"))}};
  const QJsonObject document{
      {QStringLiteral("schema_version"), static_cast<qint64>(snapshot.schemaVersion)},
      {QStringLiteral("generated_at"), timestampString(snapshot.generatedAt)},
      {QStringLiteral("build"), build},
      {QStringLiteral("startup_timings"), startupTimingsArray(snapshot.startupTimings)},
      {QStringLiteral("ui_transition_timings"),
       uiTransitionTimingsArray(snapshot.uiTransitionTimings)},
      {QStringLiteral("logs"), logsArray(snapshot.logs)},
      {QStringLiteral("mutation_telemetry"), mutationTelemetryArray(snapshot.mutationTelemetry)}};
  return QJsonDocument(document).toJson(QJsonDocument::Compact);
}

} // namespace hcb
