#pragma once

#include "core/AppError.h"
#include "core/FilePath.h"
#include "core/TaskModel.h"
#include "data/SqliteWriterQueue.h"

#include <QList>
#include <QString>

#include <cstdint>
#include <future>
#include <optional>
#include <variant>

namespace hcb {

struct TaskReadRequest final {
  std::optional<QString> accountId;
  bool selectedListsOnly{false};
  std::int64_t limit{10'000};
};

using TaskReadResult = std::variant<QList<TaskModelTask>, AppError>;

class TaskReadService final {
public:
  explicit TaskReadService(FilePath databasePath);
  TaskReadService(const TaskReadService&) = delete;
  TaskReadService& operator=(const TaskReadService&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] std::future<TaskReadResult> list(TaskReadRequest request = {});

private:
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
