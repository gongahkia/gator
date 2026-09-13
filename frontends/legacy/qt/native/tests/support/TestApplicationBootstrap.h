#pragma once

#include "app/AppPaths.h"
#include "app/AppServices.h"
#include "core/Clock.h"
#include "core/SettingsRegistry.h"

#include <utility>

namespace hcb::test {

class TestClock final : public Clock {
public:
  TestClock(WallTimePoint wallTime, MonotonicTimePoint monotonicTime)
      : wallTime_(wallTime), monotonicTime_(monotonicTime) {}

  [[nodiscard]] WallTimePoint wallNow() const noexcept override { return wallTime_; }
  [[nodiscard]] MonotonicTimePoint monotonicNow() const noexcept override { return monotonicTime_; }

  void setTimes(WallTimePoint wallTime, MonotonicTimePoint monotonicTime) noexcept {
    wallTime_ = wallTime;
    monotonicTime_ = monotonicTime;
  }

private:
  WallTimePoint wallTime_;
  MonotonicTimePoint monotonicTime_;
};

class TestApplicationBootstrap final {
public:
  TestApplicationBootstrap(AppPaths paths, WallTimePoint wallTime, MonotonicTimePoint monotonicTime)
      : paths_(std::move(paths)), clock_(wallTime, monotonicTime) {}

  TestApplicationBootstrap(const TestApplicationBootstrap&) = delete;
  TestApplicationBootstrap& operator=(const TestApplicationBootstrap&) = delete;

  [[nodiscard]] AppServices makeServices() & { return AppServices(paths_, clock_, settings_); }
  [[nodiscard]] TestClock& clock() noexcept { return clock_; }
  [[nodiscard]] SettingsRegistry& settings() noexcept { return settings_; }

private:
  AppPaths paths_;
  TestClock clock_;
  SettingsRegistry settings_;
};

} // namespace hcb::test
