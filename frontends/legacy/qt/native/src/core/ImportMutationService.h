#pragma once

#include "core/AppError.h"
#include "core/CalendarMutationService.h"
#include "core/Clock.h"
#include "core/FilePath.h"
#include "core/TaskMutationService.h"
#include "data/SqliteWriterQueue.h"

#include <future>
#include <variant>

namespace hcb {

struct ImportMutationReceipt final {
  qsizetype taskCount{0};
  qsizetype eventCount{0};
};

using ImportMutationResult = std::variant<ImportMutationReceipt, AppError>;

class ImportMutationService final {
public:
  ImportMutationService(FilePath databasePath, const Clock& clock);
  ImportMutationService(const ImportMutationService&) = delete;
  ImportMutationService& operator=(const ImportMutationService&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] std::future<ImportMutationResult>
  create(QList<TaskCreateInput> tasks, QList<CalendarEventCreateInput> events);

private:
  const Clock& clock_;
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
