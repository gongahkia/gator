#include "core/NativeQuickCaptureBenchmark.h"

#include "core/Clock.h"
#include "core/FilePath.h"
#include "core/TaskMutationService.h"
#include "data/SqliteConnection.h"
#include "sqlite3.h"

#include <QDir>
#include <QElapsedTimer>
#include <QFileInfo>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QTemporaryDir>

#include <algorithm>
#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr std::size_t kMaximumIterations = 20;
constexpr auto kReadyTimeout = std::chrono::seconds(5);

[[nodiscard]] bool execute(sqlite3* handle, const char* sql) {
  if (handle == nullptr) {
    return false;
  }
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  sqlite3_free(errorMessage);
  return result == SQLITE_OK;
}

[[nodiscard]] bool seedQuickCaptureList(const FilePath& databasePath) {
  SqliteConnectionResult connectionResult =
      SqliteConnectionFactory::open(databasePath, SqliteOpenMode::ReadWriteCreate);
  if (!std::holds_alternative<SqliteConnection>(connectionResult)) {
    return false;
  }
  SqliteConnection connection = std::move(std::get<SqliteConnection>(connectionResult));
  sqlite3* const handle = connection.nativeHandle();
  return execute(handle,
                 "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
                 "missing_scopes_json, updated_at) VALUES "
                 "('quick-capture-account', 'google', 'connected', '[]', '[]', "
                 "'2026-01-01T00:00:00.000Z')") &&
         execute(handle,
                 "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) "
                 "VALUES ('quick-capture-list', 'quick-capture-account', 'quick-capture', "
                 "'Quick capture', "
                 "'2026-01-01T00:00:00.000Z')");
}

} // namespace

std::optional<NativeQuickCaptureBenchmarkResult>
NativeQuickCaptureBenchmark::run(std::size_t iterations) {
  if (iterations == 0 || iterations > kMaximumIterations) {
    return std::nullopt;
  }
  QTemporaryDir temporaryDirectory;
  if (!temporaryDirectory.isValid()) {
    return std::nullopt;
  }
  const std::optional<FilePath> databasePath =
      FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                 .filePath(QStringLiteral("quick-capture.sqlite")));
  if (!databasePath.has_value()) {
    return std::nullopt;
  }
  SystemClock clock;
  TaskMutationService tasks(*databasePath, clock);
  const std::shared_future<SqliteWriteResult> ready = tasks.ready();
  if (ready.wait_for(kReadyTimeout) != std::future_status::ready) {
    return std::nullopt;
  }
  const SqliteWriteResult& initialization = ready.get();
  if (initialization.has_value()) {
    return std::nullopt;
  }
  if (!seedQuickCaptureList(*databasePath)) {
    return std::nullopt;
  }
  std::vector<qint64> samples;
  samples.reserve(iterations);
  for (std::size_t iteration = 0; iteration < iterations; ++iteration) {
    QElapsedTimer timer;
    timer.start();
    const qulonglong captureNumber = static_cast<qulonglong>(iteration) + 1U;
    std::future<TaskMutationResult> future =
        tasks.create({.taskListId = QStringLiteral("quick-capture-list"),
                      .title = QStringLiteral("Quick capture %1").arg(captureNumber)});
    const TaskMutationResult result = future.get();
    if (!std::holds_alternative<TaskMutationReceipt>(result)) {
      return std::nullopt;
    }
    samples.push_back(timer.nsecsElapsed());
  }
  return summarize(std::move(samples));
}

std::optional<NativeQuickCaptureBenchmarkResult>
NativeQuickCaptureBenchmark::summarize(std::vector<qint64> samplesNanoseconds) {
  if (samplesNanoseconds.empty() || std::any_of(samplesNanoseconds.cbegin(),
                                                samplesNanoseconds.cend(),
                                                [](qint64 sample) { return sample < 0; })) {
    return std::nullopt;
  }
  std::sort(samplesNanoseconds.begin(), samplesNanoseconds.end());
  const qint64 minimum = samplesNanoseconds.front();
  const qint64 median = samplesNanoseconds[samplesNanoseconds.size() / 2];
  const qint64 maximum = samplesNanoseconds.back();
  return NativeQuickCaptureBenchmarkResult{.samplesNanoseconds = std::move(samplesNanoseconds),
                                           .minimumNanoseconds = minimum,
                                           .medianNanoseconds = median,
                                           .maximumNanoseconds = maximum};
}

QByteArray NativeQuickCaptureBenchmark::toJson(const NativeQuickCaptureBenchmarkResult& result) {
  QJsonArray samples;
  for (const qint64 sample : result.samplesNanoseconds) {
    samples.append(sample);
  }
  return QJsonDocument(QJsonObject{{QStringLiteral("schema_version"), 1},
                                   {QStringLiteral("iterations"),
                                    static_cast<qint64>(result.samplesNanoseconds.size())},
                                   {QStringLiteral("minimum_ns"), result.minimumNanoseconds},
                                   {QStringLiteral("median_ns"), result.medianNanoseconds},
                                   {QStringLiteral("maximum_ns"), result.maximumNanoseconds},
                                   {QStringLiteral("samples_ns"), samples}})
      .toJson(QJsonDocument::Compact);
}

} // namespace hcb
