#pragma once

#include <QByteArray>

#include <cstddef>
#include <cstdint>
#include <optional>
#include <vector>

namespace hcb {

struct NativeTaskScrollBenchmarkResult final {
  std::size_t taskCount;
  std::vector<qint64> bulkSelectionSamplesNanoseconds;
  qint64 bulkSelectionMedianNanoseconds;
  qint64 firstCachedRenderNanoseconds;
  std::vector<qint64> samplesNanoseconds;
  qint64 minimumNanoseconds;
  qint64 medianNanoseconds;
  qint64 maximumNanoseconds;
};

class NativeTaskScrollBenchmark final {
public:
  [[nodiscard]] static std::optional<NativeTaskScrollBenchmarkResult> run(std::size_t frameCount);
  [[nodiscard]] static std::optional<NativeTaskScrollBenchmarkResult>
  summarize(std::size_t taskCount,
            std::vector<qint64> bulkSelectionSamplesNanoseconds,
            qint64 firstCachedRenderNanoseconds,
            std::vector<qint64> samplesNanoseconds);
  [[nodiscard]] static QByteArray toJson(const NativeTaskScrollBenchmarkResult& result);
};

} // namespace hcb
