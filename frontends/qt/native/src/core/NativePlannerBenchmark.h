#pragma once

#include "core/NativePerformanceFixture.h"

#include <QByteArray>

#include <cstddef>
#include <cstdint>
#include <optional>

namespace hcb {

struct NativePlannerBenchmarkResult final {
  std::size_t eventCount{0};
  std::size_t timedDayCount{0};
  std::size_t timedLayoutCount{0};
  std::size_t allDaySegmentCount{0};
  std::size_t allDayOverflowCount{0};
  std::int64_t elapsedNanoseconds{0};
};

class NativePlannerBenchmark final {
public:
  [[nodiscard]] static std::optional<NativePlannerBenchmarkResult>
  run(const NativePerformanceFixture& fixture);
  [[nodiscard]] static QByteArray toJson(const NativePlannerBenchmarkResult& result);
};

} // namespace hcb
