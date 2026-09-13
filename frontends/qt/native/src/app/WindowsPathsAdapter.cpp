#include "app/WindowsPathsAdapter.h"

#include <QStandardPaths>

namespace hcb {

std::optional<WindowsPathLocations> WindowsPathsAdapter::discover() {
#if defined(Q_OS_WIN)
  const QString dataDirectory = QStandardPaths::writableLocation(QStandardPaths::AppDataLocation);
  const QString cacheDirectory = QStandardPaths::writableLocation(QStandardPaths::CacheLocation);
  if (dataDirectory.isEmpty() || cacheDirectory.isEmpty()) {
    return std::nullopt;
  }
  return WindowsPathLocations{.dataDirectory = dataDirectory, .cacheDirectory = cacheDirectory};
#else
  return std::nullopt;
#endif
}

} // namespace hcb
