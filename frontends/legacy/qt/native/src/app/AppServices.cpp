#include "app/AppServices.h"

#include <utility>

namespace hcb {

AppServices::AppServices(AppPaths paths, Clock& clock, SettingsRegistry& settings)
    : paths_(std::move(paths)), clock_(clock), settings_(settings) {}

const AppPaths& AppServices::paths() const noexcept { return paths_; }

const Clock& AppServices::clock() const noexcept { return clock_; }

SettingsRegistry& AppServices::settings() noexcept { return settings_; }

const SettingsRegistry& AppServices::settings() const noexcept { return settings_; }

} // namespace hcb
