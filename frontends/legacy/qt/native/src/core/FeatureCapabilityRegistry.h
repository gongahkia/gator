#pragma once

#include <atomic>
#include <cstdint>
#include <initializer_list>

namespace hcb {

enum class FeatureCapability : std::uint8_t {
  Tasks,
  Calendar,
  Notes,
  Settings,
  Count,
  Invalid = 255
};

class FeatureCapabilityRegistry final {
public:
  FeatureCapabilityRegistry() = default;
  FeatureCapabilityRegistry(std::initializer_list<FeatureCapability> enabled) noexcept {
    for (const FeatureCapability capability : enabled) {
      static_cast<void>(enable(capability));
    }
  }

  FeatureCapabilityRegistry(const FeatureCapabilityRegistry&) = delete;
  FeatureCapabilityRegistry& operator=(const FeatureCapabilityRegistry&) = delete;

  [[nodiscard]] bool isEnabled(FeatureCapability capability) const noexcept {
    const std::uint64_t capabilityMask = mask(capability);
    return capabilityMask != 0 && (enabled_.load(std::memory_order_acquire) & capabilityMask) != 0;
  }

  [[nodiscard]] bool enable(FeatureCapability capability) noexcept {
    return update(capability, true);
  }

  [[nodiscard]] bool disable(FeatureCapability capability) noexcept {
    return update(capability, false);
  }

private:
  static constexpr std::uint64_t mask(FeatureCapability capability) noexcept {
    const auto index = static_cast<std::uint8_t>(capability);
    return index < static_cast<std::uint8_t>(FeatureCapability::Count) ? std::uint64_t{1} << index
                                                                       : 0;
  }

  [[nodiscard]] bool update(FeatureCapability capability, bool enableCapability) noexcept {
    const std::uint64_t capabilityMask = mask(capability);
    if (capabilityMask == 0) {
      return false;
    }

    std::uint64_t current = enabled_.load(std::memory_order_acquire);
    while (true) {
      const bool isEnabled = (current & capabilityMask) != 0;
      if (isEnabled == enableCapability) {
        return false;
      }
      const std::uint64_t next =
          enableCapability ? current | capabilityMask : current & ~capabilityMask;
      if (enabled_.compare_exchange_weak(
              current, next, std::memory_order_acq_rel, std::memory_order_acquire)) {
        return true;
      }
    }
  }

  std::atomic<std::uint64_t> enabled_{0};
};

} // namespace hcb
