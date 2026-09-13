#pragma once

#include "core/AppError.h"
#include "data/SqliteConnection.h"

#include <functional>
#include <optional>
#include <span>
#include <variant>
#include <vector>

namespace hcb {

using SqliteMigrationApply = std::function<std::optional<AppError>(SqliteConnection& connection)>;

struct SqliteMigration final {
  int version{0};
  QString name;
  QString checksum;
  SqliteMigrationApply apply;
  std::optional<QString> acceptedLegacyChecksum;
};

struct SqliteMigrationRunResult final {
  int version{0};
  std::vector<int> appliedVersions;
};

using SqliteMigrationRunResultOrError = std::variant<SqliteMigrationRunResult, AppError>;

class SqliteMigrationRunner final {
public:
  [[nodiscard]] static SqliteMigrationRunResultOrError
  run(SqliteConnection& connection, std::span<const SqliteMigration> migrations);
};

} // namespace hcb
