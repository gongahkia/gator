#pragma once

#include "data/SqliteMigrationRunner.h"

namespace hcb {

class LocalSchema final {
public:
  [[nodiscard]] static SqliteMigrationRunResultOrError initialize(SqliteConnection& connection);
};

} // namespace hcb
