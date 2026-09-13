#pragma once

#include <QString>

#include <optional>

namespace hcb {

struct LinuxPathLocations final {
  QString dataDirectory;
  QString cacheDirectory;
};

class LinuxPathsAdapter final {
public:
  [[nodiscard]] static std::optional<LinuxPathLocations> discover();
};

} // namespace hcb
