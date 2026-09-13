#include "core/SyncCheckpointStore.h"

#include "data/LocalSchema.h"
#include "sqlite3.h"

#include <QByteArray>
#include <QCryptographicHash>
#include <QDateTime>
#include <QJsonDocument>
#include <QTimeZone>

#include <array>
#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumIdentifierLength = 256;
constexpr qsizetype kMaximumSyncTokenLength = 8'192;
constexpr qsizetype kMaximumMetadataBytes = 16'384;
constexpr qsizetype kMaximumTimestampLength = 64;

[[nodiscard]] AppError databaseError(const QString& message, int result) {
  return AppError(AppErrorCode::Database, message.arg(result));
}

[[nodiscard]] AppError validationError(const QString& message) {
  return AppError(AppErrorCode::Validation, message);
}

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

[[nodiscard]] bool isValidIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= kMaximumIdentifierLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] bool isValidToken(const QString& value) {
  return !value.isEmpty() && value.size() <= kMaximumSyncTokenLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] std::optional<QByteArray> encodedMetadata(const QJsonObject& metadata) {
  const QByteArray encoded = QJsonDocument(metadata).toJson(QJsonDocument::Compact);
  return encoded.isEmpty() || encoded.size() > kMaximumMetadataBytes || encoded.contains('\0')
             ? std::nullopt
             : std::optional<QByteArray>(encoded);
}

[[nodiscard]] bool isValidKey(const SyncCheckpointKey& key) {
  return isValidIdentifier(key.accountId) && isValidIdentifier(key.resourceId);
}

[[nodiscard]] QString resourceTypeText(SyncCheckpointResourceType resourceType) {
  switch (resourceType) {
  case SyncCheckpointResourceType::TaskListWatermark:
    return QStringLiteral("tasks");
  case SyncCheckpointResourceType::CalendarList:
    return QStringLiteral("calendar_list");
  case SyncCheckpointResourceType::CalendarEvent:
    return QStringLiteral("calendar_event");
  }
  return {};
}

[[nodiscard]] QString checkpointId(const SyncCheckpointKey& key) {
  const QByteArray source = (key.accountId + QChar::Null + resourceTypeText(key.resourceType) +
                             QChar::Null + key.resourceId)
                                .toUtf8();
  return QStringLiteral("sync-checkpoint-") +
         QString::fromLatin1(QCryptographicHash::hash(source, QCryptographicHash::Sha256).toHex());
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
                   QStringLiteral("SQLite sync-checkpoint binding failed (%1)"), result));
}

[[nodiscard]] std::optional<QString> requiredText(sqlite3_stmt* statement, int index) {
  if (sqlite3_column_type(statement, index) != SQLITE_TEXT) {
    return std::nullopt;
  }
  const auto* text = reinterpret_cast<const char*>(sqlite3_column_text(statement, index));
  const int length = sqlite3_column_bytes(statement, index);
  if (text == nullptr || length <= 0) {
    return std::nullopt;
  }
  const QString value = QString::fromUtf8(text, length);
  return value.size() <= kMaximumSyncTokenLength && !value.contains(QChar::Null)
             ? std::optional<QString>(value)
             : std::nullopt;
}

[[nodiscard]] SyncCheckpointLookupResult findStoredCheckpoint(SqliteConnection& connection,
                                                              const SyncCheckpointKey& key) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite sync-checkpoint connection is unavailable"));
  }
  constexpr char sql[] = R"(
SELECT checkpoint_value, metadata_json, last_successful_sync_at, updated_at
FROM local_sync_checkpoints
WHERE account_id = ?1 AND resource_type = ?2 AND resource_id = ?3 AND checkpoint_type = 'sync_token'
LIMIT 1
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite sync-checkpoint read preparation failed (%1)"),
                         prepareResult);
  }
  const QString resourceType = resourceTypeText(key.resourceType);
  for (const auto& [index, value] :
       {std::pair{1, &key.accountId}, std::pair{2, &resourceType}, std::pair{3, &key.resourceId}}) {
    if (const std::optional<AppError> error = bindText(statement, index, *value);
        error.has_value()) {
      sqlite3_finalize(statement);
      return *error;
    }
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult == SQLITE_DONE) {
    const int finalizeResult = sqlite3_finalize(statement);
    return finalizeResult == SQLITE_OK
               ? SyncCheckpointLookupResult(std::optional<SyncCheckpoint>{})
               : SyncCheckpointLookupResult(databaseError(
                     QStringLiteral("SQLite sync-checkpoint read finalization failed (%1)"),
                     finalizeResult));
  }
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite sync-checkpoint read failed (%1)"), stepResult);
  }
  const std::optional<QString> token = requiredText(statement, 0);
  const std::optional<QString> metadataText = requiredText(statement, 1);
  const std::optional<QString> successfulAt = requiredText(statement, 2);
  const std::optional<QString> updatedAt = requiredText(statement, 3);
  const int finalizeResult = sqlite3_finalize(statement);
  const QJsonDocument metadata = metadataText.has_value()
                                     ? QJsonDocument::fromJson(metadataText->toUtf8())
                                     : QJsonDocument();
  if (!token.has_value() || !metadataText.has_value() || !metadata.isObject() ||
      !successfulAt.has_value() || !updatedAt.has_value() ||
      successfulAt->size() > kMaximumTimestampLength ||
      updatedAt->size() > kMaximumTimestampLength) {
    return AppError(AppErrorCode::Database, QStringLiteral("Stored sync checkpoint is invalid"));
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite sync-checkpoint read finalization failed (%1)"),
                         finalizeResult);
  }
  return std::optional<SyncCheckpoint>(SyncCheckpoint{.key = key,
                                                      .syncToken = *token,
                                                      .metadata = metadata.object(),
                                                      .lastSuccessfulSyncAt = *successfulAt,
                                                      .updatedAt = *updatedAt});
}

[[nodiscard]] SyncCheckpointSaveResult saveStoredCheckpoint(SqliteConnection& connection,
                                                            const SyncCheckpoint& checkpoint) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite sync-checkpoint connection is unavailable"));
  }
  constexpr char sql[] = R"(
INSERT INTO local_sync_checkpoints (
  id, account_id, resource_type, resource_id, checkpoint_type, checkpoint_value,
  metadata_json, last_successful_sync_at, updated_at
) VALUES (?1, ?2, ?3, ?4, 'sync_token', ?5, ?6, ?7, ?8)
ON CONFLICT(account_id, resource_type, resource_id, checkpoint_type) DO UPDATE SET
  checkpoint_value = excluded.checkpoint_value,
  metadata_json = excluded.metadata_json,
  last_successful_sync_at = excluded.last_successful_sync_at,
  updated_at = excluded.updated_at
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite sync-checkpoint save preparation failed (%1)"),
                         prepareResult);
  }
  const QString id = checkpointId(checkpoint.key);
  const QString resourceType = resourceTypeText(checkpoint.key.resourceType);
  const std::optional<QByteArray> metadata = encodedMetadata(checkpoint.metadata);
  if (!metadata.has_value()) {
    return validationError(QStringLiteral("Sync checkpoint metadata is invalid"));
  }
  const QString metadataText = QString::fromUtf8(*metadata);
  const std::array<std::pair<int, const QString*>, 8> bindings = {
      std::pair{1, &id},
      std::pair{2, &checkpoint.key.accountId},
      std::pair{3, &resourceType},
      std::pair{4, &checkpoint.key.resourceId},
      std::pair{5, &checkpoint.syncToken},
      std::pair{6, &metadataText},
      std::pair{7, &checkpoint.lastSuccessfulSyncAt},
      std::pair{8, &checkpoint.updatedAt}};
  for (const auto& [index, value] : bindings) {
    if (const std::optional<AppError> error = bindText(statement, index, *value);
        error.has_value()) {
      sqlite3_finalize(statement);
      return *error;
    }
  }
  const int stepResult = sqlite3_step(statement);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite sync-checkpoint save failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite sync-checkpoint save finalization failed (%1)"),
                         finalizeResult);
  }
  return checkpoint;
}

[[nodiscard]] SyncCheckpointEraseResult eraseStoredCheckpoint(SqliteConnection& connection,
                                                              const SyncCheckpointKey& key) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite sync-checkpoint connection is unavailable"));
  }
  constexpr char sql[] = R"(
DELETE FROM local_sync_checkpoints
WHERE account_id = ?1 AND resource_type = ?2 AND resource_id = ?3 AND checkpoint_type = 'sync_token'
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite sync-checkpoint deletion preparation failed (%1)"),
                         prepareResult);
  }
  const QString resourceType = resourceTypeText(key.resourceType);
  for (const auto& [index, value] :
       {std::pair{1, &key.accountId}, std::pair{2, &resourceType}, std::pair{3, &key.resourceId}}) {
    if (const std::optional<AppError> error = bindText(statement, index, *value);
        error.has_value()) {
      sqlite3_finalize(statement);
      return *error;
    }
  }
  const int stepResult = sqlite3_step(statement);
  const bool erased = sqlite3_changes(handle) == 1;
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite sync-checkpoint deletion failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite sync-checkpoint deletion finalization failed (%1)"),
                         finalizeResult);
  }
  return erased;
}

} // namespace

SyncCheckpointStore::SyncCheckpointStore(FilePath databasePath, const Clock& clock)
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

std::shared_future<SqliteWriteResult> SyncCheckpointStore::ready() const { return initialization_; }

std::future<SyncCheckpointLookupResult> SyncCheckpointStore::find(SyncCheckpointKey key) {
  if (!isValidKey(key)) {
    return readyFuture(SyncCheckpointLookupResult(
        validationError(QStringLiteral("Sync checkpoint key is invalid"))));
  }
  return writerQueue_.enqueueResult([key = std::move(key)](SqliteConnection& connection) {
    return findStoredCheckpoint(connection, key);
  });
}

std::future<SyncCheckpointSaveResult> SyncCheckpointStore::save(SyncCheckpointKey key,
                                                                const QString& syncToken,
                                                                QJsonObject metadata) {
  if (!isValidKey(key) || !isValidToken(syncToken) || !encodedMetadata(metadata).has_value()) {
    return readyFuture(SyncCheckpointSaveResult(
        validationError(QStringLiteral("Sync checkpoint input is invalid"))));
  }
  const QString savedAt = timestamp(clock_);
  SyncCheckpoint checkpoint{.key = std::move(key),
                            .syncToken = syncToken,
                            .metadata = std::move(metadata),
                            .lastSuccessfulSyncAt = savedAt,
                            .updatedAt = savedAt};
  return writerQueue_.enqueueResult(
      [checkpoint = std::move(checkpoint)](SqliteConnection& connection) {
        return saveStoredCheckpoint(connection, checkpoint);
      });
}

std::future<SyncCheckpointEraseResult> SyncCheckpointStore::erase(SyncCheckpointKey key) {
  if (!isValidKey(key)) {
    return readyFuture(SyncCheckpointEraseResult(
        validationError(QStringLiteral("Sync checkpoint key is invalid"))));
  }
  return writerQueue_.enqueueResult([key = std::move(key)](SqliteConnection& connection) {
    return eraseStoredCheckpoint(connection, key);
  });
}

} // namespace hcb
