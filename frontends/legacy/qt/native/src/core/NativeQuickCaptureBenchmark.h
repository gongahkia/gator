#pragma once

#include <QByteArray>

#include <cstddef>
#include <cstdint>
#include <optional>
#include <vector>

namespace hcb {

struct NativeQuickCaptureBenchmarkResult final {
  std::vector<qint64> samplesNanoseconds;
  qint64 minimumNanoseconds;
  qint64 medianNanoseconds;
  qint64 maximumNanoseconds;
};

class NativeQuickCaptureBenchmark final {
public:
  [[nodiscard]] static std::optional<NativeQuickCaptureBenchmarkResult> run(std::size_t iterations);
  [[nodiscard]] static std::optional<NativeQuickCaptureBenchmarkResult>
  summarize(std::vector<qint64> samplesNanoseconds);
  [[nodiscard]] static QByteArray toJson(const NativeQuickCaptureBenchmarkResult& result);
};

} // namespace hcb
