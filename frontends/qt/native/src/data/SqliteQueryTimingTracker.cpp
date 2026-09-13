#include "data/SqliteQueryTimingTracker.h"

#include "sqlite3.h"

#include <algorithm>
#include <iterator>

namespace hcb {

SqliteQueryTimingTracker::SqliteQueryTimingTracker(const Clock& clock, std::size_t capacity)
    : clock_(clock), capacity_(std::max<std::size_t>(capacity, 1)) {}

std::vector<SqliteQueryTimingSample> SqliteQueryTimingTracker::samples() const {
  std::lock_guard<std::mutex> lock(mutex_);
  return {samples_.cbegin(), samples_.cend()};
}

std::vector<SqliteQueryTimingSample>
SqliteQueryTimingTracker::slowSamples(std::chrono::nanoseconds minimumElapsed) const {
  const std::chrono::nanoseconds threshold =
      std::max(minimumElapsed, std::chrono::nanoseconds::zero());
  std::lock_guard<std::mutex> lock(mutex_);
  std::vector<SqliteQueryTimingSample> slowSamples;
  slowSamples.reserve(samples_.size());
  std::copy_if(
      samples_.cbegin(),
      samples_.cend(),
      std::back_inserter(slowSamples),
      [threshold](const SqliteQueryTimingSample& sample) { return sample.elapsed >= threshold; });
  return slowSamples;
}

std::size_t SqliteQueryTimingTracker::size() const {
  std::lock_guard<std::mutex> lock(mutex_);
  return samples_.size();
}

void SqliteQueryTimingTracker::clear() {
  std::lock_guard<std::mutex> lock(mutex_);
  samples_.clear();
}

int SqliteQueryTimingTracker::profileCallback(unsigned eventType,
                                              void* context,
                                              void* statement,
                                              void* elapsedNanoseconds) noexcept {
  if (eventType != SQLITE_TRACE_PROFILE || context == nullptr || statement == nullptr ||
      elapsedNanoseconds == nullptr) {
    return 0;
  }
  try {
    auto* const tracker = static_cast<SqliteQueryTimingTracker*>(context);
    const auto* const elapsed = static_cast<const sqlite3_int64*>(elapsedNanoseconds);
    tracker->record(static_cast<sqlite3_stmt*>(statement), *elapsed);
  } catch (...) {
    return 0; // sqlite trace callbacks must not throw
  }
  return 0;
}

void SqliteQueryTimingTracker::record(sqlite3_stmt* statement, std::int64_t elapsedNanoseconds) {
  const std::chrono::nanoseconds elapsed = elapsedNanoseconds < 0
                                               ? std::chrono::nanoseconds::zero()
                                               : std::chrono::nanoseconds(elapsedNanoseconds);
  const WallTimePoint timestamp = clock_.wallNow();
  const bool readOnly = sqlite3_stmt_readonly(statement) != 0;
  std::lock_guard<std::mutex> lock(mutex_);
  samples_.push_back(SqliteQueryTimingSample{nextSequence_++, timestamp, elapsed, readOnly});
  while (samples_.size() > capacity_) {
    samples_.pop_front();
  }
}

} // namespace hcb
