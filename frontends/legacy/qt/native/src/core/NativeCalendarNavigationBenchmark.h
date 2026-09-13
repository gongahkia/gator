#pragma once

#include <QByteArray>

#include <cstddef>
#include <cstdint>
#include <optional>
#include <vector>

namespace hcb {

struct NativeCalendarNavigationBenchmarkResult final {
  std::size_t eventCount;
  std::vector<qint64> samplesNanoseconds;
  qint64 minimumNanoseconds;
  qint64 medianNanoseconds;
  qint64 maximumNanoseconds;
};

class NativeCalendarNavigationBenchmark final {
public:
  [[nodiscard]] static std::optional<NativeCalendarNavigationBenchmarkResult>
  run(std::size_t frameCount);
  [[nodiscard]] static std::optional<NativeCalendarNavigationBenchmarkResult>
  summarize(std::size_t eventCount, std::vector<qint64> samplesNanoseconds);
  [[nodiscard]] static QByteArray toJson(const NativeCalendarNavigationBenchmarkResult& result);
};

} // namespace hcb
