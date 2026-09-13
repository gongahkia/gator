#pragma once

#include <QString>

#include <optional>

namespace hcb {

struct MacOSPathLocations final {
  QString dataDirectory;
  QString cacheDirectory;
};

class MacOSPathsAdapter final {
public:
  [[nodiscard]] static std::optional<MacOSPathLocations> discover();
};

} // namespace hcb
