#include "core/DiagnosticsSnapshot.h"

#include "core/SecretRedactor.h"

#include <utility>

namespace hcb {

DiagnosticsSnapshot
DiagnosticsSnapshotBuilder::build(const Clock& clock,
                                  DiagnosticsBuildInfo buildInfo,
                                  const StartupTimingTracker& startupTimings,
                                  const UiTransitionTimingTracker& uiTransitionTimings,
                                  const StructuredLogger& logger,
                                  QList<MutationTelemetryRecord> mutationTelemetry) {
  return DiagnosticsSnapshot{diagnosticsSchemaVersion,
                             clock.wallNow(),
                             redactBuildInfo(std::move(buildInfo)),
                             startupTimings.spans(),
                             uiTransitionTimings.spans(),
                             logger.entries(),
                             std::move(mutationTelemetry)};
}

DiagnosticsBuildInfo DiagnosticsSnapshotBuilder::redactBuildInfo(DiagnosticsBuildInfo buildInfo) {
  const auto redact = [](const QString& value, qsizetype maximumLength) {
    const QString safeValue = SecretRedactor::redactText(value, maximumLength);
    return safeValue.isEmpty() ? QStringLiteral("unknown") : safeValue;
  };
  buildInfo.applicationName = redact(buildInfo.applicationName, 120);
  buildInfo.applicationVersion = redact(buildInfo.applicationVersion, 80);
  buildInfo.platform = redact(buildInfo.platform, 80);
  return buildInfo;
}

} // namespace hcb
