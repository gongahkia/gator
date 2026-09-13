#pragma once

#include <atomic>
#include <memory>
#include <utility>

namespace hcb {

class CancellationToken final {
public:
  CancellationToken() = default;

  [[nodiscard]] bool stop_possible() const noexcept { return state_ != nullptr; }
  [[nodiscard]] bool stop_requested() const noexcept {
    return state_ != nullptr && state_->load(std::memory_order_acquire);
  }

private:
  explicit CancellationToken(std::shared_ptr<std::atomic_bool> state) : state_(std::move(state)) {}

  std::shared_ptr<std::atomic_bool> state_;

  friend class CancellationSource;
};

class CancellationSource final {
public:
  CancellationSource() = default;
  CancellationSource(const CancellationSource&) = delete;
  CancellationSource& operator=(const CancellationSource&) = delete;

  [[nodiscard]] CancellationToken token() const noexcept { return CancellationToken(state_); }
  [[nodiscard]] bool requestStop() noexcept {
    bool expected = false;
    return state_->compare_exchange_strong(expected, true, std::memory_order_acq_rel);
  }
  [[nodiscard]] bool stopRequested() const noexcept {
    return state_->load(std::memory_order_acquire);
  }

private:
  std::shared_ptr<std::atomic_bool> state_{std::make_shared<std::atomic_bool>(false)};
};

} // namespace hcb
