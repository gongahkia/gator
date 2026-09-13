#pragma once

#include <QByteArray>
#include <QString>
#include <QStringList>

#include <chrono>
#include <optional>
#include <vector>

namespace hcb {

struct NativeLaunchBenchmarkResult final {
  std::vector<qint64> samplesMilliseconds;
  qint64 minimumMilliseconds;
  qint64 medianMilliseconds;
  qint64 maximumMilliseconds;
};

class NativeLaunchBenchmark final {
public:
  [[nodiscard]] static std::optional<NativeLaunchBenchmarkResult>
  measure(const QString& executable,
          const QStringList& arguments,
          std::size_t iterations,
          std::chrono::milliseconds timeout,
          QString* error);
  [[nodiscard]] static std::optional<NativeLaunchBenchmarkResult>
  summarize(std::vector<qint64> samplesMilliseconds);
  [[nodiscard]] static QByteArray toJson(const NativeLaunchBenchmarkResult& result);
};

} // namespace hcb
