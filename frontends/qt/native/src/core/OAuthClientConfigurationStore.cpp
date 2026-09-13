#include "core/OAuthClientConfigurationStore.h"

#include "data/LocalSchema.h"
#include "sqlite3.h"

#include <QByteArray>
#include <QDateTime>
#include <QJsonDocument>
#include <QJsonObject>
#include <QString>
#include <QTimeZone>

#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMinimumClientIdLength = 10;
constexpr qsizetype kMaximumClientIdLength = 500;
constexpr qsizetype kMaximumClientSecretLength = 8'192;
constexpr qsizetype kMaximumTimestampLength = 64;
constexpr char kSettingsScope[] = "google";
constexpr char kClientIdKey[] = "oauthClientId";

[[nodiscard]] AppError databaseError(const QString& message, int result) {
  return AppError(AppErrorCode::Database, message.arg(result));
}

[[nodiscard]] AppError validationError(QString message) {
  return AppError(AppErrorCode::Validation, std::move(message));
}

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

[[nodiscard]] bool isValidClientId(const QString& clientId) {
  return clientId.size() >= kMinimumClientIdLength && clientId.size() <= kMaximumClientIdLength &&
         !clientId.contains(QChar::Null);
}

[[nodiscard]] bool isValidClientSecret(const QString& clientSecret) {
  return clientSecret.size() <= kMaximumClientSecretLength && !clientSecret.contains(QChar::Null);
}

[[nodiscard]] bool isValidTimestamp(const QString& updatedAt) {
  return !updatedAt.isEmpty() && updatedAt == updatedAt.trimmed() &&
         updatedAt.size() <= kMaximumTimestampLength && !updatedAt.contains(QChar::Null);
}

[[nodiscard]] QString timestamp(const Clock& clock) {
  const auto milliseconds =
      std::chrono::duration_cast<std::chrono::milliseconds>(clock.wallNow().time_since_epoch());
  return QDateTime::fromMSecsSinceEpoch(milliseconds.count(), QTimeZone::UTC)
      .toString(Qt::ISODateWithMs);
}

[[nodiscard]] std::optional<AppError>
bindText(sqlite3_stmt* statement, int index, const QString& value) {
  const QByteArray utf8 = value.toUtf8();
  const int result = sqlite3_bind_text(
      statement, index, utf8.constData(), static_cast<int>(utf8.size()), SQLITE_TRANSIENT);
  return result == SQLITE_OK
             ? std::nullopt
             : std::optional<AppError>(databaseError(
                   QStringLiteral("SQLite OAuth configuration binding failed (%1)"), result));
}

[[nodiscard]] std::optional<QString> textAt(sqlite3_stmt* statement, int index) {
  if (sqlite3_column_type(statement, index) != SQLITE_TEXT) {
    return std::nullopt;
  }
  const auto* value = reinterpret_cast<const char*>(sqlite3_column_text(statement, index));
  const int byteCount = sqlite3_column_bytes(statement, index);
  return value == nullptr || byteCount < 0
             ? std::nullopt
             : std::optional<QString>(QString::fromUtf8(value, byteCount));
}

[[nodiscard]] OAuthClientConfigurationReadResult
readStoredConfiguration(SqliteConnection& connection) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite OAuth configuration connection is unavailable"));
  }
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle,
                         "SELECT json_type(value_json), "
                         "CASE json_type(value_json) "
                         "WHEN 'text' THEN json_extract(value_json, '$') "
                         "WHEN 'object' THEN json_extract(value_json, '$.clientId') END, "
                         "CASE json_type(value_json) "
                         "WHEN 'object' THEN json_extract(value_json, '$.clientSecret') END, updated_at "
                         "FROM local_settings WHERE scope = ?1 AND key = ?2 LIMIT 1",
                         -1,
                         SQLITE_PREPARE_PERSISTENT,
                         &statement,
                         nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite OAuth configuration read preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindText(statement, 1, QString::fromLatin1(kSettingsScope));
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  if (const std::optional<AppError> error =
          bindText(statement, 2, QString::fromLatin1(kClientIdKey));
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult == SQLITE_DONE) {
    const int finalizeResult = sqlite3_finalize(statement);
    return finalizeResult == SQLITE_OK
               ? OAuthClientConfigurationReadResult(std::optional<OAuthClientConfiguration>{})
               : OAuthClientConfigurationReadResult(databaseError(
                     QStringLiteral("SQLite OAuth configuration read finalization failed (%1)"),
                     finalizeResult));
  }
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite OAuth configuration read failed (%1)"), stepResult);
  }
  const std::optional<QString> type = textAt(statement, 0);
  const std::optional<QString> clientId = textAt(statement, 1);
  const std::optional<QString> clientSecret = textAt(statement, 2);
  const std::optional<QString> updatedAt = textAt(statement, 3);
  const int finalizeResult = sqlite3_finalize(statement);
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite OAuth configuration read finalization failed (%1)"),
                         finalizeResult);
  }
  if (!type.has_value() || (*type != QStringLiteral("text") && *type != QStringLiteral("object")) ||
      !clientId.has_value() || !updatedAt.has_value() || !isValidClientId(*clientId) ||
      (clientSecret.has_value() && !isValidClientSecret(*clientSecret)) ||
      !isValidTimestamp(*updatedAt)) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Stored OAuth client configuration is invalid"));
  }
  return std::optional<OAuthClientConfiguration>(
      OAuthClientConfiguration{.clientId = *clientId,
                               .clientSecret = clientSecret.value_or(QString()),
                               .updatedAt = *updatedAt});
}

[[nodiscard]] OAuthClientConfigurationMutationResultOrError saveStoredConfiguration(
    SqliteConnection& connection,
    const QString& clientId,
    const QString& clientSecret,
    const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite OAuth configuration connection is unavailable"));
  }
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle,
                         "INSERT INTO local_settings (scope, key, value_json, updated_at) "
                         "VALUES (?1, ?2, json(?3), ?4) "
                         "ON CONFLICT(scope, key) DO UPDATE SET value_json = excluded.value_json, "
                         "updated_at = excluded.updated_at "
                         "WHERE local_settings.value_json IS NOT excluded.value_json",
                         -1,
                         SQLITE_PREPARE_PERSISTENT,
                         &statement,
                         nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite OAuth configuration save preparation failed (%1)"),
                         prepareResult);
  }
  QJsonObject configuration{{QStringLiteral("clientId"), clientId}};
  if (!clientSecret.isEmpty()) {
    configuration.insert(QStringLiteral("clientSecret"), clientSecret);
  }
  const QString valueJson =
      QString::fromUtf8(QJsonDocument(configuration).toJson(QJsonDocument::Compact));
  for (const auto& [index, value] : {std::pair{1, QString::fromLatin1(kSettingsScope)},
                                     std::pair{2, QString::fromLatin1(kClientIdKey)},
                                     std::pair{3, valueJson},
                                     std::pair{4, updatedAt}}) {
    if (const std::optional<AppError> error = bindText(statement, index, value);
        error.has_value()) {
      sqlite3_finalize(statement);
      return *error;
    }
  }
  const int stepResult = sqlite3_step(statement);
  const int changes = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite OAuth configuration save failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite OAuth configuration save finalization failed (%1)"),
                         finalizeResult);
  }
  return changes == 0 ? OAuthClientConfigurationMutationResult::Unchanged
                      : OAuthClientConfigurationMutationResult::Changed;
}

[[nodiscard]] OAuthClientConfigurationMutationResultOrError
clearStoredConfiguration(SqliteConnection& connection) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite OAuth configuration connection is unavailable"));
  }
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(handle,
                                               "DELETE FROM local_settings "
                                               "WHERE scope = ?1 AND key = ?2",
                                               -1,
                                               SQLITE_PREPARE_PERSISTENT,
                                               &statement,
                                               nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite OAuth configuration clear preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindText(statement, 1, QString::fromLatin1(kSettingsScope));
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  if (const std::optional<AppError> error =
          bindText(statement, 2, QString::fromLatin1(kClientIdKey));
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changes = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite OAuth configuration clear failed (%1)"),
                         stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(
        QStringLiteral("SQLite OAuth configuration clear finalization failed (%1)"),
        finalizeResult);
  }
  return changes == 0 ? OAuthClientConfigurationMutationResult::Unchanged
                      : OAuthClientConfigurationMutationResult::Changed;
}

} // namespace

OAuthClientConfigurationStore::OAuthClientConfigurationStore(FilePath databasePath,
                                                             const Clock& clock)
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

std::shared_future<SqliteWriteResult> OAuthClientConfigurationStore::ready() const {
  return initialization_;
}

std::future<OAuthClientConfigurationReadResult> OAuthClientConfigurationStore::load() {
  return writerQueue_.enqueueResult(
      [](SqliteConnection& connection) { return readStoredConfiguration(connection); });
}

std::future<OAuthClientConfigurationMutationResultOrError>
OAuthClientConfigurationStore::save(QString clientId, QString clientSecret) {
  clientId = clientId.trimmed();
  clientSecret = clientSecret.trimmed();
  if (!isValidClientId(clientId) || !isValidClientSecret(clientSecret)) {
    return readyFuture(OAuthClientConfigurationMutationResultOrError(
        validationError(QStringLiteral("OAuth client ID is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [clientId = std::move(clientId), clientSecret = std::move(clientSecret), updatedAt](
          SqliteConnection& connection) {
        return saveStoredConfiguration(connection, clientId, clientSecret, updatedAt);
      });
}

std::future<OAuthClientConfigurationMutationResultOrError> OAuthClientConfigurationStore::clear() {
  return writerQueue_.enqueueResult(
      [](SqliteConnection& connection) { return clearStoredConfiguration(connection); });
}

} // namespace hcb
