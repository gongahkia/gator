#include "core/CalendarReadService.h"

#include "data/LocalSchema.h"
#include "sqlite3.h"

#include <QByteArray>
#include <QDateTime>
#include <QString>

#include <algorithm>
#include <future>
#include <limits>
#include <optional>
#include <utility>
#include <variant>

namespace hcb {
namespace {

constexpr qsizetype kMaximumIdentifierLength = 256;
constexpr std::int64_t kMaximumCalendarPageLimit = 100;
constexpr std::int64_t kMaximumEventPageLimit = 25'000;
constexpr std::int64_t kMaximumInstanceCacheTargetLimit = 1'000;
constexpr qsizetype kMaximumCalendarFilterCount = 25;
constexpr qint64 kMaximumRangeDurationMilliseconds = 397LL * 24 * 60 * 60 * 1000;

using CalendarDecodeResult = std::variant<CalendarSummary, AppError>;
using CalendarEventDecodeResult = std::variant<CalendarEventSummary, AppError>;

[[nodiscard]] AppError databaseError(const QString& message, int result) {
  return AppError(AppErrorCode::Database, message.arg(result));
}

[[nodiscard]] bool isValidIdentifier(const QString& value) {
  return !value.isEmpty() && value == value.trimmed() && value.size() <= kMaximumIdentifierLength &&
         !value.contains(QChar::Null);
}

[[nodiscard]] std::optional<QDateTime> parseDateTime(const QString& value) {
  if (value.size() > 64 || !value.contains(u'T')) {
    return std::nullopt;
  }
  const QDateTime dateTime = QDateTime::fromString(value, Qt::ISODateWithMs);
  return dateTime.isValid() ? std::optional<QDateTime>(dateTime) : std::nullopt;
}

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

[[nodiscard]] std::optional<AppError>
bindText(sqlite3_stmt* statement, int index, const QString& value) {
  const QByteArray utf8 = value.toUtf8();
  const int result = sqlite3_bind_text(
      statement, index, utf8.constData(), static_cast<int>(utf8.size()), SQLITE_TRANSIENT);
  if (result != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite calendar binding failed (%1)"), result);
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<AppError>
bindInteger(sqlite3_stmt* statement, int index, std::int64_t value) {
  const int result = sqlite3_bind_int64(statement, index, value);
  if (result != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite calendar binding failed (%1)"), result);
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<QString> optionalText(sqlite3_stmt* statement, int index) {
  if (sqlite3_column_type(statement, index) == SQLITE_NULL) {
    return std::nullopt;
  }
  const auto* value = reinterpret_cast<const char*>(sqlite3_column_text(statement, index));
  const int byteCount = sqlite3_column_bytes(statement, index);
  if (value == nullptr || byteCount < 0) {
    return std::nullopt;
  }
  return QString::fromUtf8(value, byteCount);
}

[[nodiscard]] std::optional<QString> requiredText(sqlite3_stmt* statement, int index) {
  if (sqlite3_column_type(statement, index) != SQLITE_TEXT) {
    return std::nullopt;
  }
  return optionalText(statement, index);
}

[[nodiscard]] std::optional<std::int64_t> optionalInteger(sqlite3_stmt* statement, int index) {
  if (sqlite3_column_type(statement, index) == SQLITE_NULL) {
    return std::nullopt;
  }
  if (sqlite3_column_type(statement, index) != SQLITE_INTEGER) {
    return std::nullopt;
  }
  return sqlite3_column_int64(statement, index);
}

[[nodiscard]] bool isStoredBoolean(sqlite3_stmt* statement, int index) {
  const int value = sqlite3_column_int(statement, index);
  return value == 0 || value == 1;
}

[[nodiscard]] CalendarDecodeResult decodeCalendar(sqlite3_stmt* statement) {
  const std::optional<QString> id = requiredText(statement, 0);
  const std::optional<QString> accountId = requiredText(statement, 1);
  const std::optional<QString> remoteId = requiredText(statement, 2);
  const std::optional<QString> title = requiredText(statement, 3);
  const std::optional<QString> updatedAt = requiredText(statement, 15);
  if (!id.has_value() || !accountId.has_value() || !remoteId.has_value() || !title.has_value() ||
      !updatedAt.has_value() || !isStoredBoolean(statement, 10) || !isStoredBoolean(statement, 11) ||
      !isStoredBoolean(statement, 12)) {
    return AppError(AppErrorCode::Database, QStringLiteral("Stored calendar row is invalid"));
  }
  const std::int64_t eventCount = sqlite3_column_int64(statement, 16);
  if (eventCount < 0) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("Stored calendar event count is invalid"));
  }
  return CalendarSummary{.id = *id,
                         .accountId = *accountId,
                         .remoteId = *remoteId,
                         .title = *title,
                         .description = optionalText(statement, 4),
                         .timeZone = optionalText(statement, 5),
                         .colorId = optionalText(statement, 6),
                         .backgroundColor = optionalText(statement, 7),
                         .foregroundColor = optionalText(statement, 8),
                         .accessRole = optionalText(statement, 9),
                         .selected = sqlite3_column_int(statement, 10) == 1,
                         .hidden = sqlite3_column_int(statement, 11) == 1,
                         .primary = sqlite3_column_int(statement, 12) == 1,
                         .etag = optionalText(statement, 13),
                         .remoteUpdatedAt = optionalText(statement, 14),
                         .updatedAt = *updatedAt,
                         .eventCount = eventCount};
}

[[nodiscard]] CalendarEventDecodeResult decodeCalendarEvent(sqlite3_stmt* statement) {
  const std::optional<QString> id = requiredText(statement, 0);
  const std::optional<QString> calendarId = requiredText(statement, 1);
  const std::optional<QString> status = requiredText(statement, 5);
  const std::optional<QString> title = requiredText(statement, 6);
  const std::optional<QString> startAt = requiredText(statement, 9);
  const std::optional<QString> endAt = requiredText(statement, 11);
  const std::optional<QString> attendeeEmailsJson = requiredText(statement, 21);
  const std::optional<QString> attendeeDetailsJson = requiredText(statement, 22);
  const std::optional<QString> remindersJson = requiredText(statement, 23);
  const std::optional<QString> attachmentsJson = requiredText(statement, 31);
  const std::optional<QString> guestPermissionsJson = requiredText(statement, 32);
  const std::optional<QString> statusPropertiesJson = requiredText(statement, 33);
  const std::optional<QString> updatedAt = requiredText(statement, 28);
  if (!id.has_value() || !calendarId.has_value() || !status.has_value() || !title.has_value() ||
      !startAt.has_value() || !endAt.has_value() || !updatedAt.has_value() ||
      !isStoredBoolean(statement, 13) || !attendeeEmailsJson.has_value() ||
      !attendeeDetailsJson.has_value() || !remindersJson.has_value() ||
      !isStoredBoolean(statement, 24) || !attachmentsJson.has_value() ||
      !guestPermissionsJson.has_value() || !statusPropertiesJson.has_value() ||
      !isStoredBoolean(statement, 29)) {
    return AppError(AppErrorCode::Database, QStringLiteral("Stored calendar event row is invalid"));
  }
  return CalendarEventSummary{.id = *id,
                              .calendarId = *calendarId,
                              .remoteId = optionalText(statement, 2),
                              .recurringRemoteId = optionalText(statement, 3),
                              .originalStartAt = optionalText(statement, 4),
                              .status = *status,
                              .title = *title,
                              .description = optionalText(statement, 7),
                              .location = optionalText(statement, 8),
                              .startAt = *startAt,
                              .startTimeZone = optionalText(statement, 10),
                              .endAt = *endAt,
                              .endTimeZone = optionalText(statement, 12),
                              .allDay = sqlite3_column_int(statement, 13) == 1,
                              .recurrenceRule = optionalText(statement, 14),
                              .colorId = optionalText(statement, 15),
                              .transparency = optionalText(statement, 16),
                              .visibility = optionalText(statement, 17),
                              .timeZone = optionalText(statement, 18),
                              .hcbKind = optionalText(statement, 19),
                              .eventType = optionalText(statement, 20),
                              .attendeeEmailsJson = *attendeeEmailsJson,
                              .attendeeDetailsJson = *attendeeDetailsJson,
                              .remindersJson = *remindersJson,
                              .remindersUseDefault = sqlite3_column_int(statement, 24) == 1,
                              .conferenceJson = optionalText(statement, 30),
                              .attachmentsJson = *attachmentsJson,
                              .guestPermissionsJson = *guestPermissionsJson,
                              .statusPropertiesJson = *statusPropertiesJson,
                              .etag = optionalText(statement, 25),
                              .sequence = optionalInteger(statement, 26),
                              .remoteUpdatedAt = optionalText(statement, 27),
                              .updatedAt = *updatedAt,
                              .instanceRangeCached = sqlite3_column_int(statement, 29) == 1};
}

constexpr char calendarProjectionSql[] = R"(
SELECT calendars.id, calendars.account_id, calendars.remote_id, calendars.title,
       calendars.description, calendars.time_zone, calendars.color_id, calendars.background_color,
       calendars.foreground_color, calendars.access_role, calendars.is_selected, calendars.is_hidden,
       calendars.is_primary, calendars.etag, calendars.remote_updated_at, calendars.updated_at,
       COUNT(events.id) AS event_count
FROM local_calendars AS calendars
LEFT JOIN local_calendar_events AS events
  ON events.calendar_id = calendars.id
 AND events.deleted_at IS NULL
 AND events.status != 'cancelled'
WHERE calendars.deleted_at IS NULL
)";

[[nodiscard]] QString calendarOrderSql() {
  return QStringLiteral(
      " GROUP BY calendars.id "
      "ORDER BY calendars.is_primary DESC, calendars.title COLLATE NOCASE ASC, calendars.id ASC");
}

[[nodiscard]] CalendarLookupResult readStoredCalendar(SqliteConnection& connection,
                                                      const QString& calendarId) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar connection is unavailable"));
  }
  const QByteArray sql = QString::fromLatin1(calendarProjectionSql)
                             .append(QStringLiteral(" AND calendars.id = ?1"))
                             .append(calendarOrderSql())
                             .append(QStringLiteral(" LIMIT 1"))
                             .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite calendar lookup preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindText(statement, 1, calendarId); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  const int stepResult = sqlite3_step(statement);
  if (stepResult == SQLITE_DONE) {
    const int finalizeResult = sqlite3_finalize(statement);
    if (finalizeResult != SQLITE_OK) {
      return databaseError(QStringLiteral("SQLite calendar lookup finalization failed (%1)"),
                           finalizeResult);
    }
    return std::optional<CalendarSummary>{};
  }
  if (stepResult != SQLITE_ROW) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite calendar lookup failed (%1)"), stepResult);
  }
  const CalendarDecodeResult decoded = decodeCalendar(statement);
  const int finalizeResult = sqlite3_finalize(statement);
  if (std::holds_alternative<AppError>(decoded)) {
    return std::get<AppError>(decoded);
  }
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite calendar lookup finalization failed (%1)"),
                         finalizeResult);
  }
  return std::optional<CalendarSummary>(std::get<CalendarSummary>(decoded));
}

[[nodiscard]] CalendarListPageResult readStoredCalendars(SqliteConnection& connection,
                                                         const CalendarListReadRequest& request) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar connection is unavailable"));
  }
  QString where;
  if (!request.includeHidden) {
    where = QStringLiteral(" AND calendars.is_hidden = 0");
  }
  if (request.accountId.has_value()) {
    where.append(QStringLiteral(" AND calendars.account_id = ?1"));
  }
  if (request.selectedOnly) {
    where.append(QStringLiteral(" AND calendars.is_selected = 1"));
  }
  const int limitIndex = request.accountId.has_value() ? 2 : 1;
  const QByteArray sql =
      QString::fromLatin1(calendarProjectionSql)
          .append(where)
          .append(calendarOrderSql())
          .append(QStringLiteral(" LIMIT ?%1 OFFSET ?%2").arg(limitIndex).arg(limitIndex + 1))
          .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite calendar list preparation failed (%1)"),
                         prepareResult);
  }
  if (request.accountId.has_value()) {
    if (const std::optional<AppError> error = bindText(statement, 1, *request.accountId);
        error.has_value()) {
      sqlite3_finalize(statement);
      return *error;
    }
  }
  if (const std::optional<AppError> error = bindInteger(statement, limitIndex, request.limit);
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  if (const std::optional<AppError> error = bindInteger(statement, limitIndex + 1, request.offset);
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  QList<CalendarSummary> items;
  while (true) {
    const int stepResult = sqlite3_step(statement);
    if (stepResult == SQLITE_DONE) {
      break;
    }
    if (stepResult != SQLITE_ROW) {
      sqlite3_finalize(statement);
      return databaseError(QStringLiteral("SQLite calendar list failed (%1)"), stepResult);
    }
    const CalendarDecodeResult decoded = decodeCalendar(statement);
    if (std::holds_alternative<AppError>(decoded)) {
      sqlite3_finalize(statement);
      return std::get<AppError>(decoded);
    }
    items.append(std::get<CalendarSummary>(decoded));
  }
  const int finalizeResult = sqlite3_finalize(statement);
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite calendar list finalization failed (%1)"),
                         finalizeResult);
  }

  QString countSql = QStringLiteral("SELECT COUNT(*) FROM local_calendars WHERE deleted_at IS NULL");
  if (!request.includeHidden) {
    countSql.append(QStringLiteral(" AND is_hidden = 0"));
  }
  if (request.accountId.has_value()) {
    countSql.append(QStringLiteral(" AND account_id = ?1"));
  }
  if (request.selectedOnly) {
    countSql.append(QStringLiteral(" AND is_selected = 1"));
  }
  const QByteArray countSqlUtf8 = countSql.toUtf8();
  sqlite3_stmt* countStatement = nullptr;
  const int countPrepareResult = sqlite3_prepare_v3(
      handle, countSqlUtf8.constData(), -1, SQLITE_PREPARE_PERSISTENT, &countStatement, nullptr);
  if (countPrepareResult != SQLITE_OK) {
    sqlite3_finalize(countStatement);
    return databaseError(QStringLiteral("SQLite calendar count preparation failed (%1)"),
                         countPrepareResult);
  }
  if (request.accountId.has_value()) {
    if (const std::optional<AppError> error = bindText(countStatement, 1, *request.accountId);
        error.has_value()) {
      sqlite3_finalize(countStatement);
      return *error;
    }
  }
  const int countStepResult = sqlite3_step(countStatement);
  const std::int64_t totalKnown = sqlite3_column_int64(countStatement, 0);
  const int countFinalizeResult = sqlite3_finalize(countStatement);
  if (countStepResult != SQLITE_ROW || totalKnown < 0) {
    return databaseError(QStringLiteral("SQLite calendar count failed (%1)"), countStepResult);
  }
  if (countFinalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite calendar count finalization failed (%1)"),
                         countFinalizeResult);
  }
  const std::int64_t consumed = request.offset + static_cast<std::int64_t>(items.size());
  return CalendarListPage{
      .items = std::move(items),
      .nextOffset = consumed < totalKnown ? std::optional<std::int64_t>(consumed) : std::nullopt,
      .totalKnown = totalKnown};
}

[[nodiscard]] QString eventFilterSql(const CalendarEventRangeReadRequest& request) {
  constexpr char coverageSql[] = R"(
EXISTS (
  SELECT 1 FROM local_calendar_instance_coverage AS coverage
  WHERE coverage.calendar_id = events.calendar_id
    AND coverage.recurring_remote_id = events.recurring_remote_id
    AND coverage.range_start_at <= ?2
    AND coverage.range_end_at >= ?1
)
)";
  QString filter = QStringLiteral(
      "events.deleted_at IS NULL AND calendars.deleted_at IS NULL AND ("
      "(events.is_instance_cache = 0 AND ("
      "(events.status != 'cancelled' AND events.start_at < ?1 AND events.end_at > ?2) "
      "OR (COALESCE(recurrences.recurrence_rule, events.recurrence_rule) IS NOT NULL "
      "AND events.start_at < ?1) "
      "OR (events.status = 'cancelled' AND events.recurring_remote_id IS NOT NULL "
      "AND events.original_start_at >= ?2 AND events.original_start_at < ?1))) "
      "OR (events.is_instance_cache = 1 AND %1 AND ("
      "(events.status != 'cancelled' AND events.start_at < ?1 AND events.end_at > ?2) "
      "OR (events.status = 'cancelled' AND events.original_start_at >= ?2 "
      "AND events.original_start_at < ?1))))")
                       .arg(QString::fromLatin1(coverageSql));
  for (qsizetype index = 0; index < request.calendarIds.size(); ++index) {
    filter.append(index == 0 ? QStringLiteral(" AND events.calendar_id IN (?%1").arg(index + 3)
                             : QStringLiteral(", ?%1").arg(index + 3));
  }
  if (!request.calendarIds.isEmpty()) {
    filter.append(u')');
  }
  return filter;
}

[[nodiscard]] QString instanceCoverageSql() {
  return QStringLiteral(
      "EXISTS (SELECT 1 FROM local_calendar_instance_coverage AS coverage "
      "WHERE coverage.calendar_id = events.calendar_id "
      "AND coverage.recurring_remote_id = events.remote_id "
      "AND coverage.range_start_at <= ?2 AND coverage.range_end_at >= ?1 "
      "AND coverage.expires_at > strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))");
}

[[nodiscard]] std::optional<AppError>
bindEventFilter(sqlite3_stmt* statement, const CalendarEventRangeReadRequest& request) {
  if (const std::optional<AppError> error = bindText(statement, 1, request.endAt);
      error.has_value()) {
    return error;
  }
  if (const std::optional<AppError> error = bindText(statement, 2, request.startAt);
      error.has_value()) {
    return error;
  }
  for (qsizetype index = 0; index < request.calendarIds.size(); ++index) {
    if (const std::optional<AppError> error =
            bindText(statement, static_cast<int>(index) + 3, request.calendarIds.at(index));
        error.has_value()) {
      return error;
    }
  }
  return std::nullopt;
}

[[nodiscard]] CalendarEventPageResult
readStoredCalendarEvents(SqliteConnection& connection,
                         const CalendarEventRangeReadRequest& request) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar connection is unavailable"));
  }
  const QString filter = eventFilterSql(request);
  const int limitIndex = static_cast<int>(request.calendarIds.size()) + 3;
  const QByteArray sql =
      QStringLiteral("SELECT events.id, events.calendar_id, events.remote_id, "
                     "events.recurring_remote_id, events.original_start_at, events.status, "
                     "events.title, events.description, events.location, events.start_at, "
                     "events.start_time_zone, events.end_at, events.end_time_zone, "
                     "events.is_all_day, COALESCE(recurrences.recurrence_rule, events.recurrence_rule), events.color_id, "
                     "events.transparency, events.visibility, events.time_zone, events.hcb_kind, "
                     "events.event_type, events.attendee_emails_json, events.attendee_details_json, "
                     "events.reminders_json, events.reminders_use_default, events.etag, events.sequence, "
                     "events.remote_updated_at, events.updated_at, %1, events.conference_json, "
                     "events.attachments_json, events.guest_permissions_json, events.status_properties_json "
                     "FROM local_calendar_events AS events "
                     "INNER JOIN local_calendars AS calendars ON calendars.id = events.calendar_id "
                     "LEFT JOIN local_calendar_event_recurrences AS recurrences ON recurrences.event_id = events.id "
                     "WHERE %2 "
                     "ORDER BY events.start_at ASC, events.end_at ASC, events.id ASC "
                     "LIMIT ?%3 OFFSET ?%4")
          .arg(instanceCoverageSql(), filter)
          .arg(limitIndex)
          .arg(limitIndex + 1)
          .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite calendar event list preparation failed (%1)"),
                         prepareResult);
  }
  if (const std::optional<AppError> error = bindEventFilter(statement, request);
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  if (const std::optional<AppError> error = bindInteger(statement, limitIndex, request.limit);
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  if (const std::optional<AppError> error = bindInteger(statement, limitIndex + 1, request.offset);
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  QList<CalendarEventSummary> items;
  while (true) {
    const int stepResult = sqlite3_step(statement);
    if (stepResult == SQLITE_DONE) {
      break;
    }
    if (stepResult != SQLITE_ROW) {
      sqlite3_finalize(statement);
      return databaseError(QStringLiteral("SQLite calendar event list failed (%1)"), stepResult);
    }
    const CalendarEventDecodeResult decoded = decodeCalendarEvent(statement);
    if (std::holds_alternative<AppError>(decoded)) {
      sqlite3_finalize(statement);
      return std::get<AppError>(decoded);
    }
    items.append(std::get<CalendarEventSummary>(decoded));
  }
  const int finalizeResult = sqlite3_finalize(statement);
  if (finalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite calendar event list finalization failed (%1)"),
                         finalizeResult);
  }

  const QByteArray countSql =
      QStringLiteral("SELECT COUNT(*) FROM local_calendar_events AS events "
                     "INNER JOIN local_calendars AS calendars ON calendars.id = events.calendar_id "
                     "LEFT JOIN local_calendar_event_recurrences AS recurrences ON recurrences.event_id = events.id "
                     "WHERE %1")
          .arg(filter)
          .toUtf8();
  sqlite3_stmt* countStatement = nullptr;
  const int countPrepareResult = sqlite3_prepare_v3(
      handle, countSql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &countStatement, nullptr);
  if (countPrepareResult != SQLITE_OK) {
    sqlite3_finalize(countStatement);
    return databaseError(QStringLiteral("SQLite calendar event count preparation failed (%1)"),
                         countPrepareResult);
  }
  if (const std::optional<AppError> error = bindEventFilter(countStatement, request);
      error.has_value()) {
    sqlite3_finalize(countStatement);
    return *error;
  }
  const int countStepResult = sqlite3_step(countStatement);
  const std::int64_t totalKnown = sqlite3_column_int64(countStatement, 0);
  const int countFinalizeResult = sqlite3_finalize(countStatement);
  if (countStepResult != SQLITE_ROW || totalKnown < 0) {
    return databaseError(QStringLiteral("SQLite calendar event count failed (%1)"),
                         countStepResult);
  }
  if (countFinalizeResult != SQLITE_OK) {
    return databaseError(QStringLiteral("SQLite calendar event count finalization failed (%1)"),
                         countFinalizeResult);
  }
  const std::int64_t consumed = request.offset + static_cast<std::int64_t>(items.size());
  return CalendarEventPage{
      .items = std::move(items),
      .nextOffset = consumed < totalKnown ? std::optional<std::int64_t>(consumed) : std::nullopt,
      .totalKnown = totalKnown};
}

[[nodiscard]] CalendarRecurringInstanceCacheTargetsResult
readUncachedRecurringInstances(SqliteConnection& connection,
                               const CalendarRecurringInstanceCacheReadRequest& request) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    QStringLiteral("SQLite calendar connection is unavailable"));
  }
  QString filter = QStringLiteral(
      "events.deleted_at IS NULL AND calendars.deleted_at IS NULL "
      "AND events.is_instance_cache = 0 "
      "AND COALESCE(recurrences.recurrence_rule, events.recurrence_rule) IS NOT NULL "
      "AND events.remote_id IS NOT NULL AND events.recurring_remote_id IS NULL "
      "AND NOT EXISTS (SELECT 1 FROM local_calendar_instance_coverage AS coverage "
      "WHERE coverage.calendar_id = events.calendar_id "
      "AND coverage.recurring_remote_id = events.remote_id "
      "AND coverage.range_start_at <= ?2 AND coverage.range_end_at >= ?1 "
      "AND coverage.expires_at > strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))");
  for (qsizetype index = 0; index < request.calendarIds.size(); ++index) {
    filter.append(index == 0 ? QStringLiteral(" AND events.calendar_id IN (?%1").arg(index + 3)
                             : QStringLiteral(", ?%1").arg(index + 3));
  }
  if (!request.calendarIds.isEmpty()) {
    filter.append(u')');
  }
  const int limitIndex = static_cast<int>(request.calendarIds.size()) + 3;
  const QByteArray sql =
      QStringLiteral("SELECT events.calendar_id, calendars.remote_id, events.remote_id "
                     "FROM local_calendar_events AS events "
                     "INNER JOIN local_calendars AS calendars ON calendars.id = events.calendar_id "
                     "LEFT JOIN local_calendar_event_recurrences AS recurrences ON recurrences.event_id = events.id "
                     "WHERE %1 ORDER BY events.updated_at DESC, events.id ASC LIMIT ?%2")
          .arg(filter)
          .arg(limitIndex)
          .toUtf8();
  sqlite3_stmt* statement = nullptr;
  const int prepareResult = sqlite3_prepare_v3(
      handle, sql.constData(), -1, SQLITE_PREPARE_PERSISTENT, &statement, nullptr);
  if (prepareResult != SQLITE_OK) {
    sqlite3_finalize(statement);
    return databaseError(QStringLiteral("SQLite recurrence-cache list preparation failed (%1)"),
                         prepareResult);
  }
  CalendarEventRangeReadRequest range{.calendarIds = request.calendarIds,
                                      .startAt = request.startAt,
                                      .endAt = request.endAt};
  if (const std::optional<AppError> error = bindEventFilter(statement, range); error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  if (const std::optional<AppError> error = bindInteger(statement, limitIndex, request.limit);
      error.has_value()) {
    sqlite3_finalize(statement);
    return *error;
  }
  QList<CalendarRecurringInstanceCacheTarget> targets;
  while (true) {
    const int stepResult = sqlite3_step(statement);
    if (stepResult == SQLITE_DONE) {
      break;
    }
    if (stepResult != SQLITE_ROW) {
      sqlite3_finalize(statement);
      return databaseError(QStringLiteral("SQLite recurrence-cache list failed (%1)"), stepResult);
    }
    const std::optional<QString> calendarId = requiredText(statement, 0);
    const std::optional<QString> calendarRemoteId = requiredText(statement, 1);
    const std::optional<QString> recurringRemoteId = requiredText(statement, 2);
    if (!calendarId.has_value() || !calendarRemoteId.has_value() || !recurringRemoteId.has_value()) {
      sqlite3_finalize(statement);
      return AppError(AppErrorCode::Database,
                      QStringLiteral("Stored recurrence-cache target is invalid"));
    }
    targets.append({.calendarId = *calendarId,
                    .calendarRemoteId = *calendarRemoteId,
                    .recurringRemoteId = *recurringRemoteId});
  }
  const int finalizeResult = sqlite3_finalize(statement);
  return finalizeResult == SQLITE_OK
             ? CalendarRecurringInstanceCacheTargetsResult(std::move(targets))
             : CalendarRecurringInstanceCacheTargetsResult(
                   databaseError(QStringLiteral("SQLite recurrence-cache list finalization failed (%1)"),
                                 finalizeResult));
}

} // namespace

CalendarReadService::CalendarReadService(FilePath databasePath)
    : writerQueue_(std::move(databasePath)),
      initialization_(writerQueue_
                          .enqueue([](SqliteConnection& connection) -> SqliteWriteResult {
                            const SqliteMigrationRunResultOrError result =
                                LocalSchema::initialize(connection);
                            return std::holds_alternative<AppError>(result)
                                       ? std::optional<AppError>(std::get<AppError>(result))
                                       : std::nullopt;
                          })
                          .share()) {}

std::shared_future<SqliteWriteResult> CalendarReadService::ready() const { return initialization_; }

std::future<CalendarLookupResult> CalendarReadService::findCalendar(QString calendarId) {
  if (!isValidIdentifier(calendarId)) {
    return readyFuture(CalendarLookupResult(
        AppError(AppErrorCode::Validation, QStringLiteral("Calendar identifier is invalid"))));
  }
  return writerQueue_.enqueueResult(
      [calendarId = std::move(calendarId)](SqliteConnection& connection) {
        return readStoredCalendar(connection, calendarId);
      });
}

std::future<CalendarListPageResult>
CalendarReadService::listCalendars(CalendarListReadRequest request) {
  if ((request.accountId.has_value() && !isValidIdentifier(*request.accountId)) ||
      request.limit <= 0 || request.limit > kMaximumCalendarPageLimit || request.offset < 0 ||
      request.offset > std::numeric_limits<std::int64_t>::max() - request.limit) {
    return readyFuture(CalendarListPageResult(AppError(
        AppErrorCode::Validation, QStringLiteral("Calendar list read request is invalid"))));
  }
  return writerQueue_.enqueueResult([request = std::move(request)](SqliteConnection& connection) {
    return readStoredCalendars(connection, request);
  });
}

std::future<CalendarEventPageResult>
CalendarReadService::listEvents(CalendarEventRangeReadRequest request) {
  const std::optional<QDateTime> startAt = parseDateTime(request.startAt);
  const std::optional<QDateTime> endAt = parseDateTime(request.endAt);
  const bool invalidCalendarId =
      request.calendarIds.size() > kMaximumCalendarFilterCount ||
      std::any_of(request.calendarIds.cbegin(),
                  request.calendarIds.cend(),
                  [](const QString& calendarId) { return !isValidIdentifier(calendarId); });
  if (!startAt.has_value() || !endAt.has_value() || *endAt <= *startAt ||
      endAt->toMSecsSinceEpoch() - startAt->toMSecsSinceEpoch() >
          kMaximumRangeDurationMilliseconds ||
      invalidCalendarId || request.limit <= 0 || request.limit > kMaximumEventPageLimit ||
      request.offset < 0 ||
      request.offset > std::numeric_limits<std::int64_t>::max() - request.limit) {
    return readyFuture(CalendarEventPageResult(AppError(
        AppErrorCode::Validation, QStringLiteral("Calendar event range request is invalid"))));
  }
  return writerQueue_.enqueueResult([request = std::move(request)](SqliteConnection& connection) {
    return readStoredCalendarEvents(connection, request);
  });
}

std::future<CalendarRecurringInstanceCacheTargetsResult>
CalendarReadService::listUncachedRecurringInstances(CalendarRecurringInstanceCacheReadRequest request) {
  const std::optional<QDateTime> startAt = parseDateTime(request.startAt);
  const std::optional<QDateTime> endAt = parseDateTime(request.endAt);
  const bool invalidCalendarId =
      request.calendarIds.size() > kMaximumCalendarFilterCount ||
      std::any_of(request.calendarIds.cbegin(),
                  request.calendarIds.cend(),
                  [](const QString& calendarId) { return !isValidIdentifier(calendarId); });
  if (!startAt.has_value() || !endAt.has_value() || *endAt <= *startAt ||
      endAt->toMSecsSinceEpoch() - startAt->toMSecsSinceEpoch() >
          kMaximumRangeDurationMilliseconds ||
      invalidCalendarId || request.limit <= 0 || request.limit > kMaximumInstanceCacheTargetLimit) {
    return readyFuture(CalendarRecurringInstanceCacheTargetsResult(
        AppError(AppErrorCode::Validation, QStringLiteral("Calendar recurrence-cache request is invalid"))));
  }
  return writerQueue_.enqueueResult([request = std::move(request)](SqliteConnection& connection) {
    return readUncachedRecurringInstances(connection, request);
  });
}

} // namespace hcb
