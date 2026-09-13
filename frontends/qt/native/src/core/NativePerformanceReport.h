#pragma once

#include "core/NativeLaunchBenchmark.h"

#include <QByteArray>
#include <QString>

#include <cstdint>

namespace hcb {

struct NativePerformancePlatform final {
  QString productType;
  QString productVersion;
  QString cpuArchitecture;
};

struct NativePerformanceReport final {
  std::uint32_t schemaVersion{1};
  NativePerformancePlatform platform;
  NativeLaunchBenchmarkResult launch;
  qint64 idleDurationMilliseconds;
  qint64 idleRssBytes;
};

class NativePerformanceReportSerializer final {
public:
  [[nodiscard]] static NativePerformancePlatform currentPlatform();
  [[nodiscard]] static QByteArray toJson(const NativePerformanceReport& report);
};

} // namespace hcb
