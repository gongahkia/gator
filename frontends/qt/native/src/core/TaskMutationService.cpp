#include "core/TaskMutationService.h"

#include "core/TaskRecurrenceMarker.h"
#include "data/LocalSchema.h"
#include "data/SqliteTransaction.h"
#include "sqlite3.h"

#include <QByteArray>
#include <QDate>
#include <QDateTime>
#include <QHash>
#include <QJsonDocument>
#include <QJsonObject>
#include <QList>
#include <QSet>
#include <QString>
#include <QTime>
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
constexpr qsizetype kMaximumTitleLength = 500;
constexpr qsizetype kMaximumNotesLength = 10'000;
constexpr qsizetype kMaximumTimestampLength = 64;
constexpr qsizetype kMaximumTimeZoneLength = 128;
constexpr char kConflictMetadataKey[] = "_hcbSync";
constexpr char kDivergentDuplicateDiagnostic[] =
    "Managed recurrence has divergent duplicate occurrences in Google Tasks";

struct StoredTaskContext final {
  QString taskId;
  QString taskListId;
  QString accountId;
  QString taskListRemoteId;
  QString remoteId;
  std::optional<QString> remoteEtag;
  std::optional<QString> parentTaskId;
  std::optional<QString> parentRemoteId;
  QString title;
  std::optional<QString> notes;
  QString state;
  std::optional<QString> completedAt;
  std::optional<QString> dueAt;
  std::optional<QString> dueTimeZone;
  QString priority;
  bool isHidden{false};
  bool isAssigned{false};
  std::optional<QString> recurrenceDiagnostic;
};

struct ActiveTaskMutation final {
  QString id;
  QString operation;
  QJsonObject payload;
};

struct TaskPositionReference final {
  std::optional<QString> previousRemoteId;
  std::optional<QString> previousLocalId;
};

struct TaskRecurrenceClaim final {
  QString successorTaskId;
};

struct StoredTaskSibling final {
  QString taskId;
  QString remoteId;
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

[[nodiscard]] bool isValidNotes(const std::optional<QString>& value) {
  return !value.has_value() ||
         (value->size() <= kMaximumNotesLength && !value->contains(QChar::Null));
}

[[nodiscard]] bool isValidTimestamp(const QString& value) {
  return isValidRequiredText(value, kMaximumTimestampLength) &&
         QDateTime::fromString(value, Qt::ISODate).isValid();
}

[[nodiscard]] bool isValidDue(const std::optional<TaskDue>& due) {
  if (!due.has_value()) {
    return true;
  }
  if (!due->at.has_value()) {
    return !due->timeZone.has_value();
  }
  if (!isValidTimestamp(*due->at)) {
    return false;
  }
  if (!due->timeZone.has_value()) {
    return true;
  }
  return isValidRequiredText(*due->timeZone, kMaximumTimeZoneLength) &&
         QTimeZone(due->timeZone->toUtf8()).isValid();
}

[[nodiscard]] QString priorityText(TaskPriority priority) {
  switch (priority) {
  case TaskPriority::None:
    return QStringLiteral("none");
  case TaskPriority::Low:
    return QStringLiteral("low");
  case TaskPriority::Medium:
    return QStringLiteral("medium");
  case TaskPriority::High:
    return QStringLiteral("high");
  }
  return {};
}

[[nodiscard]] bool isValidPriority(TaskPriority priority) {
  return !priorityText(priority).isEmpty();
}

[[nodiscard]] std::optional<TaskPriority> priorityFromText(const QString& value) {
  if (value == QStringLiteral("none")) {
    return TaskPriority::None;
  }
  if (value == QStringLiteral("low")) {
    return TaskPriority::Low;
  }
  if (value == QStringLiteral("medium")) {
    return TaskPriority::Medium;
  }
  if (value == QStringLiteral("high")) {
    return TaskPriority::High;
  }
  return std::nullopt;
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
  if (result != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task binding failed (%1)"), result);
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<AppError>
bindOptionalText(sqlite3_stmt* statement, int index, const std::optional<QString>& value) {
  if (!value.has_value()) {
    const int result = sqlite3_bind_null(statement, index);
    if (result != SQLITE_OK) {
      return databaseError(QStringLiteral("SQLite task binding failed (%1)"), result);
    }
    return std::nullopt;
  }
  return bindText(statement, index, *value);
}

[[nodiscard]] std::optional<AppError> bindInteger(sqlite3_stmt* statement, int index, int value) {
  const int result = sqlite3_bind_int(statement, index, value);
  if (result != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task binding failed (%1)"), result);
  }
  return std::nullopt;
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

[[nodiscard]] std::variant<std::optional<StoredTaskContext>, AppError>
readTaskContext(SqliteConnection& connection, const QString& taskId) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
SELECT tasks.id, tasks.task_list_id, lists.account_id, lists.remote_id, tasks.remote_id, tasks.etag,
       tasks.parent_task_id, parent.remote_id, tasks.title, tasks.notes, tasks.state, tasks.due_at,
       tasks.completed_at, tasks.due_time_zone, tasks.priority, tasks.is_hidden, tasks.is_assigned,
       tasks.recurrence_diagnostic
FROM local_tasks AS tasks
INNER JOIN local_task_lists AS lists ON lists.id = tasks.task_list_id
LEFT JOIN local_tasks AS parent ON parent.id = tasks.parent_task_id
WHERE tasks.id = ?1 AND tasks.deleted_at IS NULL AND lists.deleted_at IS NULL
LIMIT 1
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task context preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, taskId); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult == SQLITE_DONE) {
    const int finalizeResult = sqlite3_finalize(statement);
    return finalizeResult == SQLITE_OK
               ? std::optional<StoredTaskContext>{}
               : std::variant<std::optional<StoredTaskContext>, AppError>(
                     databaseError(QStringLiteral("SQLite task context finalization failed (%1)"),
                                   finalizeResult));
  }
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task context lookup failed (%1)"), stepResult);
  }
  const std::optional<QString> storedTaskId = optionalText(statement, 0);
  const std::optional<QString> taskListId = optionalText(statement, 1);
  const std::optional<QString> accountId = optionalText(statement, 2);
  const std::optional<QString> taskListRemoteId = optionalText(statement, 3);
  const std::optional<QString> remoteId = optionalText(statement, 4);
  const std::optional<QString> title = optionalText(statement, 8);
  const std::optional<QString> state = optionalText(statement, 10);
  const std::optional<QString> priority = optionalText(statement, 14);
  const int isAssigned = sqlite3_column_int(statement, 16);
  if (!storedTaskId.has_value() || !taskListId.has_value() || !accountId.has_value() ||
      !taskListRemoteId.has_value() || !remoteId.has_value() || !title.has_value() ||
      !state.has_value() || !priority.has_value() || (isAssigned != 0 && isAssigned != 1)) {
    sqlite3_finalize(statement);
    return AppError(AppErrorCode::Database, QStringLiteral("Stored task is invalid"));
  }
  StoredTaskContext context{.taskId = *storedTaskId,
                            .taskListId = *taskListId,
                            .accountId = *accountId,
                            .taskListRemoteId = *taskListRemoteId,
                            .remoteId = *remoteId,
                            .remoteEtag = optionalText(statement, 5),
                            .parentTaskId = optionalText(statement, 6),
                            .parentRemoteId = optionalText(statement, 7),
                            .title = *title,
                            .notes = optionalText(statement, 9),
                            .state = *state,
                            .completedAt = optionalText(statement, 12),
                            .dueAt = optionalText(statement, 11),
                            .dueTimeZone = optionalText(statement, 13),
                            .priority = *priority,
                            .isHidden = sqlite3_column_int(statement, 15) != 0,
                            .isAssigned = isAssigned != 0,
                            .recurrenceDiagnostic = optionalText(statement, 17)};
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK
             ? std::variant<std::optional<StoredTaskContext>, AppError>(std::move(context))
             : std::variant<std::optional<StoredTaskContext>, AppError>(databaseError(
                   QStringLiteral("SQLite task context finalization failed (%1)"), finalizeResult));
}

[[nodiscard]] std::variant<bool, AppError> hasActiveTaskChild(SqliteConnection& connection,
                                                              const QString& taskId) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(handle,
                                               "SELECT EXISTS(SELECT 1 FROM local_tasks "
                                               "WHERE parent_task_id = ?1 AND deleted_at IS NULL)",
                                               -1,
                                               SQLITE_PREPARE_PERSISTENT,
                                               &statement,
                                               nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task child lookup preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, taskId); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const bool hasChild = stepResult == SQLITE_ROW && sqlite3_column_int(statement, 0) != 0;
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_ROW) {
    return databaseError(QStringLiteral("SQLite task child lookup failed (%1)"), stepResult);
  }
  return finalizeResult == SQLITE_OK
             ? std::variant<bool, AppError>(hasChild)
             : std::variant<bool, AppError>(databaseError(
                   QStringLiteral("SQLite task child lookup finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] std::optional<AppError>
validateManagedRecurrenceMutation(SqliteConnection& connection,
                                  const StoredTaskContext& task,
                                  const TaskRecurrenceNotes& recurrence) {
  if (recurrence.state != TaskRecurrenceNotesState::Managed || !recurrence.marker.has_value()) {
    return std::nullopt;
  }
  if (task.isAssigned) {
    return validationError(
        QStringLiteral("Assigned Google Task cannot use managed recurrence"));
  }
  if (task.recurrenceDiagnostic.has_value()) {
    return validationError(QStringLiteral("Managed recurrence requires recovery before editing"));
  }
  if (task.parentTaskId.has_value()) {
    return validationError(QStringLiteral("Managed recurring task cannot be a subtask"));
  }
  const std::variant<bool, AppError> childResult = hasActiveTaskChild(connection, task.taskId);
  if (std::holds_alternative<AppError>(childResult)) {
    return std::get<AppError>(childResult);
  }
  return std::get<bool>(childResult)
             ? std::optional<AppError>(
                   validationError(QStringLiteral("Managed recurring task cannot have subtasks")))
             : std::nullopt;
}

[[nodiscard]] std::variant<bool, AppError> isValidMoveDestination(SqliteConnection& connection,
                                                                  const QString& taskListId,
                                                                  const QString& accountId) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle,
                         "SELECT EXISTS(SELECT 1 FROM local_task_lists "
                         "WHERE id = ?1 AND account_id = ?2 AND deleted_at IS NULL)",
                         -1,
                         SQLITE_PREPARE_PERSISTENT,
                         &statement,
                         nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task move target preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindAll(
          statement, {bindText(statement, 1, taskListId), bindText(statement, 2, accountId)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const bool available = stepResult == SQLITE_ROW && sqlite3_column_int(statement, 0) != 0;
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_ROW) {
    return databaseError(QStringLiteral("SQLite task move target lookup failed (%1)"), stepResult);
  }
  return finalizeResult == SQLITE_OK
             ? std::variant<bool, AppError>(available)
             : std::variant<bool, AppError>(
                   databaseError(QStringLiteral("SQLite task move target finalization failed (%1)"),
                                 finalizeResult));
}

[[nodiscard]] bool isPendingRemoteId(const QString& remoteId) {
  return remoteId.startsWith(QStringLiteral("pending:"));
}

[[nodiscard]] QJsonValue googleDue(const std::optional<QString>& dueAt) {
  if (!dueAt.has_value()) {
    return QJsonValue::Null;
  }
  const QDateTime parsed = QDateTime::fromString(*dueAt, Qt::ISODate);
  return parsed.isValid() ? QJsonValue(parsed.date().toString(Qt::ISODate)) : QJsonValue::Null;
}

[[nodiscard]] QJsonObject taskSnapshot(const StoredTaskContext& task) {
  return {{QStringLiteral("title"), task.title},
          {QStringLiteral("notes"),
           task.notes.has_value() ? QJsonValue(*task.notes) : QJsonValue::Null},
          {QStringLiteral("status"),
           task.state == QStringLiteral("completed") ? QJsonValue(QStringLiteral("completed"))
                                                     : QJsonValue(QStringLiteral("needsAction"))},
          {QStringLiteral("due"), googleDue(task.dueAt)}};
}

[[nodiscard]] QJsonObject googleTaskBody(const StoredTaskContext& task) {
  QJsonObject body{{QStringLiteral("title"), task.title},
                   {QStringLiteral("status"),
                    task.state == QStringLiteral("completed")
                        ? QJsonValue(QStringLiteral("completed"))
                        : QJsonValue(QStringLiteral("needsAction"))},
                   {QStringLiteral("due"), googleDue(task.dueAt)}};
  if (task.notes.has_value()) {
    body.insert(QStringLiteral("notes"), *task.notes);
  }
  return body;
}

[[nodiscard]] QJsonObject taskPayload(const StoredTaskContext& task,
                                      bool includeRemoteIdentity,
                                      const TaskPositionReference& position = {},
                                      const std::optional<QString>& dependsOnMutationId = {}) {
  QJsonObject body = googleTaskBody(task);
  if (!includeRemoteIdentity && !task.dueAt.has_value()) {
    body.remove(QStringLiteral("due"));
  }
  QJsonObject payload{{QStringLiteral("taskListId"), task.taskListRemoteId},
                      {QStringLiteral("localTaskListId"), task.taskListId},
                      {QStringLiteral("localTaskId"), task.taskId},
                      {QStringLiteral("task"), std::move(body)}};
  if (includeRemoteIdentity) {
    payload.insert(QStringLiteral("remoteTaskId"), task.remoteId);
  }
  if (task.parentTaskId.has_value()) {
    if (task.parentRemoteId.has_value() && !isPendingRemoteId(*task.parentRemoteId)) {
      payload.insert(QStringLiteral("parentTaskId"), *task.parentRemoteId);
    } else {
      payload.insert(QStringLiteral("parentTaskLocalId"), *task.parentTaskId);
    }
  }
  if (position.previousRemoteId.has_value()) {
    payload.insert(QStringLiteral("previousTaskId"), *position.previousRemoteId);
  } else if (position.previousLocalId.has_value()) {
    payload.insert(QStringLiteral("previousTaskLocalId"), *position.previousLocalId);
  }
  if (dependsOnMutationId.has_value()) {
    payload.insert(QStringLiteral("dependsOnMutationId"), *dependsOnMutationId);
  }
  return payload;
}

[[nodiscard]] QJsonObject deletePayload(const StoredTaskContext& task,
                                        const std::optional<QString>& dependsOnMutationId = {}) {
  QJsonObject payload{{QStringLiteral("taskListId"), task.taskListRemoteId},
                      {QStringLiteral("localTaskListId"), task.taskListId},
                      {QStringLiteral("remoteTaskId"), task.remoteId},
                      {QStringLiteral("localTaskId"), task.taskId}};
  if (dependsOnMutationId.has_value()) {
    payload.insert(QStringLiteral("dependsOnMutationId"), *dependsOnMutationId);
  }
  return payload;
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

[[nodiscard]] std::variant<std::optional<ActiveTaskMutation>, AppError>
findActiveTaskMutation(SqliteConnection& connection, const QString& taskId) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
SELECT id, operation, payload_json
FROM local_pending_mutations
WHERE resource_type = 'task' AND resource_id = ?1 AND status IN ('pending', 'failed')
ORDER BY created_at DESC, id DESC
LIMIT 1
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task mutation lookup preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, taskId); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult == SQLITE_DONE) {
    const int finalizeResult = sqlite3_finalize(statement);
    return finalizeResult == SQLITE_OK
               ? std::optional<ActiveTaskMutation>{}
               : std::variant<std::optional<ActiveTaskMutation>, AppError>(databaseError(
                     QStringLiteral("SQLite task mutation lookup finalization failed (%1)"),
                     finalizeResult));
  }
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task mutation lookup failed (%1)"), stepResult);
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
    return AppError(AppErrorCode::Database, QStringLiteral("Stored task mutation is invalid"));
  }
  ActiveTaskMutation mutation{
      .id = *mutationId, .operation = *operation, .payload = payloadDocument.object()};
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK
             ? std::variant<std::optional<ActiveTaskMutation>, AppError>(std::move(mutation))
             : std::variant<std::optional<ActiveTaskMutation>, AppError>(databaseError(
                   QStringLiteral("SQLite task mutation lookup finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] std::optional<AppError> replaceActiveTaskMutation(SqliteConnection& connection,
                                                                const ActiveTaskMutation& mutation,
                                                                QString operation,
                                                                QJsonObject payload,
                                                                const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
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
    return databaseError(QStringLiteral("SQLite task mutation replacement preparation failed (%1)"),
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
    return databaseError(QStringLiteral("SQLite task mutation replacement failed (%1)"),
                         stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(
        QStringLiteral("SQLite task mutation replacement finalization failed (%1)"),
        finalizeResult);
  }
  return changedRows == 1 ? std::nullopt
                          : std::optional<AppError>(
                                AppError(AppErrorCode::Database,
                                         QStringLiteral("Active task mutation was not replaced")));
}

[[nodiscard]] std::optional<AppError> removeActiveTaskMutation(SqliteConnection& connection,
                                                               const ActiveTaskMutation& mutation) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(handle,
                                               "DELETE FROM local_pending_mutations "
                                               "WHERE id = ?1 AND status IN ('pending', 'failed')",
                                               -1,
                                               SQLITE_PREPARE_PERSISTENT,
                                               &statement,
                                               nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task mutation removal preparation failed (%1)"),
                         prepareResult);
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
    return databaseError(QStringLiteral("SQLite task mutation removal failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task mutation removal finalization failed (%1)"),
                         finalizeResult);
  }
  return changedRows == 1
             ? std::nullopt
             : std::optional<AppError>(AppError(
                   AppErrorCode::Database, QStringLiteral("Active task mutation was not removed")));
}

[[nodiscard]] std::optional<AppError> insertTaskMutation(SqliteConnection& connection,
                                                         const StoredTaskContext& task,
                                                         QString operation,
                                                         QJsonObject payload,
                                                         const QString& createdAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
INSERT INTO local_pending_mutations (
  id, account_id, resource_type, resource_id, operation, payload_json, status, attempt_count,
  created_at, updated_at
) VALUES (?1, ?2, 'task', ?3, ?4, ?5, 'pending', 0, ?6, ?6)
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task mutation enqueue preparation failed (%1)"),
                         prepareResult);
  }
  const QString mutationId =
      QStringLiteral("mutation:task:") + QUuid::createUuid().toString(QUuid::WithoutBraces);
  const QString payloadJson =
      QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
  if (const std::optional<AppError> error = bindAll(statement,
                                                    {bindText(statement, 1, mutationId),
                                                     bindText(statement, 2, task.accountId),
                                                     bindText(statement, 3, task.taskId),
                                                     bindText(statement, 4, operation),
                                                     bindText(statement, 5, payloadJson),
                                                     bindText(statement, 6, createdAt)});
      error.has_value()) {
    return error;
  }
  const int stepResult = sqlite3_step(statement);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task mutation enqueue failed (%1)"), stepResult);
  }
  return finalizeResult == SQLITE_OK
             ? std::nullopt
             : std::optional<AppError>(databaseError(
                   QStringLiteral("SQLite task mutation enqueue finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] std::optional<AppError>
queueTaskMutation(SqliteConnection& connection,
                  const StoredTaskContext& before,
                  const std::optional<StoredTaskContext>& after,
                  QString operation,
                  const QString& updatedAt,
                  const std::optional<QString>& dependsOnMutationId = {},
                  const TaskPositionReference& position = {}) {
  const std::variant<std::optional<ActiveTaskMutation>, AppError> activeResult =
      findActiveTaskMutation(connection, before.taskId);
  if (std::holds_alternative<AppError>(activeResult)) {
    return std::get<AppError>(activeResult);
  }
  const std::optional<ActiveTaskMutation>& active =
      std::get<std::optional<ActiveTaskMutation>>(activeResult);
  const bool deleting = operation == QStringLiteral("task.delete");
  if (active.has_value()) {
    const QJsonValue metadata = active->payload.value(QString::fromLatin1(kConflictMetadataKey));
    if (!metadata.isObject()) {
      return AppError(AppErrorCode::Database, QStringLiteral("Stored task mutation is invalid"));
    }
    if (active->operation == QStringLiteral("task.create")) {
      if (deleting) {
        return removeActiveTaskMutation(connection, *active);
      }
      if (!after.has_value()) {
        return AppError(AppErrorCode::Database, QStringLiteral("Updated task is unavailable"));
      }
      std::optional<QString> dependency = dependsOnMutationId;
      const QJsonValue existingDependency = active->payload.value(QStringLiteral("dependsOnMutationId"));
      if (!dependency.has_value() && !existingDependency.isUndefined() &&
          !existingDependency.isNull()) {
        if (!existingDependency.isString() ||
            !isValidRequiredText(existingDependency.toString(), kMaximumIdentifierLength)) {
          return AppError(AppErrorCode::Database,
                          QStringLiteral("Stored task mutation dependency is invalid"));
        }
        dependency = existingDependency.toString();
      }
      QJsonObject payload = taskPayload(*after, false, position, dependency);
      payload.insert(QString::fromLatin1(kConflictMetadataKey), metadata);
      return replaceActiveTaskMutation(
          connection, *active, QStringLiteral("task.create"), std::move(payload), updatedAt);
    }
    if (active->operation == QStringLiteral("task.update")) {
      QJsonObject payload = deleting ? deletePayload(before, dependsOnMutationId)
                                     : taskPayload(*after, true, position);
      payload.insert(QString::fromLatin1(kConflictMetadataKey), metadata);
      return replaceActiveTaskMutation(
          connection, *active, std::move(operation), std::move(payload), updatedAt);
    }
  }
  QJsonObject payload =
      deleting ? deletePayload(before, dependsOnMutationId)
               : taskPayload(*after,
                             operation != QStringLiteral("task.create"),
                             position,
                             dependsOnMutationId);
  payload = withConflictMetadata(
      std::move(payload),
      operation == QStringLiteral("task.create") ? QJsonObject() : taskSnapshot(before),
      operation == QStringLiteral("task.create") ? std::optional<QString>{} : before.remoteEtag);
  return insertTaskMutation(
      connection, before, std::move(operation), std::move(payload), updatedAt);
}

[[nodiscard]] std::variant<TaskCreateInput, AppError> canonicalize(TaskCreateInput input) {
  input.title = input.title.trimmed();
  if (!isValidRequiredText(input.taskListId, kMaximumIdentifierLength) ||
      (input.parentTaskId.has_value() &&
       !isValidRequiredText(*input.parentTaskId, kMaximumIdentifierLength)) ||
      !isValidRequiredText(input.title, kMaximumTitleLength) || !isValidNotes(input.notes) ||
      !isValidDue(input.due) || !isValidPriority(input.priority)) {
    return validationError(QStringLiteral("Task create input is invalid"));
  }
  return input;
}

[[nodiscard]] std::variant<TaskUpdateInput, AppError> canonicalize(TaskUpdateInput input) {
  if (input.title.has_value()) {
    *input.title = input.title->trimmed();
  }
  if (!isValidRequiredText(input.taskId, kMaximumIdentifierLength) ||
      (input.parentTaskId.has_value() && input.parentTaskId->has_value() &&
       !isValidRequiredText(**input.parentTaskId, kMaximumIdentifierLength)) ||
      (input.title.has_value() && !isValidRequiredText(*input.title, kMaximumTitleLength)) ||
      !isValidNotes(input.notes) || !isValidDue(input.due) ||
      (input.priority.has_value() && !isValidPriority(*input.priority)) ||
      (!input.parentTaskId.has_value() && !input.title.has_value() && !input.notes.has_value() &&
       !input.due.has_value() && !input.priority.has_value())) {
    return validationError(QStringLiteral("Task update input is invalid"));
  }
  return input;
}

[[nodiscard]] std::optional<QString> recurrenceDate(const QString& timestamp) {
  const QDateTime dateTime = QDateTime::fromString(timestamp, Qt::ISODate);
  if (dateTime.isValid()) {
    return dateTime.date().toString(Qt::ISODate);
  }
  const QDate date = QDate::fromString(timestamp, Qt::ISODate);
  return date.isValid() ? std::optional<QString>(date.toString(Qt::ISODate)) : std::nullopt;
}

[[nodiscard]] std::variant<TaskUpdateInput, AppError>
preserveManagedRecurrence(const StoredTaskContext& before, TaskUpdateInput input) {
  const TaskRecurrenceNotes parsed = parseTaskRecurrenceNotes(before.notes.value_or(QString()));
  if (parsed.state != TaskRecurrenceNotesState::Managed || !parsed.marker.has_value()) {
    return input;
  }
  TaskRecurrenceMarker marker = *parsed.marker;
  if (input.updateManagedRecurrenceTemplate) {
    marker.templateTitle = input.title.value_or(before.title);
  }
  if (input.updateManagedRecurrenceTemplate && input.due.has_value()) {
    if (!input.due->at.has_value()) {
      return validationError(QStringLiteral("Managed recurring task requires a due date"));
    }
    const std::optional<QString> date = recurrenceDate(*input.due->at);
    if (!date.has_value()) {
      return validationError(QStringLiteral("Managed recurring task due date is invalid"));
    }
    marker.templateDueDate = *date;
  }
  if (input.updateManagedRecurrenceTemplate && input.priority.has_value()) {
    marker.templatePriority = priorityText(*input.priority);
  }
  const TaskRecurrenceSerializationResult serialized =
      serializeTaskRecurrenceNotes(input.notes.value_or(parsed.userNotes), marker);
  if (serialized.error.has_value()) {
    return validationError(*serialized.error);
  }
  input.notes = serialized.notes;
  return input;
}

[[nodiscard]] TaskMutationResult createStoredTask(SqliteConnection& connection,
                                                  const TaskCreateInput& input,
                                                  const QString& taskId,
                                                  const QString& remoteId,
                                                  const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
INSERT INTO local_tasks (
  id, task_list_id, remote_id, parent_task_id, title, notes, state, due_at, due_time_zone,
  sort_order, is_hidden, priority, created_at, updated_at
)
SELECT ?1, lists.id, ?3, ?4, ?5, ?6, 'active', ?7, ?8,
       COALESCE((SELECT MAX(tasks.sort_order) + 1
                 FROM local_tasks AS tasks
                 WHERE tasks.task_list_id = lists.id
                   AND ((?4 IS NULL AND tasks.parent_task_id IS NULL)
                        OR tasks.parent_task_id = ?4)
                   AND tasks.deleted_at IS NULL), 0),
       0, ?9, ?10, ?10
FROM local_task_lists AS lists
WHERE lists.id = ?2
  AND lists.deleted_at IS NULL
  AND (?4 IS NULL OR EXISTS (SELECT 1
                             FROM local_tasks AS parent
                             WHERE parent.id = ?4
                               AND parent.task_list_id = lists.id
                               AND parent.parent_task_id IS NULL
                               AND parent.deleted_at IS NULL))
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task create preparation failed (%1)"),
                         prepareResult);
  }
  const std::optional<QString> dueAt = input.due.has_value() ? input.due->at : std::nullopt;
  const std::optional<QString> dueTimeZone =
      input.due.has_value() ? input.due->timeZone : std::nullopt;
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, taskId),
                   bindText(statement, 2, input.taskListId),
                   bindText(statement, 3, remoteId),
                   bindOptionalText(statement, 4, input.parentTaskId),
                   bindText(statement, 5, input.title),
                   bindOptionalText(statement, 6, input.notes),
                   bindOptionalText(statement, 7, dueAt),
                   bindOptionalText(statement, 8, dueTimeZone),
                   bindText(statement, 9, priorityText(input.priority)),
                   bindText(statement, 10, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task create failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task create finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Task list is unavailable for task creation"));
  }
  return TaskMutationReceipt{.taskId = taskId, .updatedAt = updatedAt};
}

[[nodiscard]] TaskMutationResult updateStoredTask(SqliteConnection& connection,
                                                  const TaskUpdateInput& input,
                                                  const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_tasks
SET parent_task_id = CASE WHEN ?2 = 1 THEN ?3 ELSE parent_task_id END,
    sort_order = CASE WHEN ?2 = 1 THEN COALESCE((SELECT MAX(sibling.sort_order) + 1
                                                  FROM local_tasks AS sibling
                                                  WHERE sibling.task_list_id = local_tasks.task_list_id
                                                    AND sibling.id != local_tasks.id
                                                    AND sibling.deleted_at IS NULL
                                                    AND ((?3 IS NULL AND sibling.parent_task_id IS NULL)
                                                         OR sibling.parent_task_id = ?3)), 0)
                      ELSE sort_order END,
    title = CASE WHEN ?4 = 1 THEN ?5 ELSE title END,
    notes = CASE WHEN ?6 = 1 THEN ?7 ELSE notes END,
    due_at = CASE WHEN ?8 = 1 THEN ?9 ELSE due_at END,
    due_time_zone = CASE WHEN ?8 = 1 THEN ?10 ELSE due_time_zone END,
    priority = CASE WHEN ?11 = 1 THEN ?12 ELSE priority END,
    updated_at = ?13
WHERE id = ?1
  AND deleted_at IS NULL
  AND (?2 = 0 OR NOT EXISTS (SELECT 1
                             FROM local_tasks AS descendant
                             WHERE descendant.parent_task_id = local_tasks.id
                               AND descendant.deleted_at IS NULL))
  AND (?2 = 0 OR ?3 IS NULL OR EXISTS (SELECT 1
                                        FROM local_tasks AS parent
                                        WHERE parent.id = ?3
                                          AND parent.id != local_tasks.id
                                          AND parent.task_list_id = local_tasks.task_list_id
                                          AND parent.parent_task_id IS NULL
                                          AND parent.deleted_at IS NULL))
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task update preparation failed (%1)"),
                         prepareResult);
  }
  const std::optional<QString> dueAt = input.due.has_value() ? input.due->at : std::nullopt;
  const std::optional<QString> dueTimeZone =
      input.due.has_value() ? input.due->timeZone : std::nullopt;
  const QString priority = input.priority.has_value() ? priorityText(*input.priority) : QString();
  const std::optional<QString> parentTaskId =
      input.parentTaskId.has_value() ? *input.parentTaskId : std::nullopt;
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, input.taskId),
                   bindInteger(statement, 2, input.parentTaskId.has_value()),
                   bindOptionalText(statement, 3, parentTaskId),
                   bindInteger(statement, 4, input.title.has_value()),
                   bindOptionalText(statement, 5, input.title),
                   bindInteger(statement, 6, input.notes.has_value()),
                   bindOptionalText(statement, 7, input.notes),
                   bindInteger(statement, 8, input.due.has_value()),
                   bindOptionalText(statement, 9, dueAt),
                   bindOptionalText(statement, 10, dueTimeZone),
                   bindInteger(statement, 11, input.priority.has_value()),
                   bindText(statement, 12, priority),
                   bindText(statement, 13, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task update failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task update finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Task is unavailable for update"));
  }
  return TaskMutationReceipt{.taskId = input.taskId, .updatedAt = updatedAt};
}

[[nodiscard]] TaskMutationResult setStoredTaskCompletion(SqliteConnection& connection,
                                                         const QString& taskId,
                                                         bool completed,
                                                         const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_tasks
SET state = ?2,
    completed_at = ?3,
    updated_at = ?4
WHERE id = ?1 AND deleted_at IS NULL
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task completion preparation failed (%1)"),
                         prepareResult);
  }
  const std::optional<QString> completedAt =
      completed ? std::optional<QString>(updatedAt) : std::nullopt;
  if (const std::optional<AppError> error = bindAll(
          statement,
          {bindText(statement, 1, taskId),
           bindText(
               statement, 2, completed ? QStringLiteral("completed") : QStringLiteral("active")),
           bindOptionalText(statement, 3, completedAt),
           bindText(statement, 4, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task completion failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task completion finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Task is unavailable for completion"));
  }
  return TaskMutationReceipt{.taskId = taskId, .updatedAt = updatedAt};
}

[[nodiscard]] std::variant<std::optional<TaskRecurrenceClaim>, AppError>
findTaskRecurrenceClaim(SqliteConnection& connection,
                        const QString& accountId,
                        const QString& seriesId,
                        const QString& occurrenceId) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
SELECT successor_task_id
FROM local_task_recurrence_claims
WHERE account_id = ?1 AND series_id = ?2 AND occurrence_id = ?3
LIMIT 1
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite recurrence-claim lookup preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, accountId),
                   bindText(statement, 2, seriesId),
                   bindText(statement, 3, occurrenceId)});
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult == SQLITE_DONE) {
    const int finalizeResult = sqlite3_finalize(statement);
    return finalizeResult == SQLITE_OK
               ? std::variant<std::optional<TaskRecurrenceClaim>, AppError>(
                     std::optional<TaskRecurrenceClaim>{})
               : std::variant<std::optional<TaskRecurrenceClaim>, AppError>(databaseError(
                     QStringLiteral("SQLite recurrence-claim lookup finalization failed (%1)"),
                     finalizeResult));
  }
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite recurrence-claim lookup failed (%1)"), stepResult);
  }
  const std::optional<QString> successorTaskId = optionalText(statement, 0);
  const int finalizeResult = sqlite3_finalize(statement);
  if (!successorTaskId.has_value()) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Stored recurrence claim is invalid"));
  }
  return finalizeResult == SQLITE_OK
             ? std::variant<std::optional<TaskRecurrenceClaim>, AppError>(
                   TaskRecurrenceClaim{.successorTaskId = *successorTaskId})
             : std::variant<std::optional<TaskRecurrenceClaim>, AppError>(databaseError(
                   QStringLiteral("SQLite recurrence-claim lookup finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] std::optional<AppError>
insertTaskRecurrenceClaim(SqliteConnection& connection,
                          const StoredTaskContext& source,
                          const TaskRecurrenceMarker& marker,
                          const QString& successorTaskId,
                          const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
INSERT INTO local_task_recurrence_claims (
  account_id, series_id, occurrence_id, source_task_id, successor_task_id, created_at, updated_at
) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?6)
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite recurrence-claim insertion preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, source.accountId),
                   bindText(statement, 2, marker.seriesId),
                   bindText(statement, 3, marker.occurrenceId),
                   bindText(statement, 4, source.taskId),
                   bindText(statement, 5, successorTaskId),
                   bindText(statement, 6, updatedAt)});
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite recurrence-claim insertion failed (%1)"), stepResult);
  }
  return finalizeResult == SQLITE_OK
             ? std::nullopt
             : std::optional<AppError>(databaseError(
                   QStringLiteral("SQLite recurrence-claim insertion finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] std::variant<std::optional<QString>, AppError>
createTaskRecurrenceSuccessor(SqliteConnection& connection,
                              const StoredTaskContext& source,
                              const TaskRecurrenceNotes& recurrence,
                              const QString& updatedAt,
                              const std::optional<QString>& dependsOnMutationId = {}) {
  if (recurrence.state != TaskRecurrenceNotesState::Managed || !recurrence.marker.has_value()) {
    return std::optional<QString>{};
  }
  if (const std::optional<AppError> error =
          validateManagedRecurrenceMutation(connection, source, recurrence);
      error.has_value()) {
    return *error;
  }
  const std::variant<std::optional<TaskRecurrenceClaim>, AppError> claimResult =
      findTaskRecurrenceClaim(
          connection, source.accountId, recurrence.marker->seriesId, recurrence.marker->occurrenceId);
  if (std::holds_alternative<AppError>(claimResult)) {
    return std::get<AppError>(claimResult);
  }
  const std::optional<TaskRecurrenceClaim>& claim =
      std::get<std::optional<TaskRecurrenceClaim>>(claimResult);
  if (claim.has_value()) {
    return claim->successorTaskId;
  }
  const std::optional<TaskRecurrenceMarker> successorMarker =
      taskRecurrenceSuccessor(*recurrence.marker);
  if (!successorMarker.has_value()) {
    return std::optional<QString>{};
  }
  const std::optional<TaskPriority> priority = priorityFromText(source.priority);
  if (!priority.has_value()) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Stored task priority is invalid"));
  }
  TaskRecurrenceMarker marker = *successorMarker;
  marker.templateTitle = source.title;
  marker.templatePriority = source.priority;
  const TaskRecurrenceSerializationResult serialized =
      serializeTaskRecurrenceNotes(recurrence.userNotes, marker);
  if (serialized.error.has_value()) {
    return validationError(*serialized.error);
  }
  const QDate successorDue = QDate::fromString(marker.templateDueDate, Qt::ISODate);
  if (!successorDue.isValid()) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Managed recurrence successor date is invalid"));
  }
  const QString localId = QUuid::createUuid().toString(QUuid::WithoutBraces);
  const QString successorTaskId = QStringLiteral("task:") + localId;
  const QString successorRemoteId = QStringLiteral("pending:") + localId;
  const TaskCreateInput successor{
      .taskListId = source.taskListId,
      .title = source.title,
      .notes = serialized.notes,
      .due = TaskDue{.at = QDateTime(successorDue, QTime(0, 0), QTimeZone::UTC)
                                .toString(Qt::ISODateWithMs),
                     .timeZone = marker.timeZone},
      .priority = *priority};
  const TaskMutationResult created =
      createStoredTask(connection, successor, successorTaskId, successorRemoteId, updatedAt);
  if (std::holds_alternative<AppError>(created)) {
    return std::get<AppError>(created);
  }
  const std::variant<std::optional<StoredTaskContext>, AppError> successorContextResult =
      readTaskContext(connection, successorTaskId);
  if (std::holds_alternative<AppError>(successorContextResult)) {
    return std::get<AppError>(successorContextResult);
  }
  const std::optional<StoredTaskContext>& successorContext =
      std::get<std::optional<StoredTaskContext>>(successorContextResult);
  if (!successorContext.has_value()) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Managed recurrence successor is unavailable"));
  }
  if (const std::optional<AppError> error =
          queueTaskMutation(connection,
                            *successorContext,
                            successorContext,
                            QStringLiteral("task.create"),
                            updatedAt,
                            dependsOnMutationId);
      error.has_value()) {
    return *error;
  }
  if (const std::optional<AppError> error =
          insertTaskRecurrenceClaim(connection, source, *recurrence.marker, successorTaskId, updatedAt);
      error.has_value()) {
    return *error;
  }
  return successorTaskId;
}

struct ManagedTaskRecurrenceCandidate final {
  StoredTaskContext task;
  TaskRecurrenceNotes recurrence;
  bool hasActiveMutation{false};
};

[[nodiscard]] std::variant<QList<QString>, AppError>
readTaskIdsForRecurrenceScan(SqliteConnection& connection,
                             const QString& accountId,
                             const std::optional<QString>& taskListRemoteId = {}) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
SELECT tasks.id
FROM local_tasks AS tasks
INNER JOIN local_task_lists AS lists ON lists.id = tasks.task_list_id
WHERE lists.account_id = ?1
  AND (?2 IS NULL OR lists.remote_id = ?2)
  AND tasks.deleted_at IS NULL
  AND lists.deleted_at IS NULL
  AND tasks.notes IS NOT NULL
ORDER BY CASE WHEN tasks.remote_id LIKE 'pending:%' THEN 1 ELSE 0 END, tasks.remote_id, tasks.id
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite recurrence scan preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(statement, {bindText(statement, 1, accountId),
                              bindOptionalText(statement, 2, taskListRemoteId)});
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  QList<QString> taskIds;
  int stepResult = SQLITE_OK;
  while ((stepResult = sqlite3_step(statement)) == SQLITE_ROW) {
    const std::optional<QString> taskId = optionalText(statement, 0);
    if (!taskId.has_value()) {
      sqlite3_finalize(statement);
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Stored recurrence task is invalid"));
    }
    taskIds.append(*taskId);
  }
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite recurrence scan failed (%1)"), stepResult);
  }
  return finalizeResult == SQLITE_OK
             ? std::variant<QList<QString>, AppError>(std::move(taskIds))
             : std::variant<QList<QString>, AppError>(databaseError(
                   QStringLiteral("SQLite recurrence scan finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] std::variant<QList<ManagedTaskRecurrenceCandidate>, AppError>
readManagedTaskRecurrenceCandidates(SqliteConnection& connection,
                                    const QString& accountId,
                                    const std::optional<QString>& taskListRemoteId = {}) {
  const std::variant<QList<QString>, AppError> idsResult =
      readTaskIdsForRecurrenceScan(connection, accountId, taskListRemoteId);
  if (std::holds_alternative<AppError>(idsResult)) {
    return std::get<AppError>(idsResult);
  }
  QList<ManagedTaskRecurrenceCandidate> candidates;
  for (const QString& taskId : std::get<QList<QString>>(idsResult)) {
    const std::variant<std::optional<StoredTaskContext>, AppError> taskResult =
        readTaskContext(connection, taskId);
    if (std::holds_alternative<AppError>(taskResult)) {
      return std::get<AppError>(taskResult);
    }
    const std::optional<StoredTaskContext>& task =
        std::get<std::optional<StoredTaskContext>>(taskResult);
    if (!task.has_value()) {
      continue;
    }
    TaskRecurrenceNotes recurrence = parseTaskRecurrenceNotes(task->notes.value_or(QString()));
    if (recurrence.state != TaskRecurrenceNotesState::Managed || !recurrence.marker.has_value()) {
      continue;
    }
    if (task->isAssigned ||
        (task->recurrenceDiagnostic.has_value() &&
         *task->recurrenceDiagnostic != QString::fromLatin1(kDivergentDuplicateDiagnostic))) {
      continue;
    }
    const std::variant<std::optional<ActiveTaskMutation>, AppError> mutationResult =
        findActiveTaskMutation(connection, task->taskId);
    if (std::holds_alternative<AppError>(mutationResult)) {
      return std::get<AppError>(mutationResult);
    }
    candidates.append(
        {.task = *task,
         .recurrence = std::move(recurrence),
         .hasActiveMutation =
             std::get<std::optional<ActiveTaskMutation>>(mutationResult).has_value()});
  }
  return candidates;
}

[[nodiscard]] bool sameTaskRecurrencePayload(const StoredTaskContext& left,
                                             const StoredTaskContext& right) {
  return left.taskListRemoteId == right.taskListRemoteId &&
         left.parentTaskId == right.parentTaskId && left.parentRemoteId == right.parentRemoteId &&
         left.title == right.title && left.notes == right.notes && left.state == right.state &&
         left.completedAt == right.completedAt && left.dueAt == right.dueAt &&
         left.dueTimeZone == right.dueTimeZone && left.priority == right.priority &&
         left.isHidden == right.isHidden && left.isAssigned == right.isAssigned;
}

[[nodiscard]] std::optional<AppError>
setTaskRecurrenceDiagnostic(SqliteConnection& connection,
                            const QString& taskId,
                            const std::optional<QString>& diagnostic,
                            const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_tasks
SET recurrence_diagnostic = ?2, updated_at = ?3
WHERE id = ?1 AND deleted_at IS NULL
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite recurrence diagnostic preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, taskId), bindOptionalText(statement, 2, diagnostic),
                   bindText(statement, 3, updatedAt)});
      error.has_value()) {
    return error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite recurrence diagnostic update failed (%1)"),
                         stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite recurrence diagnostic finalization failed (%1)"),
                         finalizeResult);
  }
  return changedRows == 1
             ? std::nullopt
             : std::optional<AppError>(
                   AppError(AppErrorCode::Database, QStringLiteral("Stored task was not found")));
}

[[nodiscard]] QString recurrenceGroupKey(const TaskRecurrenceMarker& marker) {
  return marker.seriesId + QChar::Null + marker.occurrenceId;
}

[[nodiscard]] bool recurrenceCandidateComesFirst(const ManagedTaskRecurrenceCandidate& left,
                                                 const ManagedTaskRecurrenceCandidate& right) {
  const bool leftPending = isPendingRemoteId(left.task.remoteId);
  const bool rightPending = isPendingRemoteId(right.task.remoteId);
  if (leftPending != rightPending) {
    return !leftPending;
  }
  if (left.task.remoteId != right.task.remoteId) {
    return left.task.remoteId < right.task.remoteId;
  }
  return left.task.taskId < right.task.taskId;
}

[[nodiscard]] std::optional<AppError>
rewriteTaskRecurrenceNotes(SqliteConnection& connection,
                           const StoredTaskContext& before,
                           QString notes,
                           const QString& updatedAt) {
  if (before.notes.has_value() && *before.notes == notes) {
    return std::nullopt;
  }
  const TaskMutationResult updated = updateStoredTask(
      connection, TaskUpdateInput{.taskId = before.taskId, .notes = std::move(notes)}, updatedAt);
  if (std::holds_alternative<AppError>(updated)) {
    return std::get<AppError>(updated);
  }
  const std::variant<std::optional<StoredTaskContext>, AppError> afterResult =
      readTaskContext(connection, before.taskId);
  if (std::holds_alternative<AppError>(afterResult)) {
    return std::get<AppError>(afterResult);
  }
  const std::optional<StoredTaskContext>& after =
      std::get<std::optional<StoredTaskContext>>(afterResult);
  if (!after.has_value()) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Updated recurrence task is unavailable"));
  }
  return queueTaskMutation(connection, before, after, QStringLiteral("task.update"), updatedAt);
}

[[nodiscard]] TaskMutationResult moveStoredTaskToList(SqliteConnection& connection,
                                                      const QString& taskId,
                                                      const QString& taskListId,
                                                      const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_tasks
SET task_list_id = ?2,
    parent_task_id = NULL,
    sort_order = COALESCE((SELECT MAX(sibling.sort_order) + 1
                           FROM local_tasks AS sibling
                           WHERE sibling.task_list_id = ?2
                             AND sibling.parent_task_id IS NULL
                             AND sibling.id != local_tasks.id
                             AND sibling.deleted_at IS NULL), 0),
    updated_at = ?3
WHERE id = ?1
  AND deleted_at IS NULL
  AND task_list_id != ?2
  AND NOT EXISTS (SELECT 1 FROM local_tasks AS child
                  WHERE child.parent_task_id = local_tasks.id)
  AND EXISTS (SELECT 1
              FROM local_task_lists AS target
              INNER JOIN local_task_lists AS source
                ON source.id = local_tasks.task_list_id
              WHERE target.id = ?2
                AND target.deleted_at IS NULL
                AND source.deleted_at IS NULL
                AND target.account_id = source.account_id)
  AND NOT EXISTS (SELECT 1
                  FROM local_tasks AS sibling
                  WHERE sibling.task_list_id = ?2
                    AND sibling.remote_id = local_tasks.remote_id
                    AND sibling.id != local_tasks.id)
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task move preparation failed (%1)"), prepareResult);
  }
  if (const std::optional<AppError> error = bindAll(statement,
                                                    {bindText(statement, 1, taskId),
                                                     bindText(statement, 2, taskListId),
                                                     bindText(statement, 3, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task move failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task move finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Task is unavailable for list assignment"));
  }
  return TaskMutationReceipt{.taskId = taskId, .updatedAt = updatedAt};
}

[[nodiscard]] std::variant<QList<StoredTaskSibling>, AppError>
readTaskSiblings(SqliteConnection& connection, const StoredTaskContext& task) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
SELECT id, remote_id
FROM local_tasks
WHERE task_list_id = ?1
  AND ((?2 IS NULL AND parent_task_id IS NULL) OR parent_task_id = ?2)
  AND deleted_at IS NULL
ORDER BY sort_order ASC, id ASC
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task sibling lookup preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, task.taskListId),
                   bindOptionalText(statement, 2, task.parentTaskId)});
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  QList<StoredTaskSibling> siblings;
  int stepResult = SQLITE_ROW;
  while ((stepResult = sqlite3_step(statement)) == SQLITE_ROW) {
    const std::optional<QString> taskId = optionalText(statement, 0);
    const std::optional<QString> remoteId = optionalText(statement, 1);
    if (!taskId.has_value() || !remoteId.has_value()) {
      sqlite3_finalize(statement);
      return AppError(AppErrorCode::Database, QStringLiteral("Stored task sibling is invalid"));
    }
    siblings.append({.taskId = *taskId, .remoteId = *remoteId});
  }
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task sibling lookup failed (%1)"), stepResult);
  }
  return finalizeResult == SQLITE_OK
             ? std::variant<QList<StoredTaskSibling>, AppError>(std::move(siblings))
             : std::variant<QList<StoredTaskSibling>, AppError>(databaseError(
                   QStringLiteral("SQLite task sibling lookup finalization failed (%1)"),
                   finalizeResult));
}

[[nodiscard]] std::optional<AppError>
writeTaskSiblingOrder(SqliteConnection& connection,
                      const QList<StoredTaskSibling>& siblings,
                      const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_tasks
SET sort_order = ?2, updated_at = ?3
WHERE id = ?1 AND deleted_at IS NULL
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task reorder preparation failed (%1)"),
                         prepareResult);
  }
  for (qsizetype index = 0; index < siblings.size(); ++index) {
    sqlite3_reset(statement);
    sqlite3_clear_bindings(statement);
    if (const std::optional<AppError> error =
            bindAll(statement,
                    {bindText(statement, 1, siblings.at(index).taskId),
                     bindInteger(statement, 2, static_cast<int>(index)),
                     bindText(statement, 3, updatedAt)});
        error.has_value()) {
      sqlite3_finalize(statement);
      return error;
    }
    const int stepResult = sqlite3_step(statement);
    const int changedRows = sqlite3_changes(handle);
    if (stepResult != SQLITE_DONE || changedRows != 1) {
      sqlite3_finalize(statement);
      return stepResult != SQLITE_DONE
                 ? std::optional<AppError>(databaseError(
                       QStringLiteral("SQLite task reorder failed (%1)"), stepResult))
                 : std::optional<AppError>(AppError(
                       AppErrorCode::Database,
                       QStringLiteral("Task was unavailable while reordering siblings")));
    }
  }
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK
             ? std::nullopt
             : std::optional<AppError>(databaseError(
                   QStringLiteral("SQLite task reorder finalization failed (%1)"), finalizeResult));
}

[[nodiscard]] std::variant<TaskPositionReference, AppError>
reorderStoredTask(SqliteConnection& connection,
                  const StoredTaskContext& task,
                  TaskReorderDirection direction,
                  const QString& updatedAt) {
  const std::variant<QList<StoredTaskSibling>, AppError> siblingsResult =
      readTaskSiblings(connection, task);
  if (std::holds_alternative<AppError>(siblingsResult)) {
    return std::get<AppError>(siblingsResult);
  }
  QList<StoredTaskSibling> siblings = std::get<QList<StoredTaskSibling>>(siblingsResult);
  const auto current = std::find_if(siblings.cbegin(), siblings.cend(), [&task](const auto& sibling) {
    return sibling.taskId == task.taskId;
  });
  if (current == siblings.cend()) {
    return AppError(AppErrorCode::Database, QStringLiteral("Task is missing from its sibling set"));
  }
  const qsizetype currentIndex = static_cast<qsizetype>(std::distance(siblings.cbegin(), current));
  const bool movingEarlier = direction == TaskReorderDirection::Earlier;
  if ((movingEarlier && currentIndex == 0) ||
      (!movingEarlier && currentIndex + 1 >= siblings.size())) {
    return validationError(movingEarlier ? QStringLiteral("Task is already first among siblings")
                                         : QStringLiteral("Task is already last among siblings"));
  }
  const qsizetype targetIndex = movingEarlier ? currentIndex - 1 : currentIndex + 1;
  const StoredTaskSibling moved = *current;
  siblings.removeAt(currentIndex);
  siblings.insert(targetIndex, moved);
  if (const std::optional<AppError> error = writeTaskSiblingOrder(connection, siblings, updatedAt);
      error.has_value()) {
    return *error;
  }
  if (targetIndex == 0) {
    return TaskPositionReference{};
  }
  const StoredTaskSibling& previous = siblings.at(targetIndex - 1);
  return isPendingRemoteId(previous.remoteId)
             ? TaskPositionReference{.previousLocalId = previous.taskId}
             : TaskPositionReference{.previousRemoteId = previous.remoteId};
}

[[nodiscard]] TaskMutationResult
removeStoredTask(SqliteConnection& connection, const QString& taskId, const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_tasks
SET deleted_at = ?2,
    updated_at = ?2
WHERE id = ?1 AND deleted_at IS NULL
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task deletion preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(statement, {bindText(statement, 1, taskId), bindText(statement, 2, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task deletion failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task deletion finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Task is unavailable for deletion"));
  }
  return TaskMutationReceipt{.taskId = taskId, .updatedAt = updatedAt};
}

[[nodiscard]] TaskMutationResult
restoreStoredTask(SqliteConnection& connection,
                  const QString& taskId,
                  bool createRemote,
                  const QString& remoteId,
                  const QString& updatedAt) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_tasks
SET deleted_at = NULL,
    remote_id = CASE WHEN ?2 = 1 THEN ?3 ELSE remote_id END,
    etag = CASE WHEN ?2 = 1 THEN NULL ELSE etag END,
    updated_at = ?4
WHERE id = ?1 AND deleted_at IS NOT NULL
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task restoration preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, taskId),
                   bindInteger(statement, 2, createRemote ? 1 : 0),
                   bindText(statement, 3, remoteId),
                   bindText(statement, 4, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task restoration failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task restoration finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Task is unavailable for restoration"));
  }
  return TaskMutationReceipt{.taskId = taskId, .updatedAt = updatedAt};
}

[[nodiscard]] TaskMutationResult
reconcileStoredGoogleTask(SqliteConnection& connection,
                          const TaskRemoteReconciliationInput& input,
                          const QString& updatedAt) {
  SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
  if (std::holds_alternative<AppError>(transactionResult)) {
    return std::get<AppError>(std::move(transactionResult));
  }
  SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  constexpr char sql[] = R"(
UPDATE local_tasks
SET remote_id = CASE WHEN remote_id LIKE 'pending:%' THEN ?2 ELSE remote_id END,
    etag = COALESCE(?3, etag),
    updated_at = ?4
WHERE id = ?1
  AND (remote_id = ?2 OR remote_id LIKE 'pending:%')
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task reconciliation preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindAll(statement,
                  {bindText(statement, 1, input.localTaskId),
                   bindText(statement, 2, input.remoteTaskId),
                   bindOptionalText(statement, 3, input.remoteEtag),
                   bindText(statement, 4, updatedAt)});
      error.has_value()) {
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  const int changedRows = sqlite3_changes(handle);
  const int finalizeResult = sqlite3_finalize(statement);
  if (stepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite task reconciliation failed (%1)"), stepResult);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task reconciliation finalization failed (%1)"),
                         finalizeResult);
  }
  if (changedRows != 1) {
    return validationError(QStringLiteral("Task is unavailable for Google reconciliation"));
  }
  constexpr char pendingMutationSql[] = R"(
SELECT id, payload_json
FROM local_pending_mutations
WHERE resource_type = 'task' AND resource_id = ?1 AND status IN ('pending', 'failed')
)";
  sqlite3_stmt* pendingMutationStatement = nullptr;
  const int pendingPrepareResult = sqlite3_prepare_v3(handle,
                                                      pendingMutationSql,
                                                      -1,
                                                      SQLITE_PREPARE_PERSISTENT,
                                                      &pendingMutationStatement,
                                                      nullptr);
  if (pendingPrepareResult != SQLITE_OK) {
    sqlite3_finalize(pendingMutationStatement);
    return databaseError(
        QStringLiteral("SQLite pending task reconciliation preparation failed (%1)"),
        pendingPrepareResult);
  }
  if (const std::optional<AppError> error =
          bindText(pendingMutationStatement, 1, input.localTaskId);
      error.has_value()) {
    sqlite3_finalize(pendingMutationStatement);
    return *error;
  }
  struct PendingPayload final {
    QString mutationId;
    QJsonObject payload;
  };
  QList<PendingPayload> pendingPayloads;
  int pendingStepResult = SQLITE_ROW;
  while ((pendingStepResult = sqlite3_step(pendingMutationStatement)) == SQLITE_ROW) {
    const std::optional<QString> mutationId = optionalText(pendingMutationStatement, 0);
    const std::optional<QString> payloadJson = optionalText(pendingMutationStatement, 1);
    QJsonParseError parseError;
    const QJsonDocument document = payloadJson.has_value()
                                       ? QJsonDocument::fromJson(payloadJson->toUtf8(), &parseError)
                                       : QJsonDocument();
    if (!mutationId.has_value() || !payloadJson.has_value() ||
        parseError.error != QJsonParseError::NoError || !document.isObject()) {
      sqlite3_finalize(pendingMutationStatement);
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Stored pending task mutation is invalid"));
    }
    QJsonObject payload = document.object();
    const QJsonValue remoteTaskId = payload.value(QStringLiteral("remoteTaskId"));
    const bool usesPendingRemoteId =
        remoteTaskId.isString() && isPendingRemoteId(remoteTaskId.toString());
    const bool usesReconciledRemoteId =
        remoteTaskId.isString() && remoteTaskId.toString() == input.remoteTaskId;
    if (usesPendingRemoteId || usesReconciledRemoteId) {
      if (usesPendingRemoteId) {
        payload.insert(QStringLiteral("remoteTaskId"), input.remoteTaskId);
      }
      if (input.remoteEtag.has_value()) {
        const QJsonValue metadataValue = payload.value(QString::fromLatin1(kConflictMetadataKey));
        if (!metadataValue.isObject()) {
          sqlite3_finalize(pendingMutationStatement);
          return AppError(AppErrorCode::Database,
                          QStringLiteral("Stored pending task mutation is invalid"));
        }
        QJsonObject metadata = metadataValue.toObject();
        metadata.insert(QStringLiteral("etag"), *input.remoteEtag);
        payload.insert(QString::fromLatin1(kConflictMetadataKey), std::move(metadata));
      }
      pendingPayloads.append({.mutationId = *mutationId, .payload = std::move(payload)});
    }
  }
  const int pendingFinalizeResult = sqlite3_finalize(pendingMutationStatement);
  if (pendingStepResult != SQLITE_DONE) {
    return databaseError(QStringLiteral("SQLite pending task reconciliation lookup failed (%1)"),
                         pendingStepResult);
  }
  if (pendingFinalizeResult != SQLITE_OK) {
    return databaseError(
        QStringLiteral("SQLite pending task reconciliation lookup finalization failed (%1)"),
        pendingFinalizeResult);
  }
  constexpr char updateMutationSql[] = R"(
UPDATE local_pending_mutations
SET payload_json = ?2, updated_at = ?3
WHERE id = ?1 AND status IN ('pending', 'failed')
)";
  for (const PendingPayload& pending : pendingPayloads) {
    sqlite3_stmt* updateMutationStatement = nullptr;
    const int updatePrepareResult = sqlite3_prepare_v3(handle,
                                                       updateMutationSql,
                                                       -1,
                                                       SQLITE_PREPARE_PERSISTENT,
                                                       &updateMutationStatement,
                                                       nullptr);
    if (updatePrepareResult != SQLITE_OK) {
      sqlite3_finalize(updateMutationStatement);
      return databaseError(
          QStringLiteral("SQLite pending task reconciliation update preparation failed (%1)"),
          updatePrepareResult);
    }
    const QString payloadJson =
        QString::fromUtf8(QJsonDocument(pending.payload).toJson(QJsonDocument::Compact));
    if (const std::optional<AppError> error =
            bindAll(updateMutationStatement,
                    {bindText(updateMutationStatement, 1, pending.mutationId),
                     bindText(updateMutationStatement, 2, payloadJson),
                     bindText(updateMutationStatement, 3, updatedAt)});
        error.has_value()) {
      return *error;
    }
    const int updateStepResult = sqlite3_step(updateMutationStatement);
    const int updateChangedRows = sqlite3_changes(handle);
    const int updateFinalizeResult = sqlite3_finalize(updateMutationStatement);
    if (updateStepResult != SQLITE_DONE) {
      return databaseError(QStringLiteral("SQLite pending task reconciliation update failed (%1)"),
                           updateStepResult);
    }
    if (updateFinalizeResult != SQLITE_OK) {
      return databaseError(
          QStringLiteral("SQLite pending task reconciliation update finalization failed (%1)"),
          updateFinalizeResult);
    }
    if (updateChangedRows != 1) {
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Pending task mutation was unavailable for reconciliation"));
    }
  }
  if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
    return *error;
  }
  return TaskMutationReceipt{.taskId = input.localTaskId, .updatedAt = updatedAt};
}

} // namespace

TaskMutationService::TaskMutationService(FilePath databasePath, const Clock& clock)
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

std::shared_future<SqliteWriteResult> TaskMutationService::ready() const { return initialization_; }

std::variant<TaskCreateInput, AppError>
TaskMutationService::validateCreate(TaskCreateInput input) {
  return canonicalize(std::move(input));
}

std::future<TaskMutationResult> TaskMutationService::create(TaskCreateInput input) {
  const std::variant<TaskCreateInput, AppError> canonical = validateCreate(std::move(input));
  if (std::holds_alternative<AppError>(canonical)) {
    return readyFuture(TaskMutationResult(std::get<AppError>(canonical)));
  }
  const QString localId = QUuid::createUuid().toString(QUuid::WithoutBraces);
  const QString taskId = QStringLiteral("task:") + localId;
  const QString remoteId = QStringLiteral("pending:") + localId;
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult([input = std::get<TaskCreateInput>(canonical),
                                     taskId,
                                     remoteId,
                                     updatedAt](SqliteConnection& connection) {
    SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
    if (std::holds_alternative<AppError>(transactionResult)) {
      return TaskMutationResult(std::get<AppError>(std::move(transactionResult)));
    }
    SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
    if (input.parentTaskId.has_value()) {
      const std::variant<std::optional<StoredTaskContext>, AppError> parentResult =
          readTaskContext(connection, *input.parentTaskId);
      if (std::holds_alternative<AppError>(parentResult)) {
        return TaskMutationResult(std::get<AppError>(parentResult));
      }
      const std::optional<StoredTaskContext>& parent =
          std::get<std::optional<StoredTaskContext>>(parentResult);
      if (!parent.has_value() || parent->isAssigned ||
          parseTaskRecurrenceNotes(parent->notes.value_or(QString())).state ==
              TaskRecurrenceNotesState::Managed) {
        return TaskMutationResult(
            validationError(QStringLiteral("Task cannot be a subtask of this parent")));
      }
    }
    TaskMutationResult created = createStoredTask(connection, input, taskId, remoteId, updatedAt);
    if (std::holds_alternative<AppError>(created)) {
      return created;
    }
    const std::variant<std::optional<StoredTaskContext>, AppError> contextResult =
        readTaskContext(connection, taskId);
    if (std::holds_alternative<AppError>(contextResult)) {
      return TaskMutationResult(std::get<AppError>(contextResult));
    }
    const std::optional<StoredTaskContext>& context =
        std::get<std::optional<StoredTaskContext>>(contextResult);
    if (!context.has_value()) {
      return TaskMutationResult(
          AppError(AppErrorCode::Database, QStringLiteral("Created task is unavailable")));
    }
    if (const std::optional<AppError> error = queueTaskMutation(
            connection, *context, context, QStringLiteral("task.create"), updatedAt);
        error.has_value()) {
      return TaskMutationResult(*error);
    }
    if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
      return TaskMutationResult(*error);
    }
    return created;
  });
}

std::future<TaskBatchMutationResult> TaskMutationService::createBatch(QList<TaskCreateInput> inputs) {
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [inputs = std::move(inputs), updatedAt](SqliteConnection& connection) mutable {
        SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
        if (std::holds_alternative<AppError>(transactionResult)) {
          return TaskBatchMutationResult(std::get<AppError>(std::move(transactionResult)));
        }
        SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
        TaskBatchMutationResult created =
            createBatchWithinTransaction(connection, std::move(inputs), updatedAt);
        if (std::holds_alternative<AppError>(created)) {
          return created;
        }
        if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
          return TaskBatchMutationResult(*error);
        }
        return created;
      });
}

TaskBatchMutationResult TaskMutationService::createBatchWithinTransaction(
    SqliteConnection& connection,
    QList<TaskCreateInput> inputs,
    const QString& updatedAt) {
  constexpr qsizetype kMaximumBatchSize = 1'000;
  if (inputs.isEmpty() || inputs.size() > kMaximumBatchSize) {
    return validationError(QStringLiteral("Task creation batch is invalid"));
  }
  QList<TaskCreateInput> canonicalInputs;
  canonicalInputs.reserve(inputs.size());
  for (TaskCreateInput& input : inputs) {
    const std::variant<TaskCreateInput, AppError> canonical = validateCreate(std::move(input));
    if (std::holds_alternative<AppError>(canonical)) {
      return std::get<AppError>(canonical);
    }
    canonicalInputs.append(std::get<TaskCreateInput>(canonical));
  }
  struct PendingCreate final {
    TaskCreateInput input;
    QString taskId;
    QString remoteId;
  };
  QList<PendingCreate> creates;
  creates.reserve(canonicalInputs.size());
  for (TaskCreateInput& input : canonicalInputs) {
    const QString localId = QUuid::createUuid().toString(QUuid::WithoutBraces);
    creates.append({.input = std::move(input),
                    .taskId = QStringLiteral("task:") + localId,
                    .remoteId = QStringLiteral("pending:") + localId});
  }
  QList<TaskMutationReceipt> receipts;
  receipts.reserve(creates.size());
  for (const PendingCreate& create : creates) {
    if (create.input.parentTaskId.has_value()) {
      const std::variant<std::optional<StoredTaskContext>, AppError> parentResult =
          readTaskContext(connection, *create.input.parentTaskId);
      if (std::holds_alternative<AppError>(parentResult)) {
        return std::get<AppError>(parentResult);
      }
      const std::optional<StoredTaskContext>& parent =
          std::get<std::optional<StoredTaskContext>>(parentResult);
      if (!parent.has_value() || parent->isAssigned ||
          parseTaskRecurrenceNotes(parent->notes.value_or(QString())).state ==
              TaskRecurrenceNotesState::Managed) {
        return validationError(QStringLiteral("Task cannot be a subtask of this parent"));
      }
    }
    TaskMutationResult created =
        createStoredTask(connection, create.input, create.taskId, create.remoteId, updatedAt);
    if (std::holds_alternative<AppError>(created)) {
      return std::get<AppError>(std::move(created));
    }
    const std::variant<std::optional<StoredTaskContext>, AppError> contextResult =
        readTaskContext(connection, create.taskId);
    if (std::holds_alternative<AppError>(contextResult)) {
      return std::get<AppError>(contextResult);
    }
    const std::optional<StoredTaskContext>& context =
        std::get<std::optional<StoredTaskContext>>(contextResult);
    if (!context.has_value()) {
      return AppError(AppErrorCode::Database, QStringLiteral("Created task is unavailable"));
    }
    if (const std::optional<AppError> error = queueTaskMutation(
            connection, *context, context, QStringLiteral("task.create"), updatedAt);
        error.has_value()) {
      return *error;
    }
    receipts.append(std::get<TaskMutationReceipt>(std::move(created)));
  }
  return receipts;
}

std::future<TaskMutationResult> TaskMutationService::update(TaskUpdateInput input) {
  const std::variant<TaskUpdateInput, AppError> canonical = canonicalize(std::move(input));
  if (std::holds_alternative<AppError>(canonical)) {
    return readyFuture(TaskMutationResult(std::get<AppError>(canonical)));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult([input = std::get<TaskUpdateInput>(canonical),
                                     updatedAt](SqliteConnection& connection) {
    SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
    if (std::holds_alternative<AppError>(transactionResult)) {
      return TaskMutationResult(std::get<AppError>(std::move(transactionResult)));
    }
    SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
    const std::variant<std::optional<StoredTaskContext>, AppError> beforeResult =
        readTaskContext(connection, input.taskId);
    if (std::holds_alternative<AppError>(beforeResult)) {
      return TaskMutationResult(std::get<AppError>(beforeResult));
    }
    const std::optional<StoredTaskContext>& before =
        std::get<std::optional<StoredTaskContext>>(beforeResult);
    if (!before.has_value()) {
      return TaskMutationResult(validationError(QStringLiteral("Task is unavailable for update")));
    }
    const TaskRecurrenceNotes beforeRecurrence =
        parseTaskRecurrenceNotes(before->notes.value_or(QString()));
    if (const std::optional<AppError> error =
            validateManagedRecurrenceMutation(connection, *before, beforeRecurrence);
        error.has_value()) {
      return TaskMutationResult(*error);
    }
    if (beforeRecurrence.state == TaskRecurrenceNotesState::Managed &&
        input.parentTaskId.has_value() && input.parentTaskId->has_value()) {
      return TaskMutationResult(
          validationError(QStringLiteral("Managed recurring task cannot be a subtask")));
    }
    if (input.parentTaskId.has_value() && input.parentTaskId->has_value()) {
      const std::variant<std::optional<StoredTaskContext>, AppError> parentResult =
          readTaskContext(connection, **input.parentTaskId);
      if (std::holds_alternative<AppError>(parentResult)) {
        return TaskMutationResult(std::get<AppError>(parentResult));
      }
      const std::optional<StoredTaskContext>& parent =
          std::get<std::optional<StoredTaskContext>>(parentResult);
      if (!parent.has_value() || before->isAssigned || parent->isAssigned ||
          parseTaskRecurrenceNotes(parent->notes.value_or(QString())).state ==
              TaskRecurrenceNotesState::Managed) {
        return TaskMutationResult(
            validationError(QStringLiteral("Task cannot be a subtask of this parent")));
      }
    }
    if (before->isAssigned && input.notes.has_value() &&
        parseTaskRecurrenceNotes(*input.notes).state == TaskRecurrenceNotesState::Managed) {
      return TaskMutationResult(
          validationError(QStringLiteral("Assigned Google Task cannot use managed recurrence")));
    }
    const std::variant<TaskUpdateInput, AppError> effectiveInputResult =
        preserveManagedRecurrence(*before, input);
    if (std::holds_alternative<AppError>(effectiveInputResult)) {
      return TaskMutationResult(std::get<AppError>(effectiveInputResult));
    }
    const TaskUpdateInput& effectiveInput = std::get<TaskUpdateInput>(effectiveInputResult);
    TaskMutationResult updated = updateStoredTask(connection, effectiveInput, updatedAt);
    if (std::holds_alternative<AppError>(updated)) {
      return updated;
    }
    const std::variant<std::optional<StoredTaskContext>, AppError> afterResult =
        readTaskContext(connection, input.taskId);
    if (std::holds_alternative<AppError>(afterResult)) {
      return TaskMutationResult(std::get<AppError>(afterResult));
    }
    const std::optional<StoredTaskContext>& after =
        std::get<std::optional<StoredTaskContext>>(afterResult);
    if (!after.has_value()) {
      return TaskMutationResult(
          AppError(AppErrorCode::Database, QStringLiteral("Updated task is unavailable")));
    }
    const QString operation = effectiveInput.parentTaskId.has_value() ? QStringLiteral("task.move")
                                                                       : QStringLiteral("task.update");
    if (const std::optional<AppError> error =
            queueTaskMutation(connection, *before, after, operation, updatedAt);
        error.has_value()) {
      return TaskMutationResult(*error);
    }
    if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
      return TaskMutationResult(*error);
    }
    return updated;
  });
}

std::future<TaskMutationResult> TaskMutationService::moveToTaskList(QString taskId,
                                                                    QString taskListId) {
  if (!isValidRequiredText(taskId, kMaximumIdentifierLength) ||
      !isValidRequiredText(taskListId, kMaximumIdentifierLength)) {
    return readyFuture(
        TaskMutationResult(validationError(QStringLiteral("Task move input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult([taskId = std::move(taskId),
                                     taskListId = std::move(taskListId),
                                     updatedAt](SqliteConnection& connection) {
    SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
    if (std::holds_alternative<AppError>(transactionResult)) {
      return TaskMutationResult(std::get<AppError>(std::move(transactionResult)));
    }
    SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
    const std::variant<std::optional<StoredTaskContext>, AppError> beforeResult =
        readTaskContext(connection, taskId);
    if (std::holds_alternative<AppError>(beforeResult)) {
      return TaskMutationResult(std::get<AppError>(beforeResult));
    }
    const std::optional<StoredTaskContext>& before =
        std::get<std::optional<StoredTaskContext>>(beforeResult);
    if (!before.has_value()) {
      return TaskMutationResult(
          validationError(QStringLiteral("Task is unavailable for list assignment")));
    }
    if (before->taskListId == taskListId) {
      return TaskMutationResult(
          validationError(QStringLiteral("Task is already assigned to that task list")));
    }
    if (parseTaskRecurrenceNotes(before->notes.value_or(QString())).state ==
        TaskRecurrenceNotesState::Managed) {
      return TaskMutationResult(
          validationError(QStringLiteral("Managed recurring task cannot move between task lists")));
    }
    const std::variant<bool, AppError> childResult = hasActiveTaskChild(connection, taskId);
    if (std::holds_alternative<AppError>(childResult)) {
      return TaskMutationResult(std::get<AppError>(childResult));
    }
    if (std::get<bool>(childResult)) {
      return TaskMutationResult(
          validationError(QStringLiteral("Task with subtasks cannot move between task lists")));
    }
    const std::variant<bool, AppError> destinationResult =
        isValidMoveDestination(connection, taskListId, before->accountId);
    if (std::holds_alternative<AppError>(destinationResult)) {
      return TaskMutationResult(std::get<AppError>(destinationResult));
    }
    if (!std::get<bool>(destinationResult)) {
      return TaskMutationResult(
          validationError(QStringLiteral("Task list is unavailable for task assignment")));
    }
    if (isPendingRemoteId(before->remoteId)) {
      TaskMutationResult moved = moveStoredTaskToList(connection, taskId, taskListId, updatedAt);
      if (std::holds_alternative<AppError>(moved)) {
        return moved;
      }
      const std::variant<std::optional<StoredTaskContext>, AppError> afterResult =
          readTaskContext(connection, taskId);
      if (std::holds_alternative<AppError>(afterResult)) {
        return TaskMutationResult(std::get<AppError>(afterResult));
      }
      const std::optional<StoredTaskContext>& after =
          std::get<std::optional<StoredTaskContext>>(afterResult);
      if (!after.has_value()) {
        return TaskMutationResult(
            AppError(AppErrorCode::Database, QStringLiteral("Moved task is unavailable")));
      }
      if (const std::optional<AppError> error = queueTaskMutation(
              connection, *before, after, QStringLiteral("task.update"), updatedAt);
          error.has_value()) {
        return TaskMutationResult(*error);
      }
      if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
        return TaskMutationResult(*error);
      }
      return moved;
    }
    const std::optional<TaskPriority> priority = priorityFromText(before->priority);
    if (!priority.has_value()) {
      return TaskMutationResult(
          AppError(AppErrorCode::Database, QStringLiteral("Stored task priority is invalid")));
    }
    const QString localId = QUuid::createUuid().toString(QUuid::WithoutBraces);
    const QString destinationTaskId = QStringLiteral("task:") + localId;
    const QString destinationRemoteId = QStringLiteral("pending:") + localId;
    TaskCreateInput destination{.taskListId = taskListId,
                                .title = before->title,
                                .notes = before->notes,
                                .due =
                                    TaskDue{.at = before->dueAt, .timeZone = before->dueTimeZone},
                                .priority = *priority};
    TaskMutationResult created = createStoredTask(
        connection, destination, destinationTaskId, destinationRemoteId, updatedAt);
    if (std::holds_alternative<AppError>(created)) {
      return created;
    }
    const std::variant<std::optional<StoredTaskContext>, AppError> destinationContextResult =
        readTaskContext(connection, destinationTaskId);
    if (std::holds_alternative<AppError>(destinationContextResult)) {
      return TaskMutationResult(std::get<AppError>(destinationContextResult));
    }
    const std::optional<StoredTaskContext>& destinationContext =
        std::get<std::optional<StoredTaskContext>>(destinationContextResult);
    if (!destinationContext.has_value()) {
      return TaskMutationResult(AppError(AppErrorCode::Database,
                                         QStringLiteral("Moved task destination is unavailable")));
    }
    if (const std::optional<AppError> error = queueTaskMutation(connection,
                                                                *destinationContext,
                                                                destinationContext,
                                                                QStringLiteral("task.create"),
                                                                updatedAt);
        error.has_value()) {
      return TaskMutationResult(*error);
    }
    const std::variant<std::optional<ActiveTaskMutation>, AppError> createdMutationResult =
        findActiveTaskMutation(connection, destinationTaskId);
    if (std::holds_alternative<AppError>(createdMutationResult)) {
      return TaskMutationResult(std::get<AppError>(createdMutationResult));
    }
    const std::optional<ActiveTaskMutation>& createdMutation =
        std::get<std::optional<ActiveTaskMutation>>(createdMutationResult);
    if (!createdMutation.has_value()) {
      return TaskMutationResult(AppError(
          AppErrorCode::Database, QStringLiteral("Moved task create mutation is unavailable")));
    }
    TaskMutationResult removed = removeStoredTask(connection, taskId, updatedAt);
    if (std::holds_alternative<AppError>(removed)) {
      return removed;
    }
    if (const std::optional<AppError> error = queueTaskMutation(connection,
                                                                *before,
                                                                std::nullopt,
                                                                QStringLiteral("task.delete"),
                                                                updatedAt,
                                                                createdMutation->id);
        error.has_value()) {
      return TaskMutationResult(*error);
    }
    if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
      return TaskMutationResult(*error);
    }
    return TaskMutationResult(
        TaskMutationReceipt{.taskId = destinationTaskId, .updatedAt = updatedAt});
  });
}

std::future<TaskMutationResult>
TaskMutationService::reorder(QString taskId, TaskReorderDirection direction) {
  if (!isValidRequiredText(taskId, kMaximumIdentifierLength)) {
    return readyFuture(TaskMutationResult(
        validationError(QStringLiteral("Task reorder input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult([taskId = std::move(taskId), direction, updatedAt](
                                        SqliteConnection& connection) {
    SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
    if (std::holds_alternative<AppError>(transactionResult)) {
      return TaskMutationResult(std::get<AppError>(std::move(transactionResult)));
    }
    SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
    const std::variant<std::optional<StoredTaskContext>, AppError> beforeResult =
        readTaskContext(connection, taskId);
    if (std::holds_alternative<AppError>(beforeResult)) {
      return TaskMutationResult(std::get<AppError>(beforeResult));
    }
    const std::optional<StoredTaskContext>& before =
        std::get<std::optional<StoredTaskContext>>(beforeResult);
    if (!before.has_value()) {
      return TaskMutationResult(validationError(QStringLiteral("Task is unavailable for reorder")));
    }
    const std::variant<TaskPositionReference, AppError> positionResult =
        reorderStoredTask(connection, *before, direction, updatedAt);
    if (std::holds_alternative<AppError>(positionResult)) {
      return TaskMutationResult(std::get<AppError>(positionResult));
    }
    if (const std::optional<AppError> error = queueTaskMutation(
            connection,
            *before,
            before,
            QStringLiteral("task.move"),
            updatedAt,
            {},
            std::get<TaskPositionReference>(positionResult));
        error.has_value()) {
      return TaskMutationResult(*error);
    }
    if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
      return TaskMutationResult(*error);
    }
    return TaskMutationResult(TaskMutationReceipt{.taskId = taskId, .updatedAt = updatedAt});
  });
}

std::future<TaskMutationResult> TaskMutationService::setCompleted(QString taskId, bool completed) {
  if (!isValidRequiredText(taskId, kMaximumIdentifierLength)) {
    return readyFuture(
        TaskMutationResult(validationError(QStringLiteral("Task completion input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult([taskId = std::move(taskId), completed, updatedAt](
                                        SqliteConnection& connection) {
    SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
    if (std::holds_alternative<AppError>(transactionResult)) {
      return TaskMutationResult(std::get<AppError>(std::move(transactionResult)));
    }
    SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
    const std::variant<std::optional<StoredTaskContext>, AppError> beforeResult =
        readTaskContext(connection, taskId);
    if (std::holds_alternative<AppError>(beforeResult)) {
      return TaskMutationResult(std::get<AppError>(beforeResult));
    }
    const std::optional<StoredTaskContext>& before =
        std::get<std::optional<StoredTaskContext>>(beforeResult);
    if (!before.has_value()) {
      return TaskMutationResult(
          validationError(QStringLiteral("Task is unavailable for completion")));
    }
    const TaskRecurrenceNotes recurrence =
        parseTaskRecurrenceNotes(before->notes.value_or(QString()));
    if (completed) {
      if (const std::optional<AppError> error =
              validateManagedRecurrenceMutation(connection, *before, recurrence);
          error.has_value()) {
        return TaskMutationResult(*error);
      }
    }
    const bool shouldCreateSuccessor = completed && before->state != QStringLiteral("completed") &&
                                       recurrence.state == TaskRecurrenceNotesState::Managed;
    TaskMutationResult changed = setStoredTaskCompletion(connection, taskId, completed, updatedAt);
    if (std::holds_alternative<AppError>(changed)) {
      return changed;
    }
    const std::variant<std::optional<StoredTaskContext>, AppError> afterResult =
        readTaskContext(connection, taskId);
    if (std::holds_alternative<AppError>(afterResult)) {
      return TaskMutationResult(std::get<AppError>(afterResult));
    }
    const std::optional<StoredTaskContext>& after =
        std::get<std::optional<StoredTaskContext>>(afterResult);
    if (!after.has_value()) {
      return TaskMutationResult(
          AppError(AppErrorCode::Database, QStringLiteral("Updated task is unavailable")));
    }
    if (const std::optional<AppError> error =
            queueTaskMutation(connection, *before, after, QStringLiteral("task.update"), updatedAt);
        error.has_value()) {
      return TaskMutationResult(*error);
    }
    if (shouldCreateSuccessor) {
      const std::variant<std::optional<ActiveTaskMutation>, AppError> completionMutationResult =
          findActiveTaskMutation(connection, taskId);
      if (std::holds_alternative<AppError>(completionMutationResult)) {
        return TaskMutationResult(std::get<AppError>(completionMutationResult));
      }
      const std::optional<ActiveTaskMutation>& completionMutation =
          std::get<std::optional<ActiveTaskMutation>>(completionMutationResult);
      if (!completionMutation.has_value()) {
        return TaskMutationResult(AppError(
            AppErrorCode::Database,
            QStringLiteral("Managed recurrence completion mutation is unavailable")));
      }
      const std::variant<std::optional<QString>, AppError> successorResult =
          createTaskRecurrenceSuccessor(
              connection, *after, recurrence, updatedAt, completionMutation->id);
      if (std::holds_alternative<AppError>(successorResult)) {
        return TaskMutationResult(std::get<AppError>(successorResult));
      }
    }
    if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
      return TaskMutationResult(*error);
    }
    return changed;
  });
}

std::future<TaskMutationResult>
TaskMutationService::stopManagedRecurrence(QString taskId, TaskRecurrenceScope scope) {
  if (!isValidRequiredText(taskId, kMaximumIdentifierLength)) {
    return readyFuture(TaskMutationResult(
        validationError(QStringLiteral("Managed recurrence stop input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult([taskId = std::move(taskId), scope, updatedAt](
                                        SqliteConnection& connection) {
    SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
    if (std::holds_alternative<AppError>(transactionResult)) {
      return TaskMutationResult(std::get<AppError>(std::move(transactionResult)));
    }
    SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
    const std::variant<std::optional<StoredTaskContext>, AppError> selectedResult =
        readTaskContext(connection, taskId);
    if (std::holds_alternative<AppError>(selectedResult)) {
      return TaskMutationResult(std::get<AppError>(selectedResult));
    }
    const std::optional<StoredTaskContext>& selected =
        std::get<std::optional<StoredTaskContext>>(selectedResult);
    if (!selected.has_value()) {
      return TaskMutationResult(
          validationError(QStringLiteral("Managed recurrence task is unavailable")));
    }
    const TaskRecurrenceNotes selectedRecurrence =
        parseTaskRecurrenceNotes(selected->notes.value_or(QString()));
    if (selectedRecurrence.state != TaskRecurrenceNotesState::Managed ||
        !selectedRecurrence.marker.has_value()) {
      return TaskMutationResult(
          validationError(QStringLiteral("Task does not have managed recurrence")));
    }
    if (const std::optional<AppError> error =
            validateManagedRecurrenceMutation(connection, *selected, selectedRecurrence);
        error.has_value()) {
      return TaskMutationResult(*error);
    }
    if (scope == TaskRecurrenceScope::ThisOccurrence) {
      const std::variant<std::optional<QString>, AppError> successorResult =
          createTaskRecurrenceSuccessor(connection, *selected, selectedRecurrence, updatedAt);
      if (std::holds_alternative<AppError>(successorResult)) {
        return TaskMutationResult(std::get<AppError>(successorResult));
      }
    }
    QList<ManagedTaskRecurrenceCandidate> candidates;
    if (scope == TaskRecurrenceScope::ThisOccurrence) {
      candidates.append({.task = *selected, .recurrence = selectedRecurrence});
    } else {
      const std::variant<QList<ManagedTaskRecurrenceCandidate>, AppError> candidatesResult =
          readManagedTaskRecurrenceCandidates(connection, selected->accountId);
      if (std::holds_alternative<AppError>(candidatesResult)) {
        return TaskMutationResult(std::get<AppError>(candidatesResult));
      }
      for (const ManagedTaskRecurrenceCandidate& candidate :
           std::get<QList<ManagedTaskRecurrenceCandidate>>(candidatesResult)) {
        if (!candidate.recurrence.marker.has_value() ||
            candidate.recurrence.marker->seriesId != selectedRecurrence.marker->seriesId ||
            (scope == TaskRecurrenceScope::ThisAndFollowing &&
             candidate.recurrence.marker->ordinal < selectedRecurrence.marker->ordinal)) {
          continue;
        }
        candidates.append(candidate);
      }
    }
    if (candidates.isEmpty()) {
      return TaskMutationResult(AppError(AppErrorCode::Database,
                                         QStringLiteral("Managed recurrence task is unavailable")));
    }
    std::sort(candidates.begin(), candidates.end(), recurrenceCandidateComesFirst);
    for (const ManagedTaskRecurrenceCandidate& candidate : candidates) {
      if (const std::optional<AppError> error = rewriteTaskRecurrenceNotes(
              connection, candidate.task, candidate.recurrence.userNotes, updatedAt);
          error.has_value()) {
        return TaskMutationResult(*error);
      }
    }
    if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
      return TaskMutationResult(*error);
    }
    return TaskMutationResult(TaskMutationReceipt{.taskId = taskId, .updatedAt = updatedAt});
  });
}

std::future<TaskMutationResult>
TaskMutationService::reconfigureManagedRecurrence(QString taskId,
                                                   TaskRecurrenceFrequency frequency,
                                                   std::int32_t interval,
                                                   TaskRecurrenceEndCondition end,
                                                   std::optional<QString> recurrenceRule,
                                                   std::optional<QList<QString>> exclusionDates,
                                                   std::optional<QList<QString>> additionDates) {
  if (!isValidRequiredText(taskId, kMaximumIdentifierLength)) {
    return readyFuture(TaskMutationResult(
        validationError(QStringLiteral("Managed recurrence configuration input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [taskId = std::move(taskId), frequency, interval, end = std::move(end),
       recurrenceRule = std::move(recurrenceRule), exclusionDates = std::move(exclusionDates),
       additionDates = std::move(additionDates), updatedAt](
          SqliteConnection& connection) {
        SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
        if (std::holds_alternative<AppError>(transactionResult)) {
          return TaskMutationResult(std::get<AppError>(std::move(transactionResult)));
        }
        SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
        const std::variant<std::optional<StoredTaskContext>, AppError> selectedResult =
            readTaskContext(connection, taskId);
        if (std::holds_alternative<AppError>(selectedResult)) {
          return TaskMutationResult(std::get<AppError>(selectedResult));
        }
        const std::optional<StoredTaskContext>& selected =
            std::get<std::optional<StoredTaskContext>>(selectedResult);
        if (!selected.has_value()) {
          return TaskMutationResult(
              validationError(QStringLiteral("Managed recurrence task is unavailable")));
        }
        const TaskRecurrenceNotes recurrence =
            parseTaskRecurrenceNotes(selected->notes.value_or(QString()));
        if (recurrence.state != TaskRecurrenceNotesState::Managed || !recurrence.marker.has_value()) {
          return TaskMutationResult(
              validationError(QStringLiteral("Task does not have managed recurrence")));
        }
        if (const std::optional<AppError> error =
                validateManagedRecurrenceMutation(connection, *selected, recurrence);
            error.has_value()) {
          return TaskMutationResult(*error);
        }
        TaskRecurrenceMarker marker = *recurrence.marker;
        marker.frequency = frequency;
        marker.interval = interval;
        marker.end = std::move(end);
        if (recurrenceRule.has_value()) {
          marker.recurrenceRule = *recurrenceRule;
        }
        if (exclusionDates.has_value()) {
          marker.exclusionDates = *exclusionDates;
        }
        if (additionDates.has_value()) {
          marker.additionDates = *additionDates;
        }
        const TaskRecurrenceSerializationResult serialized =
            serializeTaskRecurrenceNotes(recurrence.userNotes, marker);
        if (serialized.error.has_value()) {
          return TaskMutationResult(validationError(*serialized.error));
        }
        if (const std::optional<AppError> error =
                rewriteTaskRecurrenceNotes(connection, *selected, serialized.notes, updatedAt);
            error.has_value()) {
          return TaskMutationResult(*error);
        }
        if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
          return TaskMutationResult(*error);
        }
        return TaskMutationResult(TaskMutationReceipt{.taskId = taskId, .updatedAt = updatedAt});
      });
}

std::future<TaskMutationResult> TaskMutationService::splitManagedRecurrence(QString taskId) {
  if (!isValidRequiredText(taskId, kMaximumIdentifierLength)) {
    return readyFuture(TaskMutationResult(
        validationError(QStringLiteral("Managed recurrence split input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult([taskId = std::move(taskId), updatedAt](
                                        SqliteConnection& connection) {
    SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
    if (std::holds_alternative<AppError>(transactionResult)) {
      return TaskMutationResult(std::get<AppError>(std::move(transactionResult)));
    }
    SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
    const std::variant<std::optional<StoredTaskContext>, AppError> selectedResult =
        readTaskContext(connection, taskId);
    if (std::holds_alternative<AppError>(selectedResult)) {
      return TaskMutationResult(std::get<AppError>(selectedResult));
    }
    const std::optional<StoredTaskContext>& selected =
        std::get<std::optional<StoredTaskContext>>(selectedResult);
    if (!selected.has_value()) {
      return TaskMutationResult(
          validationError(QStringLiteral("Managed recurrence task is unavailable")));
    }
    const TaskRecurrenceNotes selectedRecurrence =
        parseTaskRecurrenceNotes(selected->notes.value_or(QString()));
    if (selectedRecurrence.state != TaskRecurrenceNotesState::Managed ||
        !selectedRecurrence.marker.has_value() || !selected->dueAt.has_value()) {
      return TaskMutationResult(
          validationError(QStringLiteral("Managed recurrence split is unavailable")));
    }
    if (const std::optional<AppError> error =
            validateManagedRecurrenceMutation(connection, *selected, selectedRecurrence);
        error.has_value()) {
      return TaskMutationResult(*error);
    }
    const std::optional<QString> anchorDate = recurrenceDate(*selected->dueAt);
    if (!anchorDate.has_value()) {
      return TaskMutationResult(
          validationError(QStringLiteral("Managed recurrence split due date is invalid")));
    }
    const std::variant<QList<ManagedTaskRecurrenceCandidate>, AppError> candidatesResult =
        readManagedTaskRecurrenceCandidates(connection, selected->accountId);
    if (std::holds_alternative<AppError>(candidatesResult)) {
      return TaskMutationResult(std::get<AppError>(candidatesResult));
    }
    QList<ManagedTaskRecurrenceCandidate> candidates;
    QSet<std::int32_t> ordinals;
    for (const ManagedTaskRecurrenceCandidate& candidate :
         std::get<QList<ManagedTaskRecurrenceCandidate>>(candidatesResult)) {
      if (!candidate.recurrence.marker.has_value() ||
          candidate.recurrence.marker->seriesId != selectedRecurrence.marker->seriesId ||
          candidate.recurrence.marker->ordinal < selectedRecurrence.marker->ordinal) {
        continue;
      }
      if (ordinals.contains(candidate.recurrence.marker->ordinal)) {
        return TaskMutationResult(validationError(
            QStringLiteral("Managed recurrence split has duplicate occurrence identities")));
      }
      ordinals.insert(candidate.recurrence.marker->ordinal);
      candidates.append(candidate);
    }
    if (candidates.isEmpty() || !ordinals.contains(selectedRecurrence.marker->ordinal)) {
      return TaskMutationResult(AppError(AppErrorCode::Database,
                                         QStringLiteral("Managed recurrence task is unavailable")));
    }
    std::sort(candidates.begin(), candidates.end(), [](const auto& left, const auto& right) {
      return left.recurrence.marker->ordinal < right.recurrence.marker->ordinal;
    });
    const QString seriesId = QUuid::createUuid().toString(QUuid::WithoutBraces);
    TaskRecurrenceEndCondition end = selectedRecurrence.marker->end;
    if (end.kind == TaskRecurrenceEndKind::Count) {
      end.count = *end.count - selectedRecurrence.marker->ordinal;
    }
    for (const ManagedTaskRecurrenceCandidate& candidate : candidates) {
      if (!candidate.task.dueAt.has_value()) {
        return TaskMutationResult(validationError(
            QStringLiteral("Managed recurrence split occurrence has no due date")));
      }
      const std::optional<QString> dueDate = recurrenceDate(*candidate.task.dueAt);
      if (!dueDate.has_value()) {
        return TaskMutationResult(validationError(
            QStringLiteral("Managed recurrence split occurrence due date is invalid")));
      }
      TaskRecurrenceMarker marker = *candidate.recurrence.marker;
      marker.seriesId = seriesId;
      marker.ordinal -= selectedRecurrence.marker->ordinal;
      marker.occurrenceId = seriesId + QStringLiteral(":") + QString::number(marker.ordinal);
      marker.anchorDate = *anchorDate;
      marker.end = end;
      marker.templateTitle = candidate.task.title;
      marker.templateDueDate = *dueDate;
      marker.templatePriority = candidate.task.priority;
      if (candidate.task.dueTimeZone.has_value()) {
        marker.timeZone = *candidate.task.dueTimeZone;
      }
      const TaskRecurrenceSerializationResult serialized =
          serializeTaskRecurrenceNotes(candidate.recurrence.userNotes, marker);
      if (serialized.error.has_value()) {
        return TaskMutationResult(validationError(*serialized.error));
      }
      if (const std::optional<AppError> error =
              rewriteTaskRecurrenceNotes(connection, candidate.task, serialized.notes, updatedAt);
          error.has_value()) {
        return TaskMutationResult(*error);
      }
    }
    if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
      return TaskMutationResult(*error);
    }
    return TaskMutationResult(TaskMutationReceipt{.taskId = taskId, .updatedAt = updatedAt});
  });
}

std::future<TaskMutationResult> TaskMutationService::remove(QString taskId) {
  if (!isValidRequiredText(taskId, kMaximumIdentifierLength)) {
    return readyFuture(
        TaskMutationResult(validationError(QStringLiteral("Task deletion input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [taskId = std::move(taskId), updatedAt](SqliteConnection& connection) {
        SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
        if (std::holds_alternative<AppError>(transactionResult)) {
          return TaskMutationResult(std::get<AppError>(std::move(transactionResult)));
        }
        SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
        const std::variant<std::optional<StoredTaskContext>, AppError> beforeResult =
            readTaskContext(connection, taskId);
        if (std::holds_alternative<AppError>(beforeResult)) {
          return TaskMutationResult(std::get<AppError>(beforeResult));
        }
        const std::optional<StoredTaskContext>& before =
            std::get<std::optional<StoredTaskContext>>(beforeResult);
        if (!before.has_value()) {
          return TaskMutationResult(
              validationError(QStringLiteral("Task is unavailable for deletion")));
        }
        if (before->isAssigned) {
          return TaskMutationResult(
              validationError(QStringLiteral("Assigned Google Task cannot be deleted in HCB")));
        }
        TaskMutationResult removed = removeStoredTask(connection, taskId, updatedAt);
        if (std::holds_alternative<AppError>(removed)) {
          return removed;
        }
        if (const std::optional<AppError> error = queueTaskMutation(
                connection, *before, std::nullopt, QStringLiteral("task.delete"), updatedAt);
            error.has_value()) {
          return TaskMutationResult(*error);
        }
        if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
          return TaskMutationResult(*error);
        }
        return removed;
      });
}

std::future<TaskMutationResult> TaskMutationService::restore(QString taskId) {
  if (!isValidRequiredText(taskId, kMaximumIdentifierLength)) {
    return readyFuture(
        TaskMutationResult(validationError(QStringLiteral("Task restoration input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [taskId = std::move(taskId), updatedAt](SqliteConnection& connection) {
        SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
        if (std::holds_alternative<AppError>(transactionResult)) {
          return TaskMutationResult(std::get<AppError>(std::move(transactionResult)));
        }
        SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
        const std::variant<std::optional<ActiveTaskMutation>, AppError> activeResult =
            findActiveTaskMutation(connection, taskId);
        if (std::holds_alternative<AppError>(activeResult)) {
          return TaskMutationResult(std::get<AppError>(activeResult));
        }
        const std::optional<ActiveTaskMutation>& active =
            std::get<std::optional<ActiveTaskMutation>>(activeResult);
        const bool cancelPendingDelete =
            active.has_value() && active->operation == QStringLiteral("task.delete");
        const QString pendingRemoteId =
            QStringLiteral("pending:") + QUuid::createUuid().toString(QUuid::WithoutBraces);
        TaskMutationResult restored = restoreStoredTask(connection,
                                                        taskId,
                                                        !cancelPendingDelete,
                                                        pendingRemoteId,
                                                        updatedAt);
        if (std::holds_alternative<AppError>(restored)) {
          return restored;
        }
        const std::variant<std::optional<StoredTaskContext>, AppError> afterResult =
            readTaskContext(connection, taskId);
        if (std::holds_alternative<AppError>(afterResult)) {
          return TaskMutationResult(std::get<AppError>(afterResult));
        }
        const std::optional<StoredTaskContext>& after =
            std::get<std::optional<StoredTaskContext>>(afterResult);
        if (!after.has_value()) {
          return TaskMutationResult(
              AppError(AppErrorCode::Database, QStringLiteral("Restored task is unavailable")));
        }
        std::optional<AppError> queueError;
        if (cancelPendingDelete) {
          queueError = removeActiveTaskMutation(connection, *active);
        } else {
          queueError = queueTaskMutation(connection,
                                         *after,
                                         after,
                                         QStringLiteral("task.create"),
                                         updatedAt);
        }
        if (queueError.has_value()) {
          return TaskMutationResult(*queueError);
        }
        if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
          return TaskMutationResult(*error);
        }
        return restored;
      });
}

std::future<TaskMutationSnapshotResult> TaskMutationService::inspect(QList<QString> taskIds) {
  constexpr qsizetype kMaximumInspectionSize = 501; // one compatible parent plus a 500-task bulk selection
  QSet<QString> uniqueIds;
  if (taskIds.isEmpty() || taskIds.size() > kMaximumInspectionSize) {
    return readyFuture(TaskMutationSnapshotResult(
        validationError(QStringLiteral("Task inspection input is invalid"))));
  }
  for (const QString& taskId : taskIds) {
    if (!isValidRequiredText(taskId, kMaximumIdentifierLength) || uniqueIds.contains(taskId)) {
      return readyFuture(TaskMutationSnapshotResult(
          validationError(QStringLiteral("Task inspection input is invalid"))));
    }
    uniqueIds.insert(taskId);
  }
  return writerQueue_.enqueueResult([taskIds = std::move(taskIds)](SqliteConnection& connection) {
    QList<TaskMutationSnapshot> snapshots;
    snapshots.reserve(taskIds.size());
    for (const QString& taskId : taskIds) {
      const std::variant<std::optional<StoredTaskContext>, AppError> contextResult =
          readTaskContext(connection, taskId);
      if (std::holds_alternative<AppError>(contextResult)) {
        return TaskMutationSnapshotResult(std::get<AppError>(contextResult));
      }
      const std::optional<StoredTaskContext>& context =
          std::get<std::optional<StoredTaskContext>>(contextResult);
      if (!context.has_value()) {
        continue;
      }
      const std::optional<TaskPriority> priority = priorityFromText(context->priority);
      if (!priority.has_value() ||
          (context->state != QStringLiteral("active") &&
           context->state != QStringLiteral("completed"))) {
        return TaskMutationSnapshotResult(
            AppError(AppErrorCode::Database, QStringLiteral("Stored task is invalid")));
      }
      const std::variant<bool, AppError> childResult = hasActiveTaskChild(connection, taskId);
      if (std::holds_alternative<AppError>(childResult)) {
        return TaskMutationSnapshotResult(std::get<AppError>(childResult));
      }
      snapshots.append({.taskId = context->taskId,
                        .taskListId = context->taskListId,
                        .title = context->title,
                        .notes = context->notes,
                        .parentTaskId = context->parentTaskId,
                        .dueAt = context->dueAt,
                        .dueTimeZone = context->dueTimeZone,
                        .priority = *priority,
                        .completed = context->state == QStringLiteral("completed"),
                        .hasActiveChildren = std::get<bool>(childResult),
                        .managedRecurrenceSeriesId = [&context]() -> std::optional<QString> {
                          const TaskRecurrenceNotes parsed =
                              parseTaskRecurrenceNotes(context->notes.value_or(QString()));
                          return parsed.state == TaskRecurrenceNotesState::Managed &&
                                         parsed.marker.has_value()
                                     ? std::optional<QString>(parsed.marker->seriesId)
                                     : std::nullopt;
                        }(),
                        .managedRecurrenceOrdinal = [&context]() -> std::optional<std::int64_t> {
                          const TaskRecurrenceNotes parsed =
                              parseTaskRecurrenceNotes(context->notes.value_or(QString()));
                          return parsed.state == TaskRecurrenceNotesState::Managed &&
                                         parsed.marker.has_value()
                                     ? std::optional<std::int64_t>(parsed.marker->ordinal)
                                     : std::nullopt;
                        }()});
    }
    return TaskMutationSnapshotResult(std::move(snapshots));
  });
}

std::future<TaskMutationSnapshotResult>
TaskMutationService::inspectManagedSeries(QList<QString> taskIds) {
  constexpr qsizetype kMaximumInspectionSize = 500;
  if (taskIds.isEmpty() || taskIds.size() > kMaximumInspectionSize) {
    return readyFuture(TaskMutationSnapshotResult(
        validationError(QStringLiteral("Managed recurrence inspection input is invalid"))));
  }
  QSet<QString> uniqueIds;
  for (const QString& taskId : taskIds) {
    if (!isValidRequiredText(taskId, kMaximumIdentifierLength) || uniqueIds.contains(taskId)) {
      return readyFuture(TaskMutationSnapshotResult(
          validationError(QStringLiteral("Managed recurrence inspection input is invalid"))));
    }
    uniqueIds.insert(taskId);
  }
  return writerQueue_.enqueueResult([taskIds = std::move(taskIds)](SqliteConnection& connection) {
    sqlite3* const handle = connection.nativeHandle();
    if (handle == nullptr) {
      return TaskMutationSnapshotResult(
          AppError(AppErrorCode::Database, QStringLiteral("SQLite task connection is unavailable")));
    }
    QHash<QString, QSet<QString>> seriesByAccount;
    for (const QString& taskId : taskIds) {
      const std::variant<std::optional<StoredTaskContext>, AppError> contextResult =
          readTaskContext(connection, taskId);
      if (std::holds_alternative<AppError>(contextResult)) {
        return TaskMutationSnapshotResult(std::get<AppError>(contextResult));
      }
      const std::optional<StoredTaskContext>& context =
          std::get<std::optional<StoredTaskContext>>(contextResult);
      if (!context.has_value()) {
        continue;
      }
      const TaskRecurrenceNotes recurrence =
          parseTaskRecurrenceNotes(context->notes.value_or(QString()));
      if (recurrence.state == TaskRecurrenceNotesState::Managed && recurrence.marker.has_value()) {
        seriesByAccount[context->accountId].insert(recurrence.marker->seriesId);
      }
    }
    QList<TaskMutationSnapshot> snapshots;
    QSet<QString> emitted;
    for (auto account = seriesByAccount.cbegin(); account != seriesByAccount.cend(); ++account) {
      sqlite3_stmt* statement = nullptr;
      constexpr char sql[] = R"(
SELECT tasks.id
FROM local_tasks AS tasks
JOIN local_task_lists AS lists ON lists.id = tasks.task_list_id
WHERE lists.account_id = ?1
ORDER BY tasks.id
)";
      const int prepareResult = sqlite3_prepare_v2(handle, sql, -1, &statement, nullptr);
      if (prepareResult != SQLITE_OK || statement == nullptr) {
        return TaskMutationSnapshotResult(databaseError(
            QStringLiteral("SQLite managed recurrence scan preparation failed (%1)"), prepareResult));
      }
      if (const std::optional<AppError> error = bindText(statement, 1, account.key());
          error.has_value()) {
        sqlite3_finalize(statement);
        return TaskMutationSnapshotResult(*error);
      }
      for (;;) {
        const int stepResult = sqlite3_step(statement);
        if (stepResult == SQLITE_DONE) {
          break;
        }
        if (stepResult != SQLITE_ROW) {
          sqlite3_finalize(statement);
          return TaskMutationSnapshotResult(databaseError(
              QStringLiteral("SQLite managed recurrence scan failed (%1)"), stepResult));
        }
        const std::optional<QString> id = optionalText(statement, 0);
        if (!id.has_value()) {
          sqlite3_finalize(statement);
          return TaskMutationSnapshotResult(
              AppError(AppErrorCode::Database, QStringLiteral("Stored recurrence task is invalid")));
        }
        const std::variant<std::optional<StoredTaskContext>, AppError> contextResult =
            readTaskContext(connection, *id);
        if (std::holds_alternative<AppError>(contextResult)) {
          sqlite3_finalize(statement);
          return TaskMutationSnapshotResult(std::get<AppError>(contextResult));
        }
        const std::optional<StoredTaskContext>& context =
            std::get<std::optional<StoredTaskContext>>(contextResult);
        if (!context.has_value()) {
          continue;
        }
        const TaskRecurrenceNotes recurrence =
            parseTaskRecurrenceNotes(context->notes.value_or(QString()));
        if (recurrence.state != TaskRecurrenceNotesState::Managed || !recurrence.marker.has_value() ||
            !account.value().contains(recurrence.marker->seriesId) || emitted.contains(context->taskId)) {
          continue;
        }
        const std::optional<TaskPriority> priority = priorityFromText(context->priority);
        const std::variant<bool, AppError> childResult =
            hasActiveTaskChild(connection, context->taskId);
        if (!priority.has_value() || std::holds_alternative<AppError>(childResult)) {
          sqlite3_finalize(statement);
          return TaskMutationSnapshotResult(priority.has_value()
                                                ? std::get<AppError>(childResult)
                                                : AppError(AppErrorCode::Database,
                                                           QStringLiteral("Stored task is invalid")));
        }
        emitted.insert(context->taskId);
        snapshots.append({.taskId = context->taskId,
                          .taskListId = context->taskListId,
                          .title = context->title,
                          .notes = context->notes,
                          .parentTaskId = context->parentTaskId,
                          .dueAt = context->dueAt,
                          .dueTimeZone = context->dueTimeZone,
                          .priority = *priority,
                          .completed = context->state == QStringLiteral("completed"),
                          .hasActiveChildren = std::get<bool>(childResult),
                          .managedRecurrenceSeriesId = recurrence.marker->seriesId,
                          .managedRecurrenceOrdinal = recurrence.marker->ordinal});
      }
      const int finalizeResult = sqlite3_finalize(statement);
      if (finalizeResult != SQLITE_OK) {
        return TaskMutationSnapshotResult(databaseError(
            QStringLiteral("SQLite managed recurrence scan finalization failed (%1)"), finalizeResult));
      }
    }
    return TaskMutationSnapshotResult(std::move(snapshots));
  });
}

std::future<TaskMutationResult>
TaskMutationService::reconcileGoogleTask(TaskRemoteReconciliationInput input) {
  if (!isValidRequiredText(input.localTaskId, kMaximumIdentifierLength) ||
      !isValidRequiredText(input.remoteTaskId, kMaximumIdentifierLength) ||
      isPendingRemoteId(input.remoteTaskId) ||
      (input.remoteEtag.has_value() &&
       (!isValidRequiredText(*input.remoteEtag, 4'096) || input.remoteEtag->contains(u'\r') ||
        input.remoteEtag->contains(u'\n')))) {
    return readyFuture(TaskMutationResult(
        validationError(QStringLiteral("Google task reconciliation input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [input = std::move(input), updatedAt](SqliteConnection& connection) {
        return reconcileStoredGoogleTask(connection, input, updatedAt);
      });
}

std::future<TaskRecurrenceReconciliationResult>
TaskMutationService::reconcileManagedRecurrences(QString accountId, QString taskListRemoteId) {
  if (!isValidRequiredText(accountId, kMaximumIdentifierLength) ||
      !isValidRequiredText(taskListRemoteId, kMaximumIdentifierLength)) {
    return readyFuture(TaskRecurrenceReconciliationResult(
        validationError(QStringLiteral("Managed recurrence reconciliation input is invalid"))));
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [accountId = std::move(accountId), taskListRemoteId = std::move(taskListRemoteId), updatedAt](
          SqliteConnection& connection) {
        SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
        if (std::holds_alternative<AppError>(transactionResult)) {
          return TaskRecurrenceReconciliationResult(
              std::get<AppError>(std::move(transactionResult)));
        }
        SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
        const std::variant<QList<ManagedTaskRecurrenceCandidate>, AppError> candidatesResult =
            readManagedTaskRecurrenceCandidates(connection, accountId, taskListRemoteId);
        if (std::holds_alternative<AppError>(candidatesResult)) {
          return TaskRecurrenceReconciliationResult(std::get<AppError>(candidatesResult));
        }
        QList<ManagedTaskRecurrenceCandidate> candidates =
            std::get<QList<ManagedTaskRecurrenceCandidate>>(candidatesResult);
        QHash<QString, QList<ManagedTaskRecurrenceCandidate>> groups;
        for (const ManagedTaskRecurrenceCandidate& candidate : candidates) {
          groups[recurrenceGroupKey(*candidate.recurrence.marker)].append(candidate);
        }
        TaskRecurrenceReconciliation result;
        QSet<QString> removedTaskIds;
        QHash<QString, QString> unambiguousTaskIds;
        QSet<QString> divergentGroupKeys;
        for (auto group = groups.begin(); group != groups.end(); ++group) {
          QList<ManagedTaskRecurrenceCandidate>& occurrences = group.value();
          std::sort(occurrences.begin(), occurrences.end(), recurrenceCandidateComesFirst);
          const ManagedTaskRecurrenceCandidate& canonical = occurrences.first();
          if (occurrences.size() == 1) {
            if (canonical.task.recurrenceDiagnostic ==
                QString::fromLatin1(kDivergentDuplicateDiagnostic)) {
              if (const std::optional<AppError> error =
                      setTaskRecurrenceDiagnostic(connection,
                                                  canonical.task.taskId,
                                                  std::nullopt,
                                                  updatedAt);
                  error.has_value()) {
                return TaskRecurrenceReconciliationResult(*error);
              }
            }
            unambiguousTaskIds.insert(group.key(), canonical.task.taskId);
            continue;
          }
          bool exact = !canonical.hasActiveMutation;
          for (const ManagedTaskRecurrenceCandidate& occurrence : occurrences) {
            exact = exact && !occurrence.hasActiveMutation &&
                    sameTaskRecurrencePayload(canonical.task, occurrence.task);
          }
          if (!exact) {
            if (occurrences.size() > 1) {
              ++result.divergentDuplicateGroupCount;
              divergentGroupKeys.insert(group.key());
              for (const ManagedTaskRecurrenceCandidate& occurrence : occurrences) {
                if (occurrence.task.recurrenceDiagnostic ==
                    QString::fromLatin1(kDivergentDuplicateDiagnostic)) {
                  continue;
                }
                if (const std::optional<AppError> error =
                        setTaskRecurrenceDiagnostic(
                            connection,
                            occurrence.task.taskId,
                            QString::fromLatin1(kDivergentDuplicateDiagnostic),
                            updatedAt);
                    error.has_value()) {
                  return TaskRecurrenceReconciliationResult(*error);
                }
              }
            }
            continue;
          }
          unambiguousTaskIds.insert(group.key(), canonical.task.taskId);
          if (canonical.task.recurrenceDiagnostic ==
              QString::fromLatin1(kDivergentDuplicateDiagnostic)) {
            if (const std::optional<AppError> error =
                    setTaskRecurrenceDiagnostic(
                        connection, canonical.task.taskId, std::nullopt, updatedAt);
                error.has_value()) {
              return TaskRecurrenceReconciliationResult(*error);
            }
          }
          for (qsizetype index = 1; index < occurrences.size(); ++index) {
            const ManagedTaskRecurrenceCandidate& duplicate = occurrences.at(index);
            const TaskMutationResult removed =
                removeStoredTask(connection, duplicate.task.taskId, updatedAt);
            if (std::holds_alternative<AppError>(removed)) {
              return TaskRecurrenceReconciliationResult(std::get<AppError>(removed));
            }
            if (const std::optional<AppError> error = queueTaskMutation(connection,
                                                                        duplicate.task,
                                                                        std::nullopt,
                                                                        QStringLiteral("task.delete"),
                                                                        updatedAt);
                error.has_value()) {
              return TaskRecurrenceReconciliationResult(*error);
            }
            removedTaskIds.insert(duplicate.task.taskId);
            ++result.removedDuplicateCount;
          }
        }
        std::sort(candidates.begin(), candidates.end(), [](const auto& left, const auto& right) {
          if (left.recurrence.marker->seriesId != right.recurrence.marker->seriesId) {
            return left.recurrence.marker->seriesId < right.recurrence.marker->seriesId;
          }
          if (left.recurrence.marker->ordinal != right.recurrence.marker->ordinal) {
            return left.recurrence.marker->ordinal < right.recurrence.marker->ordinal;
          }
          return recurrenceCandidateComesFirst(left, right);
        });
        for (const ManagedTaskRecurrenceCandidate& source : candidates) {
          if (removedTaskIds.contains(source.task.taskId) ||
              source.task.state != QStringLiteral("completed") || source.task.parentTaskId.has_value() ||
              divergentGroupKeys.contains(recurrenceGroupKey(*source.recurrence.marker))) {
            continue;
          }
          const std::variant<bool, AppError> childResult =
              hasActiveTaskChild(connection, source.task.taskId);
          if (std::holds_alternative<AppError>(childResult)) {
            return TaskRecurrenceReconciliationResult(std::get<AppError>(childResult));
          }
          if (std::get<bool>(childResult)) {
            continue;
          }
          const std::variant<std::optional<TaskRecurrenceClaim>, AppError> claimResult =
              findTaskRecurrenceClaim(connection,
                                      source.task.accountId,
                                      source.recurrence.marker->seriesId,
                                      source.recurrence.marker->occurrenceId);
          if (std::holds_alternative<AppError>(claimResult)) {
            return TaskRecurrenceReconciliationResult(std::get<AppError>(claimResult));
          }
          if (std::get<std::optional<TaskRecurrenceClaim>>(claimResult).has_value()) {
            continue;
          }
          const std::optional<TaskRecurrenceMarker> successorMarker =
              taskRecurrenceSuccessor(*source.recurrence.marker);
          if (!successorMarker.has_value()) {
            continue;
          }
          const QString successorKey = recurrenceGroupKey(*successorMarker);
          if (divergentGroupKeys.contains(successorKey)) {
            continue;
          }
          const auto existing = unambiguousTaskIds.constFind(successorKey);
          if (existing != unambiguousTaskIds.cend() && *existing != source.task.taskId) {
            if (const std::optional<AppError> error = insertTaskRecurrenceClaim(connection,
                                                                                source.task,
                                                                                *source.recurrence.marker,
                                                                                *existing,
                                                                                updatedAt);
                error.has_value()) {
              return TaskRecurrenceReconciliationResult(*error);
            }
            continue;
          }
          const std::variant<std::optional<QString>, AppError> successorResult =
              createTaskRecurrenceSuccessor(connection,
                                            source.task,
                                            source.recurrence,
                                            updatedAt);
          if (std::holds_alternative<AppError>(successorResult)) {
            return TaskRecurrenceReconciliationResult(std::get<AppError>(successorResult));
          }
          if (std::get<std::optional<QString>>(successorResult).has_value()) {
            ++result.createdSuccessorCount;
          }
        }
        if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
          return TaskRecurrenceReconciliationResult(*error);
        }
        return TaskRecurrenceReconciliationResult(result);
      });
}

std::future<TaskRemoteIdResult> TaskMutationService::remoteTaskId(QString taskId) {
  if (!isValidRequiredText(taskId, kMaximumIdentifierLength)) {
    return readyFuture(TaskRemoteIdResult(
        validationError(QStringLiteral("Task remote ID lookup input is invalid"))));
  }
  return writerQueue_.enqueueResult([taskId = std::move(taskId)](SqliteConnection& connection) {
    const std::variant<std::optional<StoredTaskContext>, AppError> contextResult =
        readTaskContext(connection, taskId);
    if (std::holds_alternative<AppError>(contextResult)) {
      return TaskRemoteIdResult(std::get<AppError>(contextResult));
    }
    const std::optional<StoredTaskContext>& context =
        std::get<std::optional<StoredTaskContext>>(contextResult);
    return context.has_value() ? TaskRemoteIdResult(std::optional<QString>(context->remoteId))
                               : TaskRemoteIdResult(std::optional<QString>{});
  });
}

} // namespace hcb
