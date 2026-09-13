#include "app/LinuxPathsAdapter.h"

#include <QStandardPaths>

namespace hcb {

std::optional<LinuxPathLocations> LinuxPathsAdapter::discover() {
#if defined(Q_OS_LINUX)
  const QString dataDirectory = QStandardPaths::writableLocation(QStandardPaths::AppDataLocation);
  const QString cacheDirectory = QStandardPaths::writableLocation(QStandardPaths::CacheLocation);
  if (dataDirectory.isEmpty() || cacheDirectory.isEmpty()) {
    return std::nullopt;
  }
  return LinuxPathLocations{.dataDirectory = dataDirectory, .cacheDirectory = cacheDirectory};
#else
  return std::nullopt;
#endif
}

} // namespace hcb
