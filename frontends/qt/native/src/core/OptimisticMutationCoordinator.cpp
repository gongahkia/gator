#include "core/OptimisticMutationCoordinator.h"

#include "data/LocalSchema.h"
#include "sqlite3.h"

#include <QByteArray>
#include <QDateTime>
#include <QJsonDocument>
#include <QJsonParseError>
#include <QString>
#include <QTimeZone>
#include <QUuid>

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
constexpr qsizetype kMaximumOperationLength = 128;
constexpr qsizetype kMaximumPayloadBytes = 262'144;
constexpr qsizetype kMaximumTimestampLength = 64;
constexpr qsizetype kMaximumErrorCodeLength = 64;
constexpr qsizetype kMaximumErrorMessageLength = 4'096;
constexpr qsizetype kMaximumEtagLength = 4'096;
constexpr int kMaximumDueMutations = 100;
constexpr auto kMaximumLeaseDuration = std::chrono::hours(1);
constexpr char conflictMetadataKey[] = "_hcbSync";

constexpr char pendingMutationColumns[] = R"(
id, account_id, resource_type, resource_id, operation, payload_json, status, attempt_count,
next_retry_at, lease_id, lease_expires_at, last_error_code, last_error_message, created_at,
updated_at, applied_at
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

[[nodiscard]] std::optional<QString> canonicalTimestamp(const QString& value) {
  if (!isValidRequiredText(value, kMaximumTimestampLength) || !value.contains(u'T')) {
    return std::nullopt;
  }
  const QDateTime parsed = QDateTime::fromString(value, Qt::ISODateWithMs);
  return parsed.isValid() ? std::optional<QString>(parsed.toUTC().toString(Qt::ISODateWithMs))
                          : std::nullopt;
}

[[nodiscard]] QString timestamp(const Clock& clock) {
  const auto milliseconds =
      std::chrono::duration_cast<std::chrono::milliseconds>(clock.wallNow().time_since_epoch());
  return QDateTime::fromMSecsSinceEpoch(milliseconds.count(), QTimeZone::UTC)
      .toString(Qt::ISODateWithMs);
}

[[nodiscard]] QString timestampAfter(const Clock& clock, std::chrono::seconds duration) {
  const auto milliseconds = std::chrono::duration_cast<std::chrono::milliseconds>(
      (clock.wallNow() + duration).time_since_epoch());
  return QDateTime::fromMSecsSinceEpoch(milliseconds.count(), QTimeZone::UTC)
      .toString(Qt::ISODateWithMs);
}

[[nodiscard]] QString resourceText(PendingMutationResource resource) {
  switch (resource) {
  case PendingMutationResource::Task:
    return QStringLiteral("task");
  case PendingMutationResource::TaskList:
    return QStringLiteral("task_list");
  case PendingMutationResource::Event:
    return QStringLiteral("event");
  }
  return {};
}

[[nodiscard]] std::optional<PendingMutationResource> resourceFromText(const QString& value) {
  if (value == QStringLiteral("task")) {
    return PendingMutationResource::Task;
  }
  if (value == QStringLiteral("task_list")) {
    return PendingMutationResource::TaskList;
  }
  if (value == QStringLiteral("event")) {
    return PendingMutationResource::Event;
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<PendingMutationStatus> statusFromText(const QString& value) {
  if (value == QStringLiteral("pending")) {
    return PendingMutationStatus::Pending;
  }
  if (value == QStringLiteral("applying")) {
    return PendingMutationStatus::Applying;
  }
  if (value == QStringLiteral("failed")) {
    return PendingMutationStatus::Failed;
  }
  if (value == QStringLiteral("applied")) {
    return PendingMutationStatus::Applied;
  }
  if (value == QStringLiteral("cancelled")) {
    return PendingMutationStatus::Cancelled;
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<AppError>
bindText(sqlite3_stmt* statement, int index, const QString& value) {
  const QByteArray utf8 = value.toUtf8();
  const int result = sqlite3_bind_text(
      statement, index, utf8.constData(), static_cast<int>(utf8.size()), SQLITE_TRANSIENT);
  return result == SQLITE_OK ? std::nullopt
                             : std::optional<AppError>(databaseError(
                                   QStringLiteral("SQLite mutation binding failed (%1)"), result));
}

[[nodiscard]] std::optional<AppError>
bindOptionalText(sqlite3_stmt* statement, int index, const std::optional<QString>& value) {
  if (value.has_value()) {
    return bindText(statement, index, *value);
  }
  const int result = sqlite3_bind_null(statement, index);
  return result == SQLITE_OK ? std::nullopt
                             : std::optional<AppError>(databaseError(
                                   QStringLiteral("SQLite mutation binding failed (%1)"), result));
}

[[nodiscard]] std::optional<AppError> bindInteger(sqlite3_stmt* statement, int index, int value) {
  const int result = sqlite3_bind_int(statement, index, value);
  return result == SQLITE_OK ? std::nullopt
                             : std::optional<AppError>(databaseError(
                                   QStringLiteral("SQLite mutation binding failed (%1)"), result));
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

[[nodiscard]] PendingMutationResult decodeMutation(sqlite3_stmt* statement) {
  const std::optional<QString> id = optionalText(statement, 0);
  const std::optional<QString> resourceType = optionalText(statement, 2);
  const std::optional<QString> resourceId = optionalText(statement, 3);
  const std::optional<QString> operation = optionalText(statement, 4);
  const std::optional<QString> payloadJson = optionalText(statement, 5);
  const std::optional<QString> statusText = optionalText(statement, 6);
  const std::optional<QString> createdAt = optionalText(statement, 13);
  const std::optional<QString> updatedAt = optionalText(statement, 14);
  if (!id.has_value() || !resourceType.has_value() || !resourceId.has_value() ||
      !operation.has_value() || !payloadJson.has_value() || !statusText.has_value() ||
      !createdAt.has_value() || !updatedAt.has_value()) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Stored pending mutation row is invalid"));
  }
  const std::optional<PendingMutationResource> resource = resourceFromText(*resourceType);
  const std::optional<PendingMutationStatus> status = statusFromText(*statusText);
  QJsonParseError parseError;
  const QJsonDocument payloadDocument = QJsonDocument::fromJson(payloadJson->toUtf8(), &parseError);
  if (!resource.has_value() || !status.has_value() ||
      parseError.error != QJsonParseError::NoError || !payloadDocument.isObject()) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Stored pending mutation row is invalid"));
  }
  QJsonObject payload = payloadDocument.object();
  QJsonObject baseSnapshot;
  std::optional<QString> remoteEtag;
  const QJsonValue metadataValue = payload.take(QString::fromLatin1(conflictMetadataKey));
  if (!metadataValue.isUndefined()) {
    if (!metadataValue.isObject()) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Stored pending mutation conflict metadata is invalid"));
    }
    const QJsonObject metadata = metadataValue.toObject();
    const QJsonValue baseValue = metadata.value(QStringLiteral("base"));
    const QJsonValue etagValue = metadata.value(QStringLiteral("etag"));
    if (!baseValue.isObject() ||
        (!etagValue.isUndefined() &&
         (!etagValue.isString() ||
          !isValidOptionalText(std::optional<QString>(etagValue.toString()),
                               kMaximumEtagLength)))) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Stored pending mutation conflict metadata is invalid"));
    }
    baseSnapshot = baseValue.toObject();
    if (etagValue.isString()) {
      remoteEtag = etagValue.toString();
    }
  }
  return PendingMutation{.id = *id,
                         .accountId = optionalText(statement, 1),
                         .resource = *resource,
                         .resourceId = *resourceId,
                         .operation = *operation,
                         .payload = std::move(payload),
                         .baseSnapshot = std::move(baseSnapshot),
                         .remoteEtag = std::move(remoteEtag),
                         .status = *status,
                         .attemptCount = sqlite3_column_int(statement, 7),
                         .nextRetryAt = optionalText(statement, 8),
                         .leaseId = optionalText(statement, 9),
                         .leaseExpiresAt = optionalText(statement, 10),
                         .lastErrorCode = optionalText(statement, 11),
                         .lastErrorMessage = optionalText(statement, 12),
                         .createdAt = *createdAt,
                         .updatedAt = *updatedAt,
                         .appliedAt = optionalText(statement, 15)};
}

[[nodiscard]] PendingMutationLookupResult findStoredMutation(SqliteConnection& connection,
                                                             const QString& mutationId) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite mutation connection is unavailable"));
  }
  const QByteArray sql =
      QStringLiteral("SELECT %1 FROM local_pending_mutations WHERE id = ?1 LIMIT 1")
          .arg(QString::fromLatin1(pendingMutationColumns))
          .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite mutation lookup preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, mutationId); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult == SQLITE_DONE) {
    const int finalizeResult = sqlite3_finalize(statement);
    return finalizeResult == SQLITE_OK
               ? PendingMutationLookupResult(std::optional<PendingMutation>{})
               : PendingMutationLookupResult(databaseError(
                     QStringLiteral("SQLite mutation lookup finalization failed (%1)"),
                     finalizeResult));
  }
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite mutation lookup failed (%1)"), stepResult);
  }
  const PendingMutationResult decoded = decodeMutation(statement);
  const int finalizeResult = sqlite3_finalize(statement);
  if (std::holds_alternative<AppError>(decoded)) {
    return std::get<AppError>(decoded);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite mutation lookup finalization failed (%1)"),
                         finalizeResult);
  }
  return std::optional<PendingMutation>(std::get<PendingMutation>(decoded));
}

[[nodiscard]] PendingMutationResult requireStoredMutation(SqliteConnection& connection,
                                                          const QString& mutationId) {
  const PendingMutationLookupResult found = findStoredMutation(connection, mutationId);
  if (std::holds_alternative<AppError>(found)) {
    return std::get<AppError>(found);
  }
  const std::optional<PendingMutation>& mutation = std::get<std::optional<PendingMutation>>(found);
  return mutation.has_value()
             ? PendingMutationResult(*mutation)
             : PendingMutationResult(AppError(AppErrorCode::Database,
                                              QStringLiteral("Updated mutation was not found")));
}

[[nodiscard]] PendingMutationListResult
listStoredDueMutations(SqliteConnection& connection, const QString& now, int limit) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite mutation connection is unavailable"));
  }
  const QByteArray sql = QStringLiteral(R"(
SELECT %1
FROM local_pending_mutations
WHERE (status = 'pending' AND (next_retry_at IS NULL OR next_retry_at <= ?1))
   OR (status = 'failed' AND next_retry_at IS NOT NULL AND next_retry_at <= ?1)
ORDER BY created_at ASC, id ASC
LIMIT ?2
)")
                             .arg(QString::fromLatin1(pendingMutationColumns))
                             .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite due mutation preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(statement, {bindText(statement, 1, now), bindInteger(statement, 2, limit)});
      error.has_value()) {
    return *error;
  }
  QList<PendingMutation> mutations;
  while (true) {
    const int stepResult = sqlite3_step(statement);
    if (stepResult == SQLITE_DONE) {
      break;
    }
    if (stepResult != SQLITE_ROW) {
      sqlite3_finalize(statement);
      return databaseError(QStringLiteral("SQLite due mutation query failed (%1)"), stepResult);
    }
    const PendingMutationResult decoded = decodeMutation(statement);
    if (std::holds_alternative<AppError>(decoded)) {
      sqlite3_finalize(statement);
      return std::get<AppError>(decoded);
    }
    mutations.append(std::get<PendingMutation>(decoded));
  }
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK
             ? PendingMutationListResult(mutations)
             : PendingMutationListResult(databaseError(
                   QStringLiteral("SQLite due mutation finalization failed (%1)"), finalizeResult));
}

[[nodiscard]] PendingMutationListResult
listStoredActiveMutations(SqliteConnection& connection, int limit) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite mutation connection is unavailable"));
  }
  const QByteArray sql = QStringLiteral(R"(
SELECT %1
FROM local_pending_mutations
WHERE status IN ('pending', 'applying', 'failed')
ORDER BY created_at ASC, id ASC
LIMIT ?1
)")
                             .arg(QString::fromLatin1(pendingMutationColumns))
                             .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite active mutation preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindInteger(statement, 1, limit); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  QList<PendingMutation> mutations;
  while (true) {
    const int stepResult = sqlite3_step(statement);
    if (stepResult == SQLITE_DONE) {
      break;
    }
    if (stepResult != SQLITE_ROW) {
      sqlite3_finalize(statement);
      return databaseError(QStringLiteral("SQLite active mutation query failed (%1)"), stepResult);
    }
    const PendingMutationResult decoded = decodeMutation(statement);
    if (std::holds_alternative<AppError>(decoded)) {
      sqlite3_finalize(statement);
      return std::get<AppError>(decoded);
    }
    mutations.append(std::get<PendingMutation>(decoded));
  }
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK
             ? PendingMutationListResult(mutations)
             : PendingMutationListResult(databaseError(
                   QStringLiteral("SQLite active mutation finalization failed (%1)"), finalizeResult));
}

[[nodiscard]] std::variant<OptimisticMutationInput, AppError>
canonicalize(OptimisticMutationInput input) {
  if (input.payload.contains(QString::fromLatin1(conflictMetadataKey)) ||
      (input.remoteEtag.has_value() &&
       !isValidOptionalText(input.remoteEtag, kMaximumEtagLength))) {
    return validationError(QStringLiteral("Optimistic mutation input is invalid"));
  }
  QJsonObject metadata{{QStringLiteral("base"), input.baseSnapshot}};
  if (input.remoteEtag.has_value()) {
    metadata.insert(QStringLiteral("etag"), *input.remoteEtag);
  }
  input.payload.insert(QString::fromLatin1(conflictMetadataKey), std::move(metadata));
  const QByteArray payload = QJsonDocument(input.payload).toJson(QJsonDocument::Compact);
  if ((input.accountId.has_value() &&
       !isValidRequiredText(*input.accountId, kMaximumIdentifierLength)) ||
      resourceText(input.resource).isEmpty() ||
      !isValidRequiredText(input.resourceId, kMaximumIdentifierLength) ||
      !isValidRequiredText(input.operation, kMaximumOperationLength) ||
      payload.size() > kMaximumPayloadBytes) {
    return validationError(QStringLiteral("Optimistic mutation input is invalid"));
  }
  return input;
}

[[nodiscard]] std::variant<MutationFailureInput, AppError>
canonicalize(MutationFailureInput input) {
  if (input.nextRetryAt.has_value()) {
    input.nextRetryAt = canonicalTimestamp(*input.nextRetryAt);
  }
  if (!isValidRequiredText(input.mutationId, kMaximumIdentifierLength) ||
      !isValidRequiredText(input.leaseId, kMaximumIdentifierLength) ||
      !isValidRequiredText(input.errorCode, kMaximumErrorCodeLength) ||
      !isValidOptionalText(std::optional<QString>(input.errorMessage),
                           kMaximumErrorMessageLength) ||
      (input.nextRetryAt.has_value() &&
       !isValidRequiredText(*input.nextRetryAt, kMaximumTimestampLength))) {
    return validationError(QStringLiteral("Mutation failure input is invalid"));
  }
  return input;
}

[[nodiscard]] std::variant<MutationRebaseInput, AppError>
canonicalize(MutationRebaseInput input) {
  if (input.payload.contains(QString::fromLatin1(conflictMetadataKey)) ||
      !isValidRequiredText(input.mutationId, kMaximumIdentifierLength) ||
      (input.leaseId.has_value() &&
       !isValidRequiredText(*input.leaseId, kMaximumIdentifierLength)) ||
      (input.remoteEtag.has_value() &&
       !isValidOptionalText(input.remoteEtag, kMaximumEtagLength))) {
    return validationError(QStringLiteral("Pending mutation rebase input is invalid"));
  }
  QJsonObject metadata{{QStringLiteral("base"), input.baseSnapshot}};
  if (input.remoteEtag.has_value()) {
    metadata.insert(QStringLiteral("etag"), *input.remoteEtag);
  }
  input.payload.insert(QString::fromLatin1(conflictMetadataKey), std::move(metadata));
  if (QJsonDocument(input.payload).toJson(QJsonDocument::Compact).size() > kMaximumPayloadBytes) {
    return validationError(QStringLiteral("Pending mutation rebase input is invalid"));
  }
  return input;
}

[[nodiscard]] bool isValidMutationId(const QString& value) {
  return isValidRequiredText(value, kMaximumIdentifierLength);
}

[[nodiscard]] PendingMutationResult enqueueStoredMutation(SqliteConnection& connection,
                                                          const OptimisticMutationInput& input,
                                                          const QString& mutationId,
                                                          const QString& now) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite mutation connection is unavailable"));
  }
  constexpr char sql[] = R"(
INSERT INTO local_pending_mutations (
  id, account_id, resource_type, resource_id, operation, payload_json, status, attempt_count,
  created_at, updated_at
) VALUES (?1, ?2, ?3, ?4, ?5, ?6, 'pending', 0, ?7, ?7)
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite mutation enqueue preparation failed (%1)"),
                         prepareResult);
  }
  const QString payload =
      QString::fromUtf8(QJsonDocument(input.payload).toJson(QJsonDocument::Compact));
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, mutationId),
                   bindOptionalText(statement, 2, input.accountId),
                   bindText(statement, 3, resourceText(input.resource)),
                   bindText(statement, 4, input.resourceId),
                   bindText(statement, 5, input.operation),
                   bindText(statement, 6, payload),
                   bindText(statement, 7, now)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite mutation enqueue failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite mutation enqueue finalization failed (%1)"),
                         finalizeResult);
  }
  return requireStoredMutation(connection, mutationId);
}

[[nodiscard]] PendingMutationResult claimStoredMutation(SqliteConnection& connection,
                                                        const QString& mutationId,
                                                        const QString& leaseId,
                                                        const QString& now,
                                                        const QString& leaseExpiresAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite mutation connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_pending_mutations
SET status = 'applying',
    lease_id = ?2,
    lease_expires_at = ?3,
    updated_at = ?4
WHERE id = ?1
  AND ((status = 'pending' AND (next_retry_at IS NULL OR next_retry_at <= ?4))
       OR (status = 'failed' AND next_retry_at IS NOT NULL AND next_retry_at <= ?4))
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite mutation claim preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindAll(statement,
                                                    {bindText(statement, 1, mutationId),
                                                     bindText(statement, 2, leaseId),
                                                     bindText(statement, 3, leaseExpiresAt),
                                                     bindText(statement, 4, now)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite mutation claim failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite mutation claim finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Pending mutation is not due for claim"));
  }
  return requireStoredMutation(connection, mutationId);
}

[[nodiscard]] PendingMutationResult markStoredMutationApplied(SqliteConnection& connection,
                                                              const QString& mutationId,
                                                              const QString& leaseId,
                                                              const QString& now) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite mutation connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_pending_mutations
SET status = 'applied',
    next_retry_at = NULL,
    lease_id = NULL,
    lease_expires_at = NULL,
    last_error_code = NULL,
    last_error_message = NULL,
    updated_at = ?3,
    applied_at = ?3
WHERE id = ?1 AND status = 'applying' AND lease_id = ?2
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite mutation apply preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindAll(statement,
                                                    {bindText(statement, 1, mutationId),
                                                     bindText(statement, 2, leaseId),
                                                     bindText(statement, 3, now)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite mutation apply failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite mutation apply finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Pending mutation lease is unavailable"));
  }
  return requireStoredMutation(connection, mutationId);
}

[[nodiscard]] PendingMutationResult markStoredMutationFailed(SqliteConnection& connection,
                                                             const MutationFailureInput& input,
                                                             const QString& now) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite mutation connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_pending_mutations
SET status = 'failed',
    attempt_count = attempt_count + 1,
    next_retry_at = ?3,
    lease_id = NULL,
    lease_expires_at = NULL,
    last_error_code = ?4,
    last_error_message = ?5,
    updated_at = ?6
WHERE id = ?1 AND status = 'applying' AND lease_id = ?2
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite mutation failure preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, input.mutationId),
                   bindText(statement, 2, input.leaseId),
                   bindOptionalText(statement, 3, input.nextRetryAt),
                   bindText(statement, 4, input.errorCode),
                   bindText(statement, 5, input.errorMessage),
                   bindText(statement, 6, now)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite mutation failure update failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite mutation failure finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Pending mutation lease is unavailable"));
  }
  return requireStoredMutation(connection, input.mutationId);
}

[[nodiscard]] PendingMutationResult resetStoredMutation(SqliteConnection& connection,
                                                        const QString& mutationId,
                                                        const QString& now,
                                                        bool cancel) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite mutation connection is unavailable"));
  }
  const QByteArray sql = (cancel ? QStringLiteral(R"(
UPDATE local_pending_mutations
SET status = 'cancelled', next_retry_at = NULL, lease_id = NULL, lease_expires_at = NULL,
    updated_at = ?2
WHERE id = ?1 AND status IN ('pending', 'applying', 'failed')
)")
                                 : QStringLiteral(R"(
UPDATE local_pending_mutations
SET status = 'pending', next_retry_at = NULL, lease_id = NULL, lease_expires_at = NULL,
    last_error_code = NULL, last_error_message = NULL, updated_at = ?2
WHERE id = ?1 AND status = 'failed'
)"))
                             .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite mutation state preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(statement, {bindText(statement, 1, mutationId), bindText(statement, 2, now)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite mutation state update failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite mutation state finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(cancel ? QStringLiteral("Pending mutation cannot be cancelled")
                                  : QStringLiteral("Pending mutation cannot be retried"));
  }
  return requireStoredMutation(connection, mutationId);
}

[[nodiscard]] PendingMutationResult rebaseStoredMutation(SqliteConnection& connection,
                                                          const MutationRebaseInput& input,
                                                          const QString& now) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite mutation connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_pending_mutations
SET payload_json = ?3,
    status = 'pending',
    next_retry_at = NULL,
    lease_id = NULL,
    lease_expires_at = NULL,
    last_error_code = NULL,
    last_error_message = NULL,
    updated_at = ?4
WHERE id = ?1 AND (
  status = 'failed' OR (status = 'applying' AND lease_id = ?2)
)
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite mutation rebase preparation failed (%1)"),
                         prepareResult);
  }
  const QString payloadJson =
      QString::fromUtf8(QJsonDocument(input.payload).toJson(QJsonDocument::Compact));
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, input.mutationId),
                   bindOptionalText(statement, 2, input.leaseId),
                   bindText(statement, 3, payloadJson),
                   bindText(statement, 4, now)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite mutation rebase failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite mutation rebase finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Pending mutation cannot be rebased"));
  }
  return requireStoredMutation(connection, input.mutationId);
}

[[nodiscard]] std::optional<AppError> recoverExpiredStoredMutations(SqliteConnection& connection,
                                                                    const QString& now) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite mutation connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_pending_mutations
SET status = 'pending', lease_id = NULL, lease_expires_at = NULL, updated_at = ?1
WHERE status = 'applying' AND lease_expires_at <= ?1
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite mutation recovery preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, now); error.has_value()) {
    sqlite3_finalize(statement);
    return error;
  }
  const int stepResult = sqlite3_step(statement);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite mutation recovery failed (%1)"), stepResult);
  }
  return finalizeResult == SQLITE_OK
             ? std::nullopt
             : std::optional<AppError>(databaseError(
                   QStringLiteral("SQLite mutation recovery finalization failed (%1)"),
                   finalizeResult));
}

} // namespace

OptimisticMutationCoordinator::OptimisticMutationCoordinator(FilePath databasePath,
                                                             const Clock& clock)
    : clock_(clock), writerQueue_(std::move(databasePath)),
      initialization_(writerQueue_
                          .enqueue([&clock](SqliteConnection& connection) -> SqliteWriteResult {
                            const SqliteMigrationRunResultOrError result =
                                LocalSchema::initialize(connection);
                            if (std::holds_alternative<AppError>(result)) {
                              return std::get<AppError>(result);
                            }
                            return recoverExpiredStoredMutations(connection, timestamp(clock));
                          })
                          .share()) {}

std::shared_future<SqliteWriteResult> OptimisticMutationCoordinator::ready() const {
  return initialization_;
}

std::future<PendingMutationResult>
OptimisticMutationCoordinator::enqueue(OptimisticMutationInput input) {
  const std::variant<OptimisticMutationInput, AppError> canonical = canonicalize(std::move(input));
  if (std::holds_alternative<AppError>(canonical)) {
    return readyFuture(PendingMutationResult(std::get<AppError>(canonical)));
  }
  const OptimisticMutationInput storedInput = std::get<OptimisticMutationInput>(canonical);
  const QString mutationId = QStringLiteral("mutation:%1:%2")
                                 .arg(resourceText(storedInput.resource),
                                      QUuid::createUuid().toString(QUuid::WithoutBraces));
  const QString now = timestamp(clock_);
  return writerQueue_.enqueueResult([input = storedInput, mutationId, now](
                                        SqliteConnection& connection) -> PendingMutationResult {
    return enqueueStoredMutation(connection, input, mutationId, now);
  });
}

std::future<PendingMutationLookupResult> OptimisticMutationCoordinator::find(QString mutationId) {
  if (!isValidMutationId(mutationId)) {
    return readyFuture(PendingMutationLookupResult(
        validationError(QStringLiteral("Pending mutation id is invalid"))));
  }
  return writerQueue_.enqueueResult(
      [mutationId =
           std::move(mutationId)](SqliteConnection& connection) -> PendingMutationLookupResult {
        return findStoredMutation(connection, mutationId);
      });
}

std::future<PendingMutationListResult> OptimisticMutationCoordinator::listDue(int limit) {
  const int cappedLimit = std::clamp(limit, 1, kMaximumDueMutations);
  const QString now = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [now, cappedLimit](SqliteConnection& connection) -> PendingMutationListResult {
        return listStoredDueMutations(connection, now, cappedLimit);
      });
}

std::future<PendingMutationListResult> OptimisticMutationCoordinator::listActive(int limit) {
  if (limit < 1 || limit > 1'000) {
    return readyFuture(PendingMutationListResult(
        validationError(QStringLiteral("Active mutation limit is invalid"))));
  }
  return writerQueue_.enqueueResult(
      [limit](SqliteConnection& connection) -> PendingMutationListResult {
        return listStoredActiveMutations(connection, limit);
      });
}

std::future<PendingMutationResult>
OptimisticMutationCoordinator::claim(QString mutationId, std::chrono::seconds leaseDuration) {
  if (!isValidMutationId(mutationId) || leaseDuration <= std::chrono::seconds::zero() ||
      leaseDuration > kMaximumLeaseDuration) {
    return readyFuture(PendingMutationResult(
        validationError(QStringLiteral("Pending mutation claim input is invalid"))));
  }
  const QString leaseId =
      QStringLiteral("lease:%1").arg(QUuid::createUuid().toString(QUuid::WithoutBraces));
  const QString now = timestamp(clock_);
  const QString leaseExpiresAt = timestampAfter(clock_, leaseDuration);
  return writerQueue_.enqueueResult(
      [mutationId = std::move(mutationId), leaseId, now, leaseExpiresAt](
          SqliteConnection& connection) -> PendingMutationResult {
        return claimStoredMutation(connection, mutationId, leaseId, now, leaseExpiresAt);
      });
}

std::future<PendingMutationResult> OptimisticMutationCoordinator::markApplied(QString mutationId,
                                                                              QString leaseId) {
  if (!isValidMutationId(mutationId) || !isValidMutationId(leaseId)) {
    return readyFuture(PendingMutationResult(
        validationError(QStringLiteral("Pending mutation lease input is invalid"))));
  }
  const QString now = timestamp(clock_);
  return writerQueue_.enqueueResult([mutationId = std::move(mutationId),
                                     leaseId = std::move(leaseId),
                                     now](SqliteConnection& connection) -> PendingMutationResult {
    return markStoredMutationApplied(connection, mutationId, leaseId, now);
  });
}

std::future<PendingMutationResult>
OptimisticMutationCoordinator::markFailed(MutationFailureInput input) {
  const std::variant<MutationFailureInput, AppError> canonical = canonicalize(std::move(input));
  if (std::holds_alternative<AppError>(canonical)) {
    return readyFuture(PendingMutationResult(std::get<AppError>(canonical)));
  }
  const MutationFailureInput storedInput = std::get<MutationFailureInput>(canonical);
  const QString now = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [input = storedInput, now](SqliteConnection& connection) -> PendingMutationResult {
        return markStoredMutationFailed(connection, input, now);
      });
}

std::future<PendingMutationResult>
OptimisticMutationCoordinator::rebase(MutationRebaseInput input) {
  const std::variant<MutationRebaseInput, AppError> canonical = canonicalize(std::move(input));
  if (std::holds_alternative<AppError>(canonical)) {
    return readyFuture(PendingMutationResult(std::get<AppError>(canonical)));
  }
  const MutationRebaseInput storedInput = std::get<MutationRebaseInput>(canonical);
  const QString now = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [input = storedInput, now](SqliteConnection& connection) -> PendingMutationResult {
        return rebaseStoredMutation(connection, input, now);
      });
}

std::future<PendingMutationResult> OptimisticMutationCoordinator::retry(QString mutationId) {
  if (!isValidMutationId(mutationId)) {
    return readyFuture(
        PendingMutationResult(validationError(QStringLiteral("Pending mutation id is invalid"))));
  }
  const QString now = timestamp(clock_);
  return writerQueue_.enqueueResult([mutationId = std::move(mutationId),
                                     now](SqliteConnection& connection) -> PendingMutationResult {
    return resetStoredMutation(connection, mutationId, now, false);
  });
}

std::future<PendingMutationResult> OptimisticMutationCoordinator::cancel(QString mutationId) {
  if (!isValidMutationId(mutationId)) {
    return readyFuture(
        PendingMutationResult(validationError(QStringLiteral("Pending mutation id is invalid"))));
  }
  const QString now = timestamp(clock_);
  return writerQueue_.enqueueResult([mutationId = std::move(mutationId),
                                     now](SqliteConnection& connection) -> PendingMutationResult {
    return resetStoredMutation(connection, mutationId, now, true);
  });
}

} // namespace hcb
