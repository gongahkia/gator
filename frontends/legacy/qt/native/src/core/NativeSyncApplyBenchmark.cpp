#include "core/NativeSyncApplyBenchmark.h"

#include "core/Clock.h"
#include "core/FilePath.h"
#include "core/GoogleMirrorStore.h"
#include "core/NativePerformanceFixture.h"
#include "data/SqliteConnection.h"
#include "sqlite3.h"

#include <QDir>
#include <QElapsedTimer>
#include <QFileInfo>
#include <QJsonDocument>
#include <QJsonObject>
#include <QTemporaryDir>

#include <chrono>
#include <future>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr auto kReadyTimeout = std::chrono::minutes(2);
constexpr char kAccountId[] = "wrapper-scale-account";
constexpr char kTimestamp[] = "2026-01-05T09:00:00.000Z";

[[nodiscard]] bool execute(sqlite3* handle, const char* sql) {
  if (handle == nullptr) {
    return false;
  }
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  sqlite3_free(errorMessage);
  return result == SQLITE_OK;
}

[[nodiscard]] std::optional<FilePath> databasePathFor(const QTemporaryDir& directory) {
  const QString canonicalDirectory = QFileInfo(directory.path()).canonicalFilePath();
  return canonicalDirectory.isEmpty()
             ? std::nullopt
             : FilePath::fromAbsolute(
                   QDir(canonicalDirectory).filePath(QStringLiteral("sync.sqlite")));
}

[[nodiscard]] bool ready(GoogleMirrorStore& store) {
  const std::shared_future<SqliteWriteResult> initialization = store.ready();
  return initialization.wait_for(kReadyTimeout) == std::future_status::ready &&
         !initialization.get().has_value();
}

template <typename Result> [[nodiscard]] bool succeeded(std::future<Result>& future) {
  if (future.wait_for(kReadyTimeout) != std::future_status::ready) {
    return false;
  }
  const Result result = future.get();
  return std::holds_alternative<std::monostate>(result);
}

[[nodiscard]] QList<GoogleTaskListMirror> taskListsFor(const NativePerformanceFixture& fixture) {
  QList<GoogleTaskListMirror> taskLists;
  taskLists.reserve(static_cast<qsizetype>(fixture.taskLists.size()));
  for (const auto& [id, title] : fixture.taskLists) {
    taskLists.append({.id = id, .title = title, .updatedAt = QString::fromLatin1(kTimestamp)});
  }
  return taskLists;
}

[[nodiscard]] QList<GoogleTaskMirror> tasksFor(const NativePerformanceFixture& fixture,
                                               const QString& taskListId) {
  QList<GoogleTaskMirror> tasks;
  for (const NativePerformanceTaskFixture& task : fixture.tasks) {
    if (task.taskListId != taskListId) {
      continue;
    }
    tasks.append({.id = task.id,
                  .taskListId = task.taskListId,
                  .title = task.title,
                  .notes = std::optional<QString>(QStringLiteral("Generated benchmark task")),
                  .status = task.status == QStringLiteral("completed")
                                ? GoogleTaskStatus::Completed
                                : GoogleTaskStatus::NeedsAction,
                  .dueAt = task.dueAt,
                  .completedAt = task.completedAt,
                  .deleted = false,
                  .hidden = false,
                  .isAssigned = false,
                  .position = QString::number(static_cast<qulonglong>(task.sortOrder)),
                  .updatedAt = task.updatedAt});
  }
  return tasks;
}

[[nodiscard]] QList<GoogleCalendarMirror> calendarsFor(const NativePerformanceFixture& fixture) {
  QList<GoogleCalendarMirror> calendars;
  calendars.reserve(static_cast<qsizetype>(fixture.calendars.size()));
  for (qsizetype index = 0; index < static_cast<qsizetype>(fixture.calendars.size()); ++index) {
    const auto& [id, title] = fixture.calendars.at(static_cast<std::size_t>(index));
    calendars.append({.id = id,
                      .title = title,
                      .timeZone = QStringLiteral("UTC"),
                      .accessRole = GoogleCalendarAccessRole::Owner,
                      .selected = true,
                      .hidden = false,
                      .primary = index == 0,
                      .deleted = false});
  }
  return calendars;
}

[[nodiscard]] QList<GoogleCalendarEventMirror> eventsFor(const NativePerformanceFixture& fixture,
                                                         const QString& calendarId) {
  QList<GoogleCalendarEventMirror> events;
  for (const NativePerformanceEventFixture& event : fixture.eventInstances) {
    if (event.calendarId != calendarId) {
      continue;
    }
    events.append({.id = event.id,
                   .calendarId = event.calendarId,
                   .status = GoogleCalendarEventStatus::Confirmed,
                   .title = event.title,
                   .startAt = event.startsAt,
                   .startTimeZone = QStringLiteral("UTC"),
                   .endAt = event.endsAt,
                   .endTimeZone = QStringLiteral("UTC"),
                   .allDay = event.isAllDay,
                   .recurringEventId = event.recurringEventId,
                   .originalStartAt = event.originalStartAt,
                   .updatedAt = event.updatedAt});
  }
  return events;
}

[[nodiscard]] bool seedAccount(const FilePath& databasePath) {
  SqliteConnectionResult connectionResult =
      SqliteConnectionFactory::open(databasePath, SqliteOpenMode::ReadWriteCreate);
  if (!std::holds_alternative<SqliteConnection>(connectionResult)) {
    return false;
  }
  SqliteConnection connection = std::move(std::get<SqliteConnection>(connectionResult));
  return execute(connection.nativeHandle(),
                 "INSERT INTO local_accounts (id, provider, connection_state, granted_scopes_json, "
                 "missing_scopes_json, updated_at) VALUES "
                 "('wrapper-scale-account', 'google', 'connected', '[]', '[]', "
                 "'2026-01-05T09:00:00.000Z')");
}

[[nodiscard]] bool seedQueuedMutations(const FilePath& databasePath,
                                       const NativePerformanceFixture& fixture) {
  SqliteConnectionResult connectionResult =
      SqliteConnectionFactory::open(databasePath, SqliteOpenMode::ReadWriteCreate);
  if (!std::holds_alternative<SqliteConnection>(connectionResult)) {
    return false;
  }
  SqliteConnection connection = std::move(std::get<SqliteConnection>(connectionResult));
  sqlite3* const handle = connection.nativeHandle();
  if (!execute(handle, "BEGIN")) {
    return false;
  }
  for (std::size_t index = 0; index < fixture.queuedMutations.size(); ++index) {
    const NativePerformanceQueuedMutationFixture& mutation = fixture.queuedMutations.at(index);
    const char* table =
        mutation.resourceType == QStringLiteral("task") ? "local_tasks" : "local_calendar_events";
    const QByteArray sql =
        QStringLiteral("INSERT INTO local_pending_mutations "
                       "(id, account_id, resource_type, resource_id, operation, payload_json, "
                       "status, attempt_count, created_at, updated_at) "
                       "SELECT ?1, ?2, ?3, id, 'update', '{}', 'pending', 0, ?4, ?4 "
                       "FROM %1 ORDER BY id LIMIT 1 OFFSET ?5")
            .arg(QString::fromLatin1(table))
            .toUtf8();
    sqlite3_stmt* statement = nullptr;
    if (sqlite3_prepare_v3(
            handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr) !=
        SQLITE_OK) {
      sqlite3_finalize(statement);
      static_cast<void>(execute(handle, "ROLLBACK"));
      return false;
    }
    const QByteArray mutationId = mutation.id.toUtf8();
    const QByteArray resourceType = mutation.resourceType.toUtf8();
    const int bindResult =
        sqlite3_bind_text(statement,
                          1,
                          mutationId.constData(),
                          static_cast<int>(mutationId.size()),
                          SQLITE_TRANSIENT) == SQLITE_OK &&
        sqlite3_bind_text(statement, 2, kAccountId, -1, SQLITE_STATIC) == SQLITE_OK &&
        sqlite3_bind_text(statement,
                          3,
                          resourceType.constData(),
                          static_cast<int>(resourceType.size()),
                          SQLITE_TRANSIENT) == SQLITE_OK &&
        sqlite3_bind_text(statement, 4, kTimestamp, -1, SQLITE_STATIC) == SQLITE_OK &&
        sqlite3_bind_int64(statement, 5, static_cast<sqlite3_int64>(index / 2)) == SQLITE_OK;
    const int stepResult = bindResult ? sqlite3_step(statement) : SQLITE_ERROR;
    const int finalizeResult = sqlite3_finalize(statement);
    if (stepResult != SQLITE_DONE || sqlite3_changes(handle) != 1 || finalizeResult != SQLITE_OK) {
      static_cast<void>(execute(handle, "ROLLBACK"));
      return false;
    }
  }
  return execute(handle, "COMMIT");
}

} // namespace

std::optional<NativeSyncApplyBenchmarkResult> NativeSyncApplyBenchmark::run() {
  const NativePerformanceFixture fixture = NativePerformanceFixtureGenerator::wrapperScale();
  QTemporaryDir temporaryDirectory;
  if (!temporaryDirectory.isValid()) {
    return std::nullopt;
  }
  const std::optional<FilePath> databasePath = databasePathFor(temporaryDirectory);
  if (!databasePath.has_value()) {
    return std::nullopt;
  }
  SystemClock clock;
  GoogleMirrorStore store(*databasePath, clock);
  if (!ready(store) || !seedAccount(*databasePath)) {
    return std::nullopt;
  }
  QList<GoogleTaskListMirror> taskLists = taskListsFor(fixture);
  std::future<GoogleMirrorWriteResult> taskListsWrite =
      store.mergeTaskLists(QString::fromLatin1(kAccountId), std::move(taskLists), false);
  if (!succeeded(taskListsWrite)) {
    return std::nullopt;
  }
  QList<GoogleCalendarMirror> calendars = calendarsFor(fixture);
  std::future<GoogleMirrorWriteResult> calendarsWrite =
      store.mergeCalendars(QString::fromLatin1(kAccountId), std::move(calendars), false);
  if (!succeeded(calendarsWrite)) {
    return std::nullopt;
  }
  for (const auto& taskList : fixture.taskLists) {
    const QString& taskListId = taskList.first;
    std::future<GoogleMirrorWriteResult> write = store.mergeTasks(
        QString::fromLatin1(kAccountId), taskListId, tasksFor(fixture, taskListId), false);
    if (!succeeded(write)) {
      return std::nullopt;
    }
  }
  for (const auto& calendar : fixture.calendars) {
    const QString& calendarId = calendar.first;
    std::future<GoogleMirrorWriteResult> write = store.mergeCalendarEvents(
        QString::fromLatin1(kAccountId), calendarId, eventsFor(fixture, calendarId), false);
    if (!succeeded(write)) {
      return std::nullopt;
    }
  }
  if (!seedQueuedMutations(*databasePath, fixture)) {
    return std::nullopt;
  }

  QElapsedTimer timer;
  timer.start();
  for (const auto& taskList : fixture.taskLists) {
    const QString& taskListId = taskList.first;
    std::future<GoogleMirrorWriteResult> write = store.mergeTasks(
        QString::fromLatin1(kAccountId), taskListId, tasksFor(fixture, taskListId), false);
    if (!succeeded(write)) {
      return std::nullopt;
    }
  }
  for (const auto& calendar : fixture.calendars) {
    const QString& calendarId = calendar.first;
    std::future<GoogleMirrorWriteResult> write = store.mergeCalendarEvents(
        QString::fromLatin1(kAccountId), calendarId, eventsFor(fixture, calendarId), false);
    if (!succeeded(write)) {
      return std::nullopt;
    }
  }
  return NativeSyncApplyBenchmarkResult{.taskCount = fixture.counts.tasks,
                                        .eventCount = fixture.counts.eventInstances,
                                        .recurrenceExceptionCount =
                                            fixture.counts.recurrenceExceptions,
                                        .queuedMutationCount = fixture.counts.queuedMutations,
                                        .elapsedNanoseconds = timer.nsecsElapsed()};
}

QByteArray NativeSyncApplyBenchmark::toJson(const NativeSyncApplyBenchmarkResult& result) {
  return QJsonDocument(
             QJsonObject{{QStringLiteral("schema_version"), 1},
                         {QStringLiteral("task_count"), static_cast<qint64>(result.taskCount)},
                         {QStringLiteral("event_count"), static_cast<qint64>(result.eventCount)},
                         {QStringLiteral("recurrence_exception_count"),
                          static_cast<qint64>(result.recurrenceExceptionCount)},
                         {QStringLiteral("queued_mutation_count"),
                          static_cast<qint64>(result.queuedMutationCount)},
                         {QStringLiteral("elapsed_ns"), result.elapsedNanoseconds}})
      .toJson(QJsonDocument::Compact);
}

} // namespace hcb
