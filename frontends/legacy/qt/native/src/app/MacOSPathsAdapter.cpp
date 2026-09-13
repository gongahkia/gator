#include "app/MacOSPathsAdapter.h"

#include <QStandardPaths>

namespace hcb {

std::optional<MacOSPathLocations> MacOSPathsAdapter::discover() {
#if defined(Q_OS_MACOS)
  const QString dataDirectory = QStandardPaths::writableLocation(QStandardPaths::AppDataLocation);
  const QString cacheDirectory = QStandardPaths::writableLocation(QStandardPaths::CacheLocation);
  if (dataDirectory.isEmpty() || cacheDirectory.isEmpty()) {
    return std::nullopt;
  }
  return MacOSPathLocations{.dataDirectory = dataDirectory, .cacheDirectory = cacheDirectory};
#else
  return std::nullopt;
#endif
}

} // namespace hcb
