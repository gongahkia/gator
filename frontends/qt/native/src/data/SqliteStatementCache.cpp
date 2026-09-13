#include "data/SqliteStatementCache.h"

#include "sqlite3.h"

#include <QByteArray>
#include <QChar>

#include <cctype>
#include <cstdint>
#include <limits>
#include <mutex>
#include <unordered_map>
#include <utility>

namespace {

[[nodiscard]] hcb::AppError statementError(QString message, int result) {
  return hcb::AppError(hcb::AppErrorCode::Database, std::move(message).arg(result));
}

[[nodiscard]] bool hasOnlyWhitespace(const char* sql) {
  while (*sql != '\0') {
    if (std::isspace(static_cast<unsigned char>(*sql)) == 0) {
      return false;
    }
    ++sql;
  }
  return true;
}

} // namespace

struct SqliteStatementCacheState final {
  struct Entry final {
    sqlite3_stmt* statement{nullptr};
    bool leased{false};
    std::uint64_t lastUse{0};
  };

  struct Acquisition final {
    sqlite3_stmt* statement{nullptr};
    bool cached{false};
  };

  using AcquisitionResult = std::variant<Acquisition, hcb::AppError>;

  SqliteStatementCacheState(sqlite3* connection, std::size_t capacity)
      : connection_(connection), capacity_(capacity) {}

  ~SqliteStatementCacheState() {
    for (auto& [sql, entry] : entries_) {
      sqlite3_finalize(entry.statement);
    }
  }

  [[nodiscard]] AcquisitionResult acquire(std::string sql) {
    std::lock_guard lock(mutex_);
    if (!accepting_) {
      return hcb::AppError(hcb::AppErrorCode::Database,
                           QStringLiteral("SQLite statement cache is closed"));
    }
    if (connection_ == nullptr) {
      return hcb::AppError(hcb::AppErrorCode::Database,
                           QStringLiteral("SQLite statement connection is unavailable"));
    }

    const auto existing = entries_.find(sql);
    if (existing != entries_.end() && !existing->second.leased) {
      existing->second.leased = true;
      existing->second.lastUse = nextUse();
      return Acquisition{existing->second.statement, true};
    }

    sqlite3_stmt* statement = nullptr;
    const char* tail = nullptr;
    const int prepareResult = sqlite3_prepare_v3(connection_,
                                                 sql.c_str(),
                                                 static_cast<int>(sql.size() + 1),
                                                 SQLITE_PREPARE_PERSISTENT,
                                                 &statement,
                                                 &tail);
    if (prepareResult != SQLITE_OK) {
      sqlite3_finalize(statement);
      return statementError(QStringLiteral("SQLite statement preparation failed (%1)"),
                            prepareResult);
    }
    if (statement == nullptr || tail == nullptr || !hasOnlyWhitespace(tail)) {
      sqlite3_finalize(statement);
      return hcb::AppError(hcb::AppErrorCode::Database,
                           QStringLiteral("SQLite statement must contain exactly one command"));
    }

    if (capacity_ == 0 || existing != entries_.end()) {
      return Acquisition{statement, false};
    }
    if (entries_.size() == capacity_) {
      const auto leastRecentlyUsed = findLeastRecentlyUsedIdleEntry();
      if (leastRecentlyUsed == entries_.end()) {
        return Acquisition{statement, false};
      }
      const int finalizeResult = sqlite3_finalize(leastRecentlyUsed->second.statement);
      entries_.erase(leastRecentlyUsed);
      if (finalizeResult != SQLITE_OK) {
        sqlite3_finalize(statement);
        return statementError(QStringLiteral("SQLite cached statement finalization failed (%1)"),
                              finalizeResult);
      }
    }

    try {
      entries_.emplace(std::move(sql), Entry{statement, true, nextUse()});
    } catch (...) {
      sqlite3_finalize(statement);
      return hcb::AppError(hcb::AppErrorCode::Database,
                           QStringLiteral("SQLite statement cache allocation failed"));
    }
    return Acquisition{statement, true};
  }

  [[nodiscard]] std::optional<hcb::AppError>
  release(const std::string& cacheKey, sqlite3_stmt* statement, bool cached) {
    std::lock_guard lock(mutex_);
    const int resetResult = sqlite3_reset(statement);
    const int clearResult = sqlite3_clear_bindings(statement);

    if (!cached) {
      const int finalizeResult = sqlite3_finalize(statement);
      return releaseError(resetResult, clearResult, finalizeResult);
    }

    const auto entry = entries_.find(cacheKey);
    if (entry == entries_.end() || entry->second.statement != statement) {
      const int finalizeResult = sqlite3_finalize(statement);
      return releaseError(resetResult, clearResult, finalizeResult);
    }
    if (!accepting_ || resetResult != SQLITE_OK || clearResult != SQLITE_OK) {
      const int finalizeResult = sqlite3_finalize(statement);
      entries_.erase(entry);
      return releaseError(resetResult, clearResult, finalizeResult);
    }
    entry->second.leased = false;
    entry->second.lastUse = nextUse();
    return std::nullopt;
  }

  void close() {
    std::lock_guard lock(mutex_);
    accepting_ = false;
    for (auto entry = entries_.begin(); entry != entries_.end();) {
      if (entry->second.leased) {
        ++entry;
        continue;
      }
      sqlite3_finalize(entry->second.statement);
      entry = entries_.erase(entry);
    }
  }

private:
  [[nodiscard]] std::unordered_map<std::string, Entry>::iterator findLeastRecentlyUsedIdleEntry() {
    auto leastRecentlyUsed = entries_.end();
    for (auto entry = entries_.begin(); entry != entries_.end(); ++entry) {
      if (entry->second.leased || (leastRecentlyUsed != entries_.end() &&
                                   leastRecentlyUsed->second.lastUse <= entry->second.lastUse)) {
        continue;
      }
      leastRecentlyUsed = entry;
    }
    return leastRecentlyUsed;
  }

  [[nodiscard]] std::optional<hcb::AppError>
  releaseError(int resetResult, int clearResult, int finalizeResult) const {
    if (resetResult != SQLITE_OK) {
      return statementError(QStringLiteral("SQLite statement reset failed (%1)"), resetResult);
    }
    if (clearResult != SQLITE_OK) {
      return statementError(QStringLiteral("SQLite statement binding reset failed (%1)"),
                            clearResult);
    }
    if (finalizeResult != SQLITE_OK) {
      return statementError(QStringLiteral("SQLite statement finalization failed (%1)"),
                            finalizeResult);
    }
    return std::nullopt;
  }

  [[nodiscard]] std::uint64_t nextUse() noexcept { return ++useCounter_; }

  std::mutex mutex_;
  sqlite3* connection_{nullptr};
  std::size_t capacity_{0};
  bool accepting_{true};
  std::uint64_t useCounter_{0};
  std::unordered_map<std::string, Entry> entries_;
};

namespace hcb {

SqlitePreparedStatement::SqlitePreparedStatement(std::shared_ptr<SqliteStatementCacheState> state,
                                                 sqlite3_stmt* statement,
                                                 std::string cacheKey,
                                                 bool cached) noexcept
    : state_(std::move(state)), statement_(statement), cacheKey_(std::move(cacheKey)),
      cached_(cached) {}

SqlitePreparedStatement::SqlitePreparedStatement(SqlitePreparedStatement&& other) noexcept
    : state_(std::move(other.state_)), statement_(std::exchange(other.statement_, nullptr)),
      cacheKey_(std::move(other.cacheKey_)), cached_(std::exchange(other.cached_, false)) {}

SqlitePreparedStatement::~SqlitePreparedStatement() { static_cast<void>(release()); }

sqlite3_stmt* SqlitePreparedStatement::nativeHandle() const noexcept { return statement_; }

std::optional<AppError> SqlitePreparedStatement::release() {
  if (statement_ == nullptr) {
    return std::nullopt;
  }
  const std::shared_ptr<SqliteStatementCacheState> state = std::move(state_);
  sqlite3_stmt* const statement = std::exchange(statement_, nullptr);
  const std::string cacheKey = std::move(cacheKey_);
  const bool cached = std::exchange(cached_, false);
  if (!state) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite statement cache is unavailable"));
  }
  return state->release(cacheKey, statement, cached);
}

SqliteStatementCache::SqliteStatementCache(SqliteConnection& connection, std::size_t capacity)
    : state_(std::make_shared<SqliteStatementCacheState>(connection.nativeHandle(), capacity)) {}

SqliteStatementCache::~SqliteStatementCache() { state_->close(); }

SqlitePreparedStatementResult SqliteStatementCache::acquire(const QString& sql) {
  if (sql.isEmpty() || sql.contains(QChar::Null)) {
    return AppError(AppErrorCode::Database, QStringLiteral("SQLite statement SQL is invalid"));
  }
  const QByteArray utf8Sql = sql.toUtf8();
  if (utf8Sql.size() > std::numeric_limits<int>::max() - 1) {
    return AppError(AppErrorCode::Database, QStringLiteral("SQLite statement SQL is too large"));
  }
  std::string cacheKey(utf8Sql.constData(), static_cast<std::size_t>(utf8Sql.size()));
  SqliteStatementCacheState::AcquisitionResult acquisition = state_->acquire(cacheKey);
  if (std::holds_alternative<AppError>(acquisition)) {
    return std::get<AppError>(std::move(acquisition));
  }
  SqliteStatementCacheState::Acquisition acquired =
      std::get<SqliteStatementCacheState::Acquisition>(std::move(acquisition));
  return SqlitePreparedStatement(state_, acquired.statement, std::move(cacheKey), acquired.cached);
}

} // namespace hcb
