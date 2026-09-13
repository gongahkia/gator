#pragma once

#include "core/AppError.h"
#include "core/FilePath.h"

#include <memory>
#include <optional>
#include <variant>

struct sqlite3;

namespace hcb {

class SqliteQueryTimingTracker;

enum class SqliteOpenMode {
  ReadOnly,
  ReadWriteCreate
};

class SqliteConnection final {
public:
  SqliteConnection(const SqliteConnection&) = delete;
  SqliteConnection& operator=(const SqliteConnection&) = delete;
  SqliteConnection(SqliteConnection&& other) noexcept;
  SqliteConnection& operator=(SqliteConnection&& other) noexcept;
  ~SqliteConnection();

  [[nodiscard]] sqlite3* nativeHandle() const noexcept;
  [[nodiscard]] std::optional<AppError>
  installQueryTimingTracker(std::shared_ptr<SqliteQueryTimingTracker> tracker);
  void clearQueryTimingTracker() noexcept;

private:
  explicit SqliteConnection(sqlite3* handle) noexcept;

  sqlite3* handle_{nullptr};
  std::shared_ptr<SqliteQueryTimingTracker> queryTimingTracker_;

  friend class SqliteConnectionFactory;
};

using SqliteConnectionResult = std::variant<SqliteConnection, AppError>;

class SqliteConnectionFactory final {
public:
  [[nodiscard]] static SqliteConnectionResult open(const FilePath& databasePath,
                                                   SqliteOpenMode mode);
};

} // namespace hcb
