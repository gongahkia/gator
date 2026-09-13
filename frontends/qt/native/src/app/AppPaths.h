#pragma once

#include "core/FilePath.h"

#include <optional>

namespace hcb {

class AppPaths final {
public:
  [[nodiscard]] static std::optional<AppPaths> discover();
  [[nodiscard]] static AppPaths fromDirectories(FilePath dataDirectory, FilePath cacheDirectory);

  [[nodiscard]] const FilePath& dataDirectory() const noexcept;
  [[nodiscard]] const FilePath& cacheDirectory() const noexcept;

private:
  struct DataDirectory final {
    FilePath value;
  };
  struct CacheDirectory final {
    FilePath value;
  };

  AppPaths(DataDirectory dataDirectory, CacheDirectory cacheDirectory);

  FilePath dataDirectory_;
  FilePath cacheDirectory_;
};

} // namespace hcb
