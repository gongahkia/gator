#include "core/SyncScheduler.h"

#include <algorithm>
#include <condition_variable>
#include <deque>
#include <exception>
#include <mutex>
#include <optional>
#include <thread>
#include <utility>

namespace hcb {
namespace {

struct WaitingSyncRequest final {
  SyncScheduleTrigger trigger;
  std::shared_ptr<std::promise<SyncSchedulerResult>> completion;
};

[[nodiscard]] AppError cancelledError() {
  return AppError(AppErrorCode::Cancelled, QStringLiteral("Sync scheduler is stopped"));
}

[[nodiscard]] AppError executionError() {
  return AppError(AppErrorCode::Network, QStringLiteral("Scheduled sync execution failed"));
}

[[nodiscard]] const Clock& systemClock() {
  static const SystemClock clock;
  return clock;
}

[[nodiscard]] std::vector<SyncScheduleTrigger>
coalescedTriggers(const std::vector<WaitingSyncRequest>& requests) {
  std::vector<SyncScheduleTrigger> triggers;
  for (const WaitingSyncRequest& request : requests) {
    if (std::find(triggers.cbegin(), triggers.cend(), request.trigger) == triggers.cend()) {
      triggers.push_back(request.trigger);
    }
  }
  return triggers;
}

void complete(const std::vector<WaitingSyncRequest>& requests,
              const SyncSchedulerResult& result) noexcept {
  for (const WaitingSyncRequest& request : requests) {
    if (request.completion != nullptr) {
      try {
        request.completion->set_value(result);
      } catch (const std::future_error&) {
        std::terminate();
      }
    }
  }
}

} // namespace

struct SyncScheduler::State final {
  State(SyncSchedulerExecutor executorValue,
        const Clock& clockValue,
        std::size_t maximumMetricsValue)
      : executor(std::move(executorValue)), clock(clockValue),
        maximumMetrics(std::max<std::size_t>(maximumMetricsValue, 1)) {}

  std::mutex mutex;
  std::condition_variable timerWake;
  std::condition_variable workWake;
  SyncSchedulerExecutor executor;
  const Clock& clock;
  const std::size_t maximumMetrics;
  std::vector<WaitingSyncRequest> pending;
  std::deque<SyncSchedulerMetric> metrics;
  std::uint64_t nextMetricSequence{1};
  std::thread periodicThread;
  std::thread workerThread;
  std::chrono::milliseconds interval{0};
  bool online{true};
  bool running{false};
  bool periodic{false};
  bool stopped{false};
};

SyncScheduler::SyncScheduler(SyncSchedulerExecutor executor)
    : SyncScheduler(std::move(executor), systemClock()) {}

SyncScheduler::SyncScheduler(SyncSchedulerExecutor executor,
                             const Clock& clock,
                             std::size_t maximumMetrics)
    : state_(std::make_shared<State>(std::move(executor), clock, maximumMetrics)) {
  state_->workerThread = std::thread([state = state_] { workerLoop(state); });
}

SyncScheduler::~SyncScheduler() { stop(); }

std::future<SyncSchedulerResult> SyncScheduler::request(SyncScheduleTrigger trigger) {
  auto completion = std::make_shared<std::promise<SyncSchedulerResult>>();
  std::future<SyncSchedulerResult> future = completion->get_future();
  bool cancelled = false;
  {
    std::lock_guard<std::mutex> lock(state_->mutex);
    cancelled = state_->stopped;
    if (!cancelled) {
      state_->pending.push_back({.trigger = trigger, .completion = completion});
    }
  }
  if (cancelled) {
    completion->set_value(cancelledError());
  } else {
    state_->workWake.notify_one();
  }
  return future;
}

void SyncScheduler::setOnline(bool online) {
  {
    std::lock_guard<std::mutex> lock(state_->mutex);
    if (state_->stopped) {
      return;
    }
    state_->online = online;
  }
  if (online) {
    state_->workWake.notify_one();
  }
}

bool SyncScheduler::startPeriodic(std::chrono::milliseconds interval) {
  if (interval <= std::chrono::milliseconds::zero()) {
    return false;
  }
  {
    std::lock_guard<std::mutex> lock(state_->mutex);
    if (state_->stopped || state_->periodic || state_->periodicThread.joinable()) {
      return false;
    }
    state_->interval = interval;
    state_->periodic = true;
  }
  try {
    std::thread periodicThread([state = state_] {
      std::unique_lock<std::mutex> lock(state->mutex);
      while (!state->stopped && state->periodic) {
        const bool interrupted = state->timerWake.wait_for(
            lock, state->interval, [state] { return state->stopped || !state->periodic; });
        if (interrupted) {
          break;
        }
        lock.unlock();
        enqueuePeriodic(state);
        lock.lock();
      }
    });
    {
      std::lock_guard<std::mutex> lock(state_->mutex);
      if (!state_->stopped) {
        state_->periodicThread = std::move(periodicThread);
        return true;
      }
    }
    periodicThread.join();
  } catch (...) {
    std::lock_guard<std::mutex> lock(state_->mutex);
    state_->periodic = false;
    return false;
  }
  return false;
}

void SyncScheduler::stop() {
  std::vector<WaitingSyncRequest> pending;
  std::thread periodicThread;
  std::thread workerThread;
  {
    std::lock_guard<std::mutex> lock(state_->mutex);
    if (state_->stopped) {
      return;
    }
    state_->stopped = true;
    state_->periodic = false;
    pending = std::move(state_->pending);
    periodicThread = std::move(state_->periodicThread);
    workerThread = std::move(state_->workerThread);
  }
  state_->timerWake.notify_all();
  state_->workWake.notify_all();
  if (periodicThread.joinable()) {
    periodicThread.join();
  }
  complete(pending, SyncSchedulerResult(cancelledError()));
  if (workerThread.joinable()) {
    workerThread.join();
  }
}

SyncSchedulerSnapshot SyncScheduler::snapshot() const {
  std::lock_guard<std::mutex> lock(state_->mutex);
  return {.online = state_->online,
          .running = state_->running,
          .pending = !state_->pending.empty(),
          .periodic = state_->periodic,
          .stopped = state_->stopped};
}

std::vector<SyncSchedulerMetric> SyncScheduler::metrics() const {
  std::lock_guard<std::mutex> lock(state_->mutex);
  return {state_->metrics.cbegin(), state_->metrics.cend()};
}

void SyncScheduler::enqueuePeriodic(const std::shared_ptr<State>& state) {
  {
    std::lock_guard<std::mutex> lock(state->mutex);
    if (state->stopped) {
      return;
    }
    state->pending.push_back({.trigger = SyncScheduleTrigger::Periodic, .completion = nullptr});
  }
  state->workWake.notify_one();
}

void SyncScheduler::workerLoop(const std::shared_ptr<State>& state) {
  while (true) {
    std::vector<WaitingSyncRequest> requests;
    SyncSchedulerRequest request;
    {
      std::unique_lock<std::mutex> lock(state->mutex);
      state->workWake.wait(lock, [state] {
        return state->stopped || (state->online && !state->pending.empty());
      });
      if (state->stopped) {
        return;
      }
      state->running = true;
      requests = std::move(state->pending);
      state->pending.clear();
      request.triggers = coalescedTriggers(requests);
    }
    std::optional<AppError> error;
    const MonotonicTimePoint startedAt = state->clock.monotonicNow();
    try {
      error = state->executor(request);
    } catch (...) {
      error = executionError();
    }
    std::chrono::milliseconds elapsed = std::chrono::duration_cast<std::chrono::milliseconds>(
        state->clock.monotonicNow() - startedAt);
    if (elapsed < std::chrono::milliseconds::zero()) {
      elapsed = std::chrono::milliseconds::zero();
    }
    const SyncSchedulerResult result =
        error.has_value() ? SyncSchedulerResult(*error)
                          : SyncSchedulerResult(SyncSchedulerRun{.triggers = request.triggers});
    {
      std::lock_guard<std::mutex> lock(state->mutex);
      state->metrics.push_back(
          {.sequence = state->nextMetricSequence++,
           .completedAt = state->clock.wallNow(),
           .triggers = request.triggers,
           .elapsed = elapsed,
           .outcome = error.has_value() ? SyncSchedulerMetricOutcome::Failed
                                        : SyncSchedulerMetricOutcome::Succeeded,
           .errorCode =
               error.has_value() ? std::optional<AppErrorCode>(error->code()) : std::nullopt});
      while (state->metrics.size() > state->maximumMetrics) {
        state->metrics.pop_front();
      }
      state->running = false;
    }
    complete(requests, result);
  }
}

} // namespace hcb
