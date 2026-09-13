#pragma once

#include "core/AppError.h"
#include "core/FilePath.h"
#include "data/SqliteConnection.h"

#include <condition_variable>
#include <concepts>
#include <deque>
#include <exception>
#include <functional>
#include <future>
#include <memory>
#include <mutex>
#include <optional>
#include <thread>
#include <type_traits>
#include <utility>
#include <variant>

namespace hcb {

using SqliteWriteResult = std::optional<AppError>;
using SqliteWriteTask = std::function<SqliteWriteResult(SqliteConnection& connection)>;

class SqliteWriterQueue final {
public:
  explicit SqliteWriterQueue(FilePath databasePath);
  SqliteWriterQueue(const SqliteWriterQueue&) = delete;
  SqliteWriterQueue& operator=(const SqliteWriterQueue&) = delete;
  ~SqliteWriterQueue();

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] std::future<SqliteWriteResult> enqueue(SqliteWriteTask task);

  template <typename Task>
  auto enqueueResult(Task&& task)
      -> std::future<std::decay_t<std::invoke_result_t<std::decay_t<Task>, SqliteConnection&>>>;

  void shutdown();

private:
  struct PendingTask final {
    SqliteWriteTask task;
    std::promise<SqliteWriteResult> completion;
  };

  [[nodiscard]] std::optional<AppError> schedule(PendingTask pendingTask);
  void run(const FilePath& databasePath);
  void failStartup(AppError error);

  std::mutex mutex_;
  std::condition_variable workAvailable_;
  std::deque<PendingTask> pendingTasks_;
  std::optional<AppError> startupError_;
  bool accepting_{true};
  std::promise<SqliteWriteResult> readyPromise_;
  std::shared_future<SqliteWriteResult> readyFuture_;
  std::thread worker_;
};

template <typename Task>
auto SqliteWriterQueue::enqueueResult(Task&& task)
    -> std::future<std::decay_t<std::invoke_result_t<std::decay_t<Task>, SqliteConnection&>>> {
  using Result = std::decay_t<std::invoke_result_t<std::decay_t<Task>, SqliteConnection&>>;
  static_assert(!std::is_void_v<Result>, "SQLite write tasks must return a result");
  static_assert(std::constructible_from<Result, AppError>,
                "SQLite write task result must accept AppError");

  const auto completion = std::make_shared<std::promise<Result>>();
  std::future<Result> future = completion->get_future();
  const auto taskPointer = std::make_shared<std::decay_t<Task>>(std::forward<Task>(task));
  PendingTask pendingTask;
  pendingTask.task = [completion, taskPointer](SqliteConnection& connection) -> SqliteWriteResult {
    try {
      completion->set_value(std::invoke(*taskPointer, connection));
    } catch (const std::exception&) {
      completion->set_value(
          Result(AppError(AppErrorCode::Database, QStringLiteral("SQLite write task failed"))));
    } catch (...) {
      completion->set_value(
          Result(AppError(AppErrorCode::Database, QStringLiteral("SQLite write task failed"))));
    }
    return std::nullopt;
  };
  if (const std::optional<AppError> error = schedule(std::move(pendingTask)); error.has_value()) {
    completion->set_value(Result(*error));
  }
  return future;
}

} // namespace hcb
