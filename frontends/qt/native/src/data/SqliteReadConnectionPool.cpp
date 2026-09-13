#include "data/SqliteReadConnectionPool.h"

#include <exception>
#include <utility>

namespace hcb {
namespace {

[[nodiscard]] AppError poolError(QString message) {
  return AppError(AppErrorCode::Database, std::move(message));
}

} // namespace

SqliteReadConnectionPool::SqliteReadConnectionPool(FilePath databasePath,
                                                   std::size_t connectionCount)
    : pendingStarts_(connectionCount), readyFuture_(readyPromise_.get_future().share()) {
  if (connectionCount == 0) {
    startupError_ = poolError(QStringLiteral("SQLite read connection count must be positive"));
    accepting_ = false;
    readyResolved_ = true;
    readyPromise_.set_value(startupError_);
    return;
  }

  workers_.reserve(connectionCount);
  try {
    for (std::size_t index = 0; index < connectionCount; ++index) {
      workers_.emplace_back([this, databasePath] { run(databasePath); });
    }
  } catch (...) {
    shutdown();
    throw;
  }
}

SqliteReadConnectionPool::~SqliteReadConnectionPool() { shutdown(); }

std::shared_future<std::optional<AppError>> SqliteReadConnectionPool::ready() const {
  return readyFuture_;
}

void SqliteReadConnectionPool::shutdown() {
  std::deque<PendingTask> pendingTasks;
  const AppError closedError = poolError(QStringLiteral("SQLite read connection pool is closed"));
  {
    std::lock_guard lock(mutex_);
    accepting_ = false;
    if (!readyResolved_) {
      readyResolved_ = true;
      readyPromise_.set_value(closedError);
    }
    pendingTasks.swap(pendingTasks_);
  }
  for (PendingTask& pendingTask : pendingTasks) {
    pendingTask.fail(closedError);
  }
  workAvailable_.notify_all();
  for (std::thread& worker : workers_) {
    if (worker.joinable() && std::this_thread::get_id() != worker.get_id()) {
      worker.join();
    }
  }
}

std::optional<AppError> SqliteReadConnectionPool::schedule(PendingTask pendingTask) {
  {
    std::lock_guard lock(mutex_);
    if (startupError_.has_value()) {
      return startupError_;
    }
    if (!accepting_) {
      return poolError(QStringLiteral("SQLite read connection pool is closed"));
    }
    pendingTasks_.push_back(std::move(pendingTask));
  }
  workAvailable_.notify_one();
  return std::nullopt;
}

void SqliteReadConnectionPool::run(FilePath databasePath) {
  SqliteConnectionResult connectionResult =
      SqliteConnectionFactory::open(databasePath, SqliteOpenMode::ReadOnly);
  if (std::holds_alternative<AppError>(connectionResult)) {
    failStartup(std::get<AppError>(std::move(connectionResult)));
    return;
  }
  SqliteConnection connection = std::move(std::get<SqliteConnection>(connectionResult));

  {
    std::lock_guard lock(mutex_);
    if (!accepting_) {
      return;
    }
    --pendingStarts_;
    if (pendingStarts_ == 0 && !readyResolved_) {
      readyResolved_ = true;
      readyPromise_.set_value(std::nullopt);
    }
  }

  while (true) {
    PendingTask pendingTask;
    {
      std::unique_lock lock(mutex_);
      workAvailable_.wait(lock, [this] { return !pendingTasks_.empty() || !accepting_; });
      if (pendingTasks_.empty()) {
        return;
      }
      pendingTask = std::move(pendingTasks_.front());
      pendingTasks_.pop_front();
    }
    pendingTask.run(connection);
  }
}

void SqliteReadConnectionPool::failStartup(AppError error) {
  std::deque<PendingTask> pendingTasks;
  {
    std::lock_guard lock(mutex_);
    if (startupError_.has_value()) {
      return;
    }
    startupError_ = error;
    accepting_ = false;
    if (!readyResolved_) {
      readyResolved_ = true;
      readyPromise_.set_value(error);
    }
    pendingTasks.swap(pendingTasks_);
  }
  for (PendingTask& pendingTask : pendingTasks) {
    pendingTask.fail(error);
  }
  workAvailable_.notify_all();
}

} // namespace hcb
