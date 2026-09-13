#include "core/SyncConflictStore.h"

#include "data/LocalSchema.h"
#include "sqlite3.h"

#include <QByteArray>
#include <QCryptographicHash>
#include <QDateTime>
#include <QJsonDocument>
#include <QJsonParseError>
#include <QString>
#include <QTimeZone>

#include <algorithm>
#include <chrono>
#include <future>
#include <initializer_list>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumIdentifierLength = 256;
constexpr qsizetype kMaximumErrorCodeLength = 64;
constexpr qsizetype kMaximumErrorMessageLength = 4'096;
constexpr qsizetype kMaximumPayloadBytes = 262'144;
constexpr qsizetype kMaximumEtagLength = 4'096;
constexpr int kMaximumListLimit = 100;

constexpr char conflictColumns[] = R"(
id, account_id, resource_type, resource_id, mutation_id, error_code, error_message, payload_json,
created_at, updated_at, resolution, resolved_at
)";

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

[[nodiscard]] bool isValidRequiredText(const QString& value, qsizetype maximumLength) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= maximumLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] bool isValidOptionalText(const std::optional<QString>& value,
                                       qsizetype maximumLength) {
  return !value.has_value() || (value->size() <= maximumLength && !value->contains(QChar::Null));
}

[[nodiscard]] QString resourceText(SyncConflictResource resource) {
  switch (resource) {
  case SyncConflictResource::Task:
    return QStringLiteral("task");
  case SyncConflictResource::TaskList:
    return QStringLiteral("task_list");
  case SyncConflictResource::Event:
    return QStringLiteral("event");
  }
  return {};
}

[[nodiscard]] std::optional<SyncConflictResource> resourceFromText(const QString& value) {
  if (value == QStringLiteral("task")) {
    return SyncConflictResource::Task;
  }
  if (value == QStringLiteral("task_list")) {
    return SyncConflictResource::TaskList;
  }
  if (value == QStringLiteral("event")) {
    return SyncConflictResource::Event;
  }
  return std::nullopt;
}

[[nodiscard]] QString resolutionText(SyncConflictResolution resolution) {
  switch (resolution) {
  case SyncConflictResolution::KeepLocal:
    return QStringLiteral("keep_local");
  case SyncConflictResolution::KeepRemote:
    return QStringLiteral("keep_remote");
  }
  return {};
}

[[nodiscard]] std::optional<SyncConflictResolution> resolutionFromText(const QString& value) {
  if (value == QStringLiteral("keep_local")) {
    return SyncConflictResolution::KeepLocal;
  }
  if (value == QStringLiteral("keep_remote")) {
    return SyncConflictResolution::KeepRemote;
  }
  return std::nullopt;
}

[[nodiscard]] QString policyText(SyncConflictPolicy policy) {
  switch (policy) {
  case SyncConflictPolicy::PreferGoogle:
    return QStringLiteral("prefer_google");
  case SyncConflictPolicy::PreferHcb:
    return QStringLiteral("prefer_hcb");
  case SyncConflictPolicy::AskEachTime:
    return QStringLiteral("ask_each_time");
  }
  return {};
}

[[nodiscard]] std::optional<SyncConflictPolicy> policyFromText(const QString& value) {
  if (value == QStringLiteral("prefer_google")) {
    return SyncConflictPolicy::PreferGoogle;
  }
  if (value == QStringLiteral("prefer_hcb")) {
    return SyncConflictPolicy::PreferHcb;
  }
  if (value == QStringLiteral("ask_each_time")) {
    return SyncConflictPolicy::AskEachTime;
  }
  return std::nullopt;
}

[[nodiscard]] QString timestamp(const Clock& clock) {
  const auto milliseconds =
      std::chrono::duration_cast<std::chrono::milliseconds>(clock.wallNow().time_since_epoch());
  return QDateTime::fromMSecsSinceEpoch(milliseconds.count(), QTimeZone::UTC)
      .toString(Qt::ISODateWithMs);
}

[[nodiscard]] QString conflictId(const QString& mutationId) {
  const QByteArray digest =
      QCryptographicHash::hash(mutationId.toUtf8(), QCryptographicHash::Algorithm::Sha256).toHex();
  return QStringLiteral("conflict:") + QString::fromLatin1(digest);
}

[[nodiscard]] std::optional<AppError>
bindText(sqlite3_stmt* statement, int index, const QString& value) {
  const QByteArray utf8 = value.toUtf8();
  const int result = sqlite3_bind_text(
      statement, index, utf8.constData(), static_cast<int>(utf8.size()), SQLITE_TRANSIENT);
  return result == SQLITE_OK
             ? std::nullopt
             : std::optional<AppError>(databaseError(
                   QStringLiteral("SQLite sync-conflict binding failed (%1)"), result));
}

[[nodiscard]] std::optional<AppError>
bindOptionalText(sqlite3_stmt* statement, int index, const std::optional<QString>& value) {
  if (value.has_value()) {
    return bindText(statement, index, *value);
  }
  const int result = sqlite3_bind_null(statement, index);
  return result == SQLITE_OK
             ? std::nullopt
             : std::optional<AppError>(databaseError(
                   QStringLiteral("SQLite sync-conflict binding failed (%1)"), result));
}

[[nodiscard]] std::optional<AppError> bindInteger(sqlite3_stmt* statement, int index, int value) {
  const int result = sqlite3_bind_int(statement, index, value);
  return result == SQLITE_OK
             ? std::nullopt
             : std::optional<AppError>(databaseError(
                   QStringLiteral("SQLite sync-conflict binding failed (%1)"), result));
}

[[nodiscard]] std::optional<AppError>
bindAll(sqlite3_stmt* statement, const std::initializer_list<std::optional<AppError>>& results) {
  for (const std::optional<AppError>& result : results) {
    if (result.has_value()) {
      sqlite3_finalize(statement);
      return result;
    }
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<QString> optionalText(sqlite3_stmt* statement, int index) {
  if (sqlite3_column_type(statement, index) == SQLITE_NULL) {
    return std::nullopt;
  }
  const auto* value = reinterpret_cast<const char*>(sqlite3_column_text(statement, index));
  const int size = sqlite3_column_bytes(statement, index);
  return value == nullptr || size < 0 ? std::nullopt
                                      : std::optional<QString>(QString::fromUtf8(value, size));
}

[[nodiscard]] SyncConflictResult decodeConflict(sqlite3_stmt* statement) {
  const std::optional<QString> id = optionalText(statement, 0);
  const std::optional<QString> resourceType = optionalText(statement, 2);
  const std::optional<QString> resourceId = optionalText(statement, 3);
  const std::optional<QString> mutationId = optionalText(statement, 4);
  const std::optional<QString> errorCode = optionalText(statement, 5);
  const std::optional<QString> errorMessage = optionalText(statement, 6);
  const std::optional<QString> payloadJson = optionalText(statement, 7);
  const std::optional<QString> createdAt = optionalText(statement, 8);
  const std::optional<QString> updatedAt = optionalText(statement, 9);
  if (!id.has_value() || !resourceType.has_value() || !resourceId.has_value() ||
      !mutationId.has_value() || !errorCode.has_value() || !errorMessage.has_value() ||
      !payloadJson.has_value() || !createdAt.has_value() || !updatedAt.has_value()) {
    return AppError(AppErrorCode::Database, QStringLiteral("Stored sync conflict row is invalid"));
  }
  const std::optional<SyncConflictResource> resource = resourceFromText(*resourceType);
  const std::optional<QString> resolutionTextValue = optionalText(statement, 10);
  const std::optional<SyncConflictResolution> resolution =
      resolutionTextValue.has_value() ? resolutionFromText(*resolutionTextValue)
                                      : std::optional<SyncConflictResolution>{};
  QJsonParseError parseError;
  const QJsonDocument payload = QJsonDocument::fromJson(payloadJson->toUtf8(), &parseError);
  if (!resource.has_value() || (!resolution.has_value() && resolutionTextValue.has_value()) ||
      parseError.error != QJsonParseError::NoError || !payload.isObject()) {
    return AppError(AppErrorCode::Database, QStringLiteral("Stored sync conflict row is invalid"));
  }
  const QJsonObject storedPayload = payload.object();
  QJsonObject baseSnapshot;
  QJsonObject localPayload = storedPayload;
  QJsonObject remoteSnapshot;
  std::optional<QString> remoteEtag;
  SyncConflictPolicy policy = SyncConflictPolicy::PreferGoogle;
  if (storedPayload.contains(QStringLiteral("base")) ||
      storedPayload.contains(QStringLiteral("local")) ||
      storedPayload.contains(QStringLiteral("remote")) ||
      storedPayload.contains(QStringLiteral("policy"))) {
    const QJsonValue baseValue = storedPayload.value(QStringLiteral("base"));
    const QJsonValue localValue = storedPayload.value(QStringLiteral("local"));
    const QJsonValue remoteValue = storedPayload.value(QStringLiteral("remote"));
    const QJsonValue etagValue = storedPayload.value(QStringLiteral("remoteEtag"));
    const QJsonValue policyValue = storedPayload.value(QStringLiteral("policy"));
    if (!baseValue.isObject() || !localValue.isObject() || !remoteValue.isObject() ||
        !policyValue.isString() ||
        (!etagValue.isUndefined() &&
         (!etagValue.isString() || etagValue.toString().size() > kMaximumEtagLength ||
          etagValue.toString().contains(QChar::Null)))) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Stored sync conflict payload is invalid"));
    }
    const std::optional<SyncConflictPolicy> storedPolicy = policyFromText(policyValue.toString());
    if (!storedPolicy.has_value()) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Stored sync conflict payload is invalid"));
    }
    baseSnapshot = baseValue.toObject();
    localPayload = localValue.toObject();
    remoteSnapshot = remoteValue.toObject();
    if (etagValue.isString()) {
      remoteEtag = etagValue.toString();
    }
    policy = *storedPolicy;
  }
  return SyncConflict{.id = *id,
                      .accountId = optionalText(statement, 1),
                      .resource = *resource,
                      .resourceId = *resourceId,
                      .mutationId = *mutationId,
                      .errorCode = *errorCode,
                      .errorMessage = *errorMessage,
                      .baseSnapshot = std::move(baseSnapshot),
                      .localPayload = std::move(localPayload),
                      .remoteSnapshot = std::move(remoteSnapshot),
                      .remoteEtag = std::move(remoteEtag),
                      .policy = policy,
                      .createdAt = *createdAt,
                      .updatedAt = *updatedAt,
                      .resolution = resolution,
                      .resolvedAt = optionalText(statement, 11)};
}

[[nodiscard]] std::variant<SyncConflictInput, AppError> canonicalize(SyncConflictInput input) {
  QJsonObject payload{{QStringLiteral("base"), input.baseSnapshot},
                      {QStringLiteral("local"), input.localPayload},
                      {QStringLiteral("remote"), input.remoteSnapshot},
                      {QStringLiteral("policy"), policyText(input.policy)}};
  if (input.remoteEtag.has_value()) {
    payload.insert(QStringLiteral("remoteEtag"), *input.remoteEtag);
  }
  const QByteArray payloadBytes = QJsonDocument(payload).toJson(QJsonDocument::Compact);
  if ((input.accountId.has_value() &&
       !isValidRequiredText(*input.accountId, kMaximumIdentifierLength)) ||
      resourceText(input.resource).isEmpty() ||
      !isValidRequiredText(input.resourceId, kMaximumIdentifierLength) ||
      !isValidRequiredText(input.mutationId, kMaximumIdentifierLength) ||
      !isValidRequiredText(input.errorCode, kMaximumErrorCodeLength) ||
      !isValidRequiredText(input.errorMessage, kMaximumErrorMessageLength) ||
      !isValidOptionalText(input.remoteEtag, kMaximumEtagLength) ||
      policyText(input.policy).isEmpty() || payloadBytes.size() > kMaximumPayloadBytes) {
    return validationError(QStringLiteral("Sync conflict input is invalid"));
  }
  return input;
}

[[nodiscard]] SyncConflictResult recordStoredConflict(SqliteConnection& connection,
                                                      const SyncConflictInput& input,
                                                      const QString& id,
                                                      const QString& now) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite sync-conflict connection is unavailable"));
  }
  constexpr char sql[] = R"(
INSERT INTO local_sync_conflicts (
  id, account_id, resource_type, resource_id, mutation_id, error_code, error_message,
  payload_json, status, created_at, updated_at
) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, 'unresolved', ?9, ?9)
ON CONFLICT(mutation_id) DO UPDATE SET
  account_id = excluded.account_id,
  resource_type = excluded.resource_type,
  resource_id = excluded.resource_id,
  error_code = excluded.error_code,
  error_message = excluded.error_message,
  payload_json = excluded.payload_json,
  status = 'unresolved',
  resolution = NULL,
  resolved_at = NULL,
  updated_at = excluded.updated_at
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite sync-conflict record preparation failed (%1)"),
                         prepareResult);
  }
  QJsonObject payload{{QStringLiteral("base"), input.baseSnapshot},
                      {QStringLiteral("local"), input.localPayload},
                      {QStringLiteral("remote"), input.remoteSnapshot},
                      {QStringLiteral("policy"), policyText(input.policy)}};
  if (input.remoteEtag.has_value()) {
    payload.insert(QStringLiteral("remoteEtag"), *input.remoteEtag);
  }
  const QString payloadText =
      QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, id),
                   bindOptionalText(statement, 2, input.accountId),
                   bindText(statement, 3, resourceText(input.resource)),
                   bindText(statement, 4, input.resourceId),
                   bindText(statement, 5, input.mutationId),
                   bindText(statement, 6, input.errorCode),
                   bindText(statement, 7, input.errorMessage),
                   bindText(statement, 8, payloadText),
                   bindText(statement, 9, now)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite sync-conflict record failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite sync-conflict record finalization failed (%1)"),
                         finalizeResult);
  }
  const QByteArray lookupSql =
      QStringLiteral("SELECT %1 FROM local_sync_conflicts WHERE mutation_id = ?1 LIMIT 1")
          .arg(QString::fromLatin1(conflictColumns))
          .toUtf8();
  statement = nullptr;
  const int lookupPrepare = sqlite3_prepare_v3(
      handle, lookupSql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (lookupPrepare != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite sync-conflict lookup preparation failed (%1)"),
                         lookupPrepare);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, input.mutationId);
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int lookupStep = sqlite3_step(statement);
  const SyncConflictResult decoded =
      lookupStep == SQLITE_ROW
          ? decodeConflict(statement)
          : SyncConflictResult(databaseError(
                QStringLiteral("SQLite sync-conflict lookup failed (%1)"), lookupStep));
  const int lookupFinalize = sqlite3_finalize(statement);
  if (std::holds_alternative<AppError>(decoded)) {
    return std::get<AppError>(decoded);
  }
  return lookupFinalize == SQLITE_OK
             ? decoded
             : SyncConflictResult(databaseError(
                   QStringLiteral("SQLite sync-conflict lookup finalization failed (%1)"),
                   lookupFinalize));
}

[[nodiscard]] SyncConflictListResult listStoredUnresolvedConflicts(SqliteConnection& connection,
                                                                   int limit) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite sync-conflict connection is unavailable"));
  }
  const QByteArray sql = QStringLiteral(R"(
SELECT %1 FROM local_sync_conflicts
WHERE status = 'unresolved'
ORDER BY updated_at ASC, id ASC
LIMIT ?1
)")
                             .arg(QString::fromLatin1(conflictColumns))
                             .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite sync-conflict list preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindInteger(statement, 1, limit); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  QList<SyncConflict> conflicts;
  while (true) {
    const int stepResult = sqlite3_step(statement);
    if (stepResult == SQLITE_DONE) {
      break;
    }
    if (stepResult != SQLITE_ROW) {
      sqlite3_finalize(statement);
      return databaseError(QStringLiteral("SQLite sync-conflict list failed (%1)"), stepResult);
    }
    const SyncConflictResult decoded = decodeConflict(statement);
    if (std::holds_alternative<AppError>(decoded)) {
      sqlite3_finalize(statement);
      return std::get<AppError>(decoded);
    }
    conflicts.append(std::get<SyncConflict>(decoded));
  }
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK
             ? SyncConflictListResult(std::move(conflicts))
             : SyncConflictListResult(databaseError(
                   QStringLiteral("SQLite sync-conflict list finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] SyncConflictListResult listStoredResolvedConflicts(SqliteConnection& connection,
                                                                 int limit) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite sync-conflict connection is unavailable"));
  }
  const QByteArray sql = QStringLiteral(R"(
SELECT %1 FROM local_sync_conflicts
WHERE status = 'resolved'
ORDER BY resolved_at DESC, id ASC
LIMIT ?1
)")
                             .arg(QString::fromLatin1(conflictColumns))
                             .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite sync-conflict history preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindInteger(statement, 1, limit); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  QList<SyncConflict> conflicts;
  while (true) {
    const int stepResult = sqlite3_step(statement);
    if (stepResult == SQLITE_DONE) {
      break;
    }
    if (stepResult != SQLITE_ROW) {
      sqlite3_finalize(statement);
      return databaseError(QStringLiteral("SQLite sync-conflict history lookup failed (%1)"),
                           stepResult);
    }
    const SyncConflictResult decoded = decodeConflict(statement);
    if (std::holds_alternative<AppError>(decoded)) {
      sqlite3_finalize(statement);
      return std::get<AppError>(decoded);
    }
    conflicts.append(std::get<SyncConflict>(decoded));
  }
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK
             ? SyncConflictListResult(std::move(conflicts))
             : SyncConflictListResult(databaseError(
                   QStringLiteral("SQLite sync-conflict history finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] SyncConflictResult resolveStoredConflict(SqliteConnection& connection,
                                                       const QString& conflictIdValue,
                                                       SyncConflictResolution resolution,
                                                       const QString& now) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite sync-conflict connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_sync_conflicts
SET status = 'resolved', resolution = ?2, resolved_at = ?3, updated_at = ?3
WHERE id = ?1 AND status = 'unresolved'
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite sync-conflict resolve preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, conflictIdValue),
                   bindText(statement, 2, resolutionText(resolution)),
                   bindText(statement, 3, now)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite sync-conflict resolve failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite sync-conflict resolve finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Sync conflict is unavailable for resolution"));
  }
  const QByteArray lookup =
      QStringLiteral("SELECT %1 FROM local_sync_conflicts WHERE id = ?1 LIMIT 1")
          .arg(QString::fromLatin1(conflictColumns))
          .toUtf8();
  statement = nullptr;
  const int lookupPrepare = sqlite3_prepare_v3(
      handle, lookup.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (lookupPrepare != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite sync-conflict lookup preparation failed (%1)"),
                         lookupPrepare);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, conflictIdValue);
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int lookupStep = sqlite3_step(statement);
  const SyncConflictResult decoded =
      lookupStep == SQLITE_ROW
          ? decodeConflict(statement)
          : SyncConflictResult(databaseError(
                QStringLiteral("SQLite sync-conflict lookup failed (%1)"), lookupStep));
  const int lookupFinalize = sqlite3_finalize(statement);
  if (std::holds_alternative<AppError>(decoded)) {
    return std::get<AppError>(decoded);
  }
  return lookupFinalize == SQLITE_OK
             ? decoded
             : SyncConflictResult(databaseError(
                   QStringLiteral("SQLite sync-conflict lookup finalization failed (%1)"),
                   lookupFinalize));
}

} // namespace

SyncConflictStore::SyncConflictStore(FilePath databasePath, const Clock& clock)
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

std::shared_future<SqliteWriteResult> SyncConflictStore::ready() const { return initialization_; }

std::future<SyncConflictResult> SyncConflictStore::record(SyncConflictInput input) {
  const std::variant<SyncConflictInput, AppError> canonical = canonicalize(std::move(input));
  if (std::holds_alternative<AppError>(canonical)) {
    return readyFuture(SyncConflictResult(std::get<AppError>(canonical)));
  }
  const SyncConflictInput storedInput = std::get<SyncConflictInput>(canonical);
  const QString now = timestamp(clock_);
  return writerQueue_.enqueueResult([input = storedInput,
                                     id = conflictId(storedInput.mutationId),
                                     now](SqliteConnection& connection) -> SyncConflictResult {
    return recordStoredConflict(connection, input, id, now);
  });
}

std::future<SyncConflictListResult> SyncConflictStore::listUnresolved(int limit) {
  const int cappedLimit = std::clamp(limit, 1, kMaximumListLimit);
  return writerQueue_.enqueueResult(
      [cappedLimit](SqliteConnection& connection) -> SyncConflictListResult {
        return listStoredUnresolvedConflicts(connection, cappedLimit);
      });
}

std::future<SyncConflictListResult> SyncConflictStore::listResolved(int limit) {
  const int cappedLimit = std::clamp(limit, 1, kMaximumListLimit);
  return writerQueue_.enqueueResult(
      [cappedLimit](SqliteConnection& connection) -> SyncConflictListResult {
        return listStoredResolvedConflicts(connection, cappedLimit);
      });
}

std::future<SyncConflictResult> SyncConflictStore::resolve(QString conflictIdValue,
                                                           SyncConflictResolution resolution) {
  if (!isValidRequiredText(conflictIdValue, kMaximumIdentifierLength) ||
      resolutionText(resolution).isEmpty()) {
    return readyFuture(
        SyncConflictResult(validationError(QStringLiteral("Sync conflict resolution is invalid"))));
  }
  const QString now = timestamp(clock_);
  return writerQueue_.enqueueResult([conflictIdValue = std::move(conflictIdValue), resolution, now](
                                        SqliteConnection& connection) {
    return resolveStoredConflict(connection, conflictIdValue, resolution, now);
  });
}

} // namespace hcb
