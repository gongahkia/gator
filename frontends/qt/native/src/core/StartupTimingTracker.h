#pragma once

#include "core/Clock.h"
#include "core/StructuredLogger.h"

#include <chrono>
#include <cstddef>
#include <vector>

namespace hcb {

struct StartupTimingSpan final {
  QString name;
  std::chrono::milliseconds elapsed;
};

class StartupTimingTracker final {
public:
  StartupTimingTracker(const Clock& clock, StructuredLogger& logger, std::size_t maximumSpans = 32);

  bool mark(QStringView name);
  [[nodiscard]] std::vector<StartupTimingSpan> spans() const;

private:
  const Clock& clock_;
  StructuredLogger& logger_;
  const MonotonicTimePoint startedAt_;
  const std::size_t maximumSpans_;
  std::vector<StartupTimingSpan> spans_;
};

} // namespace hcb
