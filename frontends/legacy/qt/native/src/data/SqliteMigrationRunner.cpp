#include "data/SqliteMigrationRunner.h"

#include "data/SqliteTransaction.h"
#include "sqlite3.h"

#include <QByteArray>
#include <QChar>

#include <algorithm>
#include <exception>
#include <limits>
#include <utility>

namespace hcb {
namespace {

constexpr const char* schemaMigrationsSql = R"(
CREATE TABLE IF NOT EXISTS local_schema_migrations (
  version INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TEXT NOT NULL,
  checksum TEXT NOT NULL
)
)";

[[nodiscard]] AppError migrationError(const QString& message, int result) {
  return AppError(AppErrorCode::Database, message.arg(result));
}

[[nodiscard]] std::optional<AppError>
execute(sqlite3* handle, const char* sql, const QString& message) {
  char* sqliteErrorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &sqliteErrorMessage);
  sqlite3_free(sqliteErrorMessage);
  if (result != SQLITE_OK) {
    return migrationError(message, result);
  }
  return std::nullopt;
}

[[nodiscard]] bool isChecksum(const QString& checksum) {
  if (checksum.size() != 64) {
    return false;
  }
  return std::all_of(checksum.cbegin(), checksum.cend(), [](QChar character) {
    return (character >= QLatin1Char('0') && character <= QLatin1Char('9')) ||
           (character >= QLatin1Char('a') && character <= QLatin1Char('f'));
  });
}

[[nodiscard]] std::optional<AppError>
validateMigrations(std::span<const SqliteMigration> migrations) {
  int previousVersion = 0;
  for (const SqliteMigration& migration : migrations) {
    if (migration.version <= previousVersion || migration.name.trimmed().isEmpty() ||
        migration.name.contains(QChar::Null) || !isChecksum(migration.checksum) ||
        (migration.acceptedLegacyChecksum.has_value() &&
         !isChecksum(*migration.acceptedLegacyChecksum)) ||
        !migration.apply) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("SQLite migration catalogue is invalid"));
    }
    previousVersion = migration.version;
  }
  return std::nullopt;
}

struct AppliedMigration final {
  int version{0};
  QString name;
  std::optional<QString> checksum;
};

using AppliedMigrationResult = std::variant<std::vector<AppliedMigration>, AppError>;

using ChecksumColumnResult = std::variant<bool, AppError>;

[[nodiscard]] ChecksumColumnResult hasChecksumColumn(sqlite3* handle) {
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(handle,
                                               "PRAGMA table_info(local_schema_migrations)",
                                               -1,
                                               SQLITE_PREPARE_PERSISTENT,
                                               &statement,
                                               nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return migrationError(QStringLiteral("SQLite migration schema read failed (%1)"),
                          prepareResult);
  }

  bool found = false;
  int stepResult = SQLITE_OK;
  while ((stepResult = sqlite3_step(statement)) == SQLITE_ROW) {
    if (sqlite3_column_type(statement, 1) != SQLITE_TEXT) {
      continue;
    }
    const char* const name = reinterpret_cast<const char*>(sqlite3_column_text(statement, 1));
    const int nameSize = sqlite3_column_bytes(statement, 1);
    if (name != nullptr && QString::fromUtf8(name, nameSize) == QStringLiteral("checksum")) {
      found = true;
      break;
    }
  }
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE && stepResult != SQLITE_ROW) {
    return migrationError(QStringLiteral("SQLite migration schema read failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return migrationError(QStringLiteral("SQLite migration schema finalization failed (%1)"),
                          finalizeResult);
  }
  return found;
}

[[nodiscard]] AppliedMigrationResult readAppliedMigrations(sqlite3* handle) {
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle,
      "SELECT version, name, checksum FROM local_schema_migrations ORDER BY version",
      -1,
      SQLITE_PREPARE_PERSISTENT,
      &statement,
      nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return migrationError(QStringLiteral("SQLite migration history read failed (%1)"),
                          prepareResult);
  }

  std::vector<AppliedMigration> applied;
  int stepResult = SQLITE_OK;
  while ((stepResult = sqlite3_step(statement)) == SQLITE_ROW) {
    const sqlite3_int64 version = sqlite3_column_int64(statement, 0);
    if (version < std::numeric_limits<int>::min() || version > std::numeric_limits<int>::max()) {
      sqlite3_finalize(statement);
      return AppError(AppErrorCode::Database,
                      QStringLiteral("SQLite migration history version is invalid"));
    }
    if (sqlite3_column_type(statement, 1) != SQLITE_TEXT) {
      sqlite3_finalize(statement);
      return AppError(AppErrorCode::Database,
                      QStringLiteral("SQLite migration history name is invalid"));
    }
    const char* const name = reinterpret_cast<const char*>(sqlite3_column_text(statement, 1));
    const int nameSize = sqlite3_column_bytes(statement, 1);
    if (name == nullptr) {
      sqlite3_finalize(statement);
      return AppError(AppErrorCode::Database,
                      QStringLiteral("SQLite migration history name is invalid"));
    }

    std::optional<QString> checksum;
    const int checksumType = sqlite3_column_type(statement, 2);
    if (checksumType == SQLITE_TEXT) {
      const char* const checksumText =
          reinterpret_cast<const char*>(sqlite3_column_text(statement, 2));
      const int checksumSize = sqlite3_column_bytes(statement, 2);
      if (checksumText == nullptr) {
        sqlite3_finalize(statement);
        return AppError(AppErrorCode::Database,
                        QStringLiteral("SQLite migration history checksum is invalid"));
      }
      checksum = QString::fromUtf8(checksumText, checksumSize);
    } else if (checksumType != SQLITE_NULL) {
      sqlite3_finalize(statement);
      return AppError(AppErrorCode::Database,
                      QStringLiteral("SQLite migration history checksum is invalid"));
    }
    applied.push_back(
        {static_cast<int>(version), QString::fromUtf8(name, nameSize), std::move(checksum)});
  }
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return migrationError(QStringLiteral("SQLite migration history read failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return migrationError(QStringLiteral("SQLite migration history finalization failed (%1)"),
                          finalizeResult);
  }
  return applied;
}

[[nodiscard]] std::optional<AppError>
validateAppliedHistory(const std::vector<AppliedMigration>& applied,
                       std::span<const SqliteMigration> migrations) {
  if (applied.size() > migrations.size()) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite migration history is newer than this application"));
  }
  for (size_t index = 0; index < applied.size(); ++index) {
    const AppliedMigration& recorded = applied[index];
    const SqliteMigration& expected = migrations[index];
    if (recorded.version != expected.version) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("SQLite migration history is discontinuous"));
    }
    if (recorded.name != expected.name) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("SQLite migration history name does not match"));
    }
    const bool checksumMatches =
        recorded.checksum.has_value() &&
        (*recorded.checksum == expected.checksum ||
         (expected.acceptedLegacyChecksum.has_value() &&
          *recorded.checksum == *expected.acceptedLegacyChecksum));
    if (recorded.checksum.has_value() &&
        (!isChecksum(*recorded.checksum) || !checksumMatches)) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("SQLite migration history checksum does not match"));
    }
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<AppError>
backfillChecksum(sqlite3* handle, int version, const QString& checksum) {
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(handle,
                                               R"(
UPDATE local_schema_migrations
SET checksum = ?1
WHERE version = ?2 AND checksum IS NULL
)",
                                               -1,
                                               SQLITE_PREPARE_PERSISTENT,
                                               &statement,
                                               nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return migrationError(QStringLiteral("SQLite migration checksum preparation failed (%1)"),
                          prepareResult);
  }

  const QByteArray checksumText = checksum.toUtf8();
  const int bindChecksumResult = sqlite3_bind_text(statement,
                                                   1,
                                                   checksumText.constData(),
                                                   static_cast<int>(checksumText.size()),
                                                   SQLITE_TRANSIENT);
  const int bindVersionResult = sqlite3_bind_int(statement, 2, version);
  const int stepResult = bindChecksumResult == SQLITE_OK && bindVersionResult == SQLITE_OK
                             ? sqlite3_step(statement)
                             : SQLITE_ERROR;
  const int changes = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (bindChecksumResult != SQLITE_OK) {
    return migrationError(QStringLiteral("SQLite migration checksum binding failed (%1)"),
                          bindChecksumResult);
  }
  if (bindVersionResult != SQLITE_OK) {
    return migrationError(QStringLiteral("SQLite migration version binding failed (%1)"),
                          bindVersionResult);
  }
  if (stepResult != SQLITE_DONE) {
    return migrationError(QStringLiteral("SQLite migration checksum write failed (%1)"),
                          stepResult);
  }
  if (changes != 1) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite migration checksum backfill failed"));
  }
  if (finalizeResult != SQLITE_OK) {
    return migrationError(QStringLiteral("SQLite migration checksum finalization failed (%1)"),
                          finalizeResult);
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<AppError> recordMigration(sqlite3* handle,
                                                      const SqliteMigration& migration) {
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(handle,
                                               R"(
INSERT INTO local_schema_migrations (version, name, applied_at, checksum)
VALUES (?1, ?2, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), ?3)
)",
                                               -1,
                                               SQLITE_PREPARE_PERSISTENT,
                                               &statement,
                                               nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return migrationError(QStringLiteral("SQLite migration record preparation failed (%1)"),
                          prepareResult);
  }

  const QByteArray name = migration.name.toUtf8();
  const QByteArray checksum = migration.checksum.toUtf8();
  if (name.size() > std::numeric_limits<int>::max() ||
      checksum.size() > std::numeric_limits<int>::max()) {
    sqlite3_finalize(statement);
    return AppError(AppErrorCode::Database, QStringLiteral("SQLite migration name is too large"));
  }
  const int bindVersionResult = sqlite3_bind_int(statement, 1, migration.version);
  const int bindNameResult = sqlite3_bind_text(
      statement, 2, name.constData(), static_cast<int>(name.size()), SQLITE_TRANSIENT);
  const int bindChecksumResult = sqlite3_bind_text(
      statement, 3, checksum.constData(), static_cast<int>(checksum.size()), SQLITE_TRANSIENT);
  const int stepResult = bindVersionResult == SQLITE_OK && bindNameResult == SQLITE_OK &&
                                 bindChecksumResult == SQLITE_OK
                             ? sqlite3_step(statement)
                             : SQLITE_ERROR;
  const int finalizeResult = sqlite3_finalize(statement);
  if (bindVersionResult != SQLITE_OK) {
    return migrationError(QStringLiteral("SQLite migration version binding failed (%1)"),
                          bindVersionResult);
  }
  if (bindNameResult != SQLITE_OK) {
    return migrationError(QStringLiteral("SQLite migration name binding failed (%1)"),
                          bindNameResult);
  }
  if (bindChecksumResult != SQLITE_OK) {
    return migrationError(QStringLiteral("SQLite migration checksum binding failed (%1)"),
                          bindChecksumResult);
  }
  if (stepResult != SQLITE_DONE) {
    return migrationError(QStringLiteral("SQLite migration record write failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return migrationError(QStringLiteral("SQLite migration record finalization failed (%1)"),
                          finalizeResult);
  }
  return std::nullopt;
}

[[nodiscard]] AppliedMigrationResult
prepareMigrationMetadata(SqliteConnection& connection,
                         std::span<const SqliteMigration> migrations) {
  sqlite3* const handle = connection.nativeHandle();
  SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
  if (std::holds_alternative<AppError>(transactionResult)) {
    return std::get<AppError>(std::move(transactionResult));
  }
  SqliteTransaction transaction = std::move(std::get<SqliteTransaction>(transactionResult));
  if (const std::optional<AppError> error = execute(
          handle, schemaMigrationsSql, QStringLiteral("SQLite migration schema setup failed (%1)"));
      error.has_value()) {
    return *error;
  }
  ChecksumColumnResult checksumColumnResult = hasChecksumColumn(handle);
  if (std::holds_alternative<AppError>(checksumColumnResult)) {
    return std::get<AppError>(std::move(checksumColumnResult));
  }
  if (!std::get<bool>(checksumColumnResult)) {
    if (const std::optional<AppError> error =
            execute(handle,
                    "ALTER TABLE local_schema_migrations ADD COLUMN checksum TEXT",
                    QStringLiteral("SQLite migration checksum schema setup failed (%1)"));
        error.has_value()) {
      return *error;
    }
  }

  AppliedMigrationResult appliedResult = readAppliedMigrations(handle);
  if (std::holds_alternative<AppError>(appliedResult)) {
    return std::get<AppError>(std::move(appliedResult));
  }
  const std::vector<AppliedMigration>& applied =
      std::get<std::vector<AppliedMigration>>(appliedResult);
  if (const std::optional<AppError> error = validateAppliedHistory(applied, migrations);
      error.has_value()) {
    return *error;
  }
  for (const AppliedMigration& recorded : applied) {
    if (recorded.checksum.has_value()) {
      continue;
    }
    const auto migration = std::find_if(
        migrations.begin(), migrations.end(), [&recorded](const SqliteMigration& candidate) {
          return candidate.version == recorded.version;
        });
    if (migration == migrations.end()) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("SQLite migration history is newer than this application"));
    }
    if (const std::optional<AppError> error =
            backfillChecksum(handle, recorded.version, migration->checksum);
        error.has_value()) {
      return *error;
    }
  }
  if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
    return *error;
  }
  return applied;
}

} // namespace

SqliteMigrationRunResultOrError
SqliteMigrationRunner::run(SqliteConnection& connection,
                           std::span<const SqliteMigration> migrations) {
  if (const std::optional<AppError> error = validateMigrations(migrations); error.has_value()) {
    return *error;
  }
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite migration connection is unavailable"));
  }
  AppliedMigrationResult appliedResult = prepareMigrationMetadata(connection, migrations);
  if (std::holds_alternative<AppError>(appliedResult)) {
    return std::get<AppError>(std::move(appliedResult));
  }
  const std::vector<AppliedMigration> applied =
      std::get<std::vector<AppliedMigration>>(std::move(appliedResult));

  SqliteMigrationRunResult result;
  result.version = migrations.empty() ? 0 : migrations.back().version;
  for (size_t index = 0; index < migrations.size(); ++index) {
    const SqliteMigration& migration = migrations[index];
    if (index < applied.size()) {
      continue;
    }
    SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
    if (std::holds_alternative<AppError>(transactionResult)) {
      return std::get<AppError>(std::move(transactionResult));
    }
    SqliteTransaction transaction = std::move(std::get<SqliteTransaction>(transactionResult));
    try {
      if (const std::optional<AppError> error = migration.apply(connection); error.has_value()) {
        return *error;
      }
    } catch (const std::exception&) {
      return AppError(AppErrorCode::Database, QStringLiteral("SQLite migration failed"));
    } catch (...) {
      return AppError(AppErrorCode::Database, QStringLiteral("SQLite migration failed"));
    }
    if (const std::optional<AppError> error = recordMigration(handle, migration);
        error.has_value()) {
      return *error;
    }
    if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
      return *error;
    }
    result.appliedVersions.push_back(migration.version);
  }
  return result;
}

} // namespace hcb
