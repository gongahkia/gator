#include "core/LocalSearchService.h"

#include "core/LocalSearchQuery.h"
#include "core/TaskRecurrenceMarker.h"

#include "sqlite3.h"

#include <QRegularExpression>

#include <algorithm>
#include <future>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr int kMaximumPageSize = 100;
constexpr int kMaximumOffset = 10'000;

struct QuerySpec final {
  LocalSearchResource resource;
  const char* ftsTable;
  const char* sql;
};

struct StoredCandidate final {
  LocalSearchCandidate candidate;
  QString container;
  QString taskState;
  bool hidden{false};
  bool deleted{false};
  QString scheduledAt;
  QString priority;
  bool hasBody{false};
};

constexpr QuerySpec querySpecs[] = {
    {LocalSearchResource::TaskList,
     "local_task_lists_fts",
     R"(SELECT f.task_list_id, lists.title, '', lists.title, '', 0, '', '', '', 0
         FROM local_task_lists_fts AS f
         INNER JOIN local_task_lists AS lists ON lists.id = f.task_list_id
         WHERE local_task_lists_fts MATCH ?1 AND lists.deleted_at IS NULL)"},
    {LocalSearchResource::Task,
     "local_tasks_fts",
     R"(SELECT f.task_id, tasks.title, COALESCE(tasks.notes, ''), lists.title,
                tasks.state, tasks.is_hidden, COALESCE(tasks.deleted_at, ''),
                COALESCE(tasks.due_at, ''), tasks.priority,
                CASE WHEN length(trim(COALESCE(tasks.notes, ''))) > 0 THEN 1 ELSE 0 END
         FROM local_tasks_fts AS f
         INNER JOIN local_tasks AS tasks ON tasks.id = f.task_id
         INNER JOIN local_task_lists AS lists ON lists.id = tasks.task_list_id
         WHERE local_tasks_fts MATCH ?1 AND lists.deleted_at IS NULL)"},
    {LocalSearchResource::Note,
     "local_notes_fts",
     R"(SELECT f.note_id, f.title, f.body, lists.title, tasks.state, tasks.is_hidden,
                COALESCE(tasks.deleted_at, ''), '', 'none',
                CASE WHEN length(trim(f.body)) > 0 THEN 1 ELSE 0 END
         FROM local_notes_fts AS f
         INNER JOIN local_tasks AS tasks ON tasks.id = f.note_id
         INNER JOIN local_task_lists AS lists ON lists.id = f.list_id
         WHERE local_notes_fts MATCH ?1 AND lists.deleted_at IS NULL)"},
    {LocalSearchResource::Calendar,
     "local_calendars_fts",
     R"(SELECT f.calendar_id, calendars.title, '', calendars.title, '', 0, '', '', '', 0
         FROM local_calendars_fts AS f
         INNER JOIN local_calendars AS calendars ON calendars.id = f.calendar_id
         WHERE local_calendars_fts MATCH ?1 AND calendars.deleted_at IS NULL)"},
    {LocalSearchResource::Event,
     "local_calendar_events_fts",
     R"(SELECT f.calendar_event_id, events.title,
                TRIM(COALESCE(events.description, '') || ' ' || COALESCE(events.location, '')),
                calendars.title, '', 0, COALESCE(events.deleted_at, ''), events.start_at, '',
                CASE WHEN length(trim(COALESCE(events.description, ''))) > 0 THEN 1 ELSE 0 END
         FROM local_calendar_events_fts AS f
         INNER JOIN local_calendar_events AS events ON events.id = f.calendar_event_id
         INNER JOIN local_calendars AS calendars ON calendars.id = events.calendar_id
         WHERE local_calendar_events_fts MATCH ?1 AND events.status != 'cancelled'
               AND calendars.deleted_at IS NULL)"},
};

[[nodiscard]] AppError databaseError(const QString& message, int code) {
  return AppError(AppErrorCode::Database, message.arg(code));
}

[[nodiscard]] QString ftsQuery(const QString& query) {
  static const QRegularExpression word(QStringLiteral("[\\p{L}\\p{N}_]+"));
  QRegularExpressionMatchIterator matches = word.globalMatch(query);
  QStringList terms;
  while (matches.hasNext()) {
    terms.append(QStringLiteral("\"") + matches.next().captured() + QStringLiteral("\"*"));
  }
  return terms.join(QStringLiteral(" AND "));
}

[[nodiscard]] std::optional<QString> columnText(sqlite3_stmt* statement, int column) {
  const auto* value = reinterpret_cast<const char*>(sqlite3_column_text(statement, column));
  const int size = sqlite3_column_bytes(statement, column);
  return value == nullptr || size < 0 ? std::nullopt
                                      : std::optional<QString>(QString::fromUtf8(value, size));
}

[[nodiscard]] bool containsCaseInsensitive(const QString& haystack, const QString& needle) {
  return haystack.contains(needle, Qt::CaseInsensitive);
}

[[nodiscard]] bool matchesDateFilter(const QString& value, const LocalSearchDateFilter& filter) {
  if (filter.match == LocalSearchDateMatch::Any) {
    return !value.isEmpty();
  }
  if (filter.match == LocalSearchDateMatch::None) {
    return value.isEmpty();
  }
  const QDate date = QDate::fromString(value.left(10), Qt::ISODate);
  if (!date.isValid()) {
    return false;
  }
  switch (filter.match) {
  case LocalSearchDateMatch::Exact:
    return date == filter.first;
  case LocalSearchDateMatch::Before:
    return date < filter.first;
  case LocalSearchDateMatch::After:
    return date > filter.first;
  case LocalSearchDateMatch::Range:
    return date >= filter.first && date <= filter.last;
  case LocalSearchDateMatch::Any:
  case LocalSearchDateMatch::None:
    break;
  }
  return false;
}

[[nodiscard]] bool allowsResource(const LocalSearchParsedQuery& query,
                                  LocalSearchResource resource) {
  return query.resources.isEmpty() || query.resources.contains(resource);
}

[[nodiscard]] bool matchesFilters(const StoredCandidate& stored,
                                  const LocalSearchParsedQuery& query) {
  const LocalSearchResource resource = stored.candidate.resource;
  if (!allowsResource(query, resource)) {
    return false;
  }
  if (query.hasBody.has_value() &&
      (resource == LocalSearchResource::TaskList || resource == LocalSearchResource::Calendar ||
       stored.hasBody != *query.hasBody)) {
    return false;
  }
  if (query.taskList.has_value() &&
      (resource == LocalSearchResource::Calendar || resource == LocalSearchResource::Event ||
       !containsCaseInsensitive(stored.container, *query.taskList))) {
    return false;
  }
  if (query.calendar.has_value() &&
      (resource == LocalSearchResource::TaskList || resource == LocalSearchResource::Task ||
       resource == LocalSearchResource::Note ||
       !containsCaseInsensitive(stored.container, *query.calendar))) {
    return false;
  }
  if (query.taskStatus.has_value() || query.due.has_value() || query.priority.has_value()) {
    if (resource != LocalSearchResource::Task) {
      return false;
    }
  }
  if (query.start.has_value() && resource != LocalSearchResource::Event) {
    return false;
  }
  if (resource == LocalSearchResource::Task) {
    if (query.taskStatus.has_value()) {
      const QString& status = *query.taskStatus;
      if ((status == QStringLiteral("deleted") && !stored.deleted) ||
          (status != QStringLiteral("deleted") && stored.deleted) ||
          (status == QStringLiteral("hidden") && !stored.hidden) ||
          ((status == QStringLiteral("active") || status == QStringLiteral("completed")) &&
           (stored.hidden || stored.taskState != status))) {
        return false;
      }
    } else if (stored.deleted) {
      return false;
    }
    if (query.due.has_value() && !matchesDateFilter(stored.scheduledAt, *query.due)) {
      return false;
    }
    if (query.priority.has_value() && stored.priority != *query.priority) {
      return false;
    }
  }
  if (resource == LocalSearchResource::Event) {
    if (stored.deleted ||
        (query.start.has_value() && !matchesDateFilter(stored.scheduledAt, *query.start))) {
      return false;
    }
  }
  return true;
}

[[nodiscard]] std::variant<QList<LocalSearchCandidate>, AppError>
readCandidates(SqliteConnection& connection,
               const QString& fts,
               const LocalSearchParsedQuery& query,
               const CancellationToken& cancellation) {
  if (cancellation.stop_requested()) {
    return AppError(AppErrorCode::Cancelled, QStringLiteral("Search request was cancelled"));
  }
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite search connection is unavailable"));
  }
  QList<LocalSearchCandidate> candidates;
  candidates.reserve(512);
  for (const QuerySpec& spec : querySpecs) {
    if (cancellation.stop_requested()) {
      return AppError(AppErrorCode::Cancelled, QStringLiteral("Search request was cancelled"));
    }
    sqlite3_stmt* statement = nullptr;
    QString sql = QString::fromLatin1(spec.sql);
    if (fts.isEmpty()) {
      sql.replace(QString::fromLatin1(spec.ftsTable) + QStringLiteral(" MATCH ?1"),
                  QStringLiteral("?1 = ?1"));
    }
    const int prepareResult = sqlite3_prepare_v3(
        handle, sql.toUtf8().constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
    if (prepareResult != SQLITE_OK) {
      sqlite3_finalize(statement);
      return databaseError(QStringLiteral("SQLite search preparation failed (%1)"), prepareResult);
    }
    const QByteArray encodedQuery = fts.toUtf8();
    const int bindQueryResult = sqlite3_bind_text(statement,
                                                  1,
                                                  encodedQuery.constData(),
                                                  static_cast<int>(encodedQuery.size()),
                                                  SQLITE_TRANSIENT);
    if (bindQueryResult != SQLITE_OK) {
      sqlite3_finalize(statement);
      return databaseError(QStringLiteral("SQLite search binding failed (%1)"), bindQueryResult);
    }
    while (true) {
      if (cancellation.stop_requested()) {
        sqlite3_finalize(statement);
        return AppError(AppErrorCode::Cancelled, QStringLiteral("Search request was cancelled"));
      }
      const int stepResult = sqlite3_step(statement);
      if (stepResult == SQLITE_DONE) {
        break;
      }
      if (stepResult != SQLITE_ROW) {
        sqlite3_finalize(statement);
        return databaseError(QStringLiteral("SQLite search query failed (%1)"), stepResult);
      }
      const std::optional<QString> id = columnText(statement, 0);
      const std::optional<QString> title = columnText(statement, 1);
      const std::optional<QString> detail = columnText(statement, 2);
      const std::optional<QString> container = columnText(statement, 3);
      const std::optional<QString> state = columnText(statement, 4);
      const std::optional<QString> deletedAt = columnText(statement, 6);
      const std::optional<QString> scheduledAt = columnText(statement, 7);
      const std::optional<QString> priority = columnText(statement, 8);
      if (!id.has_value() || !title.has_value() || !detail.has_value() || !container.has_value() ||
          !state.has_value() || !deletedAt.has_value() || !scheduledAt.has_value() ||
          !priority.has_value()) {
        sqlite3_finalize(statement);
        return AppError(AppErrorCode::Database, QStringLiteral("SQLite search row is invalid"));
      }
      const QString renderedDetail = spec.resource == LocalSearchResource::Task
                                         ? parseTaskRecurrenceNotes(*detail).userNotes
                                         : *detail;
      StoredCandidate stored{
          .candidate = {.resource = spec.resource,
                        .id = *id,
                        .title = *title,
                        .detail = renderedDetail,
                        .scheduledAt = *scheduledAt,
                        .isUndatedTask =
                            spec.resource == LocalSearchResource::Task && scheduledAt->isEmpty()},
          .container = *container,
          .taskState = *state,
          .hidden = sqlite3_column_int(statement, 5) != 0,
          .deleted = !deletedAt->isEmpty(),
          .scheduledAt = *scheduledAt,
          .priority = *priority,
          .hasBody = sqlite3_column_int(statement, 9) != 0};
      if (matchesFilters(stored, query)) {
        candidates.append(std::move(stored.candidate));
      }
    }
    const int finalizeResult = sqlite3_finalize(statement);
    if (finalizeResult != SQLITE_OK) {
      return databaseError(QStringLiteral("SQLite search finalization failed (%1)"),
                           finalizeResult);
    }
  }
  return candidates;
}

[[nodiscard]] LocalSearchPageResult searchStored(SqliteConnection& connection,
                                                 const LocalSearchRequest& request,
                                                 const UnifiedLocalSearchRanker& ranker,
                                                 const CancellationToken& cancellation) {
  if (cancellation.stop_requested()) {
    return AppError(AppErrorCode::Cancelled, QStringLiteral("Search request was cancelled"));
  }
  const LocalSearchQueryResult parsedResult = LocalSearchQuery::parse(request.query);
  if (std::holds_alternative<AppError>(parsedResult)) {
    return std::get<AppError>(parsedResult);
  }
  const LocalSearchParsedQuery& parsed = std::get<LocalSearchParsedQuery>(parsedResult);
  const QString query = ftsQuery(parsed.plainText);
  const std::variant<QList<LocalSearchCandidate>, AppError> candidates =
      readCandidates(connection, query, parsed, cancellation);
  if (std::holds_alternative<AppError>(candidates)) {
    return std::get<AppError>(candidates);
  }
  const QList<LocalSearchRankedResult> ranked =
      ranker.rank(parsed.plainText, std::get<QList<LocalSearchCandidate>>(candidates), 0);
  const int rankedCount = static_cast<int>(ranked.size());
  const int start = std::min(request.offset, rankedCount);
  const int end = std::min(start + request.limit, rankedCount);
  QList<LocalSearchRankedResult> page;
  page.reserve(end - start);
  for (int index = start; index < end; ++index) {
    page.append(ranked.at(index));
  }
  return LocalSearchPage{
      .items = std::move(page), .totalKnown = rankedCount, .hasMore = end < rankedCount};
}

} // namespace

LocalSearchService::LocalSearchService(FilePath databasePath, std::size_t connectionCount)
    : readPool_(
          std::make_unique<SqliteReadConnectionPool>(std::move(databasePath), connectionCount)) {}

std::shared_future<std::optional<AppError>> LocalSearchService::ready() const {
  return readPool_->ready();
}

std::future<LocalSearchPageResult>
LocalSearchService::search(LocalSearchRequest request, const CancellationToken& cancellation) {
  if (cancellation.stop_requested()) {
    std::promise<LocalSearchPageResult> completion;
    std::future<LocalSearchPageResult> future = completion.get_future();
    completion.set_value(
        AppError(AppErrorCode::Cancelled, QStringLiteral("Search request was cancelled")));
    return future;
  }
  if (request.query.trimmed().isEmpty() || request.offset < 0 || request.offset > kMaximumOffset ||
      request.limit < 1) {
    std::promise<LocalSearchPageResult> completion;
    std::future<LocalSearchPageResult> future = completion.get_future();
    completion.set_value(
        AppError(AppErrorCode::Validation, QStringLiteral("Search request is invalid")));
    return future;
  }
  request.limit = std::clamp(request.limit, 1, kMaximumPageSize);
  auto queued = readPool_->enqueue(
      [this, request = std::move(request), cancellation](SqliteConnection& connection) {
        return searchStored(connection, request, ranker_, cancellation);
      });
  return std::async(std::launch::async, [queued = std::move(queued)]() mutable {
    auto result = queued.get();
    return std::holds_alternative<AppError>(result)
               ? LocalSearchPageResult(std::get<AppError>(std::move(result)))
               : std::get<LocalSearchPageResult>(std::move(result));
  });
}

} // namespace hcb
