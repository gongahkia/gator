#pragma once

#include <chrono>

namespace hcb {

using WallTimePoint = std::chrono::system_clock::time_point;
using MonotonicTimePoint = std::chrono::steady_clock::time_point;

class Clock {
public:
  virtual ~Clock() = default;

  [[nodiscard]] virtual WallTimePoint wallNow() const noexcept = 0;
  [[nodiscard]] virtual MonotonicTimePoint monotonicNow() const noexcept = 0;
};

class SystemClock final : public Clock {
public:
  [[nodiscard]] WallTimePoint wallNow() const noexcept override {
    return std::chrono::system_clock::now();
  }

  [[nodiscard]] MonotonicTimePoint monotonicNow() const noexcept override {
    return std::chrono::steady_clock::now();
  }
};

} // namespace hcb
