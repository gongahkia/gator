#include "data/SqliteWriterQueue.h"

#include <exception>
#include <utility>

namespace hcb {
namespace {

[[nodiscard]] AppError queueError(QString message) {
  return AppError(AppErrorCode::Database, std::move(message));
}

} // namespace

SqliteWriterQueue::SqliteWriterQueue(FilePath databasePath)
    : readyFuture_(readyPromise_.get_future().share()),
      worker_([this, databasePath = std::move(databasePath)]() mutable { run(databasePath); }) {}

SqliteWriterQueue::~SqliteWriterQueue() { shutdown(); }

std::shared_future<SqliteWriteResult> SqliteWriterQueue::ready() const { return readyFuture_; }

std::future<SqliteWriteResult> SqliteWriterQueue::enqueue(SqliteWriteTask task) {
  std::promise<SqliteWriteResult> completion;
  std::future<SqliteWriteResult> future = completion.get_future();
  if (!task) {
    completion.set_value(queueError(QStringLiteral("SQLite write task is empty")));
    return future;
  }

  {
    std::lock_guard lock(mutex_);
    if (startupError_.has_value()) {
      completion.set_value(startupError_);
      return future;
    }
    if (!accepting_) {
      completion.set_value(queueError(QStringLiteral("SQLite writer queue is closed")));
      return future;
    }
    pendingTasks_.push_back(PendingTask{std::move(task), std::move(completion)});
  }
  workAvailable_.notify_one();
  return future;
}

std::optional<AppError> SqliteWriterQueue::schedule(PendingTask pendingTask) {
  {
    std::lock_guard lock(mutex_);
    if (startupError_.has_value()) {
      return startupError_;
    }
    if (!accepting_) {
      return queueError(QStringLiteral("SQLite writer queue is closed"));
    }
    pendingTasks_.push_back(std::move(pendingTask));
  }
  workAvailable_.notify_one();
  return std::nullopt;
}

void SqliteWriterQueue::shutdown() {
  {
    std::lock_guard lock(mutex_);
    accepting_ = false;
  }
  workAvailable_.notify_one();
  if (worker_.joinable() && std::this_thread::get_id() != worker_.get_id()) {
    worker_.join();
  }
}

void SqliteWriterQueue::run(const FilePath& databasePath) {
  SqliteConnectionResult connectionResult =
      SqliteConnectionFactory::open(databasePath, SqliteOpenMode::ReadWriteCreate);
  if (std::holds_alternative<AppError>(connectionResult)) {
    failStartup(std::get<AppError>(std::move(connectionResult)));
    return;
  }
  SqliteConnection connection = std::move(std::get<SqliteConnection>(connectionResult));
  readyPromise_.set_value(std::nullopt);

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

    try {
      pendingTask.completion.set_value(pendingTask.task(connection));
    } catch (const std::exception&) {
      pendingTask.completion.set_value(queueError(QStringLiteral("SQLite write task failed")));
    } catch (...) {
      pendingTask.completion.set_value(queueError(QStringLiteral("SQLite write task failed")));
    }
  }
}

void SqliteWriterQueue::failStartup(AppError error) {
  {
    std::lock_guard lock(mutex_);
    startupError_ = error;
    accepting_ = false;
    readyPromise_.set_value(error);
    while (!pendingTasks_.empty()) {
      pendingTasks_.front().completion.set_value(error);
      pendingTasks_.pop_front();
    }
  }
  workAvailable_.notify_all();
}

} // namespace hcb
