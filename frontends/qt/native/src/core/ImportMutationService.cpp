#include "core/ImportMutationService.h"

#include "data/LocalSchema.h"
#include "data/SqliteTransaction.h"

#include <QDateTime>
#include <QTimeZone>

#include <chrono>
#include <utility>

namespace hcb {

namespace {

[[nodiscard]] QString timestamp(const Clock& clock) {
  const auto milliseconds =
      std::chrono::duration_cast<std::chrono::milliseconds>(clock.wallNow().time_since_epoch());
  return QDateTime::fromMSecsSinceEpoch(milliseconds.count(), QTimeZone::UTC)
      .toString(Qt::ISODateWithMs);
}

} // namespace

ImportMutationService::ImportMutationService(FilePath databasePath, const Clock& clock)
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

std::shared_future<SqliteWriteResult> ImportMutationService::ready() const {
  return initialization_;
}

std::future<ImportMutationResult>
ImportMutationService::create(QList<TaskCreateInput> tasks,
                              QList<CalendarEventCreateInput> events) {
  constexpr qsizetype kMaximumImportItems = 1'000;
  if (tasks.isEmpty() && events.isEmpty()) {
    std::promise<ImportMutationResult> completion;
    std::future<ImportMutationResult> future = completion.get_future();
    completion.set_value(
        AppError(AppErrorCode::Validation, QStringLiteral("Import has no eligible rows")));
    return future;
  }
  if (tasks.size() + events.size() > kMaximumImportItems) {
    std::promise<ImportMutationResult> completion;
    std::future<ImportMutationResult> future = completion.get_future();
    completion.set_value(
        AppError(AppErrorCode::Validation, QStringLiteral("Import exceeds the 1000-row limit")));
    return future;
  }
  const QString updatedAt = timestamp(clock_);
  return writerQueue_.enqueueResult(
      [tasks = std::move(tasks), events = std::move(events), updatedAt](
          SqliteConnection& connection) mutable {
        SqliteTransactionResult transactionResult = SqliteTransaction::begin(connection);
        if (std::holds_alternative<AppError>(transactionResult)) {
          return ImportMutationResult(std::get<AppError>(std::move(transactionResult)));
        }
        SqliteTransaction transaction = std::get<SqliteTransaction>(std::move(transactionResult));
        qsizetype taskCount = 0;
        qsizetype eventCount = 0;
        if (!tasks.isEmpty()) {
          TaskBatchMutationResult taskResult = TaskMutationService::createBatchWithinTransaction(
              connection, std::move(tasks), updatedAt);
          if (std::holds_alternative<AppError>(taskResult)) {
            return ImportMutationResult(std::get<AppError>(std::move(taskResult)));
          }
          taskCount = std::get<QList<TaskMutationReceipt>>(taskResult).size();
        }
        if (!events.isEmpty()) {
          CalendarEventBatchMutationResult eventResult =
              CalendarMutationService::createBatchWithinTransaction(
                  connection, std::move(events), updatedAt);
          if (std::holds_alternative<AppError>(eventResult)) {
            return ImportMutationResult(std::get<AppError>(std::move(eventResult)));
          }
          eventCount = std::get<QList<CalendarEventMutationReceipt>>(eventResult).size();
        }
        if (const std::optional<AppError> error = transaction.commit(); error.has_value()) {
          return ImportMutationResult(*error);
        }
        return ImportMutationResult(ImportMutationReceipt{.taskCount = taskCount,
                                                           .eventCount = eventCount});
      });
}

} // namespace hcb
