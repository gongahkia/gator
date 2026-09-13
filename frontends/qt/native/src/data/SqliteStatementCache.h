#pragma once

#include "core/AppError.h"
#include "data/SqliteConnection.h"

#include <cstddef>
#include <memory>
#include <optional>
#include <string>
#include <variant>

struct sqlite3_stmt;
struct SqliteStatementCacheState;

namespace hcb {

class SqlitePreparedStatement final {
public:
  SqlitePreparedStatement(const SqlitePreparedStatement&) = delete;
  SqlitePreparedStatement& operator=(const SqlitePreparedStatement&) = delete;
  SqlitePreparedStatement(SqlitePreparedStatement&& other) noexcept;
  SqlitePreparedStatement& operator=(SqlitePreparedStatement&&) = delete;
  ~SqlitePreparedStatement();

  [[nodiscard]] sqlite3_stmt* nativeHandle() const noexcept;
  [[nodiscard]] std::optional<AppError> release();

private:
  SqlitePreparedStatement(std::shared_ptr<SqliteStatementCacheState> state,
                          sqlite3_stmt* statement,
                          std::string cacheKey,
                          bool cached) noexcept;

  std::shared_ptr<SqliteStatementCacheState> state_;
  sqlite3_stmt* statement_{nullptr};
  std::string cacheKey_;
  bool cached_{false};

  friend class SqliteStatementCache;
};

using SqlitePreparedStatementResult = std::variant<SqlitePreparedStatement, AppError>;

class SqliteStatementCache final {
public:
  SqliteStatementCache(SqliteConnection& connection, std::size_t capacity);
  SqliteStatementCache(const SqliteStatementCache&) = delete;
  SqliteStatementCache& operator=(const SqliteStatementCache&) = delete;
  ~SqliteStatementCache();

  [[nodiscard]] SqlitePreparedStatementResult acquire(const QString& sql);

private:
  std::shared_ptr<SqliteStatementCacheState> state_;
};

} // namespace hcb
