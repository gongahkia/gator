#pragma once

#include "core/Clock.h"

#include <chrono>
#include <cstddef>
#include <cstdint>
#include <deque>
#include <mutex>
#include <vector>

struct sqlite3_stmt;

namespace hcb {

class SqliteConnection;

struct SqliteQueryTimingSample final {
  std::uint64_t sequence{0};
  WallTimePoint timestamp;
  std::chrono::nanoseconds elapsed;
  bool readOnly{false};
};

class SqliteQueryTimingTracker final {
public:
  explicit SqliteQueryTimingTracker(const Clock& clock, std::size_t capacity = 500);

  [[nodiscard]] std::vector<SqliteQueryTimingSample> samples() const;
  [[nodiscard]] std::vector<SqliteQueryTimingSample>
  slowSamples(std::chrono::nanoseconds minimumElapsed) const;
  [[nodiscard]] std::size_t size() const;
  void clear();

private:
  static int profileCallback(unsigned eventType,
                             void* context,
                             void* statement,
                             void* elapsedNanoseconds) noexcept;
  void record(sqlite3_stmt* statement, std::int64_t elapsedNanoseconds);

  const Clock& clock_;
  const std::size_t capacity_;
  mutable std::mutex mutex_;
  std::deque<SqliteQueryTimingSample> samples_;
  std::uint64_t nextSequence_{1};

  friend class SqliteConnection;
};

} // namespace hcb
