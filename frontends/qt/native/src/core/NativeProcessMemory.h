#pragma once

#include <QtGlobal>

#include <optional>

namespace hcb {

class NativeProcessMemory final {
public:
  [[nodiscard]] static std::optional<quint64> residentBytes();
};

} // namespace hcb
