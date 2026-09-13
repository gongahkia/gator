#include "data/SqliteTransaction.h"

#include "sqlite3.h"

#include <utility>

namespace hcb {
namespace {

[[nodiscard]] AppError transactionError(QString message, int result) {
  return AppError(AppErrorCode::Database, std::move(message).arg(result));
}

[[nodiscard]] int execute(sqlite3* handle, const char* sql) {
  char* sqliteErrorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &sqliteErrorMessage);
  sqlite3_free(sqliteErrorMessage);
  return result;
}

} // namespace

SqliteTransaction::SqliteTransaction(SqliteConnection& connection) noexcept
    : connection_(&connection), active_(true) {}

SqliteTransaction::SqliteTransaction(SqliteTransaction&& other) noexcept
    : connection_(std::exchange(other.connection_, nullptr)),
      active_(std::exchange(other.active_, false)) {}

SqliteTransaction::~SqliteTransaction() { static_cast<void>(rollback()); }

SqliteTransactionResult SqliteTransaction::begin(SqliteConnection& connection) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite transaction connection is unavailable"));
  }
  const int result = execute(handle, "BEGIN IMMEDIATE");
  if (result != SQLITE_OK) {
    return transactionError(QStringLiteral("SQLite transaction begin failed (%1)"), result);
  }
  return SqliteTransaction(connection);
}

std::optional<AppError> SqliteTransaction::commit() {
  return finish("COMMIT", QStringLiteral("SQLite transaction commit failed (%1)"));
}

std::optional<AppError> SqliteTransaction::rollback() {
  if (!active_) {
    return std::nullopt;
  }
  return finish("ROLLBACK", QStringLiteral("SQLite transaction rollback failed (%1)"));
}

bool SqliteTransaction::active() const noexcept { return active_; }

std::optional<AppError> SqliteTransaction::finish(const char* sql, QString errorMessage) {
  if (!active_) {
    return AppError(AppErrorCode::Database, QStringLiteral("SQLite transaction is not active"));
  }
  if (connection_ == nullptr || connection_->nativeHandle() == nullptr) {
    active_ = false;
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite transaction connection is unavailable"));
  }

  sqlite3* const handle = connection_->nativeHandle();
  const int result = execute(handle, sql);
  active_ = sqlite3_get_autocommit(handle) == 0;
  if (result != SQLITE_OK) {
    return transactionError(std::move(errorMessage), result);
  }
  return std::nullopt;
}

} // namespace hcb
