#pragma once

#include "app/AppPaths.h"
#include "core/Clock.h"
#include "core/SettingsRegistry.h"

namespace hcb {

class AppServices final {
public:
  AppServices(AppPaths paths, Clock& clock, SettingsRegistry& settings);

  [[nodiscard]] const AppPaths& paths() const noexcept;
  [[nodiscard]] const Clock& clock() const noexcept;
  [[nodiscard]] SettingsRegistry& settings() noexcept;
  [[nodiscard]] const SettingsRegistry& settings() const noexcept;

private:
  AppPaths paths_;
  Clock& clock_;
  SettingsRegistry& settings_;
};

} // namespace hcb
