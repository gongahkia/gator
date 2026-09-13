#include "core/StartupTimingTracker.h"

#include "core/SecretRedactor.h"

#include <algorithm>
#include <utility>

namespace hcb {

StartupTimingTracker::StartupTimingTracker(const Clock& clock,
                                           StructuredLogger& logger,
                                           std::size_t maximumSpans)
    : clock_(clock), logger_(logger), startedAt_(clock.monotonicNow()),
      maximumSpans_(std::max<std::size_t>(maximumSpans, 1)) {
  spans_.reserve(maximumSpans_);
}

bool StartupTimingTracker::mark(QStringView name) {
  const QString safeName = SecretRedactor::redactText(name, 120);
  if (safeName.isEmpty() || spans_.size() >= maximumSpans_ ||
      std::any_of(spans_.cbegin(), spans_.cend(), [&safeName](const StartupTimingSpan& span) {
        return span.name == safeName;
      })) {
    return false;
  }

  const MonotonicTimePoint now = clock_.monotonicNow();
  std::chrono::milliseconds elapsed =
      std::chrono::duration_cast<std::chrono::milliseconds>(now - startedAt_);
  if (elapsed < std::chrono::milliseconds::zero()) {
    elapsed = std::chrono::milliseconds::zero();
  }
  spans_.push_back(StartupTimingSpan{safeName, elapsed});
  logger_.log(LogLevel::Info,
              {u"startup", u"startup span completed"},
              {{QStringLiteral("span"), safeName},
               {QStringLiteral("elapsed_ms"), QString::number(elapsed.count())}});
  return true;
}

std::vector<StartupTimingSpan> StartupTimingTracker::spans() const { return spans_; }

} // namespace hcb
