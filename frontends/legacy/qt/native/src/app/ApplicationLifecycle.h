#pragma once

#include <atomic>

namespace hcb {

enum class ApplicationState {
  Starting,
  Ready,
  Stopping,
  Stopped,
  Failed
};

class ApplicationLifecycle final {
public:
  [[nodiscard]] ApplicationState state() const noexcept {
    return state_.load(std::memory_order_acquire);
  }

  [[nodiscard]] bool markReady() noexcept {
    return transition(ApplicationState::Starting, ApplicationState::Ready);
  }

  [[nodiscard]] bool requestStop() noexcept {
    ApplicationState current = state();
    while (current == ApplicationState::Starting || current == ApplicationState::Ready ||
           current == ApplicationState::Failed) {
      if (state_.compare_exchange_weak(current,
                                       ApplicationState::Stopping,
                                       std::memory_order_acq_rel,
                                       std::memory_order_acquire)) {
        return true;
      }
    }
    return false;
  }

  [[nodiscard]] bool markStopped() noexcept {
    return transition(ApplicationState::Stopping, ApplicationState::Stopped);
  }

  [[nodiscard]] bool fail() noexcept {
    ApplicationState current = state();
    while (current == ApplicationState::Starting || current == ApplicationState::Ready) {
      if (state_.compare_exchange_weak(current,
                                       ApplicationState::Failed,
                                       std::memory_order_acq_rel,
                                       std::memory_order_acquire)) {
        return true;
      }
    }
    return false;
  }

private:
  [[nodiscard]] bool transition(ApplicationState expected, ApplicationState next) noexcept {
    return state_.compare_exchange_strong(
        expected, next, std::memory_order_acq_rel, std::memory_order_acquire);
  }

  std::atomic<ApplicationState> state_{ApplicationState::Starting};
};

} // namespace hcb
