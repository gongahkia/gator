#include "core/TaskListMutationService.h"

#include "data/LocalSchema.h"
#include "data/SqliteTransaction.h"
#include "sqlite3.h"

#include <QByteArray>
#include <QDateTime>
#include <QJsonDocument>
#include <QJsonObject>
#include <QList>
#include <QString>
#include <QTimeZone>
#include <QUuid>

#include <chrono>
#include <future>
#include <initializer_list>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumIdentifierLength = 256;
constexpr qsizetype kMaximumTitleLength = 1'024;
constexpr char kConflictMetadataKey[] = "_hcbSync";

struct StoredTaskListContext final {
  QString taskListId;
  QString accountId;
  QString remoteId;
  std::optional<QString> remoteEtag;
  QString title;
};

struct ActiveTaskListMutation final {
  QString id;
  QString operation;
  QJsonObject payload;
};

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

[[nodiscard]] bool isValidIdentifier(const QString& value) {
  return isValidRequiredText(value, kMaximumIdentifierLength) && !value.contains(u'/') &&
         !value.contains(u'\\') && !value.contains(u'?') && !value.contains(u'#');
}

[[nodiscard]] bool isPendingRemoteId(const QString& value) {
  return value.startsWith(QStringLiteral("pending:"));
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
  return result == SQLITE_OK ? std::nullopt
                             : std::optional<AppError>(databaseError(
                                   QStringLiteral("SQLite task-list binding failed (%1)"), result));
}

[[nodiscard]] std::optional<AppError>
bindOptionalText(sqlite3_stmt* statement, int index, const std::optional<QString>& value) {
  if (value.has_value()) {
    return bindText(statement, index, *value);
  }
  const int result = sqlite3_bind_null(statement, index);
  return result == SQLITE_OK ? std::nullopt
                             : std::optional<AppError>(databaseError(
                                   QStringLiteral("SQLite task-list binding failed (%1)"), result));
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

[[nodiscard]] std::variant<std::optional<StoredTaskListContext>, AppError>
readTaskListContext(SqliteConnection& connection, const QString& taskListId) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task-list connection is unavailable"));
  }
  constexpr char sql[] = R"(
SELECT id, account_id, remote_id, etag, title
FROM local_task_lists
WHERE id = ?1 AND deleted_at IS NULL
LIMIT 1
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task-list context preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, taskListId); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult == SQLITE_DONE) {
    const int finalizeResult = sqlite3_finalize(statement);
    return finalizeResult == SQLITE_OK
               ? std::optional<StoredTaskListContext>{}
               : std::variant<std::optional<StoredTaskListContext>, AppError>(databaseError(
                     QStringLiteral("SQLite task-list context finalization failed (%1)"),
                     finalizeResult));
  }
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task-list context lookup failed (%1)"), stepResult);
  }
  const std::optional<QString> storedId = optionalText(statement, 0);
  const std::optional<QString> accountId = optionalText(statement, 1);
  const std::optional<QString> remoteId = optionalText(statement, 2);
  const std::optional<QString> title = optionalText(statement, 4);
  if (!storedId.has_value() || !accountId.has_value() || !remoteId.has_value() ||
      !title.has_value()) {
    sqlite3_finalize(statement);
    return AppError(AppErrorCode::Database, QStringLiteral("Stored task list is invalid"));
  }
  StoredTaskListContext context{.taskListId = *storedId,
                                .accountId = *accountId,
                                .remoteId = *remoteId,
                                .remoteEtag = optionalText(statement, 3),
                                .title = *title};
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK
             ? std::variant<std::optional<StoredTaskListContext>, AppError>(std::move(context))
             : std::variant<std::optional<StoredTaskListContext>, AppError>(databaseError(
                   QStringLiteral("SQLite task-list context finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] QJsonObject taskListSnapshot(const StoredTaskListContext& taskList) {
  return {{QStringLiteral("title"), taskList.title}};
}

[[nodiscard]] QJsonObject taskListPayload(const StoredTaskListContext& taskList,
                                          bool includeRemoteIdentity) {
  QJsonObject payload{
      {QStringLiteral("localTaskListId"), taskList.taskListId},
      {QStringLiteral("taskList"), QJsonObject{{QStringLiteral("title"), taskList.title}}}};
  if (includeRemoteIdentity) {
    payload.insert(QStringLiteral("remoteTaskListId"), taskList.remoteId);
  }
  return payload;
}

[[nodiscard]] QJsonObject deletePayload(const StoredTaskListContext& taskList) {
  return {{QStringLiteral("localTaskListId"), taskList.taskListId},
          {QStringLiteral("remoteTaskListId"), taskList.remoteId}};
}

[[nodiscard]] QJsonObject withConflictMetadata(QJsonObject payload,
                                               QJsonObject baseSnapshot,
                                               const std::optional<QString>& remoteEtag) {
  QJsonObject metadata{{QStringLiteral("base"), std::move(baseSnapshot)}};
  if (remoteEtag.has_value()) {
    metadata.insert(QStringLiteral("etag"), *remoteEtag);
  }
  payload.insert(QString::fromLatin1(kConflictMetadataKey), std::move(metadata));
  return payload;
}

[[nodiscard]] std::variant<std::optional<ActiveTaskListMutation>, AppError>
findActiveTaskListMutation(SqliteConnection& connection, const QString& taskListId) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task-list connection is unavailable"));
  }
  constexpr char sql[] = R"(
SELECT id, operation, payload_json
FROM local_pending_mutations
WHERE resource_type = 'task_list' AND resource_id = ?1 AND status IN ('pending', 'failed')
ORDER BY created_at DESC, id DESC
LIMIT 1
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task-list mutation lookup preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, taskListId); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult == SQLITE_DONE) {
    const int finalizeResult = sqlite3_finalize(statement);
    return finalizeResult == SQLITE_OK
               ? std::optional<ActiveTaskListMutation>{}
               : std::variant<std::optional<ActiveTaskListMutation>, AppError>(databaseError(
                     QStringLiteral("SQLite task-list mutation lookup finalization failed (%1)"),
                     finalizeResult));
  }
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task-list mutation lookup failed (%1)"),
                         stepResult);
  }
  const std::optional<QString> mutationId = optionalText(statement, 0);
  const std::optional<QString> operation = optionalText(statement, 1);
  const std::optional<QString> payloadJson = optionalText(statement, 2);
  QJsonParseError parseError;
  const QJsonDocument payloadDocument =
      payloadJson.has_value() ? QJsonDocument::fromJson(payloadJson->toUtf8(), &parseError)
                              : QJsonDocument();
  if (!mutationId.has_value() || !operation.has_value() || !payloadJson.has_value() ||
      parseError.error != QJsonParseError::NoError || !payloadDocument.isObject()) {
    sqlite3_finalize(statement);
    return AppError(AppErrorCode::Database, QStringLiteral("Stored task-list mutation is invalid"));
  }
  ActiveTaskListMutation mutation{
      .id = *mutationId, .operation = *operation, .payload = payloadDocument.object()};
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK
             ? std::variant<std::optional<ActiveTaskListMutation>, AppError>(std::move(mutation))
             : std::variant<std::optional<ActiveTaskListMutation>, AppError>(databaseError(
                   QStringLiteral("SQLite task-list mutation lookup finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] std::optional<AppError>
replaceActiveTaskListMutation(SqliteConnection& connection,
                              const ActiveTaskListMutation& mutation,
                              QString operation,
                              QJsonObject payload,
                              const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task-list connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_pending_mutations
SET operation = ?2, payload_json = ?3, status = 'pending', next_retry_at = NULL,
    last_error_code = NULL, last_error_message = NULL, updated_at = ?4
WHERE id = ?1 AND status IN ('pending', 'failed')
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(
        QStringLiteral("SQLite task-list mutation replacement preparation failed (%1)"),
        prepareResult);
  }
  const QString payloadJson =
      QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
  if (const std::optional<AppError> error = bindAll(statement,
                                                    {bindText(statement, 1, mutation.id),
                                                     bindText(statement, 2, operation),
                                                     bindText(statement, 3, payloadJson),
                                                     bindText(statement, 4, updatedAt)});
      error.has_value()) {
    return error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task-list mutation replacement failed (%1)"),
                         stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(
        QStringLiteral("SQLite task-list mutation replacement finalization failed (%1)"),
        finalizeResult);
  }
  return changedRows == 1 ? std::nullopt
                          : std::optional<AppError>(AppError(
                                AppErrorCode::Database,
                                QStringLiteral("Active task-list mutation was not replaced")));
}

[[nodiscard]] std::optional<AppError>
removeActiveTaskListMutation(SqliteConnection& connection, const ActiveTaskListMutation& mutation) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task-list connection is unavailable"));
  }
  constexpr char sql[] = R"(
DELETE FROM local_pending_mutations
WHERE id = ?1 AND status IN ('pending', 'failed')
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(
        QStringLiteral("SQLite task-list mutation removal preparation failed (%1)"), prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, mutation.id);
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task-list mutation removal failed (%1)"),
                         stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(
        QStringLiteral("SQLite task-list mutation removal finalization failed (%1)"),
        finalizeResult);
  }
  return changedRows == 1 ? std::nullopt
                          : std::optional<AppError>(AppError(
                                AppErrorCode::Database,
                                QStringLiteral("Active task-list mutation was not removed")));
}

[[nodiscard]] std::optional<AppError> insertTaskListMutation(SqliteConnection& connection,
                                                             const StoredTaskListContext& taskList,
                                                             QString operation,
                                                             QJsonObject payload,
                                                             const QString& createdAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task-list connection is unavailable"));
  }
  constexpr char sql[] = R"(
INSERT INTO local_pending_mutations (
  id, account_id, resource_type, resource_id, operation, payload_json, status, attempt_count,
  created_at, updated_at
) VALUES (?1, ?2, 'task_list', ?3, ?4, ?5, 'pending', 0, ?6, ?6)
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(
        QStringLiteral("SQLite task-list mutation enqueue preparation failed (%1)"), prepareResult);
  }
  const QString mutationId =
      QStringLiteral("mutation:task-list:") + QUuid::createUuid().toString(QUuid::WithoutBraces);
  const QString payloadJson =
      QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
  if (const std::optional<AppError> error = bindAll(statement,
                                                    {bindText(statement, 1, mutationId),
                                                     bindText(statement, 2, taskList.accountId),
                                                     bindText(statement, 3, taskList.taskListId),
                                                     bindText(statement, 4, operation),
                                                     bindText(statement, 5, payloadJson),
                                                     bindText(statement, 6, createdAt)});
      error.has_value()) {
    return error;
  }
  const int stepResult = sqlite3_step(statement);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task-list mutation enqueue failed (%1)"),
                         stepResult);
  }
  return finalizeResult == SQLITE_OK
             ? std::nullopt
             : std::optional<AppError>(databaseError(
                   QStringLiteral("SQLite task-list mutation enqueue finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] std::optional<AppError>
queueTaskListMutation(SqliteConnection& connection,
                      const StoredTaskListContext& before,
                      const std::optional<StoredTaskListContext>& after,
                      QString operation,
                      const QString& updatedAt) {
  const std::variant<std::optional<ActiveTaskListMutation>, AppError> activeResult =
      findActiveTaskListMutation(connection, before.taskListId);
  if (std::holds_alternative<AppError>(activeResult)) {
    return std::get<AppError>(activeResult);
  }
  const std::optional<ActiveTaskListMutation>& active =
      std::get<std::optional<ActiveTaskListMutation>>(activeResult);
  const bool deleting = operation == QStringLiteral("task_list.delete");
  if (active.has_value()) {
    const QJsonValue metadata = active->payload.value(QString::fromLatin1(kConflictMetadataKey));
    if (!metadata.isObject()) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Stored task-list mutation is invalid"));
    }
    if (active->operation == QStringLiteral("task_list.create")) {
      if (deleting) {
        return removeActiveTaskListMutation(connection, *active);
      }
      if (!after.has_value()) {
        return AppError(AppErrorCode::Database, QStringLiteral("Updated task list is unavailable"));
      }
      QJsonObject payload = taskListPayload(*after, false);
      payload.insert(QString::fromLatin1(kConflictMetadataKey), metadata);
      return replaceActiveTaskListMutation(
          connection, *active, QStringLiteral("task_list.create"), std::move(payload), updatedAt);
    }
    if (active->operation == QStringLiteral("task_list.update")) {
      QJsonObject payload = deleting ? deletePayload(before) : taskListPayload(*after, true);
      payload.insert(QString::fromLatin1(kConflictMetadataKey), metadata);
      return replaceActiveTaskListMutation(
          connection, *active, std::move(operation), std::move(payload), updatedAt);
    }
    if (active->operation == QStringLiteral("task_list.delete")) {
      return std::nullopt;
    }
  }
  QJsonObject payload =
      deleting ? deletePayload(before)
               : taskListPayload(*after, operation != QStringLiteral("task_list.create"));
  payload = withConflictMetadata(
      std::move(payload),
      operation == QStringLiteral("task_list.create") ? QJsonObject() : taskListSnapshot(before),
      operation == QStringLiteral("task_list.create") ? std::optional<QString>{}
                                                      : before.remoteEtag);
  return insertTaskListMutation(
      connection, before, std::move(operation), std::move(payload), updatedAt);
}

[[nodiscard]] std::variant<TaskListCreateInput, AppError> canonicalize(TaskListCreateInput input) {
  input.title = input.title.trimmed();
  if (!isValidIdentifier(input.accountId) ||
      !isValidRequiredText(input.title, kMaximumTitleLength)) {
    return validationError(QStringLiteral("Task-list create input is invalid"));
  }
  return input;
}

[[nodiscard]] std::variant<TaskListUpdateInput, AppError> canonicalize(TaskListUpdateInput input) {
  input.title = input.title.trimmed();
  if (!isValidIdentifier(input.taskListId) ||
      !isValidRequiredText(input.title, kMaximumTitleLength)) {
    return validationError(QStringLiteral("Task-list update input is invalid"));
  }
  return input;
}

[[nodiscard]] TaskListMutationResult createStoredTaskList(SqliteConnection& connection,
                                                          const TaskListCreateInput& input,
                                                          const QString& taskListId,
                                                          const QString& remoteId,
                                                          const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task-list connection is unavailable"));
  }
  constexpr char sql[] = R"(
INSERT INTO local_task_lists (
  id, account_id, remote_id, title, sort_order, is_selected, created_at, updated_at
)
SELECT ?1, accounts.id, ?3, ?4,
       COALESCE((SELECT MAX(sort_order) + 1 FROM local_task_lists
                 WHERE account_id = accounts.id AND deleted_at IS NULL), 0),
       1, ?5, ?5
FROM local_accounts AS accounts
WHERE accounts.id = ?2 AND accounts.provider = 'google' AND accounts.deleted_at IS NULL
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task-list create preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindAll(statement,
                                                    {bindText(statement, 1, taskListId),
                                                     bindText(statement, 2, input.accountId),
                                                     bindText(statement, 3, remoteId),
                                                     bindText(statement, 4, input.title),
                                                     bindText(statement, 5, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task-list create failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task-list create finalization failed (%1)"),
                         finalizeResult);
  }
  return changedRows == 1 ? TaskListMutationResult(TaskListMutationReceipt{.taskListId = taskListId,
                                                                           .updatedAt = updatedAt})
                          : TaskListMutationResult(validationError(QStringLiteral(
                                "Google account is unavailable for task-list creation")));
}

[[nodiscard]] TaskListMutationResult updateStoredTaskList(SqliteConnection& connection,
                                                          const TaskListUpdateInput& input,
                                                          const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task-list connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_task_lists
SET title = ?2, updated_at = ?3
WHERE id = ?1 AND deleted_at IS NULL
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task-list update preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindAll(statement,
                                                    {bindText(statement, 1, input.taskListId),
                                                     bindText(statement, 2, input.title),
                                                     bindText(statement, 3, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task-list update failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task-list update finalization failed (%1)"),
                         finalizeResult);
  }
  return changedRows == 1
             ? TaskListMutationResult(
                   TaskListMutationReceipt{.taskListId = input.taskListId, .updatedAt = updatedAt})
             : TaskListMutationResult(
                   validationError(QStringLiteral("Task list is unavailable for update")));
}

[[nodiscard]] TaskListMutationResult setStoredTaskListSelected(
    SqliteConnection& connection, const TaskListSelectionInput& input, const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task-list connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_task_lists
SET is_selected = ?2, updated_at = ?3
WHERE id = ?1 AND deleted_at IS NULL
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task-list selection preparation failed (%1)"),
                         prepareResult);
  }
  const int selected = input.selected ? 1 : 0;
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, input.taskListId),
                   sqlite3_bind_int(statement, 2, selected) == SQLITE_OK
                       ? std::optional<AppError>{}
                       : std::optional<AppError>(AppError(
                             AppErrorCode::Database,
                             QStringLiteral("SQLite task-list selection binding failed"))),
                   bindText(statement, 3, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task-list selection failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task-list selection finalization failed (%1)"),
                         finalizeResult);
  }
  return changedRows == 1
             ? TaskListMutationResult(TaskListMutationReceipt{.taskListId = input.taskListId,
                                                               .updatedAt = updatedAt})
             : TaskListMutationResult(
                   validationError(QStringLiteral("Task list is unavailable for selection")));
}

[[nodiscard]] std::variant<bool, AppError> hasApplyingTaskMutation(SqliteConnection& connection,
                                                                   const QString& taskListId) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task-list connection is unavailable"));
  }
  constexpr char sql[] = R"(
SELECT EXISTS(
  SELECT 1
  FROM local_pending_mutations AS mutations
  INNER JOIN local_tasks AS tasks ON tasks.id = mutations.resource_id
  WHERE mutations.resource_type = 'task' AND mutations.status = 'applying'
    AND tasks.task_list_id = ?1
)
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(
        QStringLiteral("SQLite task-list applying-mutation lookup preparation failed (%1)"),
        prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, taskListId); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const bool applying = stepResult == SQLITE_ROW && sqlite3_column_int(statement, 0) != 0;
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_ROW) {
    return databaseError(QStringLiteral("SQLite task-list applying-mutation lookup failed (%1)"),
                         stepResult);
  }
  return finalizeResult == SQLITE_OK
             ? std::variant<bool, AppError>(applying)
             : std::variant<bool, AppError>(databaseError(
                   QStringLiteral(
                       "SQLite task-list applying-mutation lookup finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] std::optional<AppError> cancelQueuedTaskMutations(SqliteConnection& connection,
                                                                const QString& taskListId,
                                                                const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task-list connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_pending_mutations
SET status = 'cancelled', next_retry_at = NULL, lease_id = NULL, lease_expires_at = NULL,
    last_error_code = 'list_deleted', last_error_message = 'Task list was deleted locally',
    updated_at = ?2
WHERE resource_type = 'task' AND status IN ('pending', 'failed')
  AND resource_id IN (SELECT id FROM local_tasks WHERE task_list_id = ?1)
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task-list cancellation preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindAll(
          statement, {bindText(statement, 1, taskListId), bindText(statement, 2, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task-list cancellation failed (%1)"), stepResult);
  }
  return finalizeResult == SQLITE_OK
             ? std::nullopt
             : std::optional<AppError>(databaseError(
                   QStringLiteral("SQLite task-list cancellation finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] TaskListMutationResult removeStoredTaskList(SqliteConnection& connection,
                                                          const QString& taskListId,
                                                          const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task-list connection is unavailable"));
  }
  constexpr char taskSql[] = R"(
UPDATE local_tasks
SET deleted_at = ?2, updated_at = ?2
WHERE task_list_id = ?1 AND deleted_at IS NULL
)";
  sqlite3_stmt* taskStatement = nullptr;
  const int taskPrepareResult =
      sqlite3_prepare_v3(handle, taskSql, -1, SQLITE_PREPARE_PERSISTENT, &taskStatement, nullptr);
  if (taskPrepareResult != SQLITE_OK) {
    sqlite3_finalize(taskStatement);
    return databaseError(QStringLiteral("SQLite task-list task deletion preparation failed (%1)"),
                         taskPrepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(taskStatement,
                  {bindText(taskStatement, 1, taskListId), bindText(taskStatement, 2, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int taskStepResult = sqlite3_step(taskStatement);
  const int taskFinalizeResult = sqlite3_finalize(taskStatement);
  if (taskStepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task-list task deletion failed (%1)"),
                         taskStepResult);
  }
  if (taskFinalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task-list task deletion finalization failed (%1)"),
                         taskFinalizeResult);
  }
  constexpr char listSql[] = R"(
UPDATE local_task_lists
SET deleted_at = ?2, updated_at = ?2
WHERE id = ?1 AND deleted_at IS NULL
)";
  sqlite3_stmt* listStatement = nullptr;
  const int listPrepareResult =
      sqlite3_prepare_v3(handle, listSql, -1, SQLITE_PREPARE_PERSISTENT, &listStatement, nullptr);
  if (listPrepareResult != SQLITE_OK) {
    sqlite3_finalize(listStatement);
    return databaseError(QStringLiteral("SQLite task-list deletion preparation failed (%1)"),
                         listPrepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(listStatement,
                  {bindText(listStatement, 1, taskListId), bindText(listStatement, 2, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int listStepResult = sqlite3_step(listStatement);
  const int listChangedRows = sqlite3_changes(handle);
  const int listFinalizeResult = sqlite3_finalize(listStatement);
  if (listStepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task-list deletion failed (%1)"), listStepResult);
  }
  if (listFinalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task-list deletion finalization failed (%1)"),
                         listFinalizeResult);
  }
  return listChangedRows == 1
             ? TaskListMutationResult(
                   TaskListMutationReceipt{.taskListId = taskListId, .updatedAt = updatedAt})
             : TaskListMutationResult(
                   validationError(QStringLiteral("Task list is unavailable for deletion")));
}

[[nodiscard]] TaskListMutationResult
reconcileStoredGoogleTaskList(SqliteConnection& connection,
                              const TaskListRemoteReconciliationInput& input,
                              const QString& updatedAt) {
  SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
  if (std::holds_alternative<AppError>(transactionResult)) {
    return std::get<AppError>(std::move(transactionResult));
  }
  SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task-list connection is unavailable"));
  }
  constexpr char listSql[] = R"(
UPDATE local_task_lists
SET remote_id = CASE WHEN remote_id LIKE 'pending:%' THEN ?2 ELSE remote_id END,
    etag = COALESCE(?3, etag), updated_at = ?4
WHERE id = ?1 AND (remote_id = ?2 OR remote_id LIKE 'pending:%')
)";
  sqlite3_stmt* listStatement = nullptr;
  const int listPrepareResult =
      sqlite3_prepare_v3(handle, listSql, -1, SQLITE_PREPARE_PERSISTENT, &listStatement, nullptr);
  if (listPrepareResult != SQLITE_OK) {
    sqlite3_finalize(listStatement);
    return databaseError(QStringLiteral("SQLite task-list reconciliation preparation failed (%1)"),
                         listPrepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(listStatement,
                  {bindText(listStatement, 1, input.localTaskListId),
                   bindText(listStatement, 2, input.remoteTaskListId),
                   bindOptionalText(listStatement, 3, input.remoteEtag),
                   bindText(listStatement, 4, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int listStepResult = sqlite3_step(listStatement);
  const int listChangedRows = sqlite3_changes(handle);
  const int listFinalizeResult = sqlite3_finalize(listStatement);
  if (listStepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task-list reconciliation failed (%1)"),
                         listStepResult);
  }
  if (listFinalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task-list reconciliation finalization failed (%1)"),
                         listFinalizeResult);
  }
  if (listChangedRows != 1) {
    return validationError(QStringLiteral("Task list is unavailable for Google reconciliation"));
  }
  constexpr char pendingSql[] = R"(
SELECT id, payload_json
FROM local_pending_mutations
WHERE resource_type = 'task_list' AND resource_id = ?1 AND status IN ('pending', 'failed')
)";
  sqlite3_stmt* pendingStatement = nullptr;
  const int pendingPrepareResult = sqlite3_prepare_v3(
      handle, pendingSql, -1, SQLITE_PREPARE_PERSISTENT, &pendingStatement, nullptr);
  if (pendingPrepareResult != SQLITE_OK) {
    sqlite3_finalize(pendingStatement);
    return databaseError(
        QStringLiteral("SQLite pending task-list reconciliation preparation failed (%1)"),
        pendingPrepareResult);
  }
  if (const std::optional<AppError> error = bindText(pendingStatement, 1, input.localTaskListId);
      error.has_value()) {
    sqlite3_finalize(pendingStatement);
    return *error;
  }
  struct PendingPayload final {
    QString mutationId;
    QJsonObject payload;
  };
  QList<PendingPayload> pendingPayloads;
  int pendingStepResult = SQLITE_ROW;
  while ((pendingStepResult = sqlite3_step(pendingStatement)) == SQLITE_ROW) {
    const std::optional<QString> mutationId = optionalText(pendingStatement, 0);
    const std::optional<QString> payloadJson = optionalText(pendingStatement, 1);
    QJsonParseError parseError;
    const QJsonDocument document = payloadJson.has_value()
                                       ? QJsonDocument::fromJson(payloadJson->toUtf8(), &parseError)
                                       : QJsonDocument();
    if (!mutationId.has_value() || !payloadJson.has_value() ||
        parseError.error != QJsonParseError::NoError || !document.isObject()) {
      sqlite3_finalize(pendingStatement);
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Stored pending task-list mutation is invalid"));
    }
    QJsonObject payload = document.object();
    const QJsonValue remoteId = payload.value(QStringLiteral("remoteTaskListId"));
    const bool usesPendingRemoteId = remoteId.isString() && isPendingRemoteId(remoteId.toString());
    const bool usesReconciledRemoteId =
        remoteId.isString() && remoteId.toString() == input.remoteTaskListId;
    if (usesPendingRemoteId || usesReconciledRemoteId) {
      if (usesPendingRemoteId) {
        payload.insert(QStringLiteral("remoteTaskListId"), input.remoteTaskListId);
      }
      if (input.remoteEtag.has_value()) {
        const QJsonValue metadataValue = payload.value(QString::fromLatin1(kConflictMetadataKey));
        if (!metadataValue.isObject()) {
          sqlite3_finalize(pendingStatement);
          return AppError(AppErrorCode::Database,
                          QStringLiteral("Stored pending task-list mutation is invalid"));
        }
        QJsonObject metadata = metadataValue.toObject();
        metadata.insert(QStringLiteral("etag"), *input.remoteEtag);
        payload.insert(QString::fromLatin1(kConflictMetadataKey), std::move(metadata));
      }
      pendingPayloads.append({.mutationId = *mutationId, .payload = std::move(payload)});
    }
  }
  const int pendingFinalizeResult = sqlite3_finalize(pendingStatement);
  if (pendingStepResult != SQLITE_DONE) {
    return databaseError(
        QStringLiteral("SQLite pending task-list reconciliation lookup failed (%1)"),
        pendingStepResult);
  }
  if (pendingFinalizeResult != SQLITE_OK) {
    return databaseError(
        QStringLiteral("SQLite pending task-list reconciliation lookup finalization failed (%1)"),
        pendingFinalizeResult);
  }
  constexpr char updateSql[] = R"(
UPDATE local_pending_mutations
SET payload_json = ?2, updated_at = ?3
WHERE id = ?1 AND status IN ('pending', 'failed')
)";
  for (const PendingPayload& pending : pendingPayloads) {
    sqlite3_stmt* updateStatement = nullptr;
    const int updatePrepareResult = sqlite3_prepare_v3(
        handle, updateSql, -1, SQLITE_PREPARE_PERSISTENT, &updateStatement, nullptr);
    if (updatePrepareResult != SQLITE_OK) {
      sqlite3_finalize(updateStatement);
      return databaseError(
          QStringLiteral("SQLite pending task-list reconciliation update preparation failed (%1)"),
          updatePrepareResult);
    }
    const QString payloadJson =
        QString::fromUtf8(QJsonDocument(pending.payload).toJson(QJsonDocument::Compact));
    if (const std::optional<AppError> error =
            bindAll(updateStatement,
                    {bindText(updateStatement, 1, pending.mutationId),
                     bindText(updateStatement, 2, payloadJson),
                     bindText(updateStatement, 3, updatedAt)});
        error.has_value()) {
      return *error;
    }
    const int updateStepResult = sqlite3_step(updateStatement);
    const int updateChangedRows = sqlite3_changes(handle);
    const int updateFinalizeResult = sqlite3_finalize(updateStatement);
    if (updateStepResult != SQLITE_DONE) {
      return databaseError(
          QStringLiteral("SQLite pending task-list reconciliation update failed (%1)"),
          updateStepResult);
    }
    if (updateFinalizeResult != SQLITE_OK) {
      return databaseError(
          QStringLiteral("SQLite pending task-list reconciliation update finalization failed (%1)"),
          updateFinalizeResult);
    }
    if (updateChangedRows != 1) {
      return AppError(
          AppErrorCode::Database,
          QStringLiteral("Pending task-list mutation was unavailable for reconciliation"));
    }
  }
  if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
    return *error;
  }
  return TaskListMutationReceipt{.taskListId = input.localTaskListId, .updatedAt = updatedAt};
}

} // namespace

TaskListMutationService::TaskListMutationService(FilePath databasePath, const Clock& clock)
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

std::shared_future<SqliteWriteResult> TaskListMutationService::ready() const {
  return initialization_;
}

std::future<TaskListMutationResult> TaskListMutationService::create(TaskListCreateInput input) {
  const std::variant<TaskListCreateInput, AppError> canonical = canonicalize(std::move(input));
  if (std::holds_alternative<AppError>(canonical)) {
    return readyFuture(TaskListMutationResult(std::get<AppError>(canonical)));
  }
  const QString localId = QUuid::createUuid().toString(QUuid::WithoutBraces);
  const QString taskListId = QStringLiteral("task-list:") + localId;
  const QString remoteId = QStringLiteral("pending:") + localId;
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [input = std::get<TaskListCreateInput>(canonical), taskListId, remoteId, updatedAt](
          SqliteConnection& connection) {
        SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
        if (std::holds_alternative<AppError>(transactionResult)) {
          return TaskListMutationResult(std::get<AppError>(std::move(transactionResult)));
        }
        SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
        TaskListMutationResult created =
            createStoredTaskList(connection, input, taskListId, remoteId, updatedAt);
        if (std::holds_alternative<AppError>(created)) {
          return created;
        }
        const std::variant<std::optional<StoredTaskListContext>, AppError> contextResult =
            readTaskListContext(connection, taskListId);
        if (std::holds_alternative<AppError>(contextResult)) {
          return TaskListMutationResult(std::get<AppError>(contextResult));
        }
        const std::optional<StoredTaskListContext>& context =
            std::get<std::optional<StoredTaskListContext>>(contextResult);
        if (!context.has_value()) {
          return TaskListMutationResult(
              AppError(AppErrorCode::Database, QStringLiteral("Created task list is unavailable")));
        }
        if (const std::optional<AppError> error = queueTaskListMutation(
                connection, *context, context, QStringLiteral("task_list.create"), updatedAt);
            error.has_value()) {
          return TaskListMutationResult(*error);
        }
        if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
          return TaskListMutationResult(*error);
        }
        return created;
      });
}

std::future<TaskListMutationResult> TaskListMutationService::update(TaskListUpdateInput input) {
  const std::variant<TaskListUpdateInput, AppError> canonical = canonicalize(std::move(input));
  if (std::holds_alternative<AppError>(canonical)) {
    return readyFuture(TaskListMutationResult(std::get<AppError>(canonical)));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [input = std::get<TaskListUpdateInput>(canonical), updatedAt](SqliteConnection& connection) {
        SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
        if (std::holds_alternative<AppError>(transactionResult)) {
          return TaskListMutationResult(std::get<AppError>(std::move(transactionResult)));
        }
        SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
        const std::variant<std::optional<StoredTaskListContext>, AppError> beforeResult =
            readTaskListContext(connection, input.taskListId);
        if (std::holds_alternative<AppError>(beforeResult)) {
          return TaskListMutationResult(std::get<AppError>(beforeResult));
        }
        const std::optional<StoredTaskListContext>& before =
            std::get<std::optional<StoredTaskListContext>>(beforeResult);
        if (!before.has_value()) {
          return TaskListMutationResult(
              validationError(QStringLiteral("Task list is unavailable for update")));
        }
        TaskListMutationResult updated = updateStoredTaskList(connection, input, updatedAt);
        if (std::holds_alternative<AppError>(updated)) {
          return updated;
        }
        const std::variant<std::optional<StoredTaskListContext>, AppError> afterResult =
            readTaskListContext(connection, input.taskListId);
        if (std::holds_alternative<AppError>(afterResult)) {
          return TaskListMutationResult(std::get<AppError>(afterResult));
        }
        const std::optional<StoredTaskListContext>& after =
            std::get<std::optional<StoredTaskListContext>>(afterResult);
        if (!after.has_value()) {
          return TaskListMutationResult(
              AppError(AppErrorCode::Database, QStringLiteral("Updated task list is unavailable")));
        }
        if (const std::optional<AppError> error = queueTaskListMutation(
                connection, *before, after, QStringLiteral("task_list.update"), updatedAt);
            error.has_value()) {
          return TaskListMutationResult(*error);
        }
        if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
          return TaskListMutationResult(*error);
        }
        return updated;
      });
}

std::future<TaskListMutationResult>
TaskListMutationService::setSelected(TaskListSelectionInput input) {
  if (!isValidIdentifier(input.taskListId)) {
    return readyFuture(TaskListMutationResult(
        validationError(QStringLiteral("Task-list selection input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [input = std::move(input), updatedAt](SqliteConnection& connection) {
        return setStoredTaskListSelected(connection, input, updatedAt);
      });
}

std::future<TaskListMutationResult> TaskListMutationService::remove(QString taskListId) {
  if (!isValidIdentifier(taskListId)) {
    return readyFuture(TaskListMutationResult(
        validationError(QStringLiteral("Task-list deletion input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [taskListId = std::move(taskListId), updatedAt](SqliteConnection& connection) {
        SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
        if (std::holds_alternative<AppError>(transactionResult)) {
          return TaskListMutationResult(std::get<AppError>(std::move(transactionResult)));
        }
        SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
        const std::variant<std::optional<StoredTaskListContext>, AppError> beforeResult =
            readTaskListContext(connection, taskListId);
        if (std::holds_alternative<AppError>(beforeResult)) {
          return TaskListMutationResult(std::get<AppError>(beforeResult));
        }
        const std::optional<StoredTaskListContext>& before =
            std::get<std::optional<StoredTaskListContext>>(beforeResult);
        if (!before.has_value()) {
          return TaskListMutationResult(
              validationError(QStringLiteral("Task list is unavailable for deletion")));
        }
        const std::variant<bool, AppError> applyingResult =
            hasApplyingTaskMutation(connection, taskListId);
        if (std::holds_alternative<AppError>(applyingResult)) {
          return TaskListMutationResult(std::get<AppError>(applyingResult));
        }
        if (std::get<bool>(applyingResult)) {
          return TaskListMutationResult(validationError(
              QStringLiteral("Task list has a task mutation in progress; retry deletion shortly")));
        }
        if (const std::optional<AppError> error =
                cancelQueuedTaskMutations(connection, taskListId, updatedAt);
            error.has_value()) {
          return TaskListMutationResult(*error);
        }
        TaskListMutationResult removed = removeStoredTaskList(connection, taskListId, updatedAt);
        if (std::holds_alternative<AppError>(removed)) {
          return removed;
        }
        if (const std::optional<AppError> error = queueTaskListMutation(
                connection, *before, std::nullopt, QStringLiteral("task_list.delete"), updatedAt);
            error.has_value()) {
          return TaskListMutationResult(*error);
        }
        if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
          return TaskListMutationResult(*error);
        }
        return removed;
      });
}

std::future<TaskListMutationResult>
TaskListMutationService::reconcileGoogleTaskList(TaskListRemoteReconciliationInput input) {
  if (!isValidIdentifier(input.localTaskListId) || !isValidIdentifier(input.remoteTaskListId) ||
      isPendingRemoteId(input.remoteTaskListId) ||
      (input.remoteEtag.has_value() &&
       (!isValidRequiredText(*input.remoteEtag, 4'096) || input.remoteEtag->contains(u'\r') ||
        input.remoteEtag->contains(u'\n')))) {
    return readyFuture(TaskListMutationResult(
        validationError(QStringLiteral("Google task-list reconciliation input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [input = std::move(input), updatedAt](SqliteConnection& connection) {
        return reconcileStoredGoogleTaskList(connection, input, updatedAt);
      });
}

std::future<TaskListRemoteIdResult> TaskListMutationService::remoteTaskListId(QString taskListId) {
  if (!isValidIdentifier(taskListId)) {
    return readyFuture(TaskListRemoteIdResult(
        validationError(QStringLiteral("Task-list remote ID lookup input is invalid"))));
  }
  return writerQueue_.enqueueResult([taskListId =
                                         std::move(taskListId)](SqliteConnection& connection) {
    const std::variant<std::optional<StoredTaskListContext>, AppError> contextResult =
        readTaskListContext(connection, taskListId);
    if (std::holds_alternative<AppError>(contextResult)) {
      return TaskListRemoteIdResult(std::get<AppError>(contextResult));
    }
    const std::optional<StoredTaskListContext>& context =
        std::get<std::optional<StoredTaskListContext>>(contextResult);
    return context.has_value() ? TaskListRemoteIdResult(std::optional<QString>(context->remoteId))
                               : TaskListRemoteIdResult(std::optional<QString>{});
  });
}

} // namespace hcb
