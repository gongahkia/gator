#pragma once

#include <QByteArray>

#include <cstddef>
#include <cstdint>
#include <optional>

namespace hcb {

struct NativeSyncApplyBenchmarkResult final {
  std::size_t taskCount;
  std::size_t eventCount;
  std::size_t recurrenceExceptionCount;
  std::size_t queuedMutationCount;
  qint64 elapsedNanoseconds;
};

class NativeSyncApplyBenchmark final {
public:
  [[nodiscard]] static std::optional<NativeSyncApplyBenchmarkResult> run();
  [[nodiscard]] static QByteArray toJson(const NativeSyncApplyBenchmarkResult& result);
};

} // namespace hcb
