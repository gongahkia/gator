#pragma once

#include "core/AppError.h"
#include "data/SqliteConnection.h"

#include <optional>
#include <variant>

namespace hcb {

class SqliteTransaction final {
public:
  SqliteTransaction(const SqliteTransaction&) = delete;
  SqliteTransaction& operator=(const SqliteTransaction&) = delete;
  SqliteTransaction(SqliteTransaction&& other) noexcept;
  SqliteTransaction& operator=(SqliteTransaction&&) = delete;
  ~SqliteTransaction();

  [[nodiscard]] static std::variant<SqliteTransaction, AppError>
  begin(SqliteConnection& connection);
  [[nodiscard]] std::optional<AppError> commit();
  [[nodiscard]] std::optional<AppError> rollback();
  [[nodiscard]] bool active() const noexcept;

private:
  explicit SqliteTransaction(SqliteConnection& connection) noexcept;
  [[nodiscard]] std::optional<AppError> finish(const char* sql, QString errorMessage);

  SqliteConnection* connection_{nullptr};
  bool active_{false};
};

using SqliteTransactionResult = std::variant<SqliteTransaction, AppError>;

} // namespace hcb
