#include "core/SettingsService.h"

#include "data/LocalSchema.h"
#include "sqlite3.h"

#include <QByteArray>
#include <QDateTime>
#include <QString>
#include <QTimeZone>

#include <chrono>
#include <future>
#include <limits>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumSettingIdentifierLength = 128;
constexpr qsizetype kMaximumSettingJsonBytes = 262'144;

using JsonValidityResult = std::variant<bool, AppError>;

[[nodiscard]] AppError databaseError(const QString& message, int result) {
  return AppError(AppErrorCode::Database, message.arg(result));
}

[[nodiscard]] bool isValidIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() &&
         value.size() <= kMaximumSettingIdentifierLength && !value.contains(QChar::Null);
}

[[nodiscard]] QString timestamp(const Clock& clock) {
  const auto milliseconds =
      std::chrono::duration_cast<std::chrono::milliseconds>(clock.wallNow().time_since_epoch());
  return QDateTime::fromMSecsSinceEpoch(milliseconds.count(), QTimeZone::UTC)
      .toString(Qt::ISODateWithMs);
}

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

[[nodiscard]] std::optional<AppError>
bindText(sqlite3_stmt* statement, int index, const QByteArray& value) {
  const int result = sqlite3_bind_text(
      statement, index, value.constData(), static_cast<int>(value.size()), SQLITE_TRANSIENT);
  if (result != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite settings binding failed (%1)"), result);
  }
  return std::nullopt;
}

[[nodiscard]] JsonValidityResult isValidJson(sqlite3* handle, const QByteArray& valueJson) {
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, "SELECT json_valid(?1)", -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite settings JSON validation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, valueJson); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const bool valid = stepResult == SQLITE_ROW && sqlite3_column_int(statement, 0) != 0;
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_ROW) {
    return databaseError(QStringLiteral("SQLite settings JSON validation failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite settings JSON validation finalization failed (%1)"),
                         finalizeResult);
  }
  return valid;
}

[[nodiscard]] SettingsJsonReadResult
readStoredJson(SqliteConnection& connection, const QString& scope, const QString& key) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite settings connection is unavailable"));
  }
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(handle,
                                               "SELECT value_json FROM local_settings "
                                               "WHERE scope = ?1 AND key = ?2",
                                               -1,
                                               SQLITE_PREPARE_PERSISTENT,
                                               &statement,
                                               nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite settings read preparation failed (%1)"),
                         prepareResult);
  }
  const QByteArray scopeUtf8 = scope.toUtf8();
  const QByteArray keyUtf8 = key.toUtf8();
  if (const std::optional<AppError> error = bindText(statement, 1, scopeUtf8); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  if (const std::optional<AppError> error = bindText(statement, 2, keyUtf8); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  std::optional<QString> value;
  if (stepResult == SQLITE_ROW && sqlite3_column_type(statement, 0) == SQLITE_TEXT) {
    const auto* text = reinterpret_cast<const char*>(sqlite3_column_text(statement, 0));
    const int textSize = sqlite3_column_bytes(statement, 0);
    if (text == nullptr) {
      sqlite3_finalize(statement);
      return AppError(AppErrorCode::Database, QStringLiteral("SQLite settings value is invalid"));
    }
    value = QString::fromUtf8(text, textSize);
  }
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_ROW && stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite settings read failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite settings read finalization failed (%1)"),
                         finalizeResult);
  }
  return value;
}

[[nodiscard]] SettingsMutationResultOrError writeStoredJson(SqliteConnection& connection,
                                                            const QString& scope,
                                                            const QString& key,
                                                            const QString& valueJson,
                                                            const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite settings connection is unavailable"));
  }
  const QByteArray valueUtf8 = valueJson.toUtf8();
  const JsonValidityResult validity = isValidJson(handle, valueUtf8);
  if (std::holds_alternative<AppError>(validity)) {
    return std::get<AppError>(validity);
  }
  if (!std::get<bool>(validity)) {
    return AppError(AppErrorCode::Validation, QStringLiteral("Settings JSON value is invalid"));
  }

  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle,
                         "INSERT INTO local_settings (scope, key, value_json, updated_at) "
                         "VALUES (?1, ?2, ?3, ?4) "
                         "ON CONFLICT(scope, key) DO UPDATE SET value_json = excluded.value_json, "
                         "updated_at = excluded.updated_at "
                         "WHERE local_settings.value_json IS NOT excluded.value_json",
                         -1,
                         SQLITE_PREPARE_PERSISTENT,
                         &statement,
                         nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite settings write preparation failed (%1)"),
                         prepareResult);
  }
  const QByteArray scopeUtf8 = scope.toUtf8();
  const QByteArray keyUtf8 = key.toUtf8();
  const QByteArray timestampUtf8 = updatedAt.toUtf8();
  for (const auto& [index, value] : {std::pair{1, &scopeUtf8},
                                     std::pair{2, &keyUtf8},
                                     std::pair{3, &valueUtf8},
                                     std::pair{4, &timestampUtf8}}) {
    if (const std::optional<AppError> error = bindText(statement, index, *value);
        error.has_value()) {
      sqlite3_finalize(statement);
      return *error;
    }
  }
  const int stepResult = sqlite3_step(statement);
  const int changes = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite settings write failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite settings write finalization failed (%1)"),
                         finalizeResult);
  }
  return changes == 0 ? SettingsMutationResult::Unchanged : SettingsMutationResult::Changed;
}

[[nodiscard]] SettingsMutationResultOrError
eraseStoredJson(SqliteConnection& connection, const QString& scope, const QString& key) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite settings connection is unavailable"));
  }
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle,
                         "DELETE FROM local_settings WHERE scope = ?1 AND key = ?2",
                         -1,
                         SQLITE_PREPARE_PERSISTENT,
                         &statement,
                         nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite settings delete preparation failed (%1)"),
                         prepareResult);
  }
  const QByteArray scopeUtf8 = scope.toUtf8();
  const QByteArray keyUtf8 = key.toUtf8();
  if (const std::optional<AppError> error = bindText(statement, 1, scopeUtf8); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  if (const std::optional<AppError> error = bindText(statement, 2, keyUtf8); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changes = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite settings delete failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite settings delete finalization failed (%1)"),
                         finalizeResult);
  }
  return changes == 0 ? SettingsMutationResult::Unchanged : SettingsMutationResult::Changed;
}

} // namespace

SettingsService::SettingsService(FilePath databasePath, const Clock& clock)
    : clock_(clock), writerQueue_(std::move(databasePath)),
      initialization_(writerQueue_
                          .enqueue([](SqliteConnection& connection) -> SqliteWriteResult {
                            const SqliteMigrationRunResultOrError result =
                                LocalSchema::initialize(connection);
                            return std::holds_alternative<AppError>(result)
                                       ? std::optional<AppError>(std::get<AppError>(result))
                                       : std::nullopt;
                          })
                          .share()) {}

std::shared_future<SqliteWriteResult> SettingsService::ready() const { return initialization_; }

std::future<SettingsJsonReadResult> SettingsService::readJson(QString scope, QString key) {
  if (!isValidIdentifier(scope) || !isValidIdentifier(key)) {
    return readyFuture(SettingsJsonReadResult(
        AppError(AppErrorCode::Validation, QStringLiteral("Settings scope or key is invalid"))));
  }
  return writerQueue_.enqueueResult(
      [scope = std::move(scope), key = std::move(key)](SqliteConnection& connection) {
        return readStoredJson(connection, scope, key);
      });
}

std::future<SettingsMutationResultOrError>
SettingsService::writeJson(QString scope, QString key, QString valueJson) {
  if (!isValidIdentifier(scope) || !isValidIdentifier(key) || valueJson.isEmpty() ||
      valueJson.toUtf8().size() > kMaximumSettingJsonBytes) {
    return readyFuture(SettingsMutationResultOrError(
        AppError(AppErrorCode::Validation, QStringLiteral("Settings input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [scope = std::move(scope), key = std::move(key), valueJson = std::move(valueJson), updatedAt](
          SqliteConnection& connection) {
        return writeStoredJson(connection, scope, key, valueJson, updatedAt);
      });
}

std::future<SettingsMutationResultOrError> SettingsService::erase(QString scope, QString key) {
  if (!isValidIdentifier(scope) || !isValidIdentifier(key)) {
    return readyFuture(SettingsMutationResultOrError(
        AppError(AppErrorCode::Validation, QStringLiteral("Settings scope or key is invalid"))));
  }
  return writerQueue_.enqueueResult(
      [scope = std::move(scope), key = std::move(key)](SqliteConnection& connection) {
        return eraseStoredJson(connection, scope, key);
      });
}

} // namespace hcb
