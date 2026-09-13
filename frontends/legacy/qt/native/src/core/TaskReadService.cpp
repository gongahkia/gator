#include "core/TaskReadService.h"

#include "core/TaskRecurrenceMarker.h"
#include "data/LocalSchema.h"
#include "sqlite3.h"

#include <QByteArray>

#include <future>
#include <limits>
#include <optional>
#include <utility>

namespace hcb {
namespace {

constexpr qsizetype kMaximumIdentifierLength = 256;
constexpr std::int64_t kMaximumPageLimit = 10'000;

[[nodiscard]] AppError databaseError(QString message, int result) {
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

[[nodiscard]] bool isValidIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= kMaximumIdentifierLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] std::optional<AppError>
bindText(sqlite3_stmt* statement, int index, const QString& value) {
  const QByteArray utf8 = value.toUtf8();
  const int result = sqlite3_bind_text(
      statement, index, utf8.constData(), static_cast<int>(utf8.size()), SQLITE_TRANSIENT);
  return result == SQLITE_OK ? std::nullopt
                             : std::optional<AppError>(databaseError(
                                   QStringLiteral("SQLite task binding failed (%1)"), result));
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

[[nodiscard]] std::optional<QString> requiredText(sqlite3_stmt* statement, int index) {
  return sqlite3_column_type(statement, index) == SQLITE_TEXT ? optionalText(statement, index)
                                                              : std::nullopt;
}

[[nodiscard]] std::optional<TaskPriority> taskPriority(const QString& value) {
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

[[nodiscard]] TaskReadResult readStoredTasks(SqliteConnection& connection,
                                             const TaskReadRequest& request) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite task connection is unavailable"));
  }
  QString filter = QStringLiteral("tasks.deleted_at IS NULL AND tasks.is_hidden = 0 "
                                  "AND lists.deleted_at IS NULL");
  if (request.accountId.has_value()) {
    filter.append(QStringLiteral(" AND lists.account_id = ?"));
  }
  if (request.selectedListsOnly) {
    filter.append(QStringLiteral(" AND lists.is_selected = 1"));
  }
  const QByteArray sql =
      QStringLiteral(
          "SELECT tasks.id, tasks.task_list_id, lists.title, tasks.parent_task_id, tasks.title, "
          "tasks.notes, tasks.due_at, tasks.due_time_zone, tasks.priority, tasks.state, "
          "tasks.sort_order, tasks.is_assigned, tasks.recurrence_diagnostic "
          "FROM local_tasks AS tasks INNER JOIN local_task_lists AS lists "
          "ON lists.id = tasks.task_list_id WHERE %1 ORDER BY lists.sort_order, "
          "lists.title COLLATE NOCASE, tasks.sort_order, tasks.id LIMIT ?")
          .arg(filter)
          .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite task list preparation failed (%1)"), prepareResult);
  }
  int parameter = 1;
  if (request.accountId.has_value()) {
    if (const std::optional<AppError> error = bindText(statement, parameter++, *request.accountId);
        error.has_value()) {
      sqlite3_finalize(statement);
      return *error;
    }
  }
  if (sqlite3_bind_int64(statement, parameter, request.limit) != SQLITE_OK) {
    sqlite3_finalize(statement);
    return AppError(AppErrorCode::Database, QStringLiteral("SQLite task limit binding failed"));
  }
  QList<TaskModelTask> tasks;
  while (true) {
    const int stepResult = sqlite3_step(statement);
    if (stepResult == SQLITE_DONE) {
      break;
    }
    if (stepResult != SQLITE_ROW) {
      sqlite3_finalize(statement);
      return databaseError(QStringLiteral("SQLite task list failed (%1)"), stepResult);
    }
    const std::optional<QString> id = requiredText(statement, 0);
    const std::optional<QString> taskListId = requiredText(statement, 1);
    const std::optional<QString> taskListTitle = requiredText(statement, 2);
    const std::optional<QString> title = requiredText(statement, 4);
    const std::optional<QString> priority = requiredText(statement, 8);
    const std::optional<QString> state = requiredText(statement, 9);
    const std::int64_t sortOrder = sqlite3_column_int64(statement, 10);
    const int assigned = sqlite3_column_int(statement, 11);
    const std::optional<QString> storedDiagnostic = optionalText(statement, 12);
    const std::optional<TaskPriority> decodedPriority =
        priority.has_value() ? taskPriority(*priority) : std::nullopt;
    if (!id.has_value() || !taskListId.has_value() || !taskListTitle.has_value() ||
        !title.has_value() || !state.has_value() || !decodedPriority.has_value() || sortOrder < 0 ||
        (*state != QStringLiteral("active") && *state != QStringLiteral("completed")) ||
        (assigned != 0 && assigned != 1)) {
      sqlite3_finalize(statement);
      return AppError(AppErrorCode::Database, QStringLiteral("Stored task row is invalid"));
    }
    const std::optional<QString> dueAt = optionalText(statement, 6);
    const std::optional<QString> dueTimeZone = optionalText(statement, 7);
    const std::optional<QString> storedNotes = optionalText(statement, 5);
    const TaskRecurrenceNotes recurrence =
        parseTaskRecurrenceNotes(storedNotes.value_or(QString()));
    const QString recurrenceDiagnostic = storedDiagnostic.value_or(recurrence.diagnostic);
    const auto recurrenceFrequency = [&recurrence]() {
      if (!recurrence.marker.has_value()) {
        return -1;
      }
      return static_cast<int>(recurrence.marker->frequency);
    };
    const auto recurrenceInterval = [&recurrence]() {
      return recurrence.marker.has_value() ? recurrence.marker->interval : 1;
    };
    const auto recurrenceEndKind = [&recurrence]() {
      return recurrence.marker.has_value() ? static_cast<int>(recurrence.marker->end.kind) : 0;
    };
    const auto recurrenceEndUntil = [&recurrence]() {
      return recurrence.marker.has_value() ? recurrence.marker->end.untilDate.value_or(QString())
                                           : QString();
    };
    const auto recurrenceEndCount = [&recurrence]() {
      return recurrence.marker.has_value() ? recurrence.marker->end.count.value_or(0) : 0;
    };
    tasks.append({.id = *id,
                  .taskListId = *taskListId,
                  .taskListTitle = *taskListTitle,
                  .parentTaskId = optionalText(statement, 3),
                  .title = *title,
                  .notes = storedNotes.has_value() ? std::optional<QString>(recurrence.userNotes)
                                                   : std::nullopt,
                  .due = dueAt.has_value() || dueTimeZone.has_value()
                             ? std::optional<TaskDue>(TaskDue{.at = dueAt, .timeZone = dueTimeZone})
                             : std::nullopt,
                  .priority = *decodedPriority,
                  .completed = *state == QStringLiteral("completed"),
                  .managedRecurrence = recurrence.state == TaskRecurrenceNotesState::Managed &&
                                       recurrenceDiagnostic.isEmpty() && assigned == 0,
                  .recurrenceSummary = recurrence.marker.has_value()
                                           ? taskRecurrenceSummary(*recurrence.marker)
                                           : QString(),
                  .recurrenceSeriesId =
                      recurrence.marker.has_value() ? recurrence.marker->seriesId : QString(),
                  .recurrenceOccurrenceId =
                      recurrence.marker.has_value() ? recurrence.marker->occurrenceId : QString(),
                  .recurrenceFrequency = recurrenceFrequency(),
                  .recurrenceInterval = recurrenceInterval(),
                  .recurrenceEndKind = recurrenceEndKind(),
                  .recurrenceEndUntil = recurrenceEndUntil(),
                  .recurrenceEndCount = recurrenceEndCount(),
                  .recurrenceRule = recurrence.marker.has_value() ? recurrence.marker->recurrenceRule : QString(),
                  .recurrenceExclusionDates = recurrence.marker.has_value()
                                                 ? recurrence.marker->exclusionDates.join(QStringLiteral(","))
                                                 : QString(),
                  .recurrenceAdditionDates = recurrence.marker.has_value()
                                                ? recurrence.marker->additionDates.join(QStringLiteral(","))
                                                : QString(),
                  .recurrenceDiagnostic = recurrenceDiagnostic,
                  .sortOrder = sortOrder});
  }
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK
             ? TaskReadResult(std::move(tasks))
             : TaskReadResult(databaseError(
                   QStringLiteral("SQLite task list finalization failed (%1)"), finalizeResult));
}

} // namespace

TaskReadService::TaskReadService(FilePath databasePath)
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

std::shared_future<SqliteWriteResult> TaskReadService::ready() const { return initialization_; }

std::future<TaskReadResult> TaskReadService::list(TaskReadRequest request) {
  if ((request.accountId.has_value() && !isValidIdentifier(*request.accountId)) ||
      request.limit <= 0 || request.limit > kMaximumPageLimit) {
    return readyFuture(
        TaskReadResult(validationError(QStringLiteral("Task read request is invalid"))));
  }
  return writerQueue_.enqueueResult([request = std::move(request)](SqliteConnection& connection) {
    return readStoredTasks(connection, request);
  });
}

} // namespace hcb
