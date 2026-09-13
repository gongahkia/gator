#pragma once

#include "core/AppError.h"
#include "core/FilePath.h"
#include "data/SqliteWriterQueue.h"

#include <QList>
#include <QString>

#include <cstdint>
#include <future>
#include <optional>
#include <variant>

namespace hcb {

struct CalendarSummary final {
  QString id;
  QString accountId;
  QString remoteId;
  QString title;
  std::optional<QString> description;
  std::optional<QString> timeZone;
  std::optional<QString> colorId;
  std::optional<QString> backgroundColor;
  std::optional<QString> foregroundColor;
  std::optional<QString> accessRole;
  bool selected;
  bool hidden;
  bool primary;
  std::optional<QString> etag;
  std::optional<QString> remoteUpdatedAt;
  QString updatedAt;
  std::int64_t eventCount;
};

struct CalendarListReadRequest final {
  std::optional<QString> accountId;
  bool selectedOnly{false};
  bool includeHidden{false};
  std::int64_t limit{50};
  std::int64_t offset{0};
};

struct CalendarEventSummary final {
  QString id;
  QString calendarId;
  std::optional<QString> remoteId;
  std::optional<QString> recurringRemoteId;
  std::optional<QString> originalStartAt;
  QString status;
  QString title;
  std::optional<QString> description;
  std::optional<QString> location;
  QString startAt;
  std::optional<QString> startTimeZone;
  QString endAt;
  std::optional<QString> endTimeZone;
  bool allDay;
  std::optional<QString> recurrenceRule;
  std::optional<QString> colorId;
  std::optional<QString> transparency;
  std::optional<QString> visibility;
  std::optional<QString> timeZone;
  std::optional<QString> hcbKind;
  std::optional<QString> eventType;
  QString attendeeEmailsJson;
  QString attendeeDetailsJson;
  QString remindersJson;
  bool remindersUseDefault{true};
  std::optional<QString> conferenceJson;
  QString attachmentsJson;
  QString guestPermissionsJson;
  QString statusPropertiesJson;
  std::optional<QString> etag;
  std::optional<std::int64_t> sequence;
  std::optional<QString> remoteUpdatedAt;
  QString updatedAt;
  bool instanceRangeCached{false};
};

struct CalendarEventRangeReadRequest final {
  QList<QString> calendarIds;
  QString startAt;
  QString endAt;
  std::int64_t limit{100};
  std::int64_t offset{0};
};

struct CalendarRecurringInstanceCacheTarget final {
  QString calendarId;
  QString calendarRemoteId;
  QString recurringRemoteId;
};

struct CalendarRecurringInstanceCacheReadRequest final {
  QList<QString> calendarIds;
  QString startAt;
  QString endAt;
  std::int64_t limit{250};
};

template <typename Item> struct CalendarPage final {
  QList<Item> items;
  std::optional<std::int64_t> nextOffset;
  std::int64_t totalKnown;
};

using CalendarLookupResult = std::variant<std::optional<CalendarSummary>, AppError>;
using CalendarListPage = CalendarPage<CalendarSummary>;
using CalendarListPageResult = std::variant<CalendarListPage, AppError>;
using CalendarEventPage = CalendarPage<CalendarEventSummary>;
using CalendarEventPageResult = std::variant<CalendarEventPage, AppError>;
using CalendarRecurringInstanceCacheTargetsResult =
    std::variant<QList<CalendarRecurringInstanceCacheTarget>, AppError>;

class CalendarReadService final {
public:
  explicit CalendarReadService(FilePath databasePath);
  CalendarReadService(const CalendarReadService&) = delete;
  CalendarReadService& operator=(const CalendarReadService&) = delete;

  [[nodiscard]] std::shared_future<SqliteWriteResult> ready() const;
  [[nodiscard]] std::future<CalendarLookupResult> findCalendar(QString calendarId);
  [[nodiscard]] std::future<CalendarListPageResult>
  listCalendars(CalendarListReadRequest request = {});
  [[nodiscard]] std::future<CalendarEventPageResult>
  listEvents(CalendarEventRangeReadRequest request);
  [[nodiscard]] std::future<CalendarRecurringInstanceCacheTargetsResult>
  listUncachedRecurringInstances(CalendarRecurringInstanceCacheReadRequest request);

private:
  SqliteWriterQueue writerQueue_;
  std::shared_future<SqliteWriteResult> initialization_;
};

} // namespace hcb
