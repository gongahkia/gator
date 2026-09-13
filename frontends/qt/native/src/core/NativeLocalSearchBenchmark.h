#pragma once

#include <QByteArray>

#include <cstddef>
#include <cstdint>
#include <optional>
#include <vector>

namespace hcb {

struct NativeLocalSearchBenchmarkResult final {
  std::size_t corpusTaskCount;
  std::size_t matchedResultCount;
  std::vector<qint64> samplesNanoseconds;
  qint64 minimumNanoseconds;
  qint64 medianNanoseconds;
  qint64 maximumNanoseconds;
};

class NativeLocalSearchBenchmark final {
public:
  [[nodiscard]] static std::optional<NativeLocalSearchBenchmarkResult> run(std::size_t iterations);
  [[nodiscard]] static std::optional<NativeLocalSearchBenchmarkResult>
  summarize(std::size_t corpusTaskCount,
            std::size_t matchedResultCount,
            std::vector<qint64> samplesNanoseconds);
  [[nodiscard]] static QByteArray toJson(const NativeLocalSearchBenchmarkResult& result);
};

} // namespace hcb
