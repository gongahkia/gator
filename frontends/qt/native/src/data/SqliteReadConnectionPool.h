#pragma once

#include "core/AppError.h"
#include "core/FilePath.h"
#include "data/SqliteConnection.h"

#include <condition_variable>
#include <cstddef>
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
#include <vector>

namespace hcb {

template <typename Result> using SqliteReadResult = std::variant<Result, AppError>;

class SqliteReadConnectionPool final {
public:
  SqliteReadConnectionPool(FilePath databasePath, std::size_t connectionCount);
  SqliteReadConnectionPool(const SqliteReadConnectionPool&) = delete;
  SqliteReadConnectionPool& operator=(const SqliteReadConnectionPool&) = delete;
  ~SqliteReadConnectionPool();

  [[nodiscard]] std::shared_future<std::optional<AppError>> ready() const;

  template <typename Task>
  auto enqueue(Task&& task) -> std::future<
      SqliteReadResult<std::decay_t<std::invoke_result_t<std::decay_t<Task>, SqliteConnection&>>>>;

  void shutdown();

private:
  struct PendingTask final {
    std::function<void(SqliteConnection& connection)> run;
    std::function<void(AppError error)> fail;
  };

  [[nodiscard]] std::optional<AppError> schedule(PendingTask pendingTask);
  void run(FilePath databasePath);
  void failStartup(AppError error);

  std::mutex mutex_;
  std::condition_variable workAvailable_;
  std::deque<PendingTask> pendingTasks_;
  std::optional<AppError> startupError_;
  bool accepting_{true};
  bool readyResolved_{false};
  std::size_t pendingStarts_{0};
  std::promise<std::optional<AppError>> readyPromise_;
  std::shared_future<std::optional<AppError>> readyFuture_;
  std::vector<std::thread> workers_;
};

template <typename Task>
auto SqliteReadConnectionPool::enqueue(Task&& task) -> std::future<
    SqliteReadResult<std::decay_t<std::invoke_result_t<std::decay_t<Task>, SqliteConnection&>>>> {
  using Result = std::decay_t<std::invoke_result_t<std::decay_t<Task>, SqliteConnection&>>;
  static_assert(!std::is_void_v<Result>, "SQLite read tasks must return a result");
  static_assert(!std::is_same_v<Result, AppError>, "SQLite read tasks cannot return AppError");

  using TaskResult = SqliteReadResult<Result>;
  const auto completion = std::make_shared<std::promise<TaskResult>>();
  std::future<TaskResult> future = completion->get_future();
  const auto taskPointer = std::make_shared<std::decay_t<Task>>(std::forward<Task>(task));
  const auto fail = [completion](AppError error) { completion->set_value(std::move(error)); };
  PendingTask pendingTask;
  pendingTask.run = [completion, taskPointer](SqliteConnection& connection) {
    try {
      completion->set_value(
          TaskResult(std::in_place_type<Result>, std::invoke(*taskPointer, connection)));
    } catch (const std::exception&) {
      completion->set_value(
          AppError(AppErrorCode::Database, QStringLiteral("SQLite read task failed")));
    } catch (...) {
      completion->set_value(
          AppError(AppErrorCode::Database, QStringLiteral("SQLite read task failed")));
    }
  };
  pendingTask.fail = fail;
  if (const std::optional<AppError> error = schedule(std::move(pendingTask)); error.has_value()) {
    fail(*error);
  }
  return future;
}

} // namespace hcb
