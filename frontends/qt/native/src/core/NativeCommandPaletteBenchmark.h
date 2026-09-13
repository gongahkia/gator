#pragma once

#include <QByteArray>
#include <QString>
#include <QStringList>

#include <chrono>
#include <optional>
#include <vector>

namespace hcb {

struct NativeCommandPaletteBenchmarkResult final {
  std::vector<qint64> samplesMilliseconds;
  qint64 minimumMilliseconds;
  qint64 medianMilliseconds;
  qint64 maximumMilliseconds;
};

class NativeCommandPaletteBenchmark final {
public:
  [[nodiscard]] static std::optional<NativeCommandPaletteBenchmarkResult>
  measure(const QString& executable,
          const QStringList& arguments,
          std::size_t iterations,
          std::chrono::milliseconds timeout,
          QString* error);
  [[nodiscard]] static std::optional<NativeCommandPaletteBenchmarkResult>
  summarize(std::vector<qint64> samplesMilliseconds);
  [[nodiscard]] static std::optional<qint64> parseElapsedMilliseconds(const QByteArray& output);
  [[nodiscard]] static QByteArray toJson(const NativeCommandPaletteBenchmarkResult& result);
};

} // namespace hcb
