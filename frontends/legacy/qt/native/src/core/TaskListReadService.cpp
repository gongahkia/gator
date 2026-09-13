#include "core/TaskListReadService.h"

#include "data/LocalSchema.h"
#include "sqlite3.h"

#include <QByteArray>
#include <QString>

#include <future>
#include <limits>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumIdentifierLength = 256;
constexpr std::int64_t kMaximumPageLimit = 100;
constexpr std::int64_t kTaskPreviewLimit = 8;

using TaskListDecodeResult = std::variant<TaskListSummary, AppError>;
using TaskTitleListResult = std::variant<QStringList, AppError>;

[[nodiscard]] AppError databaseError(const QString& message, int result) {
  return AppError(AppErrorCode::Database, message.arg(result));
}

[[nodiscard]] bool isValidIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= kMaximumIdentifierLength &&
         !value.contains(QChar::Null);
}

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

[[nodiscard]] std::optional<AppError>
bindText(sqlite3_stmt* statement, int index, const QString& value) {
  const QByteArray utf8 = value.toUtf8();
  const int result = sqlite3_bind_text(
      statement, index, utf8.constData(), static_cast<int>(utf8.size()), SQLITE_TRANSIENT);
  if (result != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task-list binding failed (%1)"), result);
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<AppError>
bindInteger(sqlite3_stmt* statement, int index, std::int64_t value) {
  const int result = sqlite3_bind_int64(statement, index, value);
  if (result != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task-list binding failed (%1)"), result);
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<QString> optionalText(sqlite3_stmt* statement, int index) {
  if (sqlite3_column_type(statement, index) == SQLITE_NULL) {
    return std::nullopt;
  }
  const auto* value = reinterpret_cast<const char*>(sqlite3_column_text(statement, index));
  const int byteCount = sqlite3_column_bytes(statement, index);
  if (value == nullptr || byteCount < 0) {
    return std::nullopt;
  }
  return QString::fromUtf8(value, byteCount);
}

[[nodiscard]] std::optional<QString> requiredText(sqlite3_stmt* statement, int index) {
  if (sqlite3_column_type(statement, index) != SQLITE_TEXT) {
    return std::nullopt;
  }
  return optionalText(statement, index);
}

[[nodiscard]] TaskListDecodeResult decodeTaskList(sqlite3_stmt* statement) {
  const std::optional<QString> id = requiredText(statement, 0);
  const std::optional<QString> accountId = requiredText(statement, 1);
  const std::optional<QString> remoteId = requiredText(statement, 2);
  const std::optional<QString> title = requiredText(statement, 3);
  const std::optional<QString> updatedAt = requiredText(statement, 8);
  if (!id.has_value() || !accountId.has_value() || !remoteId.has_value() || !title.has_value() ||
      !updatedAt.has_value()) {
    return AppError(AppErrorCode::Database, QStringLiteral("Stored task list row is invalid"));
  }
  const int selected = sqlite3_column_int(statement, 6);
  if (selected != 0 && selected != 1) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Stored task list selection is invalid"));
  }
  const std::int64_t taskCount = sqlite3_column_int64(statement, 9);
  const std::int64_t activeTaskCount = sqlite3_column_int64(statement, 10);
  if (taskCount < 0 || activeTaskCount < 0 || activeTaskCount > taskCount) {
    return AppError(AppErrorCode::Database, QStringLiteral("Stored task list counts are invalid"));
  }
  return TaskListSummary{.id = *id,
                         .accountId = *accountId,
                         .remoteId = *remoteId,
                         .title = *title,
                         .etag = optionalText(statement, 4),
                         .sortOrder = sqlite3_column_int64(statement, 5),
                         .selected = selected == 1,
                         .remoteUpdatedAt = optionalText(statement, 7),
                         .updatedAt = *updatedAt,
                         .taskCount = taskCount,
                         .activeTaskCount = activeTaskCount};
}

[[nodiscard]] TaskTitleListResult readTaskTitles(SqliteConnection& connection,
                                                  const QString& taskListId) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task-list connection is unavailable"));
  }
  constexpr char sql[] = R"(
SELECT title
FROM local_tasks
WHERE task_list_id = ?1 AND deleted_at IS NULL AND is_hidden = 0
ORDER BY sort_order ASC, id ASC
LIMIT ?2
)";
  sqlite3_stmt* statement = nullptr;
  const int prepareResult =
      sqlite3_prepare_v3(handle, sql, -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task-list preview preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error =
          bindText(statement, 1, taskListId);
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  if (const std::optional<AppError> error = bindInteger(statement, 2, kTaskPreviewLimit);
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  QStringList titles;
  while (true) {
    const int stepResult = sqlite3_step(statement);
    if (stepResult == SQLITE_DONE) {
      break;
    }
    if (stepResult != SQLITE_ROW) {
      sqlite3_finalize(statement);
      return databaseError(QStringLiteral("SQLite task-list preview lookup failed (%1)"),
                           stepResult);
    }
    const std::optional<QString> title = requiredText(statement, 0);
    if (!title.has_value()) {
      sqlite3_finalize(statement);
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Stored task-list preview task is invalid"));
    }
    titles.append(*title);
  }
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK
             ? TaskTitleListResult(std::move(titles))
             : TaskTitleListResult(databaseError(
                   QStringLiteral("SQLite task-list preview finalization failed (%1)"),
                   finalizeResult));
}

constexpr char taskListProjectionSql[] = R"(
SELECT lists.id, lists.account_id, lists.remote_id, lists.title, lists.etag, lists.sort_order,
       lists.is_selected, lists.remote_updated_at, lists.updated_at,
       COUNT(tasks.id) AS task_count,
       COALESCE(SUM(CASE WHEN tasks.state != 'completed'
                          AND tasks.is_hidden = 0
                          THEN 1 ELSE 0 END), 0) AS active_task_count
FROM local_task_lists AS lists
LEFT JOIN local_tasks AS tasks
  ON tasks.task_list_id = lists.id
 AND tasks.deleted_at IS NULL
WHERE lists.deleted_at IS NULL
)";

[[nodiscard]] QString taskListOrderSql() {
  return QStringLiteral(
      " GROUP BY lists.id "
      "ORDER BY lists.sort_order ASC, lists.title COLLATE NOCASE ASC, lists.id ASC");
}

[[nodiscard]] TaskListLookupResult readStoredTaskList(SqliteConnection& connection,
                                                      const QString& taskListId) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task-list connection is unavailable"));
  }
  const QByteArray sql = QString::fromLatin1(taskListProjectionSql)
                             .append(QStringLiteral(" AND lists.id = ?1"))
                             .append(taskListOrderSql())
                             .append(QStringLiteral(" LIMIT 1"))
                             .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task-list lookup preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, taskListId); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult == SQLITE_DONE) {
    const int finalizeResult = sqlite3_finalize(statement);
    if (finalizeResult != SQLITE_OK) {
      return databaseError(QStringLiteral("SQLite task-list lookup finalization failed (%1)"),
                           finalizeResult);
    }
    return std::optional<TaskListSummary>{};
  }
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task-list lookup failed (%1)"), stepResult);
  }
  const TaskListDecodeResult decoded = decodeTaskList(statement);
  const int finalizeResult = sqlite3_finalize(statement);
  if (std::holds_alternative<AppError>(decoded)) {
    return std::get<AppError>(decoded);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task-list lookup finalization failed (%1)"),
                         finalizeResult);
  }
  TaskListSummary taskList = std::get<TaskListSummary>(decoded);
  TaskTitleListResult taskTitles = readTaskTitles(connection, taskList.id);
  if (std::holds_alternative<AppError>(taskTitles)) {
    return std::get<AppError>(std::move(taskTitles));
  }
  taskList.taskTitles = std::get<QStringList>(std::move(taskTitles));
  return std::optional<TaskListSummary>(std::move(taskList));
}

[[nodiscard]] TaskListPageResult readStoredTaskLists(SqliteConnection& connection,
                                                     const TaskListReadRequest& request) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task-list connection is unavailable"));
  }
  QString where;
  if (request.accountId.has_value()) {
    where.append(QStringLiteral(" AND lists.account_id = ?1"));
  }
  if (request.selectedOnly) {
    where.append(QStringLiteral(" AND lists.is_selected = 1"));
  }
  const QByteArray sql =
      QString::fromLatin1(taskListProjectionSql)
          .append(where)
          .append(taskListOrderSql())
          .append(request.accountId.has_value() ? QStringLiteral(" LIMIT ?2 OFFSET ?3")
                                                : QStringLiteral(" LIMIT ?1 OFFSET ?2"))
          .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task-list list preparation failed (%1)"),
                         prepareResult);
  }
  int limitIndex = 1;
  if (request.accountId.has_value()) {
    if (const std::optional<AppError> error = bindText(statement, 1, *request.accountId);
        error.has_value()) {
      sqlite3_finalize(statement);
      return *error;
    }
    limitIndex = 2;
  }
  if (const std::optional<AppError> error = bindInteger(statement, limitIndex, request.limit);
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  if (const std::optional<AppError> error = bindInteger(statement, limitIndex + 1, request.offset);
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  QList<TaskListSummary> items;
  while (true) {
    const int stepResult = sqlite3_step(statement);
    if (stepResult == SQLITE_DONE) {
      break;
    }
    if (stepResult != SQLITE_ROW) {
      sqlite3_finalize(statement);
      return databaseError(QStringLiteral("SQLite task-list list failed (%1)"), stepResult);
    }
    const TaskListDecodeResult decoded = decodeTaskList(statement);
    if (std::holds_alternative<AppError>(decoded)) {
      sqlite3_finalize(statement);
      return std::get<AppError>(decoded);
    }
    items.append(std::get<TaskListSummary>(decoded));
  }
  const int finalizeResult = sqlite3_finalize(statement);
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task-list list finalization failed (%1)"),
                         finalizeResult);
  }

  for (TaskListSummary& taskList : items) {
    TaskTitleListResult taskTitles = readTaskTitles(connection, taskList.id);
    if (std::holds_alternative<AppError>(taskTitles)) {
      return std::get<AppError>(std::move(taskTitles));
    }
    taskList.taskTitles = std::get<QStringList>(std::move(taskTitles));
  }

  QString countSql = QStringLiteral("SELECT COUNT(*) FROM local_task_lists "
                                    "WHERE deleted_at IS NULL");
  if (request.accountId.has_value()) {
    countSql.append(QStringLiteral(" AND account_id = ?1"));
  }
  if (request.selectedOnly) {
    countSql.append(QStringLiteral(" AND is_selected = 1"));
  }
  sqlite3_stmt* countStatement = nullptr;
  const int countPrepareResult = sqlite3_prepare_v3(handle,
                                                    countSql.toUtf8().constData(),
                                                    -1,
                                                    SQLITE_PREPARE_PERSISTENT,
                                                    &countStatement,
                                                    nullptr);
  if (countPrepareResult != SQLITE_OK) {
    sqlite3_finalize(countStatement);
    return databaseError(QStringLiteral("SQLite task-list count preparation failed (%1)"),
                         countPrepareResult);
  }
  if (request.accountId.has_value()) {
    if (const std::optional<AppError> error = bindText(countStatement, 1, *request.accountId);
        error.has_value()) {
      sqlite3_finalize(countStatement);
      return *error;
    }
  }
  const int countStepResult = sqlite3_step(countStatement);
  const std::int64_t totalKnown = sqlite3_column_int64(countStatement, 0);
  const int countFinalizeResult = sqlite3_finalize(countStatement);
  if (countStepResult != SQLITE_ROW || totalKnown < 0) {
    return databaseError(QStringLiteral("SQLite task-list count failed (%1)"), countStepResult);
  }
  if (countFinalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite task-list count finalization failed (%1)"),
                         countFinalizeResult);
  }
  const std::int64_t consumed = request.offset + static_cast<std::int64_t>(items.size());
  return TaskListPage{.items = std::move(items),
                      .nextOffset = consumed < totalKnown ? std::optional<std::int64_t>(consumed)
                                                          : std::nullopt,
                      .totalKnown = totalKnown};
}

} // namespace

TaskListReadService::TaskListReadService(FilePath databasePath)
    : writerQueue_(std::move(databasePath)),
      initialization_(writerQueue_
                          .enqueue([](SqliteConnection& connection) -> SqliteWriteResult {
                            const SqliteMigrationRunResultOrError result =
                                LocalSchema::initialize(connection);
                            return std::holds_alternative<AppError>(result)
                                       ? std::optional<AppError>(std::get<AppError>(result))
                                       : std::nullopt;
                          })
                          .share()) {}

std::shared_future<SqliteWriteResult> TaskListReadService::ready() const { return initialization_; }

std::future<TaskListLookupResult> TaskListReadService::find(QString taskListId) {
  if (!isValidIdentifier(taskListId)) {
    return readyFuture(TaskListLookupResult(
        AppError(AppErrorCode::Validation, QStringLiteral("Task-list identifier is invalid"))));
  }
  return writerQueue_.enqueueResult(
      [taskListId = std::move(taskListId)](SqliteConnection& connection) {
        return readStoredTaskList(connection, taskListId);
      });
}

std::future<TaskListPageResult> TaskListReadService::list(TaskListReadRequest request) {
  if ((request.accountId.has_value() && !isValidIdentifier(*request.accountId)) ||
      request.limit <= 0 || request.limit > kMaximumPageLimit || request.offset < 0 ||
      request.offset > std::numeric_limits<std::int64_t>::max() - request.limit) {
    return readyFuture(TaskListPageResult(
        AppError(AppErrorCode::Validation, QStringLiteral("Task-list read request is invalid"))));
  }
  return writerQueue_.enqueueResult([request = std::move(request)](SqliteConnection& connection) {
    return readStoredTaskLists(connection, request);
  });
}

} // namespace hcb
