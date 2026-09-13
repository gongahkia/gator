#pragma once

#include "core/AppError.h"
#include "core/Clock.h"
#include "core/FilePath.h"
#include "data/SqliteWriterQueue.h"

#include <QString>

#include <future>
#include <optional>
#include <variant>

namespace hcb {

struct TaskListCreateInput final {
  QString accountId;
  QString title;
};

struct TaskListUpdateInput final {
  QString taskListId;
  QString title;
};

struct TaskListSelectionInput final {
  QString taskListId;
  bool selected;
};

struct TaskListMutationReceipt final {
  QString taskListId;
  QString updatedAt;
};

struct TaskListRemoteReconciliationInput final {
  QString localTaskListId;
  QString remoteTaskListId;
  std::optional<QString> remoteEtag;
};

using TaskListMutationResult = std::variant<TaskListMutationReceipt, AppError>;
using TaskListRemoteIdResult = std::variant<std::optional<QString>, AppError>;

class TaskListMutationService final {
public:
  TaskListMutationService(FilePath databasePath, const Clock& clock);
  TaskListMutationService(const TaskListMutationService&) = delete;
  TaskListMutationService& operator=(const TaskListMutationService&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] std::future<TaskListMutationResult> create(TaskListCreateInput input);
  [[nodiscard]] std::future<TaskListMutationResult> update(TaskListUpdateInput input);
  [[nodiscard]] std::future<TaskListMutationResult> setSelected(TaskListSelectionInput input);
  [[nodiscard]] std::future<TaskListMutationResult> remove(QString taskListId);
  [[nodiscard]] std::future<TaskListMutationResult>
  reconcileGoogleTaskList(TaskListRemoteReconciliationInput input);
  [[nodiscard]] std::future<TaskListRemoteIdResult> remoteTaskListId(QString taskListId);

private:
  const Clock& clock_;
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
