#pragma once

#include "core/Clock.h"
#include "core/StartupTimingTracker.h"
#include "core/StructuredLogger.h"
#include "core/MutationTelemetryStore.h"
#include "core/UiTransitionTimingTracker.h"

#include <cstdint>
#include <vector>

namespace hcb {

inline constexpr std::uint32_t diagnosticsSchemaVersion = 2;

struct DiagnosticsBuildInfo final {
  QString applicationName;
  QString applicationVersion;
  QString platform;
};

struct DiagnosticsSnapshot final {
  std::uint32_t schemaVersion{diagnosticsSchemaVersion};
  WallTimePoint generatedAt;
  DiagnosticsBuildInfo build;
  std::vector<StartupTimingSpan> startupTimings;
  std::vector<UiTransitionTimingSpan> uiTransitionTimings;
  std::vector<LogEntry> logs;
  QList<MutationTelemetryRecord> mutationTelemetry;
};

class DiagnosticsSnapshotBuilder final {
public:
  [[nodiscard]] static DiagnosticsSnapshot
  build(const Clock& clock,
        DiagnosticsBuildInfo buildInfo,
        const StartupTimingTracker& startupTimings,
        const UiTransitionTimingTracker& uiTransitionTimings,
        const StructuredLogger& logger,
        QList<MutationTelemetryRecord> mutationTelemetry = {});

private:
  [[nodiscard]] static DiagnosticsBuildInfo redactBuildInfo(DiagnosticsBuildInfo buildInfo);
};

} // namespace hcb
