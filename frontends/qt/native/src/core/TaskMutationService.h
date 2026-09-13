#pragma once

#include "core/AppError.h"
#include "core/Clock.h"
#include "core/FilePath.h"
#include "core/TaskRecurrenceMarker.h"
#include "data/SqliteWriterQueue.h"

#include <QList>
#include <QString>

#include <cstdint>
#include <future>
#include <optional>
#include <variant>

namespace hcb {

class ImportMutationService;

enum class TaskPriority : std::uint8_t {
  None,
  Low,
  Medium,
  High
};

enum class TaskReorderDirection : std::uint8_t {
  Earlier,
  Later
};

enum class TaskRecurrenceScope : std::uint8_t {
  ThisOccurrence,
  ThisAndFollowing,
  EntireSeries
};

struct TaskDue final {
  std::optional<QString> at;
  std::optional<QString> timeZone;
};

struct TaskCreateInput final {
  QString taskListId;
  std::optional<QString> parentTaskId;
  QString title;
  std::optional<QString> notes;
  std::optional<TaskDue> due;
  TaskPriority priority{TaskPriority::None};
};

struct TaskUpdateInput final {
  QString taskId;
  std::optional<std::optional<QString>> parentTaskId;
  std::optional<QString> title;
  std::optional<QString> notes;
  std::optional<TaskDue> due;
  std::optional<TaskPriority> priority;
  bool updateManagedRecurrenceTemplate{true};
};

struct TaskMutationReceipt final {
  QString taskId;
  QString updatedAt;
};

struct TaskRemoteReconciliationInput final {
  QString localTaskId;
  QString remoteTaskId;
  std::optional<QString> remoteEtag;
};

struct TaskMutationSnapshot final {
  QString taskId;
  QString taskListId;
  QString title;
  std::optional<QString> notes;
  std::optional<QString> parentTaskId;
  std::optional<QString> dueAt;
  std::optional<QString> dueTimeZone;
  TaskPriority priority{TaskPriority::None};
  bool completed{false};
  bool hasActiveChildren{false};
  std::optional<QString> managedRecurrenceSeriesId;
  std::optional<std::int64_t> managedRecurrenceOrdinal;
};

struct TaskRecurrenceReconciliation final {
  std::int64_t createdSuccessorCount{0};
  std::int64_t removedDuplicateCount{0};
  std::int64_t divergentDuplicateGroupCount{0};
};

using TaskMutationResult = std::variant<TaskMutationReceipt, AppError>;
using TaskBatchMutationResult = std::variant<QList<TaskMutationReceipt>, AppError>;
using TaskRemoteIdResult = std::variant<std::optional<QString>, AppError>;
using TaskMutationSnapshotResult = std::variant<QList<TaskMutationSnapshot>, AppError>;
using TaskRecurrenceReconciliationResult = std::variant<TaskRecurrenceReconciliation, AppError>;

class TaskMutationService final {
public:
  TaskMutationService(FilePath databasePath, const Clock& clock);
  TaskMutationService(const TaskMutationService&) = delete;
  TaskMutationService& operator=(const TaskMutationService&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] static std::variant<TaskCreateInput, AppError>
  validateCreate(TaskCreateInput input);
  [[nodiscard]] std::future<TaskMutationResult> create(TaskCreateInput input);
  [[nodiscard]] std::future<TaskBatchMutationResult> createBatch(QList<TaskCreateInput> inputs);
  [[nodiscard]] std::future<TaskMutationResult> update(TaskUpdateInput input);
  [[nodiscard]] std::future<TaskMutationResult> moveToTaskList(QString taskId, QString taskListId);
  [[nodiscard]] std::future<TaskMutationResult>
  reorder(QString taskId, TaskReorderDirection direction);
  [[nodiscard]] std::future<TaskMutationResult> setCompleted(QString taskId, bool completed);
  [[nodiscard]] std::future<TaskMutationResult>
  stopManagedRecurrence(QString taskId, TaskRecurrenceScope scope);
  [[nodiscard]] std::future<TaskMutationResult>
  reconfigureManagedRecurrence(QString taskId,
                               TaskRecurrenceFrequency frequency,
                               std::int32_t interval,
                               TaskRecurrenceEndCondition end,
                               std::optional<QString> recurrenceRule = std::nullopt,
                               std::optional<QList<QString>> exclusionDates = std::nullopt,
                               std::optional<QList<QString>> additionDates = std::nullopt);
  [[nodiscard]] std::future<TaskMutationResult> splitManagedRecurrence(QString taskId);
  [[nodiscard]] std::future<TaskMutationResult> remove(QString taskId);
  [[nodiscard]] std::future<TaskMutationResult> restore(QString taskId);
  [[nodiscard]] std::future<TaskMutationSnapshotResult> inspect(QList<QString> taskIds);
  [[nodiscard]] std::future<TaskMutationSnapshotResult>
  inspectManagedSeries(QList<QString> taskIds);
  [[nodiscard]] std::future<TaskMutationResult>
  reconcileGoogleTask(TaskRemoteReconciliationInput input);
  [[nodiscard]] std::future<TaskRecurrenceReconciliationResult>
  reconcileManagedRecurrences(QString accountId, QString taskListRemoteId);
  [[nodiscard]] std::future<TaskRemoteIdResult> remoteTaskId(QString taskId);

private:
  friend class ImportMutationService;
  [[nodiscard]] static TaskBatchMutationResult
  createBatchWithinTransaction(SqliteConnection& connection,
                               QList<TaskCreateInput> inputs,
                               const QString& updatedAt);
  const Clock& clock_;
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
