#pragma once

#include <QString>

#include <optional>

namespace hcb {

struct WindowsPathLocations final {
  QString dataDirectory;
  QString cacheDirectory;
};

class WindowsPathsAdapter final {
public:
  [[nodiscard]] static std::optional<WindowsPathLocations> discover();
};

} // namespace hcb
