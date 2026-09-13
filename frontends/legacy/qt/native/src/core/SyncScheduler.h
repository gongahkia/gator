#pragma once

#include "core/AppError.h"
#include "core/Clock.h"

#include <chrono>
#include <cstdint>
#include <cstddef>
#include <functional>
#include <future>
#include <memory>
#include <optional>
#include <variant>
#include <vector>

namespace hcb {

enum class SyncScheduleTrigger : std::uint8_t {
  Startup,
  Manual,
  NetworkRestored,
  Periodic
};

struct SyncSchedulerRequest final {
  std::vector<SyncScheduleTrigger> triggers;
};

struct SyncSchedulerRun final {
  std::vector<SyncScheduleTrigger> triggers;
};

enum class SyncSchedulerMetricOutcome : std::uint8_t {
  Succeeded,
  Failed
};

struct SyncSchedulerMetric final {
  std::uint64_t sequence;
  WallTimePoint completedAt;
  std::vector<SyncScheduleTrigger> triggers;
  std::chrono::milliseconds elapsed;
  SyncSchedulerMetricOutcome outcome;
  std::optional<AppErrorCode> errorCode;
};

struct SyncSchedulerSnapshot final {
  bool online{true};
  bool running{false};
  bool pending{false};
  bool periodic{false};
  bool stopped{false};
};

using SyncSchedulerExecutor =
    std::function<std::optional<AppError>(const SyncSchedulerRequest& request)>;
using SyncSchedulerResult = std::variant<SyncSchedulerRun, AppError>;

class SyncScheduler final {
public:
  explicit SyncScheduler(SyncSchedulerExecutor executor);
  SyncScheduler(SyncSchedulerExecutor executor,
                const Clock& clock,
                std::size_t maximumMetrics = 100);
  ~SyncScheduler();
  SyncScheduler(const SyncScheduler&) = delete;
  SyncScheduler& operator=(const SyncScheduler&) = delete;

  [[nodiscard]] std::future<SyncSchedulerResult> request(SyncScheduleTrigger trigger);
  void setOnline(bool online);
  [[nodiscard]] bool startPeriodic(std::chrono::milliseconds interval);
  void stop();
  [[nodiscard]] SyncSchedulerSnapshot snapshot() const;
  [[nodiscard]] std::vector<SyncSchedulerMetric> metrics() const;

private:
  struct State;

  static void enqueuePeriodic(const std::shared_ptr<State>& state);
  static void workerLoop(const std::shared_ptr<State>& state);

  std::shared_ptr<State> state_;
};

} // namespace hcb
