#include "data/SqliteConnection.h"

#include "data/SqliteQueryTimingTracker.h"
#include "sqlite3.h"

#include <QByteArray>
#include <QChar>
#include <QString>

#include <optional>
#include <chrono>
#include <thread>
#include <utility>

namespace hcb {
namespace {

[[nodiscard]] int flagsFor(SqliteOpenMode mode) noexcept {
  constexpr int sharedFlags =
      SQLITE_OPEN_FULLMUTEX | SQLITE_OPEN_PRIVATECACHE | SQLITE_OPEN_NOFOLLOW;
  switch (mode) {
  case SqliteOpenMode::ReadOnly:
    return SQLITE_OPEN_READONLY | sharedFlags;
  case SqliteOpenMode::ReadWriteCreate:
    return SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE | sharedFlags;
  }
  return SQLITE_OPEN_READONLY | sharedFlags;
}

[[nodiscard]] AppError configurationError(int result) {
  return AppError(AppErrorCode::Database,
                  QStringLiteral("SQLite connection configuration failed (%1)").arg(result));
}

[[nodiscard]] AppError queryTimingError(int result) {
  return AppError(AppErrorCode::Database,
                  QStringLiteral("SQLite query timing setup failed (%1)").arg(result));
}

[[nodiscard]] std::optional<AppError> executePragma(sqlite3* handle, const char* sql) {
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  sqlite3_free(errorMessage);
  if (result != SQLITE_OK) {
    return configurationError(result);
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<AppError> setWalJournalMode(sqlite3* handle) {
  constexpr int kAttempts = 50;
  for (int attempt = 0; attempt < kAttempts; ++attempt) {
    sqlite3_stmt* statement = nullptr;
    const int prepareResult =
        sqlite3_prepare_v2(handle, "PRAGMA journal_mode = WAL", -1, &statement, nullptr);
    if (prepareResult != SQLITE_OK) {
      sqlite3_finalize(statement);
      if (prepareResult != SQLITE_BUSY && prepareResult != SQLITE_LOCKED) {
        return configurationError(prepareResult);
      }
    } else {
      const int stepResult = sqlite3_step(statement);
      const auto* journalMode = stepResult == SQLITE_ROW
                                    ? reinterpret_cast<const char*>(sqlite3_column_text(statement, 0))
                                    : nullptr;
      const bool isWal = journalMode != nullptr && QString::fromUtf8(journalMode) == u"wal";
      const int finalizeResult = sqlite3_finalize(statement);
      if (finalizeResult != SQLITE_OK && finalizeResult != SQLITE_BUSY &&
          finalizeResult != SQLITE_LOCKED) {
        return configurationError(finalizeResult);
      }
      if (stepResult == SQLITE_ROW && finalizeResult == SQLITE_OK) {
        return isWal ? std::optional<AppError>{}
                     : std::optional<AppError>(AppError(
                           AppErrorCode::Database, QStringLiteral("SQLite journal mode is not WAL")));
      }
      if (stepResult != SQLITE_BUSY && stepResult != SQLITE_LOCKED && finalizeResult == SQLITE_OK) {
        return configurationError(stepResult);
      }
    }
    std::this_thread::sleep_for(std::chrono::milliseconds(10));
  }
  return configurationError(SQLITE_BUSY);
}

[[nodiscard]] std::optional<AppError> applyProductionPragmas(sqlite3* handle, SqliteOpenMode mode) {
  const int busyTimeoutResult = sqlite3_busy_timeout(handle, 30'000);
  if (busyTimeoutResult != SQLITE_OK) {
    return configurationError(busyTimeoutResult);
  }
  if (mode == SqliteOpenMode::ReadWriteCreate) {
    if (const std::optional<AppError> error = setWalJournalMode(handle); error.has_value()) {
      return error;
    }
  }
  for (const char* pragma : {"PRAGMA foreign_keys = ON",
                             "PRAGMA synchronous = FULL",
                             "PRAGMA temp_store = MEMORY",
                             "PRAGMA cache_size = -65536",
                             "PRAGMA mmap_size = 268435456"}) {
    if (const std::optional<AppError> error = executePragma(handle, pragma); error.has_value()) {
      return error;
    }
  }
  return std::nullopt;
}

} // namespace

SqliteConnection::SqliteConnection(sqlite3* handle) noexcept : handle_(handle) {}

SqliteConnection::SqliteConnection(SqliteConnection&& other) noexcept
    : handle_(std::exchange(other.handle_, nullptr)),
      queryTimingTracker_(std::move(other.queryTimingTracker_)) {}

SqliteConnection& SqliteConnection::operator=(SqliteConnection&& other) noexcept {
  if (this != &other) {
    clearQueryTimingTracker();
    sqlite3_close_v2(handle_);
    handle_ = std::exchange(other.handle_, nullptr);
    queryTimingTracker_ = std::move(other.queryTimingTracker_);
  }
  return *this;
}

SqliteConnection::~SqliteConnection() {
  clearQueryTimingTracker();
  sqlite3_close_v2(handle_);
}

sqlite3* SqliteConnection::nativeHandle() const noexcept { return handle_; }

std::optional<AppError>
SqliteConnection::installQueryTimingTracker(std::shared_ptr<SqliteQueryTimingTracker> tracker) {
  if (handle_ == nullptr || !tracker) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite query timing tracker is unavailable"));
  }
  const int result = sqlite3_trace_v2(
      handle_, SQLITE_TRACE_PROFILE, &SqliteQueryTimingTracker::profileCallback, tracker.get());
  if (result != SQLITE_OK) {
    return queryTimingError(result);
  }
  queryTimingTracker_ = std::move(tracker);
  return std::nullopt;
}

void SqliteConnection::clearQueryTimingTracker() noexcept {
  if (!queryTimingTracker_) {
    return;
  }
  if (handle_ != nullptr) {
    sqlite3_trace_v2(handle_, 0, nullptr, nullptr);
  }
  queryTimingTracker_.reset();
}

SqliteConnectionResult SqliteConnectionFactory::open(const FilePath& databasePath,
                                                     SqliteOpenMode mode) {
  const QString& path = databasePath.nativePath();
  if (path.isEmpty() || path.contains(QChar::Null)) {
    return AppError(AppErrorCode::Database, QStringLiteral("SQLite database path is invalid"));
  }

  const QByteArray utf8Path = path.toUtf8();
  sqlite3* handle = nullptr;
  const int result = sqlite3_open_v2(utf8Path.constData(), &handle, flagsFor(mode), nullptr);
  if (result != SQLITE_OK) {
    sqlite3_close_v2(handle);
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite database open failed (%1)").arg(result));
  }
  SqliteConnection connection(handle);
  if (const std::optional<AppError> error = applyProductionPragmas(handle, mode);
      error.has_value()) {
    return *error;
  }
  return connection;
}

} // namespace hcb
