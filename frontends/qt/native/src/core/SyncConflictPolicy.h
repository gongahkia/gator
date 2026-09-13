#pragma once

#include <cstdint>

namespace hcb {

enum class SyncConflictPolicy : std::uint8_t {
  PreferGoogle,
  PreferHcb,
  AskEachTime
};

} // namespace hcb
