#pragma once

#include "core/AppError.h"
#include "core/FilePath.h"
#include "data/SqliteWriterQueue.h"

#include <QList>
#include <QString>
#include <QStringList>

#include <cstdint>
#include <future>
#include <optional>
#include <variant>

namespace hcb {

struct TaskListSummary final {
  QString id;
  QString accountId;
  QString remoteId;
  QString title;
  std::optional<QString> etag;
  std::int64_t sortOrder;
  bool selected;
  std::optional<QString> remoteUpdatedAt;
  QString updatedAt;
  std::int64_t taskCount;
  std::int64_t activeTaskCount;
  QStringList taskTitles;
};

struct TaskListReadRequest final {
  std::optional<QString> accountId;
  bool selectedOnly{false};
  std::int64_t limit{50};
  std::int64_t offset{0};
};

struct TaskListPage final {
  QList<TaskListSummary> items;
  std::optional<std::int64_t> nextOffset;
  std::int64_t totalKnown;
};

using TaskListLookupResult = std::variant<std::optional<TaskListSummary>, AppError>;
using TaskListPageResult = std::variant<TaskListPage, AppError>;

class TaskListReadService final {
public:
  explicit TaskListReadService(FilePath databasePath);
  TaskListReadService(const TaskListReadService&) = delete;
  TaskListReadService& operator=(const TaskListReadService&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] std::future<TaskListLookupResult> find(QString taskListId);
  [[nodiscard]] std::future<TaskListPageResult> list(TaskListReadRequest request = {});

private:
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
