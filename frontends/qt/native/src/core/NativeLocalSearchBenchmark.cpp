#include "core/NativeLocalSearchBenchmark.h"

#include "core/FilePath.h"
#include "core/LocalSearchService.h"
#include "data/LocalSchema.h"
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

constexpr std::size_t kCorpusTaskCount = 10'000;
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

[[nodiscard]] bool seedSearchCorpus(const FilePath& databasePath) {
  SqliteConnectionResult connectionResult =
      SqliteConnectionFactory::open(databasePath, SqliteOpenMode::ReadWriteCreate);
  if (!std::holds_alternative<SqliteConnection>(connectionResult)) {
    return false;
  }
  SqliteConnection connection = std::move(std::get<SqliteConnection>(connectionResult));
  if (!std::holds_alternative<SqliteMigrationRunResult>(LocalSchema::initialize(connection))) {
    return false;
  }
  return execute(
      connection.nativeHandle(),
      "BEGIN; "
      "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
      "missing_scopes_json, updated_at) VALUES "
      "('search-account', 'google', 'connected', '[]', '[]', '2026-01-01T00:00:00.000Z'); "
      "INSERT INTO local_task_lists (id, account_id, remote_id, title, updated_at) VALUES "
      "('search-list', 'search-account', 'search-list', 'Release planning', "
      "'2026-01-01T00:00:00.000Z'); "
      "WITH RECURSIVE sequence(value) AS ("
      "SELECT 1 UNION ALL SELECT value + 1 FROM sequence WHERE value < 10000"
      ") INSERT INTO local_tasks (id, task_list_id, remote_id, title, notes, updated_at) "
      "SELECT printf('search-task-%03d', value), 'search-list', printf('search-%03d', value), "
      "printf('Release planning %03d', value), "
      "printf('Prepare release item %03d', value), '2026-01-01T00:00:00.000Z' FROM sequence; "
      "COMMIT;");
}

} // namespace

std::optional<NativeLocalSearchBenchmarkResult>
NativeLocalSearchBenchmark::run(std::size_t iterations) {
  if (iterations == 0 || iterations > kMaximumIterations) {
    return std::nullopt;
  }
  QTemporaryDir temporaryDirectory;
  if (!temporaryDirectory.isValid()) {
    return std::nullopt;
  }
  const std::optional<FilePath> databasePath =
      FilePath::fromAbsolute(QDir(QFileInfo(temporaryDirectory.path()).canonicalFilePath())
                                 .filePath(QStringLiteral("local-search.sqlite")));
  if (!databasePath.has_value() || !seedSearchCorpus(*databasePath)) {
    return std::nullopt;
  }
  LocalSearchService search(*databasePath);
  const std::shared_future<std::optional<AppError>> ready = search.ready();
  if (ready.wait_for(kReadyTimeout) != std::future_status::ready || ready.get().has_value()) {
    return std::nullopt;
  }
  std::vector<qint64> samples;
  samples.reserve(iterations);
  std::size_t matchedResultCount = 0;
  for (std::size_t iteration = 0; iteration < iterations; ++iteration) {
    QElapsedTimer timer;
    timer.start();
    std::future<LocalSearchPageResult> future =
        search.search({.query = QStringLiteral("release"), .limit = 100});
    const LocalSearchPageResult pageResult = future.get();
    if (!std::holds_alternative<LocalSearchPage>(pageResult)) {
      return std::nullopt;
    }
    const LocalSearchPage& page = std::get<LocalSearchPage>(pageResult);
    if (page.items.isEmpty()) {
      return std::nullopt;
    }
    matchedResultCount = static_cast<std::size_t>(page.items.size());
    samples.push_back(timer.nsecsElapsed());
  }
  return summarize(kCorpusTaskCount, matchedResultCount, std::move(samples));
}

std::optional<NativeLocalSearchBenchmarkResult>
NativeLocalSearchBenchmark::summarize(std::size_t corpusTaskCount,
                                      std::size_t matchedResultCount,
                                      std::vector<qint64> samplesNanoseconds) {
  if (corpusTaskCount == 0 || matchedResultCount == 0 || samplesNanoseconds.empty() ||
      std::any_of(samplesNanoseconds.cbegin(), samplesNanoseconds.cend(), [](qint64 sample) {
        return sample < 0;
      })) {
    return std::nullopt;
  }
  std::sort(samplesNanoseconds.begin(), samplesNanoseconds.end());
  const qint64 minimum = samplesNanoseconds.front();
  const qint64 median = samplesNanoseconds[samplesNanoseconds.size() / 2];
  const qint64 maximum = samplesNanoseconds.back();
  return NativeLocalSearchBenchmarkResult{.corpusTaskCount = corpusTaskCount,
                                          .matchedResultCount = matchedResultCount,
                                          .samplesNanoseconds = std::move(samplesNanoseconds),
                                          .minimumNanoseconds = minimum,
                                          .medianNanoseconds = median,
                                          .maximumNanoseconds = maximum};
}

QByteArray NativeLocalSearchBenchmark::toJson(const NativeLocalSearchBenchmarkResult& result) {
  QJsonArray samples;
  for (const qint64 sample : result.samplesNanoseconds) {
    samples.append(sample);
  }
  return QJsonDocument(QJsonObject{{QStringLiteral("schema_version"), 1},
                                   {QStringLiteral("corpus_task_count"),
                                    static_cast<qint64>(result.corpusTaskCount)},
                                   {QStringLiteral("matched_result_count"),
                                    static_cast<qint64>(result.matchedResultCount)},
                                   {QStringLiteral("iterations"),
                                    static_cast<qint64>(result.samplesNanoseconds.size())},
                                   {QStringLiteral("minimum_ns"), result.minimumNanoseconds},
                                   {QStringLiteral("median_ns"), result.medianNanoseconds},
                                   {QStringLiteral("maximum_ns"), result.maximumNanoseconds},
                                   {QStringLiteral("samples_ns"), samples}})
      .toJson(QJsonDocument::Compact);
}

} // namespace hcb
